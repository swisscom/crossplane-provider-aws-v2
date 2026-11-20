/*
Copyright 2021 The Crossplane Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package key

import (
	"context"

	"github.com/aws/aws-sdk-go-v2/aws"
	svcsdk "github.com/aws/aws-sdk-go-v2/service/kms"
	svcsdktypes "github.com/aws/aws-sdk-go-v2/service/kms/types"
	"github.com/crossplane/crossplane-runtime/v2/pkg/controller"
	"github.com/crossplane/crossplane-runtime/v2/pkg/errors"
	"github.com/crossplane/crossplane-runtime/v2/pkg/meta"
	"github.com/crossplane/crossplane-runtime/v2/pkg/reconciler/managed"
	xpv2 "github.com/crossplane/crossplane/apis/v2/core/v2"
	ctrl "sigs.k8s.io/controller-runtime"

	svcapitypes "github.com/swisscom/crossplane-provider-aws-v2/apis/kms/v1alpha1"
	awsclient "github.com/swisscom/crossplane-provider-aws-v2/internal/clients/aws"
	"github.com/swisscom/crossplane-provider-aws-v2/internal/utils/compare"
	"github.com/swisscom/crossplane-provider-aws-v2/internal/utils/pointer"
	"github.com/swisscom/crossplane-provider-aws-v2/internal/utils/policy"
)

const defaultPolicyName = "default"

func init() {
	configure = configureKey
}

func configureKey(_ ctrl.Manager, _ controller.Options) ([]option, []managed.ReconcilerOption, error) {
	return []option{setupExternal}, []managed.ReconcilerOption{
		// AWS assigns the key ID, see postCreate.
		managed.WithInitializers(),
	}, nil
}

func setupExternal(e *external) {
	h := &hooks{client: e.client}
	e.preObserve = preObserve
	e.postObserve = postObserve
	e.lateInitialize = h.lateInitialize
	e.isUpToDate = h.isUpToDate
	e.postCreate = postCreate
	e.update = h.update
	e.delete = h.delete
}

type hooks struct {
	client Client
}

func preObserve(_ context.Context, cr *svcapitypes.Key, obj *svcsdk.DescribeKeyInput) error {
	obj.KeyId = pointer.ToOrNilIfZeroValue(meta.GetExternalName(cr))
	return nil
}

func postObserve(_ context.Context, cr *svcapitypes.Key, obj *svcsdk.DescribeKeyOutput, obs managed.ExternalObservation, err error) (managed.ExternalObservation, error) {
	if err != nil {
		return obs, err
	}

	switch obj.KeyMetadata.KeyState {
	case svcsdktypes.KeyStateEnabled, svcsdktypes.KeyStateUpdating:
		cr.SetConditions(xpv2.Available())
	case svcsdktypes.KeyStateCreating:
		cr.SetConditions(xpv2.Creating())
	case svcsdktypes.KeyStatePendingDeletion, svcsdktypes.KeyStatePendingReplicaDeletion:
		cr.SetConditions(xpv2.Deleting())
		return managed.ExternalObservation{ResourceExists: false}, nil
	case svcsdktypes.KeyStateDisabled, svcsdktypes.KeyStatePendingImport, svcsdktypes.KeyStateUnavailable:
		cr.SetConditions(xpv2.Unavailable())
	}
	return obs, nil
}

func postCreate(_ context.Context, cr *svcapitypes.Key, obj *svcsdk.CreateKeyOutput, creation managed.ExternalCreation, err error) (managed.ExternalCreation, error) {
	if err != nil {
		return creation, err
	}
	meta.SetExternalName(cr, pointer.StringValue(obj.KeyMetadata.KeyId))
	return creation, nil
}

// lateInitialize has no context parameter; it takes the policy that AWS
// creates when the spec has none.
func (h *hooks) lateInitialize(in *svcapitypes.KeyParameters, obj *svcsdk.DescribeKeyOutput) error {
	if in.Policy == nil {
		resPolicy, err := h.client.GetKeyPolicy(context.TODO(), &svcsdk.GetKeyPolicyInput{
			KeyId:      obj.KeyMetadata.KeyId,
			PolicyName: aws.String(defaultPolicyName),
		})
		if err != nil {
			return awsclient.Wrap(err, "cannot get key policy")
		}
		in.Policy = resPolicy.Policy
	}
	in.Enabled = pointer.LateInitialize(in.Enabled, aws.Bool(obj.KeyMetadata.Enabled))
	return nil
}

func (h *hooks) isUpToDate(ctx context.Context, cr *svcapitypes.Key, obj *svcsdk.DescribeKeyOutput) (bool, string, error) {
	var diff []string
	p := cr.Spec.ForProvider

	if p.Description != nil && pointer.StringValue(p.Description) != pointer.StringValue(obj.KeyMetadata.Description) {
		diff = append(diff, "description")
	}
	if !isUpToDateEnableDisable(cr) {
		diff = append(diff, "enabled")
	}

	policyUpToDate, err := h.isPolicyUpToDate(ctx, cr)
	if err != nil {
		return false, "", err
	}
	if !policyUpToDate {
		diff = append(diff, "policy")
	}

	rotationUpToDate, err := h.isRotationUpToDate(ctx, cr)
	if err != nil {
		return false, "", err
	}
	if !rotationUpToDate {
		diff = append(diff, "enableKeyRotation")
	}

	tagsUpToDate, err := h.areTagsUpToDate(ctx, cr)
	if err != nil {
		return false, "", err
	}
	if !tagsUpToDate {
		diff = append(diff, "tags")
	}

	return len(diff) == 0, compare.Message(diff), nil
}

func (h *hooks) areTagsUpToDate(ctx context.Context, cr *svcapitypes.Key) (bool, error) {
	resTags, err := h.client.ListResourceTags(ctx, &svcsdk.ListResourceTagsInput{
		KeyId: pointer.ToOrNilIfZeroValue(meta.GetExternalName(cr)),
	})
	if err != nil {
		return false, awsclient.Wrap(err, "cannot list tags")
	}
	add, remove := diffTags(cr.Spec.ForProvider.Tags, resTags.Tags)
	return len(add) == 0 && len(remove) == 0, nil
}

func (h *hooks) isPolicyUpToDate(ctx context.Context, cr *svcapitypes.Key) (bool, error) {
	resPolicy, err := h.client.GetKeyPolicy(ctx, &svcsdk.GetKeyPolicyInput{
		KeyId:      pointer.ToOrNilIfZeroValue(meta.GetExternalName(cr)),
		PolicyName: aws.String(defaultPolicyName),
	})
	if err != nil {
		return false, awsclient.Wrap(err, "cannot get key policy")
	}
	specPolicy, err := policy.ParsePolicyStringPtr(cr.Spec.ForProvider.Policy)
	if err != nil {
		return false, errors.Wrap(err, "cannot parse spec policy")
	}
	currentPolicy, err := policy.ParsePolicyStringPtr(resPolicy.Policy)
	if err != nil {
		return false, errors.Wrap(err, "cannot parse current policy")
	}
	equal, _ := policy.ArePoliciesEqal(specPolicy, currentPolicy)
	return equal, nil
}

func (h *hooks) isRotationUpToDate(ctx context.Context, cr *svcapitypes.Key) (bool, error) {
	res, err := h.client.GetKeyRotationStatus(ctx, &svcsdk.GetKeyRotationStatusInput{
		KeyId: pointer.ToOrNilIfZeroValue(meta.GetExternalName(cr)),
	})
	if err != nil {
		return false, awsclient.Wrap(err, "cannot get key rotation status")
	}
	return pointer.BoolValue(cr.Spec.ForProvider.EnableKeyRotation) == res.KeyRotationEnabled, nil
}

// update calls only the operations of the fields that differ: IAM policies
// may allow some of them only, e.g. EnableKeyRotation but not
// UpdateKeyDescription.
func (h *hooks) update(ctx context.Context, cr *svcapitypes.Key) (managed.ExternalUpdate, error) {
	for _, f := range []func(context.Context, *svcapitypes.Key) error{
		h.updateDescription,
		h.updatePolicy,
		h.updateRotation,
		h.updateTags,
		h.enableDisableKey,
	} {
		if err := f(ctx, cr); err != nil {
			return managed.ExternalUpdate{}, err
		}
	}
	return managed.ExternalUpdate{}, nil
}

func (h *hooks) updateDescription(ctx context.Context, cr *svcapitypes.Key) error {
	desired := cr.Spec.ForProvider.Description
	if desired == nil {
		return nil
	}
	keyID := pointer.ToOrNilIfZeroValue(meta.GetExternalName(cr))
	res, err := h.client.DescribeKey(ctx, &svcsdk.DescribeKeyInput{KeyId: keyID})
	if err != nil {
		return awsclient.Wrap(err, errDescribe)
	}
	if *desired == pointer.StringValue(res.KeyMetadata.Description) {
		return nil
	}
	_, err = h.client.UpdateKeyDescription(ctx, &svcsdk.UpdateKeyDescriptionInput{KeyId: keyID, Description: desired})
	return awsclient.Wrap(err, errUpdate)
}

func (h *hooks) updatePolicy(ctx context.Context, cr *svcapitypes.Key) error {
	upToDate, err := h.isPolicyUpToDate(ctx, cr)
	if err != nil || upToDate {
		return err
	}
	_, err = h.client.PutKeyPolicy(ctx, &svcsdk.PutKeyPolicyInput{
		KeyId:      pointer.ToOrNilIfZeroValue(meta.GetExternalName(cr)),
		PolicyName: aws.String(defaultPolicyName),
		Policy:     cr.Spec.ForProvider.Policy,
	})
	return awsclient.Wrap(err, errUpdate)
}

func (h *hooks) updateRotation(ctx context.Context, cr *svcapitypes.Key) error {
	upToDate, err := h.isRotationUpToDate(ctx, cr)
	if err != nil || upToDate {
		return err
	}
	keyID := pointer.ToOrNilIfZeroValue(meta.GetExternalName(cr))
	if pointer.BoolValue(cr.Spec.ForProvider.EnableKeyRotation) {
		_, err = h.client.EnableKeyRotation(ctx, &svcsdk.EnableKeyRotationInput{KeyId: keyID})
	} else {
		_, err = h.client.DisableKeyRotation(ctx, &svcsdk.DisableKeyRotationInput{KeyId: keyID})
	}
	return awsclient.Wrap(err, errUpdate)
}

func (h *hooks) updateTags(ctx context.Context, cr *svcapitypes.Key) error {
	keyID := pointer.ToOrNilIfZeroValue(meta.GetExternalName(cr))
	tagsOutput, err := h.client.ListResourceTags(ctx, &svcsdk.ListResourceTagsInput{KeyId: keyID})
	if err != nil {
		return awsclient.Wrap(err, errUpdate)
	}

	add, remove := diffTags(cr.Spec.ForProvider.Tags, tagsOutput.Tags)
	if len(add) != 0 {
		if _, err := h.client.TagResource(ctx, &svcsdk.TagResourceInput{KeyId: keyID, Tags: add}); err != nil {
			return awsclient.Wrap(err, "cannot tag Key")
		}
	}
	if len(remove) != 0 {
		if _, err := h.client.UntagResource(ctx, &svcsdk.UntagResourceInput{KeyId: keyID, TagKeys: remove}); err != nil {
			return awsclient.Wrap(err, "cannot untag Key")
		}
	}
	return nil
}

func isUpToDateEnableDisable(cr *svcapitypes.Key) bool {
	return pointer.BoolValue(cr.Spec.ForProvider.Enabled) == pointer.BoolValue(cr.Status.AtProvider.Enabled)
}

func (h *hooks) enableDisableKey(ctx context.Context, cr *svcapitypes.Key) error {
	if isUpToDateEnableDisable(cr) {
		return nil
	}

	keyID := pointer.ToOrNilIfZeroValue(meta.GetExternalName(cr))
	if pointer.BoolValue(cr.Spec.ForProvider.Enabled) {
		if _, err := h.client.EnableKey(ctx, &svcsdk.EnableKeyInput{KeyId: keyID}); err != nil {
			return awsclient.Wrap(err, "cannot enable Key")
		}
		return nil
	}
	if _, err := h.client.DisableKey(ctx, &svcsdk.DisableKeyInput{KeyId: keyID}); err != nil {
		return awsclient.Wrap(err, "cannot disable Key")
	}
	return nil
}

// delete schedules the key for deletion; KMS keys can't be deleted at once.
func (h *hooks) delete(ctx context.Context, cr *svcapitypes.Key) (managed.ExternalDelete, error) {
	if cr.Status.AtProvider.DeletionDate != nil {
		return managed.ExternalDelete{}, nil
	}

	_, err := h.client.ScheduleKeyDeletion(ctx, &svcsdk.ScheduleKeyDeletionInput{
		KeyId:               pointer.ToOrNilIfZeroValue(meta.GetExternalName(cr)),
		PendingWindowInDays: pointer.Int32(cr.Spec.ForProvider.PendingWindowInDays),
	})
	return managed.ExternalDelete{}, awsclient.Wrap(err, errDelete)
}

// diffTags returns the spec tags to add or change and the keys of the tags
// to remove. Tags reserved by AWS are ignored.
func diffTags(spec []*svcapitypes.Tag, current []svcsdktypes.Tag) (add []svcsdktypes.Tag, remove []string) {
	want := make(map[string]string, len(spec))
	for _, t := range spec {
		if k := pointer.StringValue(t.TagKey); !awsclient.IsAWSTag(k) {
			want[k] = pointer.StringValue(t.TagValue)
		}
	}
	for _, t := range current {
		k := pointer.StringValue(t.TagKey)
		if awsclient.IsAWSTag(k) {
			continue
		}
		v, ok := want[k]
		switch {
		case !ok:
			remove = append(remove, k)
		case v == pointer.StringValue(t.TagValue):
			delete(want, k)
		}
	}
	for k, v := range want {
		add = append(add, svcsdktypes.Tag{TagKey: aws.String(k), TagValue: aws.String(v)})
	}
	return add, remove
}
