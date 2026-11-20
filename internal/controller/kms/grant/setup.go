/*
Copyright 2023 The Crossplane Authors.

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

	svcsdk "github.com/aws/aws-sdk-go-v2/service/kms"
	"github.com/crossplane/crossplane-runtime/v2/pkg/controller"
	"github.com/crossplane/crossplane-runtime/v2/pkg/meta"
	"github.com/crossplane/crossplane-runtime/v2/pkg/reconciler/managed"
	cpresource "github.com/crossplane/crossplane-runtime/v2/pkg/resource"
	xpv2 "github.com/crossplane/crossplane/apis/v2/core/v2"
	ctrl "sigs.k8s.io/controller-runtime"

	svcapitypes "github.com/swisscom/crossplane-provider-aws-v2/apis/kms/v1alpha1"
	awsclient "github.com/swisscom/crossplane-provider-aws-v2/internal/clients/aws"
	"github.com/swisscom/crossplane-provider-aws-v2/internal/utils/pointer"
)

const errRevokeGrant = "cannot revoke grant"

func init() {
	configure = configureGrant
}

func configureGrant(mgr ctrl.Manager, _ controller.Options) ([]option, []managed.ReconcilerOption, error) {
	return []option{setupExternal}, []managed.ReconcilerOption{
		managed.WithReferenceResolver(managed.NewAPISimpleReferenceResolver(mgr.GetClient())),
		// AWS assigns the grant ID, see postCreate.
		managed.WithInitializers(),
	}, nil
}

func setupExternal(e *external) {
	h := &hooks{client: e.client}
	e.preObserve = preObserve
	e.postObserve = postObserve
	e.preCreate = preCreate
	e.postCreate = postCreate
	e.delete = h.delete
}

type hooks struct {
	client Client
}

func preObserve(_ context.Context, cr *svcapitypes.Grant, obj *svcsdk.ListGrantsInput) error {
	obj.GrantId = pointer.ToOrNilIfZeroValue(meta.GetExternalName(cr))
	obj.KeyId = cr.Spec.ForProvider.KeyID
	return nil
}

func postObserve(_ context.Context, cr *svcapitypes.Grant, _ *svcsdk.ListGrantsOutput, obs managed.ExternalObservation, err error) (managed.ExternalObservation, error) {
	if err != nil {
		return obs, err
	}
	cr.SetConditions(xpv2.Available())
	return obs, nil
}

func preCreate(_ context.Context, cr *svcapitypes.Grant, obj *svcsdk.CreateGrantInput) error {
	obj.KeyId = cr.Spec.ForProvider.KeyID
	return nil
}

func postCreate(_ context.Context, cr *svcapitypes.Grant, obj *svcsdk.CreateGrantOutput, creation managed.ExternalCreation, err error) (managed.ExternalCreation, error) {
	if err != nil {
		return creation, err
	}
	meta.SetExternalName(cr, pointer.StringValue(obj.GrantId))
	return creation, nil
}

// delete revokes the grant; grants can't be updated.
func (h *hooks) delete(ctx context.Context, cr *svcapitypes.Grant) (managed.ExternalDelete, error) {
	_, err := h.client.RevokeGrant(ctx, &svcsdk.RevokeGrantInput{
		GrantId: pointer.ToOrNilIfZeroValue(meta.GetExternalName(cr)),
		KeyId:   cr.Spec.ForProvider.KeyID,
	})
	return managed.ExternalDelete{}, awsclient.Wrap(cpresource.Ignore(IsNotFound, err), errRevokeGrant)
}
