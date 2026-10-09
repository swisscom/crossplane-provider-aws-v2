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

package dbinstance

import (
	"context"
	"strconv"

	svcsdk "github.com/aws/aws-sdk-go-v2/service/docdb"
	svcsdktypes "github.com/aws/aws-sdk-go-v2/service/docdb/types"
	"github.com/crossplane/crossplane-runtime/v2/pkg/controller"
	"github.com/crossplane/crossplane-runtime/v2/pkg/meta"
	"github.com/crossplane/crossplane-runtime/v2/pkg/reconciler/managed"
	xpv2 "github.com/crossplane/crossplane/apis/v2/core/v2"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	svcapitypes "github.com/teeverr/provider-aws-v2/apis/docdb/v1alpha1"
	awsclient "github.com/teeverr/provider-aws-v2/internal/clients/aws"
	svcutils "github.com/teeverr/provider-aws-v2/internal/controller/docdb/utils"
	pointer "github.com/teeverr/provider-aws-v2/internal/utils/pointer"
)

type hooks struct {
	client Client
	kube   client.Client
}

func preObserve(_ context.Context, cr *svcapitypes.DBInstance, obj *svcsdk.DescribeDBInstancesInput) error {
	obj.DBInstanceIdentifier = pointer.ToOrNilIfZeroValue(meta.GetExternalName(cr))
	return nil
}

func postObserve(_ context.Context, cr *svcapitypes.DBInstance, resp *svcsdk.DescribeDBInstancesOutput, obs managed.ExternalObservation, err error) (managed.ExternalObservation, error) {
	if err != nil {
		return managed.ExternalObservation{}, err
	}

	if len(resp.DBInstances) > 0 {
		cr.Status.AtProvider.PreferredMaintenanceWindow = resp.DBInstances[0].PreferredMaintenanceWindow
	}

	switch pointer.StringValue(cr.Status.AtProvider.DBInstanceStatus) {
	case svcapitypes.DocDBInstanceStateAvailable:
		cr.Status.SetConditions(xpv2.Available())
	case svcapitypes.DocDBInstanceStateCreating:
		cr.Status.SetConditions(xpv2.Creating())
	case svcapitypes.DocDBInstanceStateDeleting:
		cr.Status.SetConditions(xpv2.Deleting())
	default:
		cr.Status.SetConditions(xpv2.Unavailable())
	}

	obs.ConnectionDetails = getConnectionDetails(cr)
	return obs, nil
}

func preUpdate(ctx context.Context, cr *svcapitypes.DBInstance, obj *svcsdk.ModifyDBInstanceInput) error {
	obj.DBInstanceIdentifier = pointer.ToOrNilIfZeroValue(meta.GetExternalName(cr))
	obj.CACertificateIdentifier = cr.Spec.ForProvider.CACertificateIdentifier
	obj.ApplyImmediately = cr.Spec.ForProvider.ApplyImmediately
	// AutoMinorVersionUpgrade is not supported by the DocumentDB ModifyDBInstance
	// API and must be omitted from update requests to avoid a terminal error.
	obj.AutoMinorVersionUpgrade = nil
	return nil
}

func preCreate(ctx context.Context, cr *svcapitypes.DBInstance, obj *svcsdk.CreateDBInstanceInput) error {
	obj.DBInstanceIdentifier = pointer.ToOrNilIfZeroValue(meta.GetExternalName(cr))
	obj.DBClusterIdentifier = cr.Spec.ForProvider.DBClusterIdentifier
	return nil
}

func postCreate(ctx context.Context, cr *svcapitypes.DBInstance, resp *svcsdk.CreateDBInstanceOutput, cre managed.ExternalCreation, err error) (managed.ExternalCreation, error) {
	if err != nil {
		return cre, err
	}

	cre.ConnectionDetails = getConnectionDetails(cr)
	return cre, nil
}

func preDelete(_ context.Context, cr *svcapitypes.DBInstance, obj *svcsdk.DeleteDBInstanceInput) (bool, error) {
	// Skip if cluster is already in deleting state
	if pointer.StringValue(cr.Status.AtProvider.DBInstanceStatus) == svcapitypes.DocDBInstanceStateDeleting {
		return true, nil
	}

	obj.DBInstanceIdentifier = pointer.ToOrNilIfZeroValue(meta.GetExternalName(cr))
	return false, nil
}

