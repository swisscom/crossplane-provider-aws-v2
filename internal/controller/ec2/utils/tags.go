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

// Package utils contains helpers shared by the ec2 controllers.
package utils

import (
	"context"
	"maps"
	"slices"
	"sort"

	"github.com/aws/aws-sdk-go-v2/aws"
	svcsdk "github.com/aws/aws-sdk-go-v2/service/ec2"
	svcsdktypes "github.com/aws/aws-sdk-go-v2/service/ec2/types"
	"github.com/crossplane/crossplane-runtime/v2/pkg/resource"

	svcapitypes "github.com/swisscom/crossplane-provider-aws-v2/apis/ec2/v1alpha1"
	awsclient "github.com/swisscom/crossplane-provider-aws-v2/internal/clients/aws"
	pointer "github.com/swisscom/crossplane-provider-aws-v2/internal/utils/pointer"
	"github.com/swisscom/crossplane-provider-aws-v2/internal/utils/tags"
)

const (
	errCreateTags = "cannot create tags"
	errDeleteTags = "cannot delete tags"
)

// AnnotationManagedTagKeys records, comma-separated, the spec tag keys the
// provider manages. Only these keys are ever removed.
const AnnotationManagedTagKeys = "ec2.aws2.crossplane.io/managed-tag-keys"

// TagClient is the part of the EC2 API used to manage tags.
type TagClient interface {
	CreateTags(context.Context, *svcsdk.CreateTagsInput, ...func(*svcsdk.Options)) (*svcsdk.CreateTagsOutput, error)
	DeleteTags(context.Context, *svcsdk.DeleteTagsInput, ...func(*svcsdk.Options)) (*svcsdk.DeleteTagsOutput, error)
}

// DesiredTags returns the spec tags plus the external tags Crossplane sets on
// every resource (crossplane-kind, crossplane-name, crossplane-providerconfig).
// Spec tags win.
func DesiredTags(mg resource.Managed, spec []*svcapitypes.Tag) map[string]string {
	tags := resource.GetExternalTags(mg)
	for _, t := range spec {
		tags[pointer.StringValue(t.Key)] = pointer.StringValue(t.Value)
	}
	return tags
}

// TagSpecifications returns the tag specification of a create call.
func TagSpecifications(rt svcsdktypes.ResourceType, tags map[string]string) []svcsdktypes.TagSpecification {
	if len(tags) == 0 {
		return nil
	}
	return []svcsdktypes.TagSpecification{{ResourceType: rt, Tags: toSDK(tags)}}
}

// ObserveTags records the desired keys of mg, see tags.Record, and reports
// whether current is up to date and whether the record changed.
func ObserveTags(mg resource.Managed, desired map[string]string, current []*svcapitypes.Tag) (upToDate, recordChanged bool) {
	recordChanged = tags.Record(mg, AnnotationManagedTagKeys, slices.Collect(maps.Keys(desired)), keys(current))
	add, remove := DiffTags(desired, current, tags.Removable(mg, AnnotationManagedTagKeys))
	return len(add)+len(remove) == 0, recordChanged
}

// DiffTags returns the tags to create (new or changed values) and the
// managed keys in current to delete. Other tags are left alone.
func DiffTags(desired map[string]string, current []*svcapitypes.Tag, managed []string) (add []svcsdktypes.Tag, remove []svcsdktypes.Tag) {
	cur := make(map[string]string, len(current))
	for _, t := range current {
		cur[pointer.StringValue(t.Key)] = pointer.StringValue(t.Value)
	}
	toAdd := map[string]string{}
	for k, v := range desired {
		if cv, ok := cur[k]; !ok || cv != v {
			toAdd[k] = v
		}
	}
	for _, k := range managed {
		_, inAWS := cur[k]
		_, wanted := desired[k]
		if inAWS && !wanted && !tags.IsAWS(k) {
			remove = append(remove, svcsdktypes.Tag{Key: aws.String(k)})
		}
	}
	return toSDK(toAdd), remove
}

// UpdateTags creates the desired tags of the resource with the given ID and
// deletes the recorded keys of mg that are no longer desired. CreateTags
// overwrites the values of existing keys.
func UpdateTags(ctx context.Context, c TagClient, mg resource.Managed, id string, desired map[string]string, current []*svcapitypes.Tag) error {
	add, remove := DiffTags(desired, current, tags.Removable(mg, AnnotationManagedTagKeys))
	if len(remove) > 0 {
		if _, err := c.DeleteTags(ctx, &svcsdk.DeleteTagsInput{Resources: []string{id}, Tags: remove}); err != nil {
			return awsclient.Wrap(err, errDeleteTags)
		}
	}
	if len(add) > 0 {
		if _, err := c.CreateTags(ctx, &svcsdk.CreateTagsInput{Resources: []string{id}, Tags: add}); err != nil {
			return awsclient.Wrap(err, errCreateTags)
		}
	}
	return nil
}

// FromSDK converts SDK tags to API tags.
func FromSDK(tags []svcsdktypes.Tag) []*svcapitypes.Tag {
	if tags == nil {
		return nil
	}
	res := make([]*svcapitypes.Tag, 0, len(tags))
	for _, t := range tags {
		res = append(res, &svcapitypes.Tag{Key: t.Key, Value: t.Value})
	}
	return res
}

func keys(t []*svcapitypes.Tag) []string {
	out := make([]string, 0, len(t))
	for _, tg := range t {
		out = append(out, pointer.StringValue(tg.Key))
	}
	return out
}

func toSDK(tags map[string]string) []svcsdktypes.Tag {
	if len(tags) == 0 {
		return nil
	}
	res := make([]svcsdktypes.Tag, 0, len(tags))
	for k, v := range tags {
		res = append(res, svcsdktypes.Tag{Key: aws.String(k), Value: aws.String(v)})
	}
	sort.Slice(res, func(i, j int) bool { return *res[i].Key < *res[j].Key })
	return res
}
