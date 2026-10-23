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

	"github.com/aws/aws-sdk-go-v2/aws"
	svcsdk "github.com/aws/aws-sdk-go-v2/service/docdb"
	svcsdktypes "github.com/aws/aws-sdk-go-v2/service/docdb/types"
	"github.com/crossplane/crossplane-runtime/v2/pkg/controller"
	"github.com/crossplane/crossplane-runtime/v2/pkg/meta"
	"github.com/crossplane/crossplane-runtime/v2/pkg/reconciler/managed"
	cpresource "github.com/crossplane/crossplane-runtime/v2/pkg/resource"
	xpv2 "github.com/crossplane/crossplane/apis/v2/core/v2"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	svcapitypes "github.com/swisscom/crossplane-provider-aws-v2/apis/docdb/v1alpha1"
	awsclient "github.com/swisscom/crossplane-provider-aws-v2/internal/clients/aws"
	svcutils "github.com/swisscom/crossplane-provider-aws-v2/internal/controller/docdb/utils"
	"github.com/swisscom/crossplane-provider-aws-v2/internal/utils/compare"
	pointer "github.com/swisscom/crossplane-provider-aws-v2/internal/utils/pointer"
)

type hooks struct {
	client Client
	kube   client.Client
	// arn is cached by Observe: the late-init update replaces the status
	// before Update runs.
	arn *string
	// tagKeysChanged makes Observe report a late-init so that the runtime
	// persists the managed tag keys.
	tagKeysChanged bool
}

// observeTags records the ARN and the managed tag keys for Update.
func (e *hooks) observeTags(ctx context.Context, mg cpresource.Managed, spec []*svcapitypes.Tag, arn *string) (bool, error) {
	e.arn = arn
	upToDate, changed, err := svcutils.ObserveTags(ctx, e.client, mg, spec, arn)
	e.tagKeysChanged = changed
	return upToDate, err
}

// tagARN prefers the ARN of this reconcile's Observe.
func (e *hooks) tagARN(status *string) *string {
	if e.arn != nil {
		return e.arn
	}
	return status
}

func preObserve(_ context.Context, cr *svcapitypes.DBInstance, obj *svcsdk.DescribeDBInstancesInput) error {
	obj.DBInstanceIdentifier = pointer.ToOrNilIfZeroValue(meta.GetExternalName(cr))
	return nil
}

func (e *hooks) postObserve(_ context.Context, cr *svcapitypes.DBInstance, resp *svcsdk.DescribeDBInstancesOutput, obs managed.ExternalObservation, err error) (managed.ExternalObservation, error) {
	if err != nil {
		return managed.ExternalObservation{}, err
	}
	obs.ResourceLateInitialized = obs.ResourceLateInitialized || e.tagKeysChanged

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
	p := &cr.Spec.ForProvider

	observed := GenerateDBInstance(resp)
	obs := &observed.Spec.ForProvider
	// A change AWS holds for the maintenance window is not sent again.
	if pmv := observed.Status.AtProvider.PendingModifiedValues; pmv != nil {
		compare.Pending(&obs.DBInstanceClass, pmv.DBInstanceClass)
		compare.Pending(&obs.CACertificateIdentifier, pmv.CACertificateIdentifier)
	}
	diff := compare.Without(DiffUpdatableParameters(p, obs),
		// Not supported by the DocumentDB ModifyDBInstance API, see preUpdate.
		"autoMinorVersionUpgrade",
		// AWS returns the key ARN, compared below.
		"performanceInsightsKMSKeyID",
		// AWS returns it in lowercase, compared below.
		"preferredMaintenanceWindow",
	)
	if !compare.EqualIgnoreCase(p.PreferredMaintenanceWindow, obs.PreferredMaintenanceWindow) {
		diff = append(diff, "preferredMaintenanceWindow")
	}
	if p.EnablePerformanceInsights != nil && *p.EnablePerformanceInsights != pointer.BoolValue(instance.PerformanceInsightsEnabled) {
		diff = append(diff, "enablePerformanceInsights")
	}
	if p.PerformanceInsightsKMSKeyID != nil && !compare.KMSKeyIDEqual(*p.PerformanceInsightsKMSKeyID, pointer.StringValue(instance.PerformanceInsightsKMSKeyId)) {
		diff = append(diff, "performanceInsightsKMSKeyID")
	}
	// Always, so new spec keys are recorded before Update adds them.
	tagsUpToDate, err := e.observeTags(ctx, cr, p.Tags, instance.DBInstanceArn)
	if err != nil {
		return false, "", err
	}
	if !tagsUpToDate {
		diff = append(diff, "tags")
	}
	return len(diff) == 0, compare.Message(diff), nil
}

func lateInitialize(cr *svcapitypes.DBInstanceParameters, resp *svcsdk.DescribeDBInstancesOutput) error {
	instance := resp.DBInstances[0]
	// First, so a foreign instance changes neither spec nor status.
	if err := svcutils.CheckEngine("DBInstance", aws.ToString(instance.DBInstanceIdentifier), instance.Engine); err != nil {
		return err
	}

	cr.AvailabilityZone = pointer.LateInitialize(cr.AvailabilityZone, instance.AvailabilityZone)
	cr.CACertificateIdentifier = pointer.LateInitialize(cr.CACertificateIdentifier, instance.CACertificateIdentifier)
	cr.PreferredMaintenanceWindow = pointer.LateInitialize(cr.PreferredMaintenanceWindow, instance.PreferredMaintenanceWindow)
	cr.PromotionTier = pointer.LateInitialize(cr.PromotionTier, pointer.Int64(instance.PromotionTier))
	return nil
}

func filterList(cr *svcapitypes.DBInstance, list *svcsdk.DescribeDBInstancesOutput) *svcsdk.DescribeDBInstancesOutput {
	id := meta.GetExternalName(cr)
	for _, instance := range list.DBInstances {
		if !compare.IdentifierEqual(aws.ToString(instance.DBInstanceIdentifier), id) {
			continue
		}
		// A deleted MR lets go of a foreign instance without touching it.
		if meta.WasDeleted(cr) && svcutils.CheckEngine("DBInstance", id, instance.Engine) != nil {
			break
		}
		return &svcsdk.DescribeDBInstancesOutput{
			Marker:      list.Marker,
			DBInstances: []svcsdktypes.DBInstance{instance},
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
	e.postObserve = h.postObserve
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
	return upd, svcutils.UpdateTagsForResource(ctx, e.client, cr, cr.Spec.ForProvider.Tags, e.tagARN(cr.Status.AtProvider.DBInstanceARN))
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
