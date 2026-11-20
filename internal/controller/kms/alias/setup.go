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
)

const (
	// aliasPrefix starts every alias name.
	aliasPrefix = "alias/"
	// maxAliases is the largest page ListAliases returns.
	maxAliases = 100

	fieldTargetKeyID = "targetKeyID"

	errNoTargetKey = "targetKeyID is not set: set it, targetKeyIDRef or targetKeyIDSelector"
)

// aliasName returns the alias name: the external name, with the "alias/"
// prefix added if it lacks it (metadata.name can't contain "/").
func aliasName(cr *svcapitypes.Alias) string {
	n := meta.GetExternalName(cr)
	if strings.HasPrefix(n, aliasPrefix) {
		return n
	}
	return aliasPrefix + n
}

// allPages makes ListAliases return the aliases of all pages: KMS has no
// operation that reads a single alias.
type allPages struct {
	Client
}

func (c allPages) ListAliases(ctx context.Context, in *svcsdk.ListAliasesInput, opts ...func(*svcsdk.Options)) (*svcsdk.ListAliasesOutput, error) {
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

func preObserve(_ context.Context, _ *svcapitypes.Alias, obj *svcsdk.ListAliasesInput) error {
	obj.Limit = aws.Int32(maxAliases)
	return nil
}

// filterList keeps the alias whose name is the alias name of cr.
func filterList(cr *svcapitypes.Alias, list *svcsdk.ListAliasesOutput) *svcsdk.ListAliasesOutput {
	name := aliasName(cr)
	for _, a := range list.Aliases {
		if aws.ToString(a.AliasName) == name {
			return &svcsdk.ListAliasesOutput{Aliases: []svcsdktypes.AliasListEntry{a}}
		}
	}
	return &svcsdk.ListAliasesOutput{}
}

func postObserve(_ context.Context, cr *svcapitypes.Alias, _ *svcsdk.ListAliasesOutput, obs managed.ExternalObservation, err error) (managed.ExternalObservation, error) {
	if err != nil || !obs.ResourceExists {
		return obs, err
	}
	cr.SetConditions(xpv2.Available())
	return obs, nil
}

// isUpToDate compares the target key; AWS returns its key ID, the spec may
// have its ARN.
func isUpToDate(_ context.Context, cr *svcapitypes.Alias, resp *svcsdk.ListAliasesOutput) (bool, string, error) {
	var diff []string
	if t := cr.Spec.ForProvider.TargetKeyID; t != nil && !compare.KMSKeyIDEqual(*t, aws.ToString(resp.Aliases[0].TargetKeyId)) {
		diff = append(diff, fieldTargetKeyID)
	}
	return len(diff) == 0, compare.Message(diff), nil
}

func preCreate(_ context.Context, cr *svcapitypes.Alias, obj *svcsdk.CreateAliasInput) error {
	if cr.Spec.ForProvider.TargetKeyID == nil {
		return errors.New(errNoTargetKey)
	}
	obj.AliasName = aws.String(aliasName(cr))
	obj.TargetKeyId = cr.Spec.ForProvider.TargetKeyID
	return nil
}

func preUpdate(_ context.Context, cr *svcapitypes.Alias, obj *svcsdk.UpdateAliasInput) error {
	if cr.Spec.ForProvider.TargetKeyID == nil {
		return errors.New(errNoTargetKey)
	}
	obj.AliasName = aws.String(aliasName(cr))
	obj.TargetKeyId = cr.Spec.ForProvider.TargetKeyID
	return nil
}

func preDelete(_ context.Context, cr *svcapitypes.Alias, obj *svcsdk.DeleteAliasInput) (bool, error) {
	obj.AliasName = aws.String(aliasName(cr))
	return false, nil
}

func setupExternal(e *external) {
	e.client = allPages{Client: e.client}
	e.preObserve = preObserve
	e.filterList = filterList
	e.postObserve = postObserve
	e.isUpToDate = isUpToDate
	e.preCreate = preCreate
	e.preUpdate = preUpdate
	e.preDelete = preDelete
}

func init() {
	configure = func(mgr ctrl.Manager, _ controller.Options) ([]option, []managed.ReconcilerOption, error) {
		return []option{setupExternal}, []managed.ReconcilerOption{
			managed.WithReferenceResolver(managed.NewAPISimpleReferenceResolver(mgr.GetClient())),
		}, nil
	}
}
