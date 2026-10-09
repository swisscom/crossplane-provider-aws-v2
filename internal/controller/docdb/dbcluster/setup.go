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

package dbcluster

import (
	"context"
	"strconv"

	"github.com/aws/aws-sdk-go-v2/aws"
	svcsdk "github.com/aws/aws-sdk-go-v2/service/docdb"
	svcsdktypes "github.com/aws/aws-sdk-go-v2/service/docdb/types"
	"github.com/crossplane/crossplane-runtime/v2/pkg/controller"
	"github.com/crossplane/crossplane-runtime/v2/pkg/errors"
	"github.com/crossplane/crossplane-runtime/v2/pkg/meta"
	"github.com/crossplane/crossplane-runtime/v2/pkg/password"
	"github.com/crossplane/crossplane-runtime/v2/pkg/reconciler/managed"
	cpresource "github.com/crossplane/crossplane-runtime/v2/pkg/resource"
	xpv2 "github.com/crossplane/crossplane/apis/v2/core/v2"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	svcapitypes "github.com/teeverr/provider-aws-v2/apis/docdb/v1alpha1"
	awsclient "github.com/teeverr/provider-aws-v2/internal/clients/aws"
	svcutils "github.com/teeverr/provider-aws-v2/internal/controller/docdb/utils"
	pointer "github.com/teeverr/provider-aws-v2/internal/utils/pointer"
)

const (
	errNotDBCluster             = "managed resource is not a DB Cluster custom resource"
	errKubeUpdateFailed         = "cannot update DBCluster instance custom resource"
	errGetPasswordSecretFailed  = "cannot get password secret"
	errSaveSecretFailed         = "failed to save generated password to Kubernetes secret"
	errRestore                  = "cannot restore DBCluster in AWS"
	errUnknownRestoreFromSource = "unknown restoreFrom source"
)

type hooks struct {
	client Client
	kube   client.Client
}

func preObserve(_ context.Context, cr *svcapitypes.DBCluster, obj *svcsdk.DescribeDBClustersInput) error {
	obj.DBClusterIdentifier = pointer.ToOrNilIfZeroValue(meta.GetExternalName(cr))
	return nil
}

func preDelete(_ context.Context, cr *svcapitypes.DBCluster, obj *svcsdk.DeleteDBClusterInput) (bool, error) {
	obj.DBClusterIdentifier = pointer.ToOrNilIfZeroValue(meta.GetExternalName(cr))
	obj.FinalDBSnapshotIdentifier = cr.Spec.ForProvider.FinalDBSnapshotIdentifier
	obj.SkipFinalSnapshot = cr.Spec.ForProvider.SkipFinalSnapshot
	return false, nil
}

func areSameElements(a1, a2 []*string) bool {
	if len(a1) != len(a2) {
		return false
	}

	m2 := make(map[string]struct{}, len(a2))
	for _, s2 := range a2 {
		m2[pointer.StringValue(s2)] = struct{}{}
	}

	for _, s1 := range a1 {
		v1 := pointer.StringValue(s1)
		if _, exists := m2[v1]; !exists {
			return false
		}
	}

	return true
}

func isEngineVersionUpToDate(cr *svcapitypes.DBCluster, out *svcsdk.DescribeDBClustersOutput) bool {
	// If EngineVersion is not set, AWS sets a default value,
	// so we do not try to update in this case
	if cr.Spec.ForProvider.EngineVersion != nil {
		if out.DBClusters[0].EngineVersion == nil {
			return false
		}

		// Upgrade is only necessary if the spec version is higher.
		// Downgrades are not possible in pointer.
		c := svcutils.CompareEngineVersions(*cr.Spec.ForProvider.EngineVersion, *out.DBClusters[0].EngineVersion)
		return c <= 0
	}
	return true
}

