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

package utils

import (
	"context"
	"sort"

	svcsdk "github.com/aws/aws-sdk-go-v2/service/docdb"
	svcsdktypes "github.com/aws/aws-sdk-go-v2/service/docdb/types"
	"github.com/crossplane/crossplane-runtime/v2/pkg/errors"
	"github.com/crossplane/crossplane-runtime/v2/pkg/resource"

	svcapitypes "github.com/swisscom/crossplane-provider-aws-v2/apis/docdb/v1alpha1"
	pointer "github.com/swisscom/crossplane-provider-aws-v2/internal/utils/pointer"
)

const (
	errListTagsForResource = "cannot list tags"
	errRemoveTags          = "cannot remove tags"
	errCreateTags          = "cannot create tags"
)

// TagClient is the part of the DocumentDB API used to manage tags.
type TagClient interface {
	ListTagsForResource(context.Context, *svcsdk.ListTagsForResourceInput, ...func(*svcsdk.Options)) (*svcsdk.ListTagsForResourceOutput, error)
	AddTagsToResource(context.Context, *svcsdk.AddTagsToResourceInput, ...func(*svcsdk.Options)) (*svcsdk.AddTagsToResourceOutput, error)
	RemoveTagsFromResource(context.Context, *svcsdk.RemoveTagsFromResourceInput, ...func(*svcsdk.Options)) (*svcsdk.RemoveTagsFromResourceOutput, error)
}

// AreTagsUpToDate for spec and resourceName
func AreTagsUpToDate(ctx context.Context, client TagClient, spec []*svcapitypes.Tag, resourceName *string) (bool, error) {
	current, err := ListTagsForResource(ctx, client, resourceName)
	if err != nil {
		return false, err
	}

	add, remove := DiffTags(spec, current)

	return len(add) == 0 && len(remove) == 0, nil
}

// UpdateTagsForResource with resourceName
func UpdateTagsForResource(ctx context.Context, client TagClient, spec []*svcapitypes.Tag, resourceName *string) error {
	current, err := ListTagsForResource(ctx, client, resourceName)
	if err != nil {
		return err
	}

	add, remove := DiffTags(spec, current)
	if len(remove) != 0 {
		if _, err := client.RemoveTagsFromResource(ctx, &svcsdk.RemoveTagsFromResourceInput{
			ResourceName: resourceName,
			TagKeys:      remove,
		}); err != nil {
			return errors.Wrap(err, errRemoveTags)
		}
	}
	if len(add) != 0 {
		if _, err := client.AddTagsToResource(ctx, &svcsdk.AddTagsToResourceInput{
			ResourceName: resourceName,
			Tags:         add,
		}); err != nil {
			return errors.Wrap(err, errCreateTags)
		}
	}

	return nil
}

// ListTagsForResource for the given resource
func ListTagsForResource(ctx context.Context, client TagClient, resourceName *string) ([]svcsdktypes.Tag, error) {
	resp, err := client.ListTagsForResource(ctx, &svcsdk.ListTagsForResourceInput{
		ResourceName: resourceName,
	})
	if err != nil {
		return nil, errors.Wrap(err, errListTagsForResource)
	}

	return resp.TagList, nil
}

// DiffTags between spec and current
func DiffTags(spec []*svcapitypes.Tag, current []svcsdktypes.Tag) (addTags []svcsdktypes.Tag, removeTags []string) {
	currentMap := make(map[string]string, len(current))
	for _, t := range current {
		currentMap[pointer.StringValue(t.Key)] = pointer.StringValue(t.Value)
	}

	specMap := make(map[string]string, len(spec))
	for _, t := range spec {
		key := pointer.StringValue(t.Key)
		val := pointer.StringValue(t.Value)
		specMap[key] = pointer.StringValue(t.Value)

		if currentVal, exists := currentMap[key]; exists {
			if currentVal != val {
				removeTags = append(removeTags, key)
				addTags = append(addTags, svcsdktypes.Tag{
					Key:   pointer.ToOrNilIfZeroValue(key),
					Value: pointer.ToOrNilIfZeroValue(val),
				})
			}
		} else {
			addTags = append(addTags, svcsdktypes.Tag{
				Key:   pointer.ToOrNilIfZeroValue(key),
				Value: pointer.ToOrNilIfZeroValue(val),
			})
		}
	}

	for _, t := range current {
		key := pointer.StringValue(t.Key)
		if _, exists := specMap[key]; !exists {
			removeTags = append(removeTags, key)
		}
	}

	return addTags, removeTags
}

// AddExternalTags to spec if they don't exist
func AddExternalTags(mg resource.Managed, spec []*svcapitypes.Tag) []*svcapitypes.Tag {
	tagMap := make(map[string]struct{}, len(spec))
	for _, t := range spec {
		tagMap[pointer.StringValue(t.Key)] = struct{}{}
	}

	tags := spec
	for _, t := range GetExternalTags(mg) {
		if _, exists := tagMap[pointer.StringValue(t.Key)]; !exists {
			tags = append(tags, t)
		}
	}

	return tags
}

// GetExternalTags is a wrapper around resource.GetExternalTags to return a sorted array instead of a map
func GetExternalTags(mg resource.Managed) []*svcapitypes.Tag {
	externalTags := []*svcapitypes.Tag{}
	for k, v := range resource.GetExternalTags(mg) {
		externalTags = append(externalTags, &svcapitypes.Tag{Key: pointer.ToOrNilIfZeroValue(k), Value: pointer.ToOrNilIfZeroValue(v)})
	}

	sort.Slice(externalTags, func(i, j int) bool {
		return pointer.StringValue(externalTags[i].Key) > pointer.StringValue(externalTags[j].Key)
	})

	return externalTags
}
