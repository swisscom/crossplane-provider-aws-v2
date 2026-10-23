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
	"github.com/crossplane/crossplane-runtime/v2/pkg/reconciler/managed"
	cpresource "github.com/crossplane/crossplane-runtime/v2/pkg/resource"
	xpv2 "github.com/crossplane/crossplane/apis/v2/core/v2"
	"k8s.io/apimachinery/pkg/util/sets"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	svcapitypes "github.com/swisscom/crossplane-provider-aws-v2/apis/docdb/v1alpha1"
	awsclient "github.com/swisscom/crossplane-provider-aws-v2/internal/clients/aws"
	svcutils "github.com/swisscom/crossplane-provider-aws-v2/internal/controller/docdb/utils"
	"github.com/swisscom/crossplane-provider-aws-v2/internal/utils/compare"
	pointer "github.com/swisscom/crossplane-provider-aws-v2/internal/utils/pointer"
)

const (
	errNotDBCluster             = "managed resource is not a DB Cluster custom resource"
	errKubeUpdateFailed         = "cannot update DBCluster instance custom resource"
	errRestore                  = "cannot restore DBCluster in AWS"
	errUnknownRestoreFromSource = "unknown restoreFrom source"
)

// transitionalStates is a deny-list on purpose: failed or unknown states
// still show their diff instead of hiding it.
var transitionalStates = sets.New(
	svcapitypes.DocDBInstanceStateCreating,
	svcapitypes.DocDBInstanceStateModifying,
	svcapitypes.DocDBInstanceStateUpgrading,
	svcapitypes.DocDBClusterStateBackingUp,
	svcapitypes.DocDBClusterStateResettingMasterCredentials,
	svcapitypes.DocDBClusterStateRebooting,
	svcapitypes.DocDBClusterStateRenaming,
	svcapitypes.DocDBClusterStateStarting,
	svcapitypes.DocDBClusterStateStopping,
	svcapitypes.DocDBClusterStateMigrating,
	svcapitypes.DocDBClusterStateMaintenance,
)

type hooks struct {
	client Client
	kube   client.Client
	// sentPassword is the password preCreate or preUpdate sent to AWS.
	sentPassword string
	// arn is cached by Observe: the late-init update replaces the status
	// before Update runs.
	arn *string
	// tagKeysChanged makes Observe report a late-init so that the runtime
	// persists the managed tag keys.
	tagKeysChanged bool
}

func preObserve(_ context.Context, cr *svcapitypes.DBCluster, obj *svcsdk.DescribeDBClustersInput) error {
	obj.DBClusterIdentifier = pointer.ToOrNilIfZeroValue(meta.GetExternalName(cr))
	obj.Filters = docDBOnly()
	return nil
}

// docDBOnly is the AWS-documented filter that hides RDS and Neptune clusters
// from DescribeDBClusters.
func docDBOnly() []svcsdktypes.Filter {
	return []svcsdktypes.Filter{{Name: aws.String("engine"), Values: []string{svcapitypes.EngineDocDB}}}
}

func preDelete(_ context.Context, cr *svcapitypes.DBCluster, obj *svcsdk.DeleteDBClusterInput) (bool, error) {
	// AWS rejects a second delete while the first one runs.
	if pointer.StringValue(cr.Status.AtProvider.Status) == svcapitypes.DocDBInstanceStateDeleting {
		return true, nil
	}
	obj.DBClusterIdentifier = pointer.ToOrNilIfZeroValue(meta.GetExternalName(cr))
	obj.FinalDBSnapshotIdentifier = cr.Spec.ForProvider.FinalDBSnapshotIdentifier
	obj.SkipFinalSnapshot = cr.Spec.ForProvider.SkipFinalSnapshot
	return false, nil
}

func isEngineVersionUpToDate(p *svcapitypes.DBClusterParameters, cluster svcsdktypes.DBCluster) bool {
	// If EngineVersion is not set, AWS sets a default value,
	// so we do not try to update in this case
	if p.EngineVersion == nil {
		return true
	}
	if cluster.EngineVersion == nil {
		return false
	}
	// Upgrade is only necessary if the spec version is higher.
	// Downgrades are not possible in AWS.
	return svcutils.CompareEngineVersions(*p.EngineVersion, *cluster.EngineVersion) <= 0
}

