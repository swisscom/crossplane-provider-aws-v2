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

package loggroup

import (
	"context"
	"sort"

	"github.com/aws/aws-sdk-go-v2/aws"
	svcsdk "github.com/aws/aws-sdk-go-v2/service/cloudwatchlogs"
	svcsdktypes "github.com/aws/aws-sdk-go-v2/service/cloudwatchlogs/types"
	"github.com/crossplane/crossplane-runtime/v2/pkg/controller"
	"github.com/crossplane/crossplane-runtime/v2/pkg/errors"
	"github.com/crossplane/crossplane-runtime/v2/pkg/meta"
	"github.com/crossplane/crossplane-runtime/v2/pkg/reconciler/managed"
	xpv2 "github.com/crossplane/crossplane/apis/v2/core/v2"
	ctrl "sigs.k8s.io/controller-runtime"

	svcapitypes "github.com/swisscom/crossplane-provider-aws-v2/apis/cloudwatchlogs/v1alpha1"
	awsclient "github.com/swisscom/crossplane-provider-aws-v2/internal/clients/aws"
	"github.com/swisscom/crossplane-provider-aws-v2/internal/utils/compare"
	pointer "github.com/swisscom/crossplane-provider-aws-v2/internal/utils/pointer"
)

const (
	errNotObserved        = "the log group was not observed before the update"
	errPutRetention       = "cannot set the retention"
	errDeleteRetention    = "cannot remove the retention"
	errAssociateKMSKey    = "cannot associate the KMS key"
	errDeletionProtection = "cannot set the deletion protection"
	errListTags           = "cannot list tags"
	errTagResource        = "cannot add tags"
	errUntagResource      = "cannot remove tags"
)

// Fields of LogGroupParameters (JSON names) that isUpToDate compares.
const (
	fieldRetention          = "retentionInDays"
	fieldKMSKeyID           = "kmsKeyID"
	fieldDeletionProtection = "deletionProtectionEnabled"
	fieldTags               = "tags"
)

// hooks keeps what Observe found for update: the reconciler calls Update in
// the same reconcile, on the same external client.
type hooks struct {
	client Client

	// observed is the log group found by isUpToDate, diff its differing
	// fields.
	observed *svcsdktypes.LogGroup
	diff     []string
	// tagsAdd and tagsRemove make the log group's tags equal the desired ones.
	tagsAdd    map[string]string
	tagsRemove []string
}

func preObserve(_ context.Context, cr *svcapitypes.LogGroup, obj *svcsdk.DescribeLogGroupsInput) error {
	obj.LogGroupNamePrefix = aws.String(meta.GetExternalName(cr))
	return nil
}

// filterList keeps the log group whose name is the external name; the
// prefix also matches longer names.
func filterList(cr *svcapitypes.LogGroup, list *svcsdk.DescribeLogGroupsOutput) *svcsdk.DescribeLogGroupsOutput {
	name := meta.GetExternalName(cr)
	for _, lg := range list.LogGroups {
		if pointer.StringValue(lg.LogGroupName) == name {
			return &svcsdk.DescribeLogGroupsOutput{LogGroups: []svcsdktypes.LogGroup{lg}}
		}
	}
	return &svcsdk.DescribeLogGroupsOutput{}
}

func postObserve(_ context.Context, cr *svcapitypes.LogGroup, _ *svcsdk.DescribeLogGroupsOutput, obs managed.ExternalObservation, err error) (managed.ExternalObservation, error) {
	if err != nil {
		return obs, err
	}
	cr.SetConditions(xpv2.Available())
	return obs, nil
}

func (h *hooks) isUpToDate(ctx context.Context, cr *svcapitypes.LogGroup, resp *svcsdk.DescribeLogGroupsOutput) (bool, string, error) {
	lg := resp.LogGroups[0]
	h.observed = &lg
	h.diff = diffLogGroup(&cr.Spec.ForProvider, &lg)

	tags, err := h.client.ListTagsForResource(ctx, &svcsdk.ListTagsForResourceInput{ResourceArn: lg.LogGroupArn})
	if err != nil {
		return false, "", awsclient.Wrap(err, errListTags)
	}
	h.tagsAdd, h.tagsRemove = diffTags(desiredTags(cr), tags.Tags)
	if len(h.tagsAdd)+len(h.tagsRemove) > 0 {
		h.diff = append(h.diff, fieldTags)
	}
	return len(h.diff) == 0, compare.Message(h.diff), nil
}

// diffLogGroup returns the JSON names of the fields that differ. Unset
// fields are not managed. The log group class can't be changed.
func diffLogGroup(p *svcapitypes.LogGroupParameters, lg *svcsdktypes.LogGroup) []string {
	var diff []string
	// AWS returns no retention for log groups that keep their events forever.
	if p.RetentionInDays != nil && *p.RetentionInDays != aws.ToInt64(pointer.Int64(lg.RetentionInDays)) {
		diff = append(diff, fieldRetention)
	}
	if p.KMSKeyID != nil && !compare.KMSKeyIDEqual(*p.KMSKeyID, pointer.StringValue(lg.KmsKeyId)) {
		diff = append(diff, fieldKMSKeyID)
	}
	if p.DeletionProtectionEnabled != nil && *p.DeletionProtectionEnabled != aws.ToBool(lg.DeletionProtectionEnabled) {
		diff = append(diff, fieldDeletionProtection)
	}
	return diff
}

