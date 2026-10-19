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
	"sort"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	svcsdk "github.com/aws/aws-sdk-go-v2/service/ec2"
	svcsdktypes "github.com/aws/aws-sdk-go-v2/service/ec2/types"
	"github.com/crossplane/crossplane-runtime/v2/pkg/resource"

	svcapitypes "github.com/swisscom/crossplane-provider-aws-v2/apis/ec2/v1alpha1"
	awsclient "github.com/swisscom/crossplane-provider-aws-v2/internal/clients/aws"
	pointer "github.com/swisscom/crossplane-provider-aws-v2/internal/utils/pointer"
)

const (
	errCreateTags = "cannot create tags"
	errDeleteTags = "cannot delete tags"

	// awsTagPrefix marks tags reserved by AWS; they can't be changed.
	awsTagPrefix = "aws:"
)

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

// DiffTags returns the tags to create (new or changed values) and to delete
// so that current equals desired. Tags reserved by AWS are left alone.
func DiffTags(desired map[string]string, current []*svcapitypes.Tag) (add []svcsdktypes.Tag, remove []svcsdktypes.Tag) {
	cur := make(map[string]string, len(current))
	for _, t := range current {
		k := pointer.StringValue(t.Key)
		if strings.HasPrefix(k, awsTagPrefix) {
			continue
		}
		cur[k] = pointer.StringValue(t.Value)
	}
	toAdd := map[string]string{}
	for k, v := range desired {
		if cv, ok := cur[k]; !ok || cv != v {
			toAdd[k] = v
		}
	}
	for k := range cur {
		if _, ok := desired[k]; !ok {
			remove = append(remove, svcsdktypes.Tag{Key: aws.String(k)})
		}
	}
	sort.Slice(remove, func(i, j int) bool { return *remove[i].Key < *remove[j].Key })
	return toSDK(toAdd), remove
}

// UpdateTags makes the tags of the resource with the given ID equal desired.
// CreateTags overwrites the values of existing keys.
func UpdateTags(ctx context.Context, c TagClient, id string, desired map[string]string, current []*svcapitypes.Tag) error {
	add, remove := DiffTags(desired, current)
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