func getConnectionDetails(cr *svcapitypes.DBCluster) managed.ConnectionDetails {
	return managed.ConnectionDetails{
		awsclient.ResourceCredentialsSecretUserKey:     []byte(pointer.StringValue(cr.Spec.ForProvider.MasterUsername)),
		awsclient.ResourceCredentialsSecretEndpointKey: []byte(pointer.StringValue(cr.Status.AtProvider.Endpoint)),
		awsclient.ResourceCredentialsSecretPortKey:     []byte(strconv.Itoa(int(pointer.Int64Value(cr.Spec.ForProvider.Port)))),
		"readerEndpoint": []byte(pointer.StringValue(cr.Status.AtProvider.ReaderEndpoint)),
	}
}
func lateInitialize(cr *svcapitypes.DBClusterParameters, resp *svcsdk.DescribeDBClustersOutput) error {
	cluster := resp.DBClusters[0]

	if cr.AvailabilityZones == nil && cluster.AvailabilityZones != nil {
		cr.AvailabilityZones = aws.StringSlice(cluster.AvailabilityZones)
	}

	cr.BackupRetentionPeriod = pointer.LateInitialize(cr.BackupRetentionPeriod, pointer.Int64(cluster.BackupRetentionPeriod))
	cr.DBClusterParameterGroupName = pointer.LateInitialize(cr.DBClusterParameterGroupName, cluster.DBClusterParameterGroup)
	cr.DBSubnetGroupName = pointer.LateInitialize(cr.DBSubnetGroupName, cluster.DBSubnetGroup)
	cr.DeletionProtection = pointer.LateInitialize(cr.DeletionProtection, cluster.DeletionProtection)
	cr.EngineVersion = pointer.LateInitialize(cr.EngineVersion, cluster.EngineVersion)
	cr.KMSKeyID = pointer.LateInitialize(cr.KMSKeyID, cluster.KmsKeyId)
	cr.Port = pointer.LateInitialize(cr.Port, pointer.Int64(cluster.Port))
	cr.PreferredBackupWindow = pointer.LateInitialize(cr.PreferredBackupWindow, cluster.PreferredBackupWindow)
	cr.PreferredMaintenanceWindow = pointer.LateInitialize(cr.PreferredMaintenanceWindow, cluster.PreferredMaintenanceWindow)
	cr.StorageEncrypted = pointer.LateInitialize(cr.StorageEncrypted, cluster.StorageEncrypted)

	if cr.EnableCloudwatchLogsExports == nil && cluster.EnabledCloudwatchLogsExports != nil {
		cr.EnableCloudwatchLogsExports = aws.StringSlice(cluster.EnabledCloudwatchLogsExports)
	}
	if cr.VPCSecurityGroupIDs == nil {
		cr.VPCSecurityGroupIDs = make([]*string, len(cluster.VpcSecurityGroups))
		for i, group := range cluster.VpcSecurityGroups {
			cr.VPCSecurityGroupIDs[i] = group.VpcSecurityGroupId
		}
	}

	return nil
}

func (e *hooks) isUpToDate(ctx context.Context, cr *svcapitypes.DBCluster, resp *svcsdk.DescribeDBClustersOutput) (bool, string, error) {
	cluster := resp.DBClusters[0]

	if pointer.StringValue(cluster.Status) == svcapitypes.DocDBInstanceStateModifying || pointer.StringValue(cluster.Status) == svcapitypes.DocDBInstanceStateUpgrading {
		return true, "", nil
	}

	_, pwChanged, err := e.getPasswordFromRef(ctx, cr)
	if err != nil || pwChanged {
		return false, "", err
	}

	switch {
	case pointer.Int64Value(cr.Spec.ForProvider.BackupRetentionPeriod) != int64(pointer.Int32Value(cluster.BackupRetentionPeriod)),
		pointer.StringValue(cr.Spec.ForProvider.DBClusterParameterGroupName) != pointer.StringValue(cluster.DBClusterParameterGroup),
		pointer.BoolValue(cr.Spec.ForProvider.DeletionProtection) != pointer.BoolValue(cluster.DeletionProtection),
		!areSameElements(cr.Spec.ForProvider.EnableCloudwatchLogsExports, aws.StringSlice(cluster.EnabledCloudwatchLogsExports)),
		!isEngineVersionUpToDate(cr, resp),
		pointer.Int64Value(cr.Spec.ForProvider.Port) != int64(pointer.Int32Value(cluster.Port)),
		pointer.StringValue(cr.Spec.ForProvider.PreferredBackupWindow) != pointer.StringValue(cluster.PreferredBackupWindow),
		pointer.StringValue(cr.Spec.ForProvider.PreferredMaintenanceWindow) != pointer.StringValue(cluster.PreferredMaintenanceWindow):
		return false, "", nil
	}

	areTagsUpToDate, err := svcutils.AreTagsUpToDate(ctx, e.client, cr.Spec.ForProvider.Tags, cluster.DBClusterArn)
	return areTagsUpToDate, "", err
}