// getConnectionDetails prefers the observed cluster's username and port: a
// restored cluster takes them from the backup, so the spec may have neither.
func getConnectionDetails(cr *svcapitypes.DBCluster, cluster *svcsdktypes.DBCluster) managed.ConnectionDetails {
	user := cr.Spec.ForProvider.MasterUsername
	port := pointer.Int64Value(cr.Spec.ForProvider.Port)
	if cluster != nil {
		user = pointer.LateInitialize(cluster.MasterUsername, user)
		if p := aws.ToInt32(cluster.Port); p != 0 {
			port = int64(p)
		}
	}
	cd := managed.ConnectionDetails{
		awsclient.ResourceCredentialsSecretUserKey:     []byte(pointer.StringValue(user)),
		awsclient.ResourceCredentialsSecretEndpointKey: []byte(pointer.StringValue(cr.Status.AtProvider.Endpoint)),
		"readerEndpoint": []byte(pointer.StringValue(cr.Status.AtProvider.ReaderEndpoint)),
	}
	if port != 0 {
		cd[awsclient.ResourceCredentialsSecretPortKey] = []byte(strconv.FormatInt(port, 10))
	}
	return cd
}
func lateInitialize(cr *svcapitypes.DBClusterParameters, resp *svcsdk.DescribeDBClustersOutput) error {
	cluster := resp.DBClusters[0]
	// First, so a foreign cluster changes neither spec nor status.
	if err := svcutils.CheckEngine("DBCluster", aws.ToString(cluster.DBClusterIdentifier), cluster.Engine); err != nil {
		return err
	}

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

	if cr.ServerlessV2ScalingConfiguration == nil && cluster.ServerlessV2ScalingConfiguration != nil {
		cr.ServerlessV2ScalingConfiguration = &svcapitypes.ServerlessV2ScalingConfiguration{
			MaxCapacity: cluster.ServerlessV2ScalingConfiguration.MaxCapacity,
			MinCapacity: cluster.ServerlessV2ScalingConfiguration.MinCapacity,
		}
	}
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

	// AWS rejects a Modify in these states; wait instead of failing.
	if transitionalStates.Has(pointer.StringValue(cluster.Status)) {
		return true, "", nil
	}

	_, pwChanged, err := e.desiredPassword(ctx, cr)
	if err != nil {
		return false, "", err
	}

	diff := diffParameters(&cr.Spec.ForProvider, resp)
	// While AWS manages the password, the referenced one is not used.
	if pwChanged && !aws.ToBool(cr.Spec.ForProvider.ManageMasterUserPassword) {
		diff = append(diff, "masterUserPasswordSecretRef")
	}

	// Always, so new spec keys are recorded before Update adds them.
	e.arn = cluster.DBClusterArn
	tagsUpToDate, changed, err := svcutils.ObserveTags(ctx, e.client, cr, cr.Spec.ForProvider.Tags, e.arn)
	if err != nil {
		return false, "", err
	}
	e.tagKeysChanged = changed
	if !tagsUpToDate {
		diff = append(diff, "tags")
	}
	return len(diff) == 0, compare.Message(diff), nil
}

// diffParameters returns the spec fields that differ from the observed
// cluster: the generated comparison plus the fields whose name or shape
// differs between ModifyDBCluster and DescribeDBClusters.
// MasterUserSecretKMSKeyID is not compared: AWS can't change the key of an
// existing managed secret.
func diffParameters(p *svcapitypes.DBClusterParameters, resp *svcsdk.DescribeDBClustersOutput) []string {
	cluster := resp.DBClusters[0]
	// EngineVersion is compared by isEngineVersionUpToDate (no downgrades).
	diff := compare.Without(DiffUpdatableParameters(p, &GenerateDBCluster(resp).Spec.ForProvider), "engineVersion", "preferredMaintenanceWindow")
	if !isEngineVersionUpToDate(p, cluster) {
		diff = append(diff, "engineVersion")
	}
	// AWS returns the window and group names in lowercase.
	if !compare.EqualIgnoreCase(p.PreferredMaintenanceWindow, cluster.PreferredMaintenanceWindow) {
		diff = append(diff, "preferredMaintenanceWindow")
	}
	if !isParameterGroupUpToDate(p, cluster) {
		diff = append(diff, "dbClusterParameterGroupName")
	}
	if !compare.IsSubset(p.EnableCloudwatchLogsExports, aws.StringSlice(cluster.EnabledCloudwatchLogsExports)) {
		diff = append(diff, "enableCloudwatchLogsExports")
	}
	if !compare.IsSubset(p.VPCSecurityGroupIDs, vpcSecurityGroupIDs(cluster.VpcSecurityGroups)) {
		diff = append(diff, "vpcSecurityGroupIDs")
	}
	if p.ManageMasterUserPassword != nil && *p.ManageMasterUserPassword != (cluster.MasterUserSecret != nil) {
		diff = append(diff, "manageMasterUserPassword")
	}
	return diff
}

