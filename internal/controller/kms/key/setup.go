/*
Copyright 2026 The Crossplane Authors.

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
	"slices"
	"sort"
	"strconv"

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
	pointer "github.com/swisscom/crossplane-provider-aws-v2/internal/utils/pointer"
)

const (
	// defaultPolicyName is the only key policy name KMS supports.
	defaultPolicyName = "default"
	// maxTags is the most tags a key can have, so one page has all of them.
	maxTags = 50

	errNotObserved     = "the key was not observed before the update"
	errFmtImmutable    = "%s cannot be changed after creation: it is %q in AWS but %q in the spec; create a new Key instead"
	errGetPolicy       = "cannot get the key policy"
	errPutPolicy       = "cannot put the key policy"
	errGetRotation     = "cannot get the key rotation status"
	errEnableRotation  = "cannot enable key rotation"
	errDisableRotation = "cannot disable key rotation"
	errEnable          = "cannot enable the key"
	errDisable         = "cannot disable the key"
	errCancelDeletion  = "cannot cancel the scheduled deletion of the key"
	errDescription     = "cannot update the key description"
	errListTags        = "cannot list tags"
	errTagResource     = "cannot add tags"
	errUntagResource   = "cannot remove tags"

	msgPendingDeletion = "the key is scheduled for deletion"
	msgPendingImport   = "the key has no key material: import it"
	msgUnavailable     = "the custom key store of the key is disconnected"
)

// Fields of KeyParameters (JSON names) that isUpToDate compares.
const (
	fieldDescription       = "description"
	fieldPolicy            = "policy"
	fieldEnabled           = "enabled"
	fieldEnableKeyRotation = "enableKeyRotation"
	fieldRotationPeriod    = "rotationPeriodInDays"
	fieldTags              = "tags"
)

// hooks keeps what Observe found for Update: the reconciler calls Update in
// the same reconcile, on the same external client.
type hooks struct {
	client Client

	// keyID is the ID of the observed key, diff its differing fields.
	keyID string
	diff  []string
	// pendingDeletion is set if the key is scheduled for deletion although
	// the resource isn't deleted.
	pendingDeletion bool
	// rotation is whether automatic rotation should be on.
	rotation bool
	// tagsAdd and tagsRemove make the key's tags equal the desired ones.
	tagsAdd    []svcsdktypes.Tag
	tagsRemove []string
}

func preObserve(_ context.Context, cr *svcapitypes.Key, obj *svcsdk.DescribeKeyInput) error {
	obj.KeyId = aws.String(meta.GetExternalName(cr))
	return nil
}

func isPendingDeletion(s svcsdktypes.KeyState) bool {
	return s == svcsdktypes.KeyStatePendingDeletion || s == svcsdktypes.KeyStatePendingReplicaDeletion
}

// postObserve sets the conditions from the key state. A key that is
// scheduled for deletion is gone for a deleted resource: KMS deletes it only
// after the waiting period.
func postObserve(_ context.Context, cr *svcapitypes.Key, resp *svcsdk.DescribeKeyOutput, obs managed.ExternalObservation, err error) (managed.ExternalObservation, error) {
	if err != nil || !obs.ResourceExists {
		return obs, err
	}
	state := resp.KeyMetadata.KeyState
	switch {
	case isPendingDeletion(state):
		if meta.WasDeleted(cr) {
			return managed.ExternalObservation{ResourceExists: false}, nil
		}
		cr.SetConditions(xpv2.Unavailable().WithMessage(msgPendingDeletion))
	case state == svcsdktypes.KeyStateCreating:
		cr.SetConditions(xpv2.Creating())
	case state == svcsdktypes.KeyStatePendingImport:
		cr.SetConditions(xpv2.Unavailable().WithMessage(msgPendingImport))
	case state == svcsdktypes.KeyStateUnavailable:
		cr.SetConditions(xpv2.Unavailable().WithMessage(msgUnavailable))
	default:
		// Enabled, Disabled (as the spec may want it) or Updating.
		cr.SetConditions(xpv2.Available())
	}
	return obs, nil
}

// checkImmutable returns an error if a field that is fixed at creation
// differs from the key.
func checkImmutable(p *svcapitypes.KeyParameters, km *svcsdktypes.KeyMetadata) error {
	var xks, multiRegion *string
	if km.XksKeyConfiguration != nil {
		xks = km.XksKeyConfiguration.Id
	}
	if p.MultiRegion != nil {
		multiRegion = aws.String(strconv.FormatBool(*p.MultiRegion))
	}
	for _, f := range []struct {
		name            string
		desired, actual *string
	}{
		{"keySpec", p.KeySpec, aws.String(string(km.KeySpec))},
		{"keyUsage", p.KeyUsage, aws.String(string(km.KeyUsage))},
		{"origin", p.Origin, aws.String(string(km.Origin))},
		{"customKeyStoreID", p.CustomKeyStoreID, km.CustomKeyStoreId},
		{"xksKeyID", p.XksKeyID, xks},
		{"multiRegion", multiRegion, aws.String(strconv.FormatBool(aws.ToBool(km.MultiRegion)))},
	} {
		if f.desired != nil && *f.desired != pointer.StringValue(f.actual) {
			return errors.Errorf(errFmtImmutable, f.name, pointer.StringValue(f.actual), *f.desired)
		}
	}
	return nil
}

// isUpToDate compares the fields that are set in the spec. Policy, rotation
// and tags need their own read operation, which is only called if the spec
// sets the field.
func (h *hooks) isUpToDate(ctx context.Context, cr *svcapitypes.Key, resp *svcsdk.DescribeKeyOutput) (bool, string, error) { //nolint:gocyclo // one check per field
	km := resp.KeyMetadata
	p := &cr.Spec.ForProvider
	if err := checkImmutable(p, km); err != nil {
		return false, "", err
	}
	h.keyID = aws.ToString(km.KeyId)
	h.pendingDeletion = isPendingDeletion(km.KeyState)
	h.diff = nil
	id := aws.String(h.keyID)

	// In other states, KMS can't enable or disable the key.
	switch km.KeyState { //nolint:exhaustive // see above
	case svcsdktypes.KeyStateEnabled, svcsdktypes.KeyStateDisabled, svcsdktypes.KeyStatePendingDeletion, svcsdktypes.KeyStatePendingReplicaDeletion:
		if p.Enabled != nil && *p.Enabled != km.Enabled {
			h.diff = append(h.diff, fieldEnabled)
		}
	}
	if p.Description != nil && *p.Description != aws.ToString(km.Description) {
		h.diff = append(h.diff, fieldDescription)
	}
	if p.Policy != nil {
		pol, err := h.client.GetKeyPolicy(ctx, &svcsdk.GetKeyPolicyInput{KeyId: id, PolicyName: aws.String(defaultPolicyName)})
		if err != nil {
			return false, "", awsclient.Wrap(err, errGetPolicy)
		}
		if !compare.PolicyEqual(p.Policy, pol.Policy) {
			h.diff = append(h.diff, fieldPolicy)
		}
	}
	if p.Tags != nil {
		tags, err := h.client.ListResourceTags(ctx, &svcsdk.ListResourceTagsInput{KeyId: id, Limit: aws.Int32(maxTags)})
		if err != nil {
			return false, "", awsclient.Wrap(err, errListTags)
		}
		h.tagsAdd, h.tagsRemove = diffTags(p.Tags, tags.Tags)
		if len(h.tagsAdd)+len(h.tagsRemove) > 0 {
			h.diff = append(h.diff, fieldTags)
		}
	}
	if p.EnableKeyRotation != nil || p.RotationPeriodInDays != nil {
		rs, err := h.client.GetKeyRotationStatus(ctx, &svcsdk.GetKeyRotationStatusInput{KeyId: id})
		if err != nil {
			return false, "", awsclient.Wrap(err, errGetRotation)
		}
		setRotationStatus(&cr.Status.AtProvider, rs)
		h.rotation = rs.KeyRotationEnabled
		if p.EnableKeyRotation != nil {
			h.rotation = *p.EnableKeyRotation
		}
		switch {
		case h.rotation != rs.KeyRotationEnabled:
			h.diff = append(h.diff, fieldEnableKeyRotation)
		case h.rotation && p.RotationPeriodInDays != nil && *p.RotationPeriodInDays != aws.ToInt64(pointer.Int64(rs.RotationPeriodInDays)):
			h.diff = append(h.diff, fieldRotationPeriod)
		}
	}

	msg := compare.Message(h.diff)
	if h.pendingDeletion {
		msg = msgPendingDeletion + "; " + msg
	}
	return len(h.diff) == 0 && !h.pendingDeletion, msg, nil
}

func setRotationStatus(o *svcapitypes.KeyObservation, rs *svcsdk.GetKeyRotationStatusOutput) {
	o.KeyRotationEnabled = aws.Bool(rs.KeyRotationEnabled)
	o.RotationPeriodInDays = pointer.Int64(rs.RotationPeriodInDays)
	o.NextRotationDate = pointer.TimeToMetaTime(rs.NextRotationDate)
}

// diffTags returns the tags to add (new or changed values) and the keys to
// remove so that the key's tags equal the desired ones. Tags reserved by AWS
// are left alone.
func diffTags(desired []*svcapitypes.Tag, current []svcsdktypes.Tag) (add []svcsdktypes.Tag, remove []string) {
	want := desiredTags(desired)
	have := make(map[string]string, len(current))
	for _, t := range current {
		have[aws.ToString(t.TagKey)] = aws.ToString(t.TagValue)
	}
	for k, v := range want {
		if cv, ok := have[k]; !ok || cv != v {
			add = append(add, svcsdktypes.Tag{TagKey: aws.String(k), TagValue: aws.String(v)})
		}
	}
	sort.Slice(add, func(i, j int) bool { return *add[i].TagKey < *add[j].TagKey })
	for k := range have {
		if _, ok := want[k]; !ok && !awsclient.IsAWSTag(k) {
			remove = append(remove, k)
		}
	}
	sort.Strings(remove)
	return add, remove
}

func desiredTags(desired []*svcapitypes.Tag) map[string]string {
	want := make(map[string]string, len(desired))
	for _, t := range desired {
		if t != nil && t.TagKey != nil {
			want[*t.TagKey] = pointer.StringValue(t.TagValue)
		}
	}
	return want
}

// update changes what isUpToDate found to differ. A key scheduled for
// deletion is restored first; KMS restores it disabled. The key is enabled
// before and disabled after the other changes, because KMS can't change the
// rotation of a disabled key.
func (h *hooks) update(ctx context.Context, cr *svcapitypes.Key) (managed.ExternalUpdate, error) { //nolint:gocyclo // one step per field
	if h.keyID == "" {
		return managed.ExternalUpdate{}, errors.New(errNotObserved)
	}
	id := aws.String(h.keyID)
	p := &cr.Spec.ForProvider
	enable := slices.Contains(h.diff, fieldEnabled) && aws.ToBool(p.Enabled)
	disable := slices.Contains(h.diff, fieldEnabled) && !aws.ToBool(p.Enabled)

	if h.pendingDeletion {
		if _, err := h.client.CancelKeyDeletion(ctx, &svcsdk.CancelKeyDeletionInput{KeyId: id}); err != nil {
			return managed.ExternalUpdate{}, awsclient.Wrap(err, errCancelDeletion)
		}
	}
	if enable {
		if _, err := h.client.EnableKey(ctx, &svcsdk.EnableKeyInput{KeyId: id}); err != nil {
			return managed.ExternalUpdate{}, awsclient.Wrap(err, errEnable)
		}
	}
	for _, f := range h.diff {
		var err error
		switch f {
		case fieldDescription:
			_, err = h.client.UpdateKeyDescription(ctx, &svcsdk.UpdateKeyDescriptionInput{KeyId: id, Description: p.Description})
			err = awsclient.Wrap(err, errDescription)
		case fieldPolicy:
			_, err = h.client.PutKeyPolicy(ctx, &svcsdk.PutKeyPolicyInput{
				KeyId:                          id,
				PolicyName:                     aws.String(defaultPolicyName),
				Policy:                         p.Policy,
				BypassPolicyLockoutSafetyCheck: aws.ToBool(p.BypassPolicyLockoutSafetyCheck),
			})
			err = awsclient.Wrap(err, errPutPolicy)
		case fieldTags:
			err = h.updateTags(ctx, id)
		case fieldEnableKeyRotation, fieldRotationPeriod:
			err = h.updateRotation(ctx, id, p)
		}
		if err != nil {
			return managed.ExternalUpdate{}, err
		}
	}
	if disable {
		if _, err := h.client.DisableKey(ctx, &svcsdk.DisableKeyInput{KeyId: id}); err != nil {
			return managed.ExternalUpdate{}, awsclient.Wrap(err, errDisable)
		}
	}
	return managed.ExternalUpdate{}, nil
}

func (h *hooks) updateRotation(ctx context.Context, id *string, p *svcapitypes.KeyParameters) error {
	if !h.rotation {
		_, err := h.client.DisableKeyRotation(ctx, &svcsdk.DisableKeyRotationInput{KeyId: id})
		return awsclient.Wrap(err, errDisableRotation)
	}
	_, err := h.client.EnableKeyRotation(ctx, &svcsdk.EnableKeyRotationInput{KeyId: id, RotationPeriodInDays: pointer.Int32(p.RotationPeriodInDays)})
	return awsclient.Wrap(err, errEnableRotation)
}

func (h *hooks) updateTags(ctx context.Context, id *string) error {
	if len(h.tagsRemove) > 0 {
		if _, err := h.client.UntagResource(ctx, &svcsdk.UntagResourceInput{KeyId: id, TagKeys: h.tagsRemove}); err != nil {
			return awsclient.Wrap(err, errUntagResource)
		}
	}
	if len(h.tagsAdd) > 0 {
		if _, err := h.client.TagResource(ctx, &svcsdk.TagResourceInput{KeyId: id, Tags: h.tagsAdd}); err != nil {
			return awsclient.Wrap(err, errTagResource)
		}
	}
	return nil
}

// postCreate sets the external name to the key ID that AWS assigned. The
// first update applies enabled and rotation, which CreateKey can't set.
func postCreate(_ context.Context, cr *svcapitypes.Key, resp *svcsdk.CreateKeyOutput, cre managed.ExternalCreation, err error) (managed.ExternalCreation, error) {
	if err != nil {
		return cre, err
	}
	meta.SetExternalName(cr, aws.ToString(resp.KeyMetadata.KeyId))
	return cre, nil
}

func preDelete(_ context.Context, cr *svcapitypes.Key, obj *svcsdk.ScheduleKeyDeletionInput) (bool, error) {
	obj.KeyId = aws.String(meta.GetExternalName(cr))
	obj.PendingWindowInDays = pointer.Int32(cr.Spec.ForProvider.PendingWindowInDays)
	return false, nil
}

func setupExternal(e *external) {
	h := &hooks{client: e.client}
	e.preObserve = preObserve
	e.postObserve = postObserve
	e.isUpToDate = h.isUpToDate
	e.update = h.update
	e.postCreate = postCreate
	e.preDelete = preDelete
}

func init() {
	configure = func(ctrl.Manager, controller.Options) ([]option, []managed.ReconcilerOption, error) {
		// AWS assigns the key ID; the default would use metadata.name.
		return []option{setupExternal}, []managed.ReconcilerOption{managed.WithInitializers()}, nil
	}
}