func (e *hooks) postUpdate(ctx context.Context, cr *svcapitypes.DBCluster, resp *svcsdk.ModifyDBClusterOutput, upd managed.ExternalUpdate, err error) (managed.ExternalUpdate, error) {
	if err != nil {
		return managed.ExternalUpdate{}, err
	}

	input, err := GenerateDescribeDBClustersInput(cr)
	if err != nil {
		return managed.ExternalUpdate{}, err
	}
	// GenerateDescribeDBClustersInput returns an empty DescribeDBClustersInput
	// and the function is generated by ack-generate, so we manually need to set the
	// DBClusterIdentifier
	input.DBClusterIdentifier = pointer.ToOrNilIfZeroValue(meta.GetExternalName(cr))
	out, err := e.client.DescribeDBClusters(ctx, input)
	if err != nil {
		return managed.ExternalUpdate{}, awsclient.Wrap(cpresource.Ignore(IsNotFound, err), errDescribe)
	}

	needsEngineVersionUpdate := !isEngineVersionUpToDate(cr, out)
	needsDBClusterParamGroupUpdate := pointer.StringValue(cr.Spec.ForProvider.DBClusterParameterGroupName) != pointer.StringValue(out.DBClusters[0].DBClusterParameterGroup)
	needsPostUpdate := needsEngineVersionUpdate || needsDBClusterParamGroupUpdate

	if needsPostUpdate {
		modifyInput := &svcsdk.ModifyDBClusterInput{
			DBClusterIdentifier:         pointer.ToOrNilIfZeroValue(meta.GetExternalName(cr)),
			ApplyImmediately:            cr.Spec.ForProvider.ApplyImmediately,
			DBClusterParameterGroupName: cr.Spec.ForProvider.DBClusterParameterGroupName,
		}
		if needsEngineVersionUpdate {
			modifyInput.EngineVersion = cr.Spec.ForProvider.EngineVersion
			modifyInput.AllowMajorVersionUpgrade = cr.Spec.ForProvider.AllowMajorVersionUpgrade
		}

		if _, err = e.client.ModifyDBCluster(ctx, modifyInput); err != nil {
			return managed.ExternalUpdate{}, err
		}
	}

	return upd, svcutils.UpdateTagsForResource(ctx, e.client, cr.Spec.ForProvider.Tags, resp.DBCluster.DBClusterArn)
}

func generateRestoreDBClusterFromSnapshotInput(cr *svcapitypes.DBCluster) *svcsdk.RestoreDBClusterFromSnapshotInput {
	p := cr.Spec.ForProvider
	res := &svcsdk.RestoreDBClusterFromSnapshotInput{
		AvailabilityZones:           aws.ToStringSlice(p.AvailabilityZones),
		DBSubnetGroupName:           p.DBSubnetGroupName,
		DeletionProtection:          p.DeletionProtection,
		EnableCloudwatchLogsExports: aws.ToStringSlice(p.EnableCloudwatchLogsExports),
		Engine:                      p.Engine,
		EngineVersion:               p.EngineVersion,
		KmsKeyId:                    p.KMSKeyID,
		Port:                        pointer.Int32(p.Port),
		Tags:                        sdkTags(p.Tags),
	}
	if p.RestoreFrom != nil && p.RestoreFrom.Snapshot != nil {
		res.SnapshotIdentifier = aws.String(p.RestoreFrom.Snapshot.SnapshotIdentifier)
	}
	return res
}

