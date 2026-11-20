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

package grant

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
	// Connection secret keys.
	connGrantID    = "grantID"
	connGrantToken = "grantToken"

	errNoKeyID      = "keyID is not set: set it, keyIDRef or keyIDSelector"
	errFmtImmutable = "grants can't be changed, but these fields differ from the grant in AWS: %s; create a new Grant instead"
)

// keyID returns the key of the grant: the observed one, so that a changed
// spec still finds the grant, else the spec.
func keyID(cr *svcapitypes.Grant) *string {
	if k := cr.Status.AtProvider.KeyID; k != nil {
		return k
	}
	return cr.Spec.ForProvider.KeyID
}

func preObserve(_ context.Context, cr *svcapitypes.Grant, obj *svcsdk.ListGrantsInput) error {
	obj.KeyId = keyID(cr)
	if obj.KeyId == nil {
		return errors.New(errNoKeyID)
	}
	obj.GrantId = aws.String(meta.GetExternalName(cr))
	return nil
}

// filterList keeps the grant whose ID is the external name.
func filterList(cr *svcapitypes.Grant, list *svcsdk.ListGrantsOutput) *svcsdk.ListGrantsOutput {
	id := meta.GetExternalName(cr)
	for _, g := range list.Grants {
		if aws.ToString(g.GrantId) == id {
			return &svcsdk.ListGrantsOutput{Grants: []svcsdktypes.GrantListEntry{g}}
		}
	}
	return &svcsdk.ListGrantsOutput{}
}

func postObserve(_ context.Context, cr *svcapitypes.Grant, _ *svcsdk.ListGrantsOutput, obs managed.ExternalObservation, err error) (managed.ExternalObservation, error) {
	if err != nil || !obs.ResourceExists {
		return obs, err
	}
	cr.SetConditions(xpv2.Available())
	return obs, nil
}

// principalEqual reports whether the desired grantee or retiring principal
// equals the observed one. For grantees that are AWS services, KMS may
// return a service principal instead of the ARN, which isn't compared.
func principalEqual(desired, observed *string) bool {
	if desired == nil || *desired == aws.ToString(observed) {
		return true
	}
	return !strings.HasPrefix(aws.ToString(observed), "arn:")
}

// isUpToDate returns an error if the spec differs from the grant: KMS can't
// change grants.
func isUpToDate(_ context.Context, cr *svcapitypes.Grant, resp *svcsdk.ListGrantsOutput) (bool, string, error) {
	p := &cr.Spec.ForProvider
	o := GenerateGrant(resp).Spec.ForProvider
	var diff []string
	if p.KeyID != nil && !compare.KMSKeyIDEqual(*p.KeyID, aws.ToString(resp.Grants[0].KeyId)) {
		diff = append(diff, "keyID")
	}
	for _, f := range []struct {
		name  string
		equal bool
	}{
		{"name", compare.IsSubset(p.Name, o.Name)},
		{"operations", compare.IsSubset(p.Operations, o.Operations)},
		{"constraints", compare.IsSubset(p.Constraints, o.Constraints)},
		{"granteePrincipal", principalEqual(p.GranteePrincipal, o.GranteePrincipal)},
		{"granteeServicePrincipal", compare.IsSubset(p.GranteeServicePrincipal, o.GranteeServicePrincipal)},
		{"retiringPrincipal", principalEqual(p.RetiringPrincipal, o.RetiringPrincipal)},
		{"retiringServicePrincipal", compare.IsSubset(p.RetiringServicePrincipal, o.RetiringServicePrincipal)},
	} {
		if !f.equal {
			diff = append(diff, f.name)
		}
	}
	if len(diff) > 0 {
		return false, "", errors.Errorf(errFmtImmutable, strings.Join(diff, ", "))
	}
	return true, "", nil
}

// preCreate names the grant after the resource unless the spec names it:
// with a name, KMS returns the existing grant when CreateGrant is retried
// with the same parameters instead of creating a duplicate.
func preCreate(_ context.Context, cr *svcapitypes.Grant, obj *svcsdk.CreateGrantInput) error {
	if cr.Spec.ForProvider.KeyID == nil {
		return errors.New(errNoKeyID)
	}
	obj.KeyId = cr.Spec.ForProvider.KeyID
	if obj.Name == nil {
		obj.Name = aws.String(cr.GetName())
	}
	return nil
}

// postCreate sets the external name to the grant ID that AWS assigned and
// publishes the grant token, which is only returned on creation.
func postCreate(_ context.Context, cr *svcapitypes.Grant, resp *svcsdk.CreateGrantOutput, cre managed.ExternalCreation, err error) (managed.ExternalCreation, error) {
	if err != nil {
		return cre, err
	}
	meta.SetExternalName(cr, aws.ToString(resp.GrantId))
	cre.ConnectionDetails = managed.ConnectionDetails{
		connGrantID:    []byte(aws.ToString(resp.GrantId)),
		connGrantToken: []byte(aws.ToString(resp.GrantToken)),
	}
	return cre, nil
}

func preDelete(_ context.Context, cr *svcapitypes.Grant, obj *svcsdk.RevokeGrantInput) (bool, error) {
	obj.KeyId = keyID(cr)
	obj.GrantId = aws.String(meta.GetExternalName(cr))
	return false, nil
}

func setupExternal(e *external) {
	e.preObserve = preObserve
	e.filterList = filterList
	e.postObserve = postObserve
	e.isUpToDate = isUpToDate
	e.preCreate = preCreate
	e.postCreate = postCreate
	e.preDelete = preDelete
}

func init() {
	configure = func(mgr ctrl.Manager, _ controller.Options) ([]option, []managed.ReconcilerOption, error) {
		return []option{setupExternal}, []managed.ReconcilerOption{
			managed.WithReferenceResolver(managed.NewAPISimpleReferenceResolver(mgr.GetClient())),
			// AWS assigns the grant ID; the default would use metadata.name.
			managed.WithInitializers(),
		}, nil
	}
}