func isParameterGroupUpToDate(p *svcapitypes.DBClusterParameters, cluster svcsdktypes.DBCluster) bool {
	return p.DBClusterParameterGroupName == nil || compare.IdentifierEqual(*p.DBClusterParameterGroupName, aws.ToString(cluster.DBClusterParameterGroup))
}

func vpcSecurityGroupIDs(groups []svcsdktypes.VpcSecurityGroupMembership) []*string {
	ids := make([]*string, 0, len(groups))
	for _, g := range groups {
		ids = append(ids, g.VpcSecurityGroupId)
	}
	return ids
}

func sdkServerlessV2ScalingConfiguration(c *svcapitypes.ServerlessV2ScalingConfiguration) *svcsdktypes.ServerlessV2ScalingConfiguration {
	if c == nil {
		return nil
	}
	return &svcsdktypes.ServerlessV2ScalingConfiguration{MaxCapacity: c.MaxCapacity, MinCapacity: c.MinCapacity}
}

func (e *hooks) postUpdate(ctx context.Context, cr *svcapitypes.DBCluster, resp *svcsdk.ModifyDBClusterOutput, upd managed.ExternalUpdate, err error) (managed.ExternalUpdate, error) {
	if err != nil {
		return managed.ExternalUpdate{}, err
	}
	// Before the second Modify, so its failure does not resend the password.
	if e.sentPassword != "" {
		if err := e.recordAppliedPassword(ctx, cr, e.sentPassword); err != nil {
			return managed.ExternalUpdate{}, err
		}
	}

	if err := e.modifyEngineVersionAndParameterGroup(ctx, cr); err != nil {
		return managed.ExternalUpdate{}, err
	}

	// postObserve does not publish a changed password; publish the one AWS
	// just accepted.
	if e.sentPassword != "" {
		upd.ConnectionDetails = managed.ConnectionDetails{awsclient.ResourceCredentialsSecretPasswordKey: []byte(e.sentPassword)}
	}

	arn := e.arn
	if arn == nil && resp != nil && resp.DBCluster != nil {
		arn = resp.DBCluster.DBClusterArn
	}
	return upd, svcutils.UpdateTagsForResource(ctx, e.client, cr, cr.Spec.ForProvider.Tags, arn)
}

// modifyEngineVersionAndParameterGroup updates the engine version and the
// cluster parameter group with a second ModifyDBCluster call when they still
// differ after the generated update.
func (e *hooks) modifyEngineVersionAndParameterGroup(ctx context.Context, cr *svcapitypes.DBCluster) error {
	input, err := GenerateDescribeDBClustersInput(cr)
	if err != nil {
		return err
	}
	// GenerateDescribeDBClustersInput returns an empty DescribeDBClustersInput
	// and the function is generated by ack-generate, so we manually need to set the
	// DBClusterIdentifier
	input.DBClusterIdentifier = pointer.ToOrNilIfZeroValue(meta.GetExternalName(cr))
	input.Filters = docDBOnly()
	out, err := e.client.DescribeDBClusters(ctx, input)
	if err != nil {
		return awsclient.Wrap(cpresource.Ignore(IsNotFound, err), errDescribe)
	}

	if len(out.DBClusters) == 0 {
		return errors.New(errDescribe)
	}
	p := &cr.Spec.ForProvider
	needsEngineVersionUpdate := !isEngineVersionUpToDate(p, out.DBClusters[0])
	needsDBClusterParamGroupUpdate := !isParameterGroupUpToDate(p, out.DBClusters[0])

	if needsEngineVersionUpdate || needsDBClusterParamGroupUpdate {
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
			return awsclient.Wrap(err, errUpdate)
		}
	}
	return nil
}