func generateRestoreDBClusterToPointInTimeInput(cr *svcapitypes.DBCluster) *svcsdk.RestoreDBClusterToPointInTimeInput {
	p := cr.Spec.ForProvider
	res := &svcsdk.RestoreDBClusterToPointInTimeInput{
		DBSubnetGroupName:           p.DBSubnetGroupName,
		DeletionProtection:          p.DeletionProtection,
		EnableCloudwatchLogsExports: aws.ToStringSlice(p.EnableCloudwatchLogsExports),
		KmsKeyId:                    p.KMSKeyID,
		Port:                        pointer.Int32(p.Port),
		VpcSecurityGroupIds:         aws.ToStringSlice(p.VPCSecurityGroupIDs),
		Tags:                        sdkTags(p.Tags),
	}
	if p.RestoreFrom != nil && p.RestoreFrom.PointInTime != nil {
		pit := p.RestoreFrom.PointInTime
		if pit.RestoreTime != nil {
			res.RestoreToTime = &pit.RestoreTime.Time
		}
		res.UseLatestRestorableTime = pit.UseLatestRestorableTime
		res.RestoreType = pit.RestoreType
		res.SourceDBClusterIdentifier = aws.String(pit.SourceDBClusterIdentifier)
	}
	return res
}

func sdkTags(tags []*svcapitypes.Tag) []svcsdktypes.Tag {
	if tags == nil {
		return nil
	}
	res := make([]svcsdktypes.Tag, 0, len(tags))
	for _, t := range tags {
		res = append(res, svcsdktypes.Tag{Key: t.Key, Value: t.Value})
	}
	return res
}

func filterList(cr *svcapitypes.DBCluster, list *svcsdk.DescribeDBClustersOutput) *svcsdk.DescribeDBClustersOutput {
	id := meta.GetExternalName(cr)
	for _, instance := range list.DBClusters {
		if pointer.StringValue(instance.DBClusterIdentifier) == id {
			return &svcsdk.DescribeDBClustersOutput{
				Marker:     list.Marker,
				DBClusters: []svcsdktypes.DBCluster{instance},
			}
		}
	}

	return &svcsdk.DescribeDBClustersOutput{
		Marker:     list.Marker,
		DBClusters: []svcsdktypes.DBCluster{},
	}
}

func generateCloudWatchExportConfiguration(spec, current []*string) *svcsdktypes.CloudwatchLogsExportConfiguration {
	toEnable := []string{}
	toDisable := []string{}

	currentMap := make(map[string]struct{}, len(current))
	for _, currentID := range current {
		currentMap[pointer.StringValue(currentID)] = struct{}{}
	}

	specMap := make(map[string]struct{}, len(spec))
	for _, specID := range spec {
		key := pointer.StringValue(specID)
		specMap[key] = struct{}{}

		if _, exists := currentMap[key]; !exists {
			toEnable = append(toEnable, key)
		}
	}

	for _, currentID := range current {
		if _, exists := specMap[pointer.StringValue(currentID)]; !exists {
			toDisable = append(toDisable, pointer.StringValue(currentID))
		}
	}

	return &svcsdktypes.CloudwatchLogsExportConfiguration{
		EnableLogTypes:  toEnable,
		DisableLogTypes: toDisable,
	}
}

