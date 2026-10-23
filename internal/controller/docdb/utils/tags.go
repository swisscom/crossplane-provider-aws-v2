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

	svcsdk "github.com/aws/aws-sdk-go-v2/service/docdb"
	svcsdktypes "github.com/aws/aws-sdk-go-v2/service/docdb/types"
	"github.com/crossplane/crossplane-runtime/v2/pkg/resource"
	"k8s.io/apimachinery/pkg/util/sets"

	svcapitypes "github.com/swisscom/crossplane-provider-aws-v2/apis/docdb/v1alpha1"
	awsclient "github.com/swisscom/crossplane-provider-aws-v2/internal/clients/aws"
	pointer "github.com/swisscom/crossplane-provider-aws-v2/internal/utils/pointer"
	"github.com/swisscom/crossplane-provider-aws-v2/internal/utils/tags"
)

const (
	errListTagsForResource = "cannot list tags"
	errRemoveTags          = "cannot remove tags"
	errCreateTags          = "cannot create tags"
)

// AnnotationManagedTagKeys records, comma-separated, the tag keys the
// provider set. Only these keys are ever removed.
const AnnotationManagedTagKeys = "docdb.aws2.crossplane.io/managed-tag-keys"

// TagClient is the part of the DocumentDB API used to manage tags.
type TagClient interface {
	ListTagsForResource(context.Context, *svcsdk.ListTagsForResourceInput, ...func(*svcsdk.Options)) (*svcsdk.ListTagsForResourceOutput, error)
	AddTagsToResource(context.Context, *svcsdk.AddTagsToResourceInput, ...func(*svcsdk.Options)) (*svcsdk.AddTagsToResourceOutput, error)
	RemoveTagsFromResource(context.Context, *svcsdk.RemoveTagsFromResourceInput, ...func(*svcsdk.Options)) (*svcsdk.RemoveTagsFromResourceOutput, error)
}

// ObserveTags lists the tags of arn, updates the managed-tag-keys
// annotation of mg in memory and reports whether the tags are up to date and
// whether the annotation changed. The caller must get the annotation
// persisted, e.g. by reporting the resource as late-initialized.
func ObserveTags(ctx context.Context, c TagClient, mg resource.Managed, spec []*svcapitypes.Tag, arn *string) (upToDate, annotationChanged bool, err error) {
	current, err := listTags(ctx, c, arn)
	if err != nil {
		return false, false, err
	}
	annotationChanged = tags.Record(mg, AnnotationManagedTagKeys, keys(spec), tagKeys(current))
	add, remove := DiffTags(spec, current, tags.Removable(mg, AnnotationManagedTagKeys))
	return len(add) == 0 && len(remove) == 0, annotationChanged, nil
}

// UpdateTagsForResource adds or updates the tags of the spec and removes
// recorded keys that left the spec.
func UpdateTagsForResource(ctx context.Context, c TagClient, mg resource.Managed, spec []*svcapitypes.Tag, arn *string) error {
	current, err := listTags(ctx, c, arn)
	if err != nil {
		return err
	}
	add, remove := DiffTags(spec, current, tags.Removable(mg, AnnotationManagedTagKeys))
	if len(add) != 0 {
		if _, err := c.AddTagsToResource(ctx, &svcsdk.AddTagsToResourceInput{ResourceName: arn, Tags: add}); err != nil {
			return awsclient.Wrap(err, errCreateTags)
		}
	}
	if len(remove) != 0 {
		if _, err := c.RemoveTagsFromResource(ctx, &svcsdk.RemoveTagsFromResourceInput{ResourceName: arn, TagKeys: remove}); err != nil {
			return awsclient.Wrap(err, errRemoveTags)
		}
	}
	return nil
}

// DiffTags returns the spec tags that are missing or have another value in
// current, and the managed keys that are in current but not in spec.
// AddTagsToResource overwrites values, so a changed value is never removed
// first. Keys that are not managed are never removed.
func DiffTags(spec []*svcapitypes.Tag, current []svcsdktypes.Tag, managed []string) (add []svcsdktypes.Tag, remove []string) {
	currentMap := make(map[string]string, len(current))
	for _, t := range current {
		currentMap[pointer.StringValue(t.Key)] = pointer.StringValue(t.Value)
	}
	for _, t := range spec {
		key, val := pointer.StringValue(t.Key), pointer.StringValue(t.Value)
		if cur, ok := currentMap[key]; ok && cur == val {
			continue
		}
		add = append(add, svcsdktypes.Tag{Key: pointer.ToOrNilIfZeroValue(key), Value: pointer.ToOrNilIfZeroValue(val)})
	}
	inSpec := sets.New(keys(spec)...)
	for _, key := range managed {
		if _, ok := currentMap[key]; ok && !inSpec.Has(key) && !tags.IsAWS(key) {
			remove = append(remove, key)
		}
	}
	return add, remove
}

func listTags(ctx context.Context, c TagClient, arn *string) ([]svcsdktypes.Tag, error) {
	resp, err := c.ListTagsForResource(ctx, &svcsdk.ListTagsForResourceInput{ResourceName: arn})
	if err != nil {
		return nil, awsclient.Wrap(err, errListTagsForResource)
	}
	return resp.TagList, nil
}

func keys(spec []*svcapitypes.Tag) []string {
	out := make([]string, 0, len(spec))
	for _, t := range spec {
		out = append(out, pointer.StringValue(t.Key))
	}
	return out
}

func tagKeys(current []svcsdktypes.Tag) []string {
	out := make([]string, 0, len(current))
	for _, t := range current {
		out = append(out, pointer.StringValue(t.Key))
	}
	return out
}