func generateRestoreDBClusterFromSnapshotInput(cr *svcapitypes.DBCluster) *svcsdk.RestoreDBClusterFromSnapshotInput {
	p := cr.Spec.ForProvider
	res := &svcsdk.RestoreDBClusterFromSnapshotInput{
		AvailabilityZones:                aws.ToStringSlice(p.AvailabilityZones),
		CopyTagsToSnapshot:               p.CopyTagsToSnapshot,
		DBClusterParameterGroupName:      p.DBClusterParameterGroupName,
		DBSubnetGroupName:                p.DBSubnetGroupName,
		DeletionProtection:               p.DeletionProtection,
		EnableCloudwatchLogsExports:      aws.ToStringSlice(p.EnableCloudwatchLogsExports),
		Engine:                           p.Engine,
		EngineVersion:                    p.EngineVersion,
		KmsKeyId:                         p.KMSKeyID,
		NetworkType:                      p.NetworkType,
		Port:                             pointer.Int32(p.Port),
		ServerlessV2ScalingConfiguration: sdkServerlessV2ScalingConfiguration(p.ServerlessV2ScalingConfiguration),
		StorageType:                      p.StorageType,
		Tags:                             sdkTags(p.Tags),
		VpcSecurityGroupIds:              aws.ToStringSlice(p.VPCSecurityGroupIDs),
	}
	if p.RestoreFrom != nil && p.RestoreFrom.Snapshot != nil {
		res.SnapshotIdentifier = aws.String(p.RestoreFrom.Snapshot.SnapshotIdentifier)
	}
	return res
}