// getPasswordFromRef returns the password of the masterUserPasswordSecretRef
// of cr and whether it differs from the one in the connection secret. Both
// secrets are in the namespace of cr.
func (e *hooks) getPasswordFromRef(ctx context.Context, cr *svcapitypes.DBCluster) (newPwd string, changed bool, err error) {
	in := cr.Spec.ForProvider.MasterUserPasswordSecretRef
	if in == nil {
		return "", false, nil
	}
	out := cr.Spec.WriteConnectionSecretToReference
	nn := types.NamespacedName{
		Name:      in.Name,
		Namespace: cr.GetNamespace(),
	}
	s := &corev1.Secret{}
	if err := e.kube.Get(ctx, nn, s); err != nil {
		return "", false, errors.Wrap(err, errGetPasswordSecretFailed)
	}
	newPwd = string(s.Data[in.Key])

	if out != nil {
		nn = types.NamespacedName{
			Name:      out.Name,
			Namespace: cr.GetNamespace(),
		}
		s = &corev1.Secret{}
		// the output secret may not exist yet, so we can skip returning an
		// error if the error is NotFound
		if err := e.kube.Get(ctx, nn, s); cpresource.IgnoreNotFound(err) != nil {
			return "", false, err
		}
		// if newPwd was set to some value, compare value in output secret with
		// newPwd
		changed = newPwd != "" && newPwd != string(s.Data[awsclient.ResourceCredentialsSecretPasswordKey])
	}
	return newPwd, changed, nil
}

func (e *hooks) savePasswordSecret(ctx context.Context, cr *svcapitypes.DBCluster, pw string) error {
	if cr.Spec.ForProvider.MasterUserPasswordSecretRef == nil {
		return errors.New("no MasterUserPasswordSecretRef given, unable to store password")
	}
	patcher := cpresource.NewAPIPatchingApplicator(e.kube)
	ref := cr.Spec.ForProvider.MasterUserPasswordSecretRef
	sc := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      ref.Name,
			Namespace: cr.GetNamespace(),
		},
		Data: map[string][]byte{
			ref.Key: []byte(pw),
		},
	}
	return patcher.Apply(ctx, sc)
}

func setupExternal(e *external) {
	h := &hooks{client: e.client, kube: e.kube}
	e.preObserve = preObserve
	e.postObserve = h.postObserve
	e.isUpToDate = h.isUpToDate
	e.preUpdate = h.preUpdate
	e.postUpdate = h.postUpdate
	e.preCreate = h.preCreate
	e.postCreate = h.postCreate
	e.preDelete = preDelete
	e.filterList = filterList
	e.lateInitialize = lateInitialize
}

func (e *hooks) postObserve(ctx context.Context, cr *svcapitypes.DBCluster, resp *svcsdk.DescribeDBClustersOutput, obs managed.ExternalObservation, err error) (managed.ExternalObservation, error) {
	if err != nil {
		return managed.ExternalObservation{}, err
	}

	cluster := resp.DBClusters[0]
	cr.Status.AtProvider.EngineVersion = cluster.EngineVersion

	obs.ConnectionDetails = getConnectionDetails(cr)

	if !meta.WasDeleted(cr) {
		pw, _, _ := e.getPasswordFromRef(ctx, cr)
		if pw != "" {
			obs.ConnectionDetails[awsclient.ResourceCredentialsSecretPasswordKey] = []byte(pw)
		}
	}

	switch pointer.StringValue(cr.Status.AtProvider.Status) {
	case svcapitypes.DocDBInstanceStateAvailable:
		cr.Status.SetConditions(xpv2.Available())
	case svcapitypes.DocDBInstanceStateCreating:
		cr.Status.SetConditions(xpv2.Creating())
	case svcapitypes.DocDBInstanceStateDeleting:
		cr.Status.SetConditions(xpv2.Deleting())
	default:
		cr.Status.SetConditions(xpv2.Unavailable())
	}
	return obs, nil
}