func getConnectionDetails(cr *svcapitypes.DBInstance) managed.ConnectionDetails {
	if cr.Status.AtProvider.Endpoint == nil {
		return nil
	}
	return managed.ConnectionDetails{
		awsclient.ResourceCredentialsSecretEndpointKey: []byte(pointer.StringValue(cr.Status.AtProvider.Endpoint.Address)),
		awsclient.ResourceCredentialsSecretPortKey:     []byte(strconv.Itoa(int(pointer.Int64Value(cr.Status.AtProvider.Endpoint.Port)))),
	}
}

func (e *hooks) isUpToDate(ctx context.Context, cr *svcapitypes.DBInstance, resp *svcsdk.DescribeDBInstancesOutput) (bool, string, error) {
	instance := resp.DBInstances[0]

	switch {
	case pointer.StringValue(cr.Spec.ForProvider.CACertificateIdentifier) != pointer.StringValue(instance.CACertificateIdentifier),
		pointer.StringValue(cr.Spec.ForProvider.DBInstanceClass) != pointer.StringValue(instance.DBInstanceClass),
		pointer.StringValue(cr.Spec.ForProvider.PreferredMaintenanceWindow) != pointer.StringValue(instance.PreferredMaintenanceWindow),
		pointer.Int64Value(cr.Spec.ForProvider.PromotionTier) != int64(pointer.Int32Value(instance.PromotionTier)):
		return false, "", nil
	}

	areTagsUpToDate, err := svcutils.AreTagsUpToDate(ctx, e.client, cr.Spec.ForProvider.Tags, instance.DBInstanceArn)
	return areTagsUpToDate, "", err
}

func lateInitialize(cr *svcapitypes.DBInstanceParameters, resp *svcsdk.DescribeDBInstancesOutput) error {
	instance := resp.DBInstances[0]

	cr.AvailabilityZone = pointer.LateInitialize(cr.AvailabilityZone, instance.AvailabilityZone)
	cr.CACertificateIdentifier = pointer.LateInitialize(cr.CACertificateIdentifier, instance.CACertificateIdentifier)
	cr.PreferredMaintenanceWindow = pointer.LateInitialize(cr.PreferredMaintenanceWindow, instance.PreferredMaintenanceWindow)
	cr.PromotionTier = pointer.LateInitialize(cr.PromotionTier, pointer.Int64(instance.PromotionTier))
	return nil
}

func filterList(cr *svcapitypes.DBInstance, list *svcsdk.DescribeDBInstancesOutput) *svcsdk.DescribeDBInstancesOutput {
	id := meta.GetExternalName(cr)
	for _, instance := range list.DBInstances {
		if pointer.StringValue(instance.DBInstanceIdentifier) == id {
			return &svcsdk.DescribeDBInstancesOutput{
				Marker:      list.Marker,
				DBInstances: []svcsdktypes.DBInstance{instance},
			}
		}
	}

	return &svcsdk.DescribeDBInstancesOutput{
		Marker:      list.Marker,
		DBInstances: []svcsdktypes.DBInstance{},
	}
}

func setupExternal(e *external) {
	h := &hooks{client: e.client, kube: e.kube}
	e.preObserve = preObserve
	e.postObserve = postObserve
	e.isUpToDate = h.isUpToDate
	e.preUpdate = preUpdate
	e.postUpdate = h.postUpdate
	e.preCreate = preCreate
	e.lateInitialize = lateInitialize
	e.postCreate = postCreate
	e.preDelete = preDelete
	e.filterList = filterList
}

func (e *hooks) postUpdate(ctx context.Context, cr *svcapitypes.DBInstance, _ *svcsdk.ModifyDBInstanceOutput, upd managed.ExternalUpdate, err error) (managed.ExternalUpdate, error) {
	if err != nil {
		return upd, err
	}
	return upd, svcutils.UpdateTagsForResource(ctx, e.client, cr.Spec.ForProvider.Tags, cr.Status.AtProvider.DBInstanceARN)
}

func configureDBInstance(mgr ctrl.Manager, _ controller.Options) ([]option, []managed.ReconcilerOption, error) {
	return []option{setupExternal}, []managed.ReconcilerOption{
		managed.WithReferenceResolver(managed.NewAPISimpleReferenceResolver(mgr.GetClient())),
		managed.WithInitializers(managed.NewNameAsExternalName(mgr.GetClient())),
	}, nil
}

func init() {
	configure = configureDBInstance
}