// desiredTags returns the spec tags: the tags the log group should have,
// besides those AWS sets.
func desiredTags(cr *svcapitypes.LogGroup) map[string]string {
	if len(cr.Spec.ForProvider.Tags) == 0 {
		return nil
	}
	tags := make(map[string]string, len(cr.Spec.ForProvider.Tags))
	for k, v := range cr.Spec.ForProvider.Tags {
		tags[k] = pointer.StringValue(v)
	}
	return tags
}

// diffTags returns the tags to add (new or changed values) and the keys to
// remove so that current equals desired. Tags reserved by AWS are left alone.
func diffTags(desired, current map[string]string) (add map[string]string, remove []string) {
	add = map[string]string{}
	for k, v := range desired {
		if cv, ok := current[k]; !ok || cv != v {
			add[k] = v
		}
	}
	for k := range current {
		if _, ok := desired[k]; !ok && !awsclient.IsAWSTag(k) {
			remove = append(remove, k)
		}
	}
	sort.Strings(remove)
	return add, remove
}

// preCreate sets the name and the tags. The retention can't be set on
// creation; the first update sets it.
func preCreate(_ context.Context, cr *svcapitypes.LogGroup, obj *svcsdk.CreateLogGroupInput) error {
	obj.LogGroupName = aws.String(meta.GetExternalName(cr))
	obj.Tags = desiredTags(cr)
	return nil
}

func preDelete(_ context.Context, cr *svcapitypes.LogGroup, obj *svcsdk.DeleteLogGroupInput) (bool, error) {
	obj.LogGroupName = aws.String(meta.GetExternalName(cr))
	return false, nil
}

// update changes what isUpToDate found to differ; AWS has an operation for
// each field.
func (h *hooks) update(ctx context.Context, cr *svcapitypes.LogGroup) (managed.ExternalUpdate, error) { //nolint:gocyclo // one step per field
	if h.observed == nil {
		return managed.ExternalUpdate{}, errors.New(errNotObserved)
	}
	name := aws.String(meta.GetExternalName(cr))
	p := &cr.Spec.ForProvider
	for _, f := range h.diff {
		var err error
		switch f {
		case fieldRetention:
			if *p.RetentionInDays == 0 {
				_, err = h.client.DeleteRetentionPolicy(ctx, &svcsdk.DeleteRetentionPolicyInput{LogGroupName: name})
				err = awsclient.Wrap(err, errDeleteRetention)
			} else {
				_, err = h.client.PutRetentionPolicy(ctx, &svcsdk.PutRetentionPolicyInput{LogGroupName: name, RetentionInDays: pointer.Int32(p.RetentionInDays)})
				err = awsclient.Wrap(err, errPutRetention)
			}
		case fieldKMSKeyID:
			// Replaces the key the log group may have.
			_, err = h.client.AssociateKmsKey(ctx, &svcsdk.AssociateKmsKeyInput{LogGroupName: name, KmsKeyId: p.KMSKeyID})
			err = awsclient.Wrap(err, errAssociateKMSKey)
		case fieldDeletionProtection:
			_, err = h.client.PutLogGroupDeletionProtection(ctx, &svcsdk.PutLogGroupDeletionProtectionInput{LogGroupIdentifier: name, DeletionProtectionEnabled: p.DeletionProtectionEnabled})
			err = awsclient.Wrap(err, errDeletionProtection)
		case fieldTags:
			err = h.updateTags(ctx)
		}
		if err != nil {
			return managed.ExternalUpdate{}, err
		}
	}
	return managed.ExternalUpdate{}, nil
}

func (h *hooks) updateTags(ctx context.Context) error {
	arn := h.observed.LogGroupArn
	if len(h.tagsRemove) > 0 {
		if _, err := h.client.UntagResource(ctx, &svcsdk.UntagResourceInput{ResourceArn: arn, TagKeys: h.tagsRemove}); err != nil {
			return awsclient.Wrap(err, errUntagResource)
		}
	}
	if len(h.tagsAdd) > 0 {
		if _, err := h.client.TagResource(ctx, &svcsdk.TagResourceInput{ResourceArn: arn, Tags: h.tagsAdd}); err != nil {
			return awsclient.Wrap(err, errTagResource)
		}
	}
	return nil
}

func setupExternal(e *external) {
	h := &hooks{client: e.client}
	e.preObserve = preObserve
	e.filterList = filterList
	e.postObserve = postObserve
	e.isUpToDate = h.isUpToDate
	e.preCreate = preCreate
	e.preDelete = preDelete
	e.update = h.update
}

func init() {
	configure = func(ctrl.Manager, controller.Options) ([]option, []managed.ReconcilerOption, error) {
		return []option{setupExternal}, nil, nil
	}
}