func (e *hooks) preUpdate(ctx context.Context, cr *svcapitypes.DBCluster, obj *svcsdk.ModifyDBClusterInput) error {
	obj.DBClusterIdentifier = pointer.ToOrNilIfZeroValue(meta.GetExternalName(cr))
	obj.CloudwatchLogsExportConfiguration = generateCloudWatchExportConfiguration(
		cr.Spec.ForProvider.EnableCloudwatchLogsExports,
		cr.Status.AtProvider.EnabledCloudwatchLogsExports)
	obj.ApplyImmediately = cr.Spec.ForProvider.ApplyImmediately

	pw, pwchanged, err := e.getPasswordFromRef(ctx, cr)
	if err != nil {
		return err
	}
	if pwchanged {
		obj.MasterUserPassword = aws.String(pw)
	}

	// ModifyDBCluster() returns error, when trying to upgrade major (minor is fine) EngineVersion:
	// "Cannot change VPC security group while doing a major version upgrade."
	// therefore EngineVersion update is entirely done separately in postUpdate
	obj.EngineVersion = nil
	// In case of a custom DBClusterParameterGroup, AWS requires for a major version update that
	// EngineVersion and DBClusterParameterGroupName are in the same ModifyDBCluster()-call
	obj.DBClusterParameterGroupName = nil

	return nil
}

func (e *hooks) preCreate(ctx context.Context, cr *svcapitypes.DBCluster, obj *svcsdk.CreateDBClusterInput) error { //nolint:gocyclo
	obj.DBClusterIdentifier = pointer.ToOrNilIfZeroValue(meta.GetExternalName(cr))

	pw, _, err := e.getPasswordFromRef(ctx, cr)
	if cpresource.IgnoreNotFound(err) != nil {
		return err
	}
	if pw == "" && aws.ToBool(&cr.Spec.ForProvider.AutogeneratePassword) {
		pw, err = password.Generate()
		if err != nil {
			return errors.Wrap(err, "unable to generate a password")
		}
		if err := e.savePasswordSecret(ctx, cr, pw); err != nil {
			return errors.Wrap(err, errSaveSecretFailed)
		}
	}

	obj.MasterUserPassword = pointer.ToOrNilIfZeroValue(pw)
	if cr.Spec.ForProvider.RestoreFrom != nil {
		switch cr.Spec.ForProvider.RestoreFrom.Source {
		case svcapitypes.RestoreSourceSnapshot:
			input := generateRestoreDBClusterFromSnapshotInput(cr)
			input.DBClusterIdentifier = obj.DBClusterIdentifier
			input.VpcSecurityGroupIds = obj.VpcSecurityGroupIds

			if _, err = e.client.RestoreDBClusterFromSnapshot(ctx, input); err != nil {
				return errors.Wrap(err, errRestore)
			}
		case svcapitypes.RestoreSourcePointInTime:
			input := generateRestoreDBClusterToPointInTimeInput(cr)
			input.DBClusterIdentifier = obj.DBClusterIdentifier
			input.VpcSecurityGroupIds = obj.VpcSecurityGroupIds

			if _, err = e.client.RestoreDBClusterToPointInTime(ctx, input); err != nil {
				return errors.Wrap(err, errRestore)
			}
		default:
			return errors.New(errUnknownRestoreFromSource)
		}
	}
	return nil
}

func (e *hooks) postCreate(ctx context.Context, cr *svcapitypes.DBCluster, resp *svcsdk.CreateDBClusterOutput, cre managed.ExternalCreation, err error) (managed.ExternalCreation, error) {
	if err != nil {
		return managed.ExternalCreation{}, err
	}

	cre.ConnectionDetails = getConnectionDetails(cr)
	pw, _, err := e.getPasswordFromRef(ctx, cr)
	if err != nil {
		return managed.ExternalCreation{}, err
	}
	if pw != "" {
		cre.ConnectionDetails[awsclient.ResourceCredentialsSecretPasswordKey] = []byte(pw)
	}

	cre.ConnectionDetails[awsclient.ResourceCredentialsSecretUserKey] = []byte(pointer.StringValue(cr.Spec.ForProvider.MasterUsername))
	// Tags are added during update
	return cre, nil
}

func configureDBCluster(mgr ctrl.Manager, _ controller.Options) ([]option, []managed.ReconcilerOption, error) {
	return []option{setupExternal}, []managed.ReconcilerOption{
		managed.WithReferenceResolver(managed.NewAPISimpleReferenceResolver(mgr.GetClient())),
		managed.WithInitializers(managed.NewNameAsExternalName(mgr.GetClient())),
	}, nil
}

func init() {
	configure = configureDBCluster
}
