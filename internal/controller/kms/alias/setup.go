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

package alias

import (
	"context"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/aws/arn"
	svcsdk "github.com/aws/aws-sdk-go-v2/service/kms"
	svcsdktypes "github.com/aws/aws-sdk-go-v2/service/kms/types"
	"github.com/crossplane/crossplane-runtime/v2/pkg/controller"
	"github.com/crossplane/crossplane-runtime/v2/pkg/errors"
	"github.com/crossplane/crossplane-runtime/v2/pkg/meta"
	"github.com/crossplane/crossplane-runtime/v2/pkg/reconciler/managed"
	xpv2 "github.com/crossplane/crossplane/apis/v2/core/v2"
	ctrl "sigs.k8s.io/controller-runtime"

	svcapitypes "github.com/swisscom/crossplane-provider-aws-v2/apis/kms/v1alpha1"
	"github.com/swisscom/crossplane-provider-aws-v2/internal/utils/compare"
	"github.com/swisscom/crossplane-provider-aws-v2/internal/utils/pointer"
)

const (
	aliasPrefix    = "alias/"
	awsAliasPrefix = "alias/aws/"

	errAWSAlias = "alias names starting with aws/ are reserved for AWS managed keys"
)

func init() {
	configure = configureAlias
}

func configureAlias(mgr ctrl.Manager, _ controller.Options) ([]option, []managed.ReconcilerOption, error) {
	return []option{setupExternal}, []managed.ReconcilerOption{
		managed.WithReferenceResolver(managed.NewAPISimpleReferenceResolver(mgr.GetClient())),
		managed.WithInitializers(managed.NewNameAsExternalName(mgr.GetClient())),
	}, nil
}

func setupExternal(e *external) {
	e.client = &listAllAliases{Client: e.client}
	e.filterList = filterList
	e.postObserve = postObserve
	e.isUpToDate = isUpToDate
	e.preCreate = preCreate
	e.preUpdate = preUpdate
	e.preDelete = preDelete
}

// listAllAliases returns the aliases of all pages: ListAliases can't filter
// by name, and filtering by the target key wouldn't find the alias after
// its target changed.
type listAllAliases struct {
	Client
}

func (c *listAllAliases) ListAliases(ctx context.Context, in *svcsdk.ListAliasesInput, opts ...func(*svcsdk.Options)) (*svcsdk.ListAliasesOutput, error) {
	out := &svcsdk.ListAliasesOutput{}
	p := svcsdk.NewListAliasesPaginator(c.Client, in)
	for p.HasMorePages() {
		page, err := p.NextPage(ctx, opts...)
		if err != nil {
			return nil, err
		}
		out.Aliases = append(out.Aliases, page.Aliases...)
	}
	return out, nil
}

func aliasName(cr *svcapitypes.Alias) *string {
	return aws.String(aliasPrefix + meta.GetExternalName(cr))
}

func filterList(cr *svcapitypes.Alias, list *svcsdk.ListAliasesOutput) *svcsdk.ListAliasesOutput {
	name := aliasName(cr)
	for _, a := range list.Aliases {
		if pointer.StringValue(a.AliasName) == *name {
			return &svcsdk.ListAliasesOutput{Aliases: []svcsdktypes.AliasListEntry{a}}
		}
	}
	return &svcsdk.ListAliasesOutput{}
}

func postObserve(_ context.Context, cr *svcapitypes.Alias, _ *svcsdk.ListAliasesOutput, obs managed.ExternalObservation, err error) (managed.ExternalObservation, error) {
	if err != nil {
		return obs, err
	}
	cr.SetConditions(xpv2.Available())
	return obs, nil
}

func isUpToDate(_ context.Context, cr *svcapitypes.Alias, resp *svcsdk.ListAliasesOutput) (bool, string, error) {
	diff := DiffUpdatableParameters(&cr.Spec.ForProvider, &GenerateAlias(resp).Spec.ForProvider)
	if keyID(cr.Spec.ForProvider.TargetKeyID) != pointer.StringValue(resp.Aliases[0].TargetKeyId) {
		diff = append(diff, "targetKeyId")
	}
	return len(diff) == 0, compare.Message(diff), nil
}

// keyID returns the key ID of a key ID or key ARN; ListAliases returns IDs.
func keyID(idOrARN *string) string {
	s := pointer.StringValue(idOrARN)
	if a, err := arn.Parse(s); err == nil {
		return strings.TrimPrefix(a.Resource, "key/")
	}
	return s
}

func preCreate(_ context.Context, cr *svcapitypes.Alias, obj *svcsdk.CreateAliasInput) error {
	obj.AliasName = aliasName(cr)
	if strings.HasPrefix(*obj.AliasName, awsAliasPrefix) {
		return errors.New(errAWSAlias)
	}
	obj.TargetKeyId = cr.Spec.ForProvider.TargetKeyID
	return nil
}

func preUpdate(_ context.Context, cr *svcapitypes.Alias, obj *svcsdk.UpdateAliasInput) error {
	obj.AliasName = aliasName(cr)
	obj.TargetKeyId = cr.Spec.ForProvider.TargetKeyID
	return nil
}

func preDelete(_ context.Context, cr *svcapitypes.Alias, obj *svcsdk.DeleteAliasInput) (bool, error) {
	obj.AliasName = aliasName(cr)
	return false, nil
}