func generateRestoreDBClusterToPointInTimeInput(cr *svcapitypes.DBCluster) *svcsdk.RestoreDBClusterToPointInTimeInput {
	p := cr.Spec.ForProvider
	res := &svcsdk.RestoreDBClusterToPointInTimeInput{
		CopyTagsToSnapshot:               p.CopyTagsToSnapshot,
		DBSubnetGroupName:                p.DBSubnetGroupName,
		DeletionProtection:               p.DeletionProtection,
		EnableCloudwatchLogsExports:      aws.ToStringSlice(p.EnableCloudwatchLogsExports),
		KmsKeyId:                         p.KMSKeyID,
		NetworkType:                      p.NetworkType,
		Port:                             pointer.Int32(p.Port),
		ServerlessV2ScalingConfiguration: sdkServerlessV2ScalingConfiguration(p.ServerlessV2ScalingConfiguration),
		StorageType:                      p.StorageType,
		Tags:                             sdkTags(p.Tags),
		VpcSecurityGroupIds:              aws.ToStringSlice(p.VPCSecurityGroupIDs),
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
	for _, cluster := range list.DBClusters {
		if !compare.IdentifierEqual(aws.ToString(cluster.DBClusterIdentifier), id) {
			continue
		}
		// A deleted MR lets go of a foreign cluster without touching it.
		if meta.WasDeleted(cr) && svcutils.CheckEngine("DBCluster", id, cluster.Engine) != nil {
			break
		}
		return &svcsdk.DescribeDBClustersOutput{
			Marker:     list.Marker,
			DBClusters: []svcsdktypes.DBCluster{cluster},
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

	obs.ResourceLateInitialized = obs.ResourceLateInitialized || e.tagKeysChanged
	obs.ConnectionDetails = getConnectionDetails(cr, &cluster)

	// While AWS manages the password, the referenced one is not the cluster's.
	if !meta.WasDeleted(cr) && !aws.ToBool(cr.Spec.ForProvider.ManageMasterUserPassword) {
		// A changed password is published by postUpdate once AWS has it:
		// the reconciler publishes these details before Update, and
		// preUpdate would then no longer see the change.
		// isUpToDate reports this error outside transitional states; here we
		// only skip publishing.
		pw, changed, _ := e.desiredPassword(ctx, cr)
		if pw != "" && !changed {
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

	// Send ManageMasterUserPassword only to switch it; the KMS key is only
	// accepted when switching it on.
	if m := cr.Spec.ForProvider.ManageMasterUserPassword; m == nil || *m == (cr.Status.AtProvider.MasterUserSecret != nil) {
		obj.ManageMasterUserPassword = nil
	}
	if !aws.ToBool(obj.ManageMasterUserPassword) {
		obj.MasterUserSecretKmsKeyId = nil
	}

	pw, pwchanged, err := e.desiredPassword(ctx, cr)
	if err != nil {
		return err
	}
	switch {
	case aws.ToBool(obj.ManageMasterUserPassword):
		// AWS generates the password; it rejects MasterUserPassword.
	case obj.ManageMasterUserPassword != nil:
		// Switching to a self-managed password requires one.
		obj.MasterUserPassword = pointer.ToOrNilIfZeroValue(pw)
	case pwchanged && !aws.ToBool(cr.Spec.ForProvider.ManageMasterUserPassword):
		// AWS rejects a password while it manages it.
		obj.MasterUserPassword = aws.String(pw)
	}
	// Without it AWS waits for the maintenance window; the flag only affects
	// the password and the identifier.
	if obj.MasterUserPassword != nil {
		obj.ApplyImmediately = aws.Bool(true)
	}
	e.sentPassword = aws.ToString(obj.MasterUserPassword)

	// ModifyDBCluster() returns error, when trying to upgrade major (minor is fine) EngineVersion:
	// "Cannot change VPC security group while doing a major version upgrade."
	// therefore EngineVersion update is entirely done separately in postUpdate
	obj.EngineVersion = nil
	// In case of a custom DBClusterParameterGroup, AWS requires for a major version update that
	// EngineVersion and DBClusterParameterGroupName are in the same ModifyDBCluster()-call
	obj.DBClusterParameterGroupName = nil

	return nil
}

func (e *hooks) preCreate(ctx context.Context, cr *svcapitypes.DBCluster, obj *svcsdk.CreateDBClusterInput) error {
	obj.DBClusterIdentifier = pointer.ToOrNilIfZeroValue(meta.GetExternalName(cr))
	pw, err := e.masterPassword(ctx, cr)
	if err != nil {
		return err
	}
	obj.MasterUserPassword = pointer.ToOrNilIfZeroValue(pw)
	e.sentPassword = pw
	return nil
}

// customExternal creates restored clusters with RestoreDBClusterFromSnapshot
// or RestoreDBClusterToPointInTime instead of CreateDBCluster.
type customExternal struct {
	*external
	hooks *hooks
}

func wrap(e *external) managed.TypedExternalClient[*svcapitypes.DBCluster] {
	return &customExternal{external: e, hooks: &hooks{client: e.client, kube: e.kube}}
}

func (c *customExternal) Create(ctx context.Context, cr *svcapitypes.DBCluster) (managed.ExternalCreation, error) {
	restoreFrom := cr.Spec.ForProvider.RestoreFrom
	if restoreFrom == nil {
		return c.external.Create(ctx, cr)
	}
	cr.Status.SetConditions(xpv2.Creating())
	// The restored cluster has the master password of its source. No state
	// is recorded, so the next Observe sees this password as not applied
	// and Update sets it.
	if _, err := c.hooks.masterPassword(ctx, cr); err != nil {
		return managed.ExternalCreation{}, err
	}

	id := pointer.ToOrNilIfZeroValue(meta.GetExternalName(cr))
	var cluster *svcsdktypes.DBCluster
	switch restoreFrom.Source {
	case svcapitypes.RestoreSourceSnapshot:
		input := generateRestoreDBClusterFromSnapshotInput(cr)
		input.DBClusterIdentifier = id
		out, err := c.client.RestoreDBClusterFromSnapshot(ctx, input)
		if err != nil {
			return managed.ExternalCreation{}, awsclient.Wrap(err, errRestore)
		}
		cluster = out.DBCluster
	case svcapitypes.RestoreSourcePointInTime:
		input := generateRestoreDBClusterToPointInTimeInput(cr)
		input.DBClusterIdentifier = id
		out, err := c.client.RestoreDBClusterToPointInTime(ctx, input)
		if err != nil {
			return managed.ExternalCreation{}, awsclient.Wrap(err, errRestore)
		}
		cluster = out.DBCluster
	default:
		return managed.ExternalCreation{}, errors.New(errUnknownRestoreFromSource)
	}
	if cluster != nil {
		observed := GenerateDBCluster(&svcsdk.DescribeDBClustersOutput{DBClusters: []svcsdktypes.DBCluster{*cluster}})
		observed.Status.AtProvider.DeepCopyInto(&cr.Status.AtProvider)
	}
	return managed.ExternalCreation{ConnectionDetails: getConnectionDetails(cr, cluster)}, nil
}

func (e *hooks) postCreate(ctx context.Context, cr *svcapitypes.DBCluster, resp *svcsdk.CreateDBClusterOutput, cre managed.ExternalCreation, err error) (managed.ExternalCreation, error) {
	if err != nil {
		return managed.ExternalCreation{}, err
	}

	var cluster *svcsdktypes.DBCluster
	if resp != nil {
		cluster = resp.DBCluster
	}
	cre.ConnectionDetails = getConnectionDetails(cr, cluster)
	if e.sentPassword != "" {
		if err := e.recordAppliedPassword(ctx, cr, e.sentPassword); err != nil {
			return managed.ExternalCreation{}, err
		}
		cre.ConnectionDetails[awsclient.ResourceCredentialsSecretPasswordKey] = []byte(e.sentPassword)
	}
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
	wrapExternal = wrap
}
