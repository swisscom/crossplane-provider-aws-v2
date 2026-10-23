package dbinstance

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	svcsdk "github.com/aws/aws-sdk-go-v2/service/rds"
	svcsdktypes "github.com/aws/aws-sdk-go-v2/service/rds/types"
	"github.com/crossplane/crossplane-runtime/v2/pkg/controller"
	"github.com/crossplane/crossplane-runtime/v2/pkg/errors"
	"github.com/crossplane/crossplane-runtime/v2/pkg/meta"
	"github.com/crossplane/crossplane-runtime/v2/pkg/reconciler/managed"
	xpv2 "github.com/crossplane/crossplane/apis/v2/core/v2"
	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"k8s.io/utils/ptr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	svcapitypes "github.com/swisscom/crossplane-provider-aws-v2/apis/rds/v1alpha1"
	awsclient "github.com/swisscom/crossplane-provider-aws-v2/internal/clients/aws"
	dbinstance "github.com/swisscom/crossplane-provider-aws-v2/internal/clients/rds"
	"github.com/swisscom/crossplane-provider-aws-v2/internal/controller/rds/utils"
	"github.com/swisscom/crossplane-provider-aws-v2/internal/utils/compare"
	"github.com/swisscom/crossplane-provider-aws-v2/internal/utils/jsonpatch"
	"github.com/swisscom/crossplane-provider-aws-v2/internal/utils/pointer"
	"github.com/swisscom/crossplane-provider-aws-v2/internal/utils/tags"
)

// error constants
const (
	errCreateReadReplica        = "cannot creat DB instance read replica"
	errS3RestoreFailed          = "cannot restore DB instance from S3 backup"
	errSnapshotRestoreFailed    = "cannot restore DB instance from snapshot"
	errPointInTimeRestoreFailed = "cannot restore DB instance from point in time"
	errUnknownRestoreSource     = "unknown DB Instance restore source"
	errAddTags                  = "cannot add tags"
	errRemoveTags               = "cannot remove tags"
	errNoARNForTags             = "cannot update tags: the DB instance ARN is unknown"
	errDescribeCluster          = "cannot describe the DB cluster of the instance"
	errLiftDeletionProtection   = "cannot turn off deletion protection before deleting"
	errTDEPassword              = "cannot read tdeCredentialPasswordSecretRef"
	errForeignEngine            = "DBInstance %q has engine %q, which is not managed by rds/DBInstance; refusing to manage it"
)

// time formats
const (
	maintenanceWindowFormat = "Mon:15:04"
	backupWindowFormat      = "15:04"
)

// database roles
const (
	databaseRoleStandalone    = "Instance"
	databaseRolePrimary       = "Primary"
	databaseRoleReadReplica   = "Replica"
	databaseRoleClusterWriter = "Writer Instance"
	databaseRoleClusterReader = "Reader Instance"
)

// other
const (
	statusDeleting = "deleting"
	statusCreating = "creating"
)

type customConnector struct {
	kube  client.Client
	usage awsclient.Tracker
}

type customExternal struct {
	external
	shared *shared
}

type shared struct {
	external
	cache *cache
}

type cache struct {
	addTags                       []svcsdktypes.Tag
	removeTags                    []string
	desiredPassword               string
	passwordChanged               bool
	backupWindowChanged           bool
	backupRetentionPeriodUpToDate bool
	engineVersionUpToDate         bool
	masterUserPasswordManaged     bool
	deletionProtection            *bool
	// tagKeysChanged makes Observe report a late-init so that the runtime
	// persists the managed tag keys before Update adds a tag.
	tagKeysChanged bool
}

func preObserve(_ context.Context, cr *svcapitypes.DBInstance, obj *svcsdk.DescribeDBInstancesInput) error {
	obj.DBInstanceIdentifier = pointer.ToOrNilIfZeroValue(meta.GetExternalName(cr))
	return nil
}

func (s *shared) preDelete(ctx context.Context, cr *svcapitypes.DBInstance, obj *svcsdk.DeleteDBInstanceInput) (bool, error) {
	obj.DBInstanceIdentifier = pointer.ToOrNilIfZeroValue(meta.GetExternalName(cr))
	obj.FinalDBSnapshotIdentifier = pointer.ToOrNilIfZeroValue(cr.Spec.ForProvider.FinalDBSnapshotIdentifier)
	obj.SkipFinalSnapshot = pointer.ToOrNilIfZeroValue(cr.Spec.ForProvider.SkipFinalSnapshot)
	obj.DeleteAutomatedBackups = cr.Spec.ForProvider.DeleteAutomatedBackups

	if pointer.StringValue(cr.Status.AtProvider.DBInstanceStatus) == statusDeleting {
		return true, nil
	}
	if !s.mustLiftDeletionProtection(cr) {
		return false, nil
	}
	// Send only this field: a full modify from the spec can fail on unrelated drift.
	_, err := s.client.ModifyDBInstance(ctx, &svcsdk.ModifyDBInstanceInput{
		DBInstanceIdentifier: obj.DBInstanceIdentifier,
		DeletionProtection:   aws.Bool(false),
		ApplyImmediately:     aws.Bool(true),
	})
	if IsNotFound(err) {
		return false, nil
	}
	if err != nil {
		return false, awsclient.Wrap(err, errLiftDeletionProtection)
	}
	// Delete in the next round; AWS may still reject it right after the modify.
	return true, nil
}

// mustLiftDeletionProtection reports whether the spec turns deletion protection
// off and AWS may still have it on. Cluster members follow their cluster.
func (s *shared) mustLiftDeletionProtection(cr *svcapitypes.DBInstance) bool {
	p := cr.Spec.ForProvider
	if p.DeletionProtection == nil || *p.DeletionProtection || p.DBClusterIdentifier != nil {
		return false
	}
	return s.cache.deletionProtection == nil || *s.cache.deletionProtection
}

func hasPreferredBackupWindowChanged(cr *svcapitypes.DBInstance, out *svcsdktypes.DBInstance) (bool, error) {
	// 1. If PreferredBackupWindow is not set, AWS sets a random window
	// so we do not try to update in this case
	// 2. AWS Backup takes ownership of PreferredBackupWindow if it is in use.
	// So we only check PreferredBackupWindow, if there is no associated RecoveryPoint.
	if cr.Spec.ForProvider.PreferredBackupWindow != nil &&
		out.AwsBackupRecoveryPointArn == nil {
		return compareTimeRanges(backupWindowFormat, cr.Spec.ForProvider.PreferredBackupWindow, out.PreferredBackupWindow)
	}
	return false, nil
}

func isEngineVersionUpToDate(cr *svcapitypes.DBInstance, out *svcsdk.DescribeDBInstancesOutput) bool {
	// If EngineVersion is not set, AWS sets a default value,
	// so we do not try to update in this case
	if cr.Spec.ForProvider.EngineVersion != nil {
		if len(out.DBInstances) == 0 || out.DBInstances[0].EngineVersion == nil {
			return false
		}

		desiredEngineVersion := ptr.Deref(cr.Spec.ForProvider.EngineVersion, "")
		currentEngineVersion := ptr.Deref(out.DBInstances[0].EngineVersion, "")
		pendingEngineVersion := ""
		if out.DBInstances[0].PendingModifiedValues != nil {
			pendingEngineVersion = ptr.Deref(out.DBInstances[0].PendingModifiedValues.EngineVersion, "")
		}
		// If the desired version matches the current version and pending version is set means an upgrade should be reverted
		// so controller should send an update request with the current version again to cancel the pending upgrade
		if desiredEngineVersion == currentEngineVersion && pendingEngineVersion != "" {
			return false
		}
		// If ApplyImmediately is false and there is a pending version, consider that as the current version for comparison
		if !ptr.Deref(cr.Spec.ForProvider.ApplyImmediately, false) && pendingEngineVersion != "" {
			currentEngineVersion = pendingEngineVersion
		}

		// Upgrade is only necessary if the spec version is higher.
		// Downgrades are not possible in pointer.
		c := utils.CompareEngineVersions(*cr.Spec.ForProvider.EngineVersion, currentEngineVersion)
		return c <= 0
	}
	return true
}

func isOptionGroupUpToDate(cr *svcapitypes.DBInstance, out *svcsdktypes.DBInstance) bool {
	// If OptionGroupName is not set, AWS sets a default OptionGroup,
	// so we do not try to update in this case
	if cr.Spec.ForProvider.OptionGroupName != nil {
		for _, group := range out.OptionGroupMemberships {
			if group.OptionGroupName != nil && (pointer.StringValue(group.OptionGroupName) == pointer.StringValue(cr.Spec.ForProvider.OptionGroupName)) {

				switch pointer.StringValue(group.Status) {
				case "pending-maintenance-apply":
					// If ApplyImmediately was turned on after the OptionGroup change was requested,
					// we can make a new Modify request
					if pointer.BoolValue(cr.Spec.ForProvider.ApplyImmediately) {
						return false
					}
					return true
				case "pending-maintenance-removal":
					return false
				default: // "in-sync", "applying", "pending-apply", "pending-removal", "removing", "failed"
					return true
				}
			}
		}
		return false
	}
	return true
}

func parseTimeWindowSpan(span, format string) (string, time.Time, error) {
	// Regex: optional day, always HH:MM
	re := regexp.MustCompile(`^\s*([A-Za-z]+:)?(\d{2}:\d{2})\s*$`)
	matches := re.FindStringSubmatch(span)
	if len(matches) != 3 {
		return "", time.Time{}, errors.New("invalid time format, expected '[day:]HH:MM'")
	}
	day := ""
	if matches[1] != "" {
		day = strings.ToLower(strings.TrimSuffix(matches[1], ":"))
	}
	t, err := time.Parse(format, strings.TrimSpace(span))
	if err != nil {
		return day, time.Time{}, err
	}
	return day, t, nil
}

// compareTimeRanges returns true if the expected window and actual window are different or if the actual window is empty.
// It returns false if they are the same or if the expected window is not set (since AWS sets a default).
func compareTimeRanges(format string, expectedWindow *string, actualWindow *string) (bool, error) {
	if pointer.StringValue(expectedWindow) == "" {
		return false, nil
	}
	if pointer.StringValue(actualWindow) == "" {
		return true, nil
	}
	leftSpans := strings.Split(*expectedWindow, "-")
	rightSpans := strings.Split(*actualWindow, "-")

	if len(leftSpans) != 2 || len(rightSpans) != 2 {
		return false, errors.New("invalid time window format")
	}

	for i := range leftSpans {
		dayA, timeA, err := parseTimeWindowSpan(leftSpans[i], format)
		if err != nil {
			return false, err
		}
		dayB, timeB, err := parseTimeWindowSpan(rightSpans[i], format)
		if err != nil {
			return false, err
		}
		if dayA != dayB || !timeA.Equal(timeB) {
			return true, nil
		}
	}
	return false, nil
}

func areVPCSecurityGroupIDsUpToDate(cr *svcapitypes.DBInstance, out *svcsdktypes.DBInstance) bool {
	// VPCSecurityGroupIDs is ignored for instances that belong to a cluster.
	if out.DBClusterIdentifier != nil {
		return true
	}

	desiredIDs := cr.Spec.ForProvider.VPCSecurityGroupIDs

	// if user is fine with default SG or lets DBCluster manage it
	// (removing all SGs is not possible, AWS will keep last set SGs)
	if len(desiredIDs) == 0 {
		return true
	}

	actualGroups := out.VpcSecurityGroups

	if len(desiredIDs) != len(actualGroups) {
		return false
	}

	actualIDs := make([]string, 0, len(actualGroups))
	for _, grp := range actualGroups {
		actualIDs = append(actualIDs, pointer.StringValue(grp.VpcSecurityGroupId))
	}

	// Sort copies: sorting the spec slice in place would rewrite the user's order.
	return slices.Equal(slices.Sorted(slices.Values(desiredIDs)), slices.Sorted(slices.Values(actualIDs)))
}

func isDBParameterGroupNameUpToDate(cr *svcapitypes.DBInstance, out *svcsdktypes.DBInstance) bool {
	desiredGroup := cr.Spec.ForProvider.DBParameterGroupName

	// if user is fine with default DBParameterGroup or lets DBCluster manage it
	if desiredGroup == nil {
		return true
	}

	actualGroups := out.DBParameterGroups

	for _, grp := range actualGroups {

		if pointer.StringValue(grp.DBParameterGroupName) == pointer.StringValue(desiredGroup) {
			return true
		}

	}

	return false
}

func filterList(cr *svcapitypes.DBInstance, obj *svcsdk.DescribeDBInstancesOutput) *svcsdk.DescribeDBInstancesOutput {
	resp := &svcsdk.DescribeDBInstancesOutput{}
	for _, dbInstance := range obj.DBInstances {
		// AWS stores identifiers in lowercase, so compare case-insensitively.
		if !compare.IdentifierEqual(pointer.StringValue(dbInstance.DBInstanceIdentifier), meta.GetExternalName(cr)) {
			continue
		}
		// Let a deleted MR go without touching a DocumentDB or Neptune instance of the same name.
		if meta.WasDeleted(cr) && isForeignEngine(dbInstance.Engine) {
			break
		}
		resp.DBInstances = append(resp.DBInstances, dbInstance)
		break
	}
	return resp
}

// isForeignEngine reports whether DescribeDBInstances returned an instance of
// another service: the RDS API also lists DocumentDB and Neptune instances.
func isForeignEngine(engine *string) bool {
	e := strings.ToLower(pointer.StringValue(engine))
	return e == "docdb" || e == "neptune"
}

func handleKmsKey(inKey *string, dbKey *string) *string {
	if inKey != nil && dbKey != nil && !strings.Contains(*inKey, "/") {
		lastInd := strings.LastIndex(*dbKey, "/")
		keyID := (*dbKey)[lastInd+1:]
		return &keyID
	}
	return dbKey
}

// isStorageTypeGP3BelowAllocatedStorageThreshold returns true if storageType is gp3 and allocatedStorage is below engine specific threshold
// See also https://docs.aws.amazon.com/AmazonRDS/latest/UserGuide/CHAP_Storage.html#gp3-storage.
func isStorageTypeGP3BelowAllocatedStorageThreshold(cr *svcapitypes.DBInstance) bool {
	if pointer.StringValue(cr.Spec.ForProvider.StorageType) != "gp3" {
		return false
	}

	return pointer.Int64Value(cr.Spec.ForProvider.AllocatedStorage) < gp3AllocatedStorageThreshold(cr)
}

// gp3AllocatedStorageThreshold returns the minimum allocatedStorage (in GB) required to provision custom iops/storageThroughput.
func gp3AllocatedStorageThreshold(cr *svcapitypes.DBInstance) int64 {
	switch pointer.StringValue(cr.Spec.ForProvider.Engine) {
	case "mariadb", "mysql", "postgres":
		return 400
	case "oracle-ee", "oracle-ee-cdb", "oracle-se2", "oracle-se2-cdb":
		return 200
	}
	return 400
}

func (c *customExternal) Create(ctx context.Context, cr *svcapitypes.DBInstance) (managed.ExternalCreation, error) {
	switch {
	case isReadReplica(cr):
		cr.Status.SetConditions(xpv2.Creating())
		cr.Status.AtProvider.DatabaseRole = aws.String(databaseRoleReadReplica)

		createDBInstanceReadReplicaInput := dbinstance.GenerateCreateDBInstanceReadReplicaInput(cr)
		createDBInstanceReadReplicaInput.DBInstanceIdentifier = pointer.ToOrNilIfZeroValue(meta.GetExternalName(cr))

		_, err := c.client.CreateDBInstanceReadReplica(ctx, createDBInstanceReadReplicaInput)
		if err != nil {
			return managed.ExternalCreation{}, awsclient.Wrap(err, errCreateReadReplica)
		}
		return managed.ExternalCreation{}, nil
	case cr.Spec.ForProvider.RestoreFrom != nil:
		return c.shared.restore(ctx, cr)
	}
	return c.external.Create(ctx, cr)
}

// preCreate prepares a standalone instance or a cluster member; read replicas
// and restores don't reach CreateDBInstance (see Create).
func (s *shared) preCreate(ctx context.Context, cr *svcapitypes.DBInstance, obj *svcsdk.CreateDBInstanceInput) (err error) {
	var pw string
	if cr.Spec.ForProvider.DBClusterIdentifier == nil { // cluster members use the cluster's password
		if pw, err = s.sourcePassword(ctx, cr, false); err != nil {
			return err
		}
	}

	obj.MasterUserPassword = pointer.ToOrNilIfZeroValue(pw)
	obj.DBInstanceIdentifier = pointer.ToOrNilIfZeroValue(meta.GetExternalName(cr))
	if obj.TdeCredentialPassword, err = s.tdePassword(ctx, cr); err != nil {
		return err
	}

	// VpcSecurityGroupIds cannot be set on an instance that belongs to a DBCluster
	// NOTE: Unlike in preUpdate we are using spec here because status is not yet available.
	if cr.Spec.ForProvider.DBClusterIdentifier == nil {
		if len(cr.Spec.ForProvider.VPCSecurityGroupIDs) > 0 {
			obj.VpcSecurityGroupIds = cr.Spec.ForProvider.VPCSecurityGroupIDs
		}
	} else {
		obj.VpcSecurityGroupIds = nil
	}

	if len(cr.Spec.ForProvider.DBSecurityGroups) > 0 {
		obj.DBSecurityGroups = cr.Spec.ForProvider.DBSecurityGroups
	}

	if cr.Spec.ForProvider.EngineVersion != nil {
		obj.EngineVersion = cr.Spec.ForProvider.EngineVersion
	}

	// Cache before the call, so a lost response can't lose the password AWS got.
	if err := s.cachePassword(ctx, cr, pw, ""); err != nil {
		return err
	}

	// for storageType gp3 below engine specific allocatedStorage threshold, do not send iops and storageThroughput
	// to avoid errors like "You can't specify IOPS or storage throughput for engine postgres and a storage size less than 400."
	// This allows users to set iops/storageThroughput to the default values themselves.
	if isStorageTypeGP3BelowAllocatedStorageThreshold(cr) {
		obj.Iops = nil
		obj.StorageThroughput = nil
	}

	return nil
}

func (s *shared) preUpdate(ctx context.Context, cr *svcapitypes.DBInstance, obj *svcsdk.ModifyDBInstanceInput) (err error) { //nolint:gocyclo
	obj.DBInstanceIdentifier = pointer.ToOrNilIfZeroValue(meta.GetExternalName(cr))
	obj.ApplyImmediately = cr.Spec.ForProvider.ApplyImmediately
	if obj.TdeCredentialPassword, err = s.tdePassword(ctx, cr); err != nil {
		return err
	}
	// Only set MasterUserPassword if it has changed, otherwise it triggers "Resetting master password" on aws side,
	// it happens because aws doesn't know current password, so any set causes a change.
	if s.cache.passwordChanged {
		obj.MasterUserPassword = pointer.ToOrNilIfZeroValue(s.cache.desiredPassword)
	}

	// ManageMasterUserPassword and MasterUserSecretKmsKeyId are only accepted when
	// turning Secrets Manager management on or off. Turning it on excludes
	// MasterUserPassword; turning it off requires one.
	manage := cr.Spec.ForProvider.ManageMasterUserPassword
	switch {
	case manage == nil || *manage == s.cache.masterUserPasswordManaged:
		obj.ManageMasterUserPassword = nil
		obj.MasterUserSecretKmsKeyId = nil
	case *manage:
		// Turning it on: AWS owns the password from now, so don't send or publish one.
		obj.MasterUserPassword = nil
		s.cache.passwordChanged = false
	default:
		// Turning it off needs a new password; the old cache is stale.
		obj.MasterUserSecretKmsKeyId = nil
		pw, err := s.passwordForManagedOff(ctx, cr)
		if err != nil {
			return err
		}
		obj.MasterUserPassword = aws.String(pw)
		s.cache.desiredPassword, s.cache.passwordChanged = pw, true
	}

	// VpcSecurityGroupIds cannot be set on an instance that belongs to a DBCluster
	if cr.Status.AtProvider.DBClusterIdentifier == nil {
		if cr.Spec.ForProvider.VPCSecurityGroupIDs != nil {
			obj.VpcSecurityGroupIds = cr.Spec.ForProvider.VPCSecurityGroupIDs
		}
	} else {
		obj.VpcSecurityGroupIds = nil
	}

	// for storageType gp3 below engine specific allocatedStorage threshold, do not send iops and storageThroughput
	// to avoid errors like "You can't specify IOPS or storage throughput for engine postgres and a storage size less than 400."
	// This allows users to set iops/storageThroughput to the default values themselves.
	if isStorageTypeGP3BelowAllocatedStorageThreshold(cr) {
		obj.Iops = nil
		obj.StorageThroughput = nil
	}

	if !s.cache.engineVersionUpToDate && cr.Spec.ForProvider.EngineVersion != nil {
		obj.EngineVersion = cr.Spec.ForProvider.EngineVersion // add EngineVersion if changed and no downgrade
	}

	// AWS Backup takes ownership of BackupRetentionPeriod and PreferredBackupWindow if it is in use.
	// Therefore we only set these fields in the ModifyDBInstanceInput if they are changed and not in use by AWS Backup.
	if !s.cache.backupWindowChanged {
		obj.PreferredBackupWindow = nil
	}
	if s.cache.backupRetentionPeriodUpToDate {
		obj.BackupRetentionPeriod = nil
	}

	// The port of an instance that belongs to a DBCluster is managed by the cluster and AWS rejects
	// ModifyDBInstance requests containing DBPortNumber with InvalidParameterCombination.
	// Spec.ForProvider.DBClusterIdentifier is set from the observed AWS state in postObserve.
	if cr.Spec.ForProvider.DBClusterIdentifier == nil {
		obj.DBPortNumber = pointer.Int32(cr.Spec.ForProvider.Port)
	}

	obj.CloudwatchLogsExportConfiguration = utils.GenerateCloudWatchExportConfiguration(
		cr.Spec.ForProvider.EnableCloudwatchLogsExports,
		aws.ToStringSlice(cr.Status.AtProvider.EnabledCloudwatchLogsExports))

	// LicenseModel cannot be modified on read replicas - AWS rejects the entire ModifyDBInstance request.
	// After LateInitialize(), licenseModel is always populated in spec.forProvider from the observed AWS state,
	// which causes it to be included in every subsequent modify call.
	if cr.Spec.ForProvider.SourceDBInstanceID != nil {
		obj.LicenseModel = nil
	}

	// Record the password before AWS gets it, marked as not applied yet: after
	// a lost response or a failed cache write the next round re-sends it
	// instead of publishing the old one. postUpdate clears the mark.
	if obj.MasterUserPassword != nil {
		if err := s.cachePassword(ctx, cr, *obj.MasterUserPassword, string(dbinstance.RestoreStateRestored)); err != nil {
			return err
		}
	}
	return nil
}

func (s *shared) postUpdate(ctx context.Context, cr *svcapitypes.DBInstance, out *svcsdk.ModifyDBInstanceOutput, upd managed.ExternalUpdate, err error) (managed.ExternalUpdate, error) {
	if err != nil {
		return upd, err
	}
	pw := s.cache.desiredPassword
	changed := s.cache.passwordChanged && pw != ""
	// Cache before anything else can fail: AWS has this password now.
	if changed {
		if err := s.cachePassword(ctx, cr, pw, ""); err != nil {
			return managed.ExternalUpdate{}, err
		}
	}
	if err := s.updateTags(ctx, cr, out); err != nil {
		return managed.ExternalUpdate{}, err
	}
	if changed {
		upd.ConnectionDetails = connectionDetails(cr, upd.ConnectionDetails, pw, false)
	}
	return upd, nil
}

// tagARN is the ARN from the modify output; ModifyDBInstance may return no
// instance, and then the observed ARN is just as good.
func tagARN(cr *svcapitypes.DBInstance, out *svcsdk.ModifyDBInstanceOutput) *string {
	if out != nil && out.DBInstance != nil && out.DBInstance.DBInstanceArn != nil {
		return out.DBInstance.DBInstanceArn
	}
	return cr.Status.AtProvider.DBInstanceARN
}

func (s *shared) updateTags(ctx context.Context, cr *svcapitypes.DBInstance, out *svcsdk.ModifyDBInstanceOutput) error {
	if len(s.cache.addTags) == 0 && len(s.cache.removeTags) == 0 {
		return nil
	}
	arn := tagARN(cr, out)
	if pointer.StringValue(arn) == "" {
		return errors.New(errNoARNForTags)
	}
	if len(s.cache.addTags) > 0 {
		if _, err := s.client.AddTagsToResource(ctx, &svcsdk.AddTagsToResourceInput{ResourceName: arn, Tags: s.cache.addTags}); err != nil {
			return awsclient.Wrap(err, errAddTags)
		}
	}
	if len(s.cache.removeTags) > 0 {
		if _, err := s.client.RemoveTagsFromResource(ctx, &svcsdk.RemoveTagsFromResourceInput{ResourceName: arn, TagKeys: s.cache.removeTags}); err != nil {
			return awsclient.Wrap(err, errRemoveTags)
		}
	}
	return nil
}

func specTagKeys(spec []*svcapitypes.Tag) []string {
	out := make([]string, 0, len(spec))
	for _, t := range spec {
		out = append(out, pointer.StringValue(t.Key))
	}
	return out
}

func sdkTagKeys(current []svcsdktypes.Tag) []string {
	out := make([]string, 0, len(current))
	for _, t := range current {
		out = append(out, pointer.StringValue(t.Key))
	}
	return out
}

func lateInitialize(in *svcapitypes.DBInstanceParameters, out *svcsdk.DescribeDBInstancesOutput) error { //nolint:gocyclo
	// (PocketMobsters): The controller should already be checking if out is nil so we *should* have a dbinstance here, always
	db := out.DBInstances[0]
	if isForeignEngine(db.Engine) {
		return errors.Errorf(errForeignEngine, pointer.StringValue(db.DBInstanceIdentifier), pointer.StringValue(db.Engine))
	}
	in.DBInstanceClass = pointer.LateInitialize(in.DBInstanceClass, db.DBInstanceClass)
	in.Engine = pointer.LateInitialize(in.Engine, db.Engine)

	in.DBClusterIdentifier = pointer.LateInitialize(in.DBClusterIdentifier, db.DBClusterIdentifier)
	// If the instance belongs to a cluster, these fields must not be late-initialized, to allow
	// the user to manage them via the cluster. AWS echoes the cluster's values per-instance in
	// DescribeDBInstances, so late-initializing them would plant cluster-owned values into the
	// spec that are then resent on every ModifyDBInstance, causing perpetual updates.
	if in.DBClusterIdentifier == nil {
		in.AllocatedStorage = pointer.LateInitialize(in.AllocatedStorage, pointer.Int64(db.AllocatedStorage))
		in.BackupRetentionPeriod = pointer.LateInitialize(in.BackupRetentionPeriod, pointer.Int64(db.BackupRetentionPeriod))
		in.CopyTagsToSnapshot = pointer.LateInitialize(in.CopyTagsToSnapshot, db.CopyTagsToSnapshot)
		in.DeletionProtection = pointer.LateInitialize(in.DeletionProtection, db.DeletionProtection)
		in.EnableIAMDatabaseAuthentication = pointer.LateInitialize(in.EnableIAMDatabaseAuthentication, db.IAMDatabaseAuthenticationEnabled)
		in.PreferredBackupWindow = pointer.LateInitialize(in.PreferredBackupWindow, db.PreferredBackupWindow)
		in.StorageEncrypted = pointer.LateInitialize(in.StorageEncrypted, db.StorageEncrypted)
		in.StorageType = pointer.LateInitialize(in.StorageType, db.StorageType)
		in.EngineVersion = pointer.LateInitialize(in.EngineVersion, db.EngineVersion)
		in.IOPS = pointer.LateInitialize(in.IOPS, pointer.Int64(db.Iops))
		in.MaxAllocatedStorage = pointer.LateInitialize(in.MaxAllocatedStorage, pointer.Int64(db.MaxAllocatedStorage))
		in.StorageThroughput = pointer.LateInitialize(in.StorageThroughput, pointer.Int64(db.StorageThroughput))
		in.MultiAZ = pointer.LateInitialize(in.MultiAZ, db.MultiAZ)
		in.AutoMinorVersionUpgrade = pointer.LateInitialize(in.AutoMinorVersionUpgrade, db.AutoMinorVersionUpgrade)
		if db.Endpoint != nil {
			in.Port = pointer.LateInitialize(in.Port, pointer.Int64(db.Endpoint.Port))
		}
		if in.DBParameterGroupName == nil {
			for i := range db.DBParameterGroups {
				if db.DBParameterGroups[i].DBParameterGroupName != nil {
					in.DBParameterGroupName = db.DBParameterGroups[i].DBParameterGroupName
					break
				}
			}
		}
		if len(in.VPCSecurityGroupIDs) == 0 && len(db.VpcSecurityGroups) != 0 {
			in.VPCSecurityGroupIDs = make([]string, len(db.VpcSecurityGroups))
			for i, val := range db.VpcSecurityGroups {
				in.VPCSecurityGroupIDs[i] = pointer.StringValue(val.VpcSecurityGroupId)
			}
		}
	}
	in.AvailabilityZone = pointer.LateInitialize(in.AvailabilityZone, db.AvailabilityZone)
	in.CACertificateIdentifier = pointer.LateInitialize(in.CACertificateIdentifier, db.CACertificateIdentifier)
	in.CharacterSetName = pointer.LateInitialize(in.CharacterSetName, db.CharacterSetName)
	in.DBName = pointer.LateInitialize(in.DBName, db.DBName)
	in.EnablePerformanceInsights = pointer.LateInitialize(in.EnablePerformanceInsights, db.PerformanceInsightsEnabled)
	kmsKey := handleKmsKey(in.KMSKeyID, db.KmsKeyId)
	in.KMSKeyID = pointer.LateInitialize(in.KMSKeyID, kmsKey)
	in.LicenseModel = pointer.LateInitialize(in.LicenseModel, db.LicenseModel)
	in.MasterUsername = pointer.LateInitialize(in.MasterUsername, db.MasterUsername)

	if pointer.Int32Value(db.MonitoringInterval) > 0 {
		in.MonitoringInterval = pointer.LateInitialize(in.MonitoringInterval, pointer.Int64(db.MonitoringInterval))
	}

	in.MonitoringRoleARN = pointer.LateInitialize(in.MonitoringRoleARN, db.MonitoringRoleArn)
	// PerformanceInsightsKMSKeyID and PerformanceInsightsRetentionPeriod are only meaningful
	// while Performance Insights is enabled. AWS omits them from the describe response when PI
	// is disabled, so a one-directional LateInitialize would leave a stale value stuck in the
	// spec forever once PI is turned off (perpetual diff and a ModifyDBInstance that AWS
	// rejects). Fill them only while PI is desired-enabled, and clear them otherwise so a
	// previously planted value cannot get stuck.
	if pointer.BoolValue(in.EnablePerformanceInsights) {
		in.PerformanceInsightsKMSKeyID = pointer.LateInitialize(in.PerformanceInsightsKMSKeyID, db.PerformanceInsightsKMSKeyId)
		in.PerformanceInsightsRetentionPeriod = pointer.LateInitialize(in.PerformanceInsightsRetentionPeriod, pointer.Int64(db.PerformanceInsightsRetentionPeriod))
	} else {
		in.PerformanceInsightsKMSKeyID = nil
		in.PerformanceInsightsRetentionPeriod = nil
	}
	in.PreferredMaintenanceWindow = pointer.LateInitialize(in.PreferredMaintenanceWindow, db.PreferredMaintenanceWindow)
	in.PromotionTier = pointer.LateInitialize(in.PromotionTier, pointer.Int64(db.PromotionTier))
	in.PubliclyAccessible = pointer.LateInitialize(in.PubliclyAccessible, db.PubliclyAccessible)
	in.Timezone = pointer.LateInitialize(in.Timezone, db.Timezone)

	if len(in.DBSecurityGroups) == 0 && len(db.DBSecurityGroups) != 0 {
		in.DBSecurityGroups = make([]string, len(db.DBSecurityGroups))
		for i, val := range db.DBSecurityGroups {
			in.DBSecurityGroups[i] = pointer.StringValue(val.DBSecurityGroupName)
		}
	}
	if pointer.StringValue(in.DBSubnetGroupName) == "" && db.DBSubnetGroup != nil {
		in.DBSubnetGroupName = db.DBSubnetGroup.DBSubnetGroupName
	}
	if len(in.EnableCloudwatchLogsExports) == 0 && len(db.EnabledCloudwatchLogsExports) != 0 {
		in.EnableCloudwatchLogsExports = aws.StringSlice(db.EnabledCloudwatchLogsExports)
	}
	if len(in.ProcessorFeatures) == 0 && len(db.ProcessorFeatures) != 0 {
		in.ProcessorFeatures = make([]*svcapitypes.ProcessorFeature, len(db.ProcessorFeatures))
		for i, val := range db.ProcessorFeatures {
			in.ProcessorFeatures[i] = &svcapitypes.ProcessorFeature{
				Name:  val.Name,
				Value: val.Value,
			}
		}
	}

	return nil
}

func (s *shared) isUpToDate(ctx context.Context, cr *svcapitypes.DBInstance, out *svcsdk.DescribeDBInstancesOutput) (upToDate bool, diff string, err error) { //nolint:gocyclo
	// If ApplyImmediately is not true we update external state of db instance with pending modified values to prevent redundant updates
	if !ptr.Deref(cr.Spec.ForProvider.ApplyImmediately, false) {
		utils.SetPmvDBInstance(out)
	}
	db := &out.DBInstances[0]
	patch, err := createPatch(out, &cr.Spec.ForProvider)
	if err != nil {
		return false, "", err
	}
	// (PocketMobsters): Certain statuses can cause us to send excessive updates because the
	// expected state of the kubernetes resource differs from the actual state of the remote
	// AWS resource temporarily. Once modifications are done, we can begin sending update requests
	// again.
	// This could be matured a bit more for specific statuses, such as not allowing storage changes
	// when the status is "storage-optimization"
	status := pointer.StringValue(out.DBInstances[0].DBInstanceStatus)
	if status == "modifying" || status == "upgrading" || status == "rebooting" || status == statusCreating || status == statusDeleting {
		return true, "", nil
	}

	autogenerate := cr.Spec.ForProvider.AutogeneratePassword
	masterUserPasswordSecretRef := cr.Spec.ForProvider.MasterUserPasswordSecretRef
	// A missing cache reads as "" with no error; any other error must not
	// read as "no cache", or a replica would skip its password check.
	cachedPassword, err := dbinstance.GetCachedPassword(ctx, s.kube, cr)
	if err != nil {
		return false, "", errors.Wrap(err, dbinstance.ErrGetCachedPassword)
	}
	cachedMasterPasswordExist := cachedPassword != ""

	// While AWS manages the password there is nothing to compare.
	s.cache.masterUserPasswordManaged = db.MasterUserSecret != nil

	// If the instance is a read replica and the password was not created before, and it is not assumed to be
	// generated/created(by autogenerate or masterUserPasswordSecretRef), we don't check the password. By
	// default, a read replica has the same credentials as the primary instance.
	if !s.cache.masterUserPasswordManaged && (pointer.StringValue(cr.Status.AtProvider.DatabaseRole) != databaseRoleReadReplica ||
		autogenerate || masterUserPasswordSecretRef != nil || cachedMasterPasswordExist) {
		passwordIsUpToDate, desiredPassword, err := dbinstance.PasswordUpToDate(ctx, s.kube, cr)
		s.cache.desiredPassword = desiredPassword
		s.cache.passwordChanged = !passwordIsUpToDate

		if err != nil {
			return false, "", errors.Wrap(err, dbinstance.ErrNoPasswordUpToDate)
		}
	}
	passwordChanged := s.cache.passwordChanged

	// (PocketMobsters): AWS reformats our preferred time windows for backups and maintenance,
	// so we can't rely on automatic equality checks for them
	maintenanceWindowChanged, err := compareTimeRanges(maintenanceWindowFormat, cr.Spec.ForProvider.PreferredMaintenanceWindow, db.PreferredMaintenanceWindow)
	if err != nil {
		return false, "", err
	}
	s.cache.backupWindowChanged, err = hasPreferredBackupWindowChanged(cr, db)
	if err != nil {
		return false, "", err
	}
	backupWindowChanged := s.cache.backupWindowChanged

	s.cache.backupRetentionPeriodUpToDate = isBackupRetentionPeriodUpToDate(cr, db)
	backupRetentionPeriodChanged := !s.cache.backupRetentionPeriodUpToDate

	// Depending on whether the instance was created as gp2 or modified from another type (s.g. gp3) to gp2,
	// AWS provides different responses for IOPS/StorageThroughput (either 0 or nil).
	// Therefore, we consider both 0 and nil to be equivalent.
	// For gp3 volumes below the engine-specific allocatedStorage threshold, AWS rejects custom
	// iops/storageThroughput values. Return an error so the resource shows SYNCED=False.
	iopsChanged := false
	storageThroughputChanged := false
	// IOPS and StorageThroughput are managed by the cluster for instances that belong to one,
	// so they are not late-initialized into the spec and must not be compared here (the spec
	// value is intentionally nil while AWS echoes the cluster's value, which would otherwise
	// look like a permanent drift).
	//
	// MultiAZ and AutoMinorVersionUpgrade are likewise cluster-managed for cluster members:
	// AWS returns them as nil in DescribeDBInstances for Aurora cluster instances while the
	// spec (often populated by a composition) carries a value, which would otherwise cause a
	// permanent diff and a ModifyDBInstance that AWS ignores. They are ignored in the cmp.Diff
	// below and only compared here for standalone instances.
	autoMinorVersionUpgradeChanged := false
	multiAZChanged := false
	if db.DBClusterIdentifier == nil {
		autoMinorVersionUpgradeChanged = pointer.BoolValue(cr.Spec.ForProvider.AutoMinorVersionUpgrade) != pointer.BoolValue(db.AutoMinorVersionUpgrade)
		multiAZChanged = pointer.BoolValue(cr.Spec.ForProvider.MultiAZ) != pointer.BoolValue(db.MultiAZ)
		if isStorageTypeGP3BelowAllocatedStorageThreshold(cr) {
			if pointer.Int64Value(cr.Spec.ForProvider.IOPS) != int64(pointer.Int32Value(db.Iops)) ||
				pointer.Int64Value(cr.Spec.ForProvider.StorageThroughput) != int64(pointer.Int32Value(db.StorageThroughput)) {
				return false, "", fmt.Errorf("cannot reconcile desired iops/storageThroughput: gp3 volumes below %dGB (engine: %s) use fixed defaults (3000 IOPS / 125 MB/s). Increase allocatedStorage to provision custom values",
					gp3AllocatedStorageThreshold(cr), pointer.StringValue(cr.Spec.ForProvider.Engine))
			}
		} else {
			iopsChanged = pointer.Int64Value(cr.Spec.ForProvider.IOPS) != int64(pointer.Int32Value(db.Iops))
			storageThroughputChanged = pointer.Int64Value(cr.Spec.ForProvider.StorageThroughput) != int64(pointer.Int32Value(db.StorageThroughput))
		}
	}
	s.cache.engineVersionUpToDate = isEngineVersionUpToDate(cr, out)
	versionChanged := !s.cache.engineVersionUpToDate

	vpcSGsChanged := !areVPCSecurityGroupIDsUpToDate(cr, db)
	dbParameterGroupChanged := !isDBParameterGroupNameUpToDate(cr, db)
	optionGroupChanged := !isOptionGroupUpToDate(cr, db)
	cloudwatchLogsExportsChanged := !utils.AreSameElements(cr.Spec.ForProvider.EnableCloudwatchLogsExports, db.EnabledCloudwatchLogsExports)
	additionalStorageVolumesDiff := diffAdditionalStorageVolumes(cr.Spec.ForProvider.AdditionalStorageVolumes, db.AdditionalStorageVolumes)

	diff = cmp.Diff(&svcapitypes.DBInstanceParameters{}, patch, cmpopts.EquateEmpty(),
		cmpopts.IgnoreTypes(&xpv2.NamespacedReference{}, &xpv2.NamespacedSelector{}, []xpv2.NamespacedReference{}),
		cmpopts.IgnoreFields(svcapitypes.DBInstanceParameters{}, "Region"),
		cmpopts.IgnoreFields(svcapitypes.DBInstanceParameters{}, "AvailabilityZone"),
		cmpopts.IgnoreFields(svcapitypes.DBInstanceParameters{}, "AllowMajorVersionUpgrade"),
		cmpopts.IgnoreFields(svcapitypes.DBInstanceParameters{}, "BackupRetentionPeriod"),
		cmpopts.IgnoreFields(svcapitypes.DBInstanceParameters{}, "DBParameterGroupName"),
		cmpopts.IgnoreFields(svcapitypes.DBInstanceParameters{}, "EngineVersion"),
		cmpopts.IgnoreFields(svcapitypes.DBInstanceParameters{}, "IOPS"),
		cmpopts.IgnoreFields(svcapitypes.DBInstanceParameters{}, "StorageThroughput"),
		// MultiAZ and AutoMinorVersionUpgrade are cluster-managed for cluster members (AWS
		// returns nil for Aurora cluster instances). They are compared explicitly above,
		// guarded to standalone instances, to avoid a permanent diff for cluster members.
		cmpopts.IgnoreFields(svcapitypes.DBInstanceParameters{}, "MultiAZ"),
		cmpopts.IgnoreFields(svcapitypes.DBInstanceParameters{}, "AutoMinorVersionUpgrade"),
		// EnableCloudwatchLogsExports is compared explicitly below via
		// utils.AreSameElements so that ordering differences between the desired spec
		// and the AWS response do not produce a spurious diff.
		cmpopts.IgnoreFields(svcapitypes.DBInstanceParameters{}, "EnableCloudwatchLogsExports"),
		cmpopts.IgnoreFields(svcapitypes.DBInstanceParameters{}, "Tags"),
		cmpopts.IgnoreFields(svcapitypes.DBInstanceParameters{}, "SkipFinalSnapshot"),
		cmpopts.IgnoreFields(svcapitypes.DBInstanceParameters{}, "FinalDBSnapshotIdentifier"),
		cmpopts.IgnoreFields(svcapitypes.DBInstanceParameters{}, "MasterUserPasswordSecretRef"),
		cmpopts.IgnoreFields(svcapitypes.DBInstanceParameters{}, "AutogeneratePassword"),
		cmpopts.IgnoreFields(svcapitypes.DBInstanceParameters{}, "PreferredMaintenanceWindow"),
		cmpopts.IgnoreFields(svcapitypes.DBInstanceParameters{}, "PreferredBackupWindow"),
		cmpopts.IgnoreFields(svcapitypes.DBInstanceParameters{}, "OptionGroupName"),
		// Compared explicitly below by volume name.
		cmpopts.IgnoreFields(svcapitypes.DBInstanceParameters{}, "AdditionalStorageVolumes"),
		// Create-only fields: ModifyDBInstance cannot change them.
		cmpopts.IgnoreFields(svcapitypes.DBInstanceParameters{}, "BackupTarget", "CustomIAMInstanceProfile",
			"DBSystemID", "NcharCharacterSetName", "TagSpecifications"),
		// Not returned by DescribeDBInstances, or not changeable once set.
		cmpopts.IgnoreFields(svcapitypes.DBInstanceParameters{}, "MasterUserAuthenticationType",
			"MasterUserSecretKMSKeyID"),
		cmpopts.IgnoreFields(svcapitypes.CustomDBInstanceParameters{}, "ApplyImmediately"),
		cmpopts.IgnoreFields(svcapitypes.CustomDBInstanceParameters{}, "RestoreFrom"),
		cmpopts.IgnoreFields(svcapitypes.CustomDBInstanceParameters{}, "VPCSecurityGroupIDs"),
		cmpopts.IgnoreFields(svcapitypes.CustomDBInstanceParameters{}, "DeleteAutomatedBackups"),
		cmpopts.IgnoreFields(svcapitypes.CustomDBInstanceParameters{},
			"SourceDBClusterID",
			"SourceDBInstanceID", "SourceDBInstanceIDRef", "SourceDBInstanceIDSelector"),
		cmpopts.IgnoreFields(svcapitypes.CustomDBInstanceParameters{}, "TagsIgnore"),
		// AWS never returns it; a new value is sent with the next modify.
		cmpopts.IgnoreFields(svcapitypes.CustomDBInstanceParameters{}, "TDECredentialPasswordSecretRef"),
	)

	// Build ignore rules: implicit aws:* + user supplied rules
	ignore := []string{"aws:*"}
	for _, r := range cr.Spec.ForProvider.TagsIgnore {
		ignore = append(ignore, r.Key)
	}
	// Reset observed tags slice to avoid accumulation across reconciles
	cr.Status.AtProvider.ObservedTags = nil
	var observedTags []svcsdktypes.Tag
	if db.TagList != nil {
		for _, tag := range db.TagList { // index discarded with _
			// Capture all tags for observability
			cr.Status.AtProvider.ObservedTags = append(cr.Status.AtProvider.ObservedTags, &svcapitypes.Tag{Key: tag.Key, Value: tag.Value})
			// Filter only for diff purposes
			if utils.ShouldIgnore(pointer.StringValue(tag.Key), ignore) {
				continue
			}
			observedTags = append(observedTags, svcsdktypes.Tag{Key: tag.Key, Value: tag.Value})
		}
	}
	// Record before Update adds anything, so every tag the provider adds is recorded.
	s.cache.tagKeysChanged = tags.Record(cr, utils.AnnotationManagedTagKeys, specTagKeys(cr.Spec.ForProvider.Tags), sdkTagKeys(observedTags))
	s.cache.addTags, s.cache.removeTags = utils.DiffTags(cr.Spec.ForProvider.Tags, observedTags, tags.Removable(cr, utils.AnnotationManagedTagKeys))
	tagsChanged := len(s.cache.addTags) != 0 || len(s.cache.removeTags) != 0

	if diff == "" && !maintenanceWindowChanged && !backupWindowChanged && !backupRetentionPeriodChanged &&
		!iopsChanged && !storageThroughputChanged && !versionChanged && !vpcSGsChanged && !dbParameterGroupChanged &&
		!optionGroupChanged && !tagsChanged && !passwordChanged && !autoMinorVersionUpgradeChanged && !multiAZChanged &&
		!cloudwatchLogsExportsChanged && additionalStorageVolumesDiff == "" {
		return true, diff, nil
	}

	diff = "Found observed difference in dbinstance\n" + diff
	if maintenanceWindowChanged {
		diff += "\ndesired maintenanceWindow: "
		diff += pointer.StringValue(cr.Spec.ForProvider.PreferredMaintenanceWindow)
		diff += "\nobserved maintenanceWindow: "
		diff += pointer.StringValue(db.PreferredMaintenanceWindow)
	}
	if backupWindowChanged {
		diff += "\ndesired backupWindow: "
		diff += pointer.StringValue(cr.Spec.ForProvider.PreferredBackupWindow)
		diff += "\nobserved backupWindow: "
		diff += pointer.StringValue(db.PreferredBackupWindow)
	}
	if iopsChanged {
		diff += fmt.Sprintf("\ndesired iops: %d \nobserved iops: %d ", pointer.Int64Value(cr.Spec.ForProvider.IOPS), int64(pointer.Int32Value(db.Iops)))
	}
	if storageThroughputChanged {
		diff += fmt.Sprintf("\ndesired storageThroughput: %d \nobserved storageThroughput: %d ", pointer.Int64Value(cr.Spec.ForProvider.StorageThroughput), int64(pointer.Int32Value(db.StorageThroughput)))
	}
	if versionChanged {
		if ptr.Deref(cr.Spec.ForProvider.EngineVersion, "") == ptr.Deref(db.EngineVersion, "") && db.PendingModifiedValues != nil && ptr.Deref(db.PendingModifiedValues.EngineVersion, "") != "" {
			diff += fmt.Sprintf("\ndesired engineVersion: %s \npending modified engineVersion: %s ", pointer.StringValue(cr.Spec.ForProvider.EngineVersion), pointer.StringValue(db.PendingModifiedValues.EngineVersion))
		} else {
			diff += fmt.Sprintf("\ndesired engineVersion: %s \nobserved engineVersion: %s ", pointer.StringValue(cr.Spec.ForProvider.EngineVersion), pointer.StringValue(db.EngineVersion))
		}
	}
	if vpcSGsChanged {
		diff += fmt.Sprintf("\ndesired vpcSecurityGroupIDs: %v \nobserved vpcSecurityGroupIDs: ", cr.Spec.ForProvider.VPCSecurityGroupIDs)
		for _, grp := range db.VpcSecurityGroups {
			diff += fmt.Sprintf("\n - %s ", pointer.StringValue(grp.VpcSecurityGroupId))
		}
	}
	if dbParameterGroupChanged {
		diff += fmt.Sprintf("\ndesired dbParameterGroupName: %s \nobserved dbParameterGroupName: %s ", pointer.StringValue(cr.Spec.ForProvider.DBParameterGroupName), firstParameterGroup(db))
	}
	if optionGroupChanged {
		diff += fmt.Sprintf("\ndesired optionGroupName: %s \nobserved optionGroupName: %s ", pointer.StringValue(cr.Spec.ForProvider.OptionGroupName), firstOptionGroup(db))
	}
	if tagsChanged {
		diff += fmt.Sprintf("\nadd %d tag(s) and remove %d tag(s)", len(s.cache.addTags), len(s.cache.removeTags))
	}
	if passwordChanged {
		diff += "\nmaster user password changed"
	}
	if autoMinorVersionUpgradeChanged {
		diff += fmt.Sprintf("\ndesired autoMinorVersionUpgrade: %t \nobserved autoMinorVersionUpgrade: %t ", pointer.BoolValue(cr.Spec.ForProvider.AutoMinorVersionUpgrade), pointer.BoolValue(db.AutoMinorVersionUpgrade))
	}
	if multiAZChanged {
		diff += fmt.Sprintf("\ndesired multiAZ: %t \nobserved multiAZ: %t ", pointer.BoolValue(cr.Spec.ForProvider.MultiAZ), pointer.BoolValue(db.MultiAZ))
	}
	if cloudwatchLogsExportsChanged {
		diff += "\nenabledCloudwatchLogsExports changed"
	}
	diff += additionalStorageVolumesDiff

	return false, diff, nil
}

func firstParameterGroup(db *svcsdktypes.DBInstance) string {
	if len(db.DBParameterGroups) == 0 {
		return ""
	}
	return pointer.StringValue(db.DBParameterGroups[0].DBParameterGroupName)
}

func firstOptionGroup(db *svcsdktypes.DBInstance) string {
	if len(db.OptionGroupMemberships) == 0 {
		return ""
	}
	return pointer.StringValue(db.OptionGroupMemberships[0].OptionGroupName)
}

func isBackupRetentionPeriodUpToDate(cr *svcapitypes.DBInstance, out *svcsdktypes.DBInstance) bool {
	// 1. If BackupRetentionPeriod is not set, AWS sets a default value
	// so we do not try to update in this case
	// 2. AWS Backup takes ownership of BackupRetentionPeriod if it is in use.
	// So we only check BackupRetentionPeriod, if there is no associated RecoveryPoint.
	if cr.Spec.ForProvider.BackupRetentionPeriod != nil &&
		out.AwsBackupRecoveryPointArn == nil {
		if pointer.Int64Value(cr.Spec.ForProvider.BackupRetentionPeriod) != int64(pointer.Int32Value(out.BackupRetentionPeriod)) {
			return false
		}
	}
	return true
}

func newCustomExternal(kube client.Client, client Client) *customExternal {
	s := &shared{cache: &cache{}}
	e := external{
		kube:           kube,
		client:         client,
		preObserve:     preObserve,
		isUpToDate:     s.isUpToDate,
		postObserve:    s.postObserve,
		preUpdate:      s.preUpdate,
		postUpdate:     s.postUpdate,
		preCreate:      s.preCreate,
		postCreate:     s.postCreate,
		preDelete:      s.preDelete,
		postDelete:     s.postDelete,
		filterList:     filterList,
		lateInitialize: lateInitialize,
	}
	s.external = e
	return &customExternal{
		external: e,
		shared:   s,
	}
}

func (s *shared) postCreate(ctx context.Context, cr *svcapitypes.DBInstance, out *svcsdk.CreateDBInstanceOutput, extCreation managed.ExternalCreation, err error) (managed.ExternalCreation, error) {
	if err != nil {
		return managed.ExternalCreation{}, err
	}
	pw, err := dbinstance.AppliedPassword(ctx, s.kube, cr)
	if err != nil {
		return managed.ExternalCreation{}, errors.Wrap(err, dbinstance.ErrGetCachedPassword)
	}
	extCreation.ConnectionDetails = connectionDetails(cr, nil, pw, ptr.Deref(cr.Spec.ForProvider.ManageMasterUserPassword, false))
	// Create no longer writes spec, so take AWS's default username from the response.
	if cr.Spec.ForProvider.MasterUsername == nil && out != nil && out.DBInstance != nil && out.DBInstance.MasterUsername != nil {
		extCreation.ConnectionDetails[awsclient.ResourceCredentialsSecretUserKey] = []byte(*out.DBInstance.MasterUsername)
	}
	return extCreation, nil
}

func (s *shared) postDelete(ctx context.Context, cr *svcapitypes.DBInstance, obj *svcsdk.DeleteDBInstanceOutput, err error) (managed.ExternalDelete, error) {
	if err != nil {
		return managed.ExternalDelete{}, err
	}

	return managed.ExternalDelete{}, dbinstance.DeleteCache(ctx, s.kube, cr)
}

func (s *shared) postObserve(ctx context.Context, cr *svcapitypes.DBInstance, resp *svcsdk.DescribeDBInstancesOutput, obs managed.ExternalObservation, err error) (managed.ExternalObservation, error) { //nolint:gocyclo
	if err != nil {
		return obs, err
	}

	obs.ResourceLateInitialized = obs.ResourceLateInitialized || s.cache.tagKeysChanged
	s.cache.tagKeysChanged = false

	cr.Status.AtProvider.KMSKeyID = resp.DBInstances[0].KmsKeyId
	s.cache.deletionProtection = resp.DBInstances[0].DeletionProtection

	cr.Spec.ForProvider.DBClusterIdentifier = resp.DBInstances[0].DBClusterIdentifier

	switch pointer.StringValue(resp.DBInstances[0].DBInstanceStatus) {
	case "available", "configuring-enhanced-monitoring", "storage-optimization", "backing-up":
		cr.SetConditions(xpv2.Available())
	case "modifying":
		cr.SetConditions(xpv2.Available().WithMessage("DB Instance is " + pointer.StringValue(resp.DBInstances[0].DBInstanceStatus) + ", availability may vary"))
	case "deleting":
		cr.SetConditions(xpv2.Deleting())
	case statusCreating:
		cr.SetConditions(xpv2.Creating())
	default:
		cr.SetConditions(xpv2.Unavailable().WithMessage("DB Instance is " + pointer.StringValue(resp.DBInstances[0].DBInstanceStatus)))
	}

	db := resp.DBInstances[0]
	switch {
	case db.ReadReplicaDBClusterIdentifiers != nil || db.ReadReplicaDBInstanceIdentifiers != nil:
		cr.Status.AtProvider.DatabaseRole = aws.String(databaseRolePrimary)
	case db.ReadReplicaSourceDBClusterIdentifier != nil || db.ReadReplicaSourceDBInstanceIdentifier != nil:
		cr.Status.AtProvider.DatabaseRole = aws.String(databaseRoleReadReplica)
	case db.DBClusterIdentifier != nil:
		inputCluster := &svcsdk.DescribeDBClustersInput{DBClusterIdentifier: db.DBClusterIdentifier}
		outputCluster, err := s.client.DescribeDBClusters(ctx, inputCluster)
		if err != nil {
			return obs, awsclient.Wrap(err, errDescribeCluster)
		}
		// Status keeps last poll's role, so reset it before looking for this instance.
		cr.Status.AtProvider.DatabaseRole = nil
		if len(outputCluster.DBClusters) == 0 {
			break
		}
		for _, c := range outputCluster.DBClusters[0].DBClusterMembers {
			if pointer.StringValue(c.DBInstanceIdentifier) == pointer.StringValue(db.DBInstanceIdentifier) {
				if pointer.BoolValue(c.IsClusterWriter) {
					cr.Status.AtProvider.DatabaseRole = aws.String(databaseRoleClusterWriter)
				} else {
					cr.Status.AtProvider.DatabaseRole = aws.String(databaseRoleClusterReader)
				}
			}
		}
	default:
		cr.Status.AtProvider.DatabaseRole = aws.String(databaseRoleStandalone)
	}

	cr.Status.AtProvider.AvailabilityZone = db.AvailabilityZone

	// Publish only what AWS has; a rotated password follows after the modify.
	awsManaged := db.MasterUserSecret != nil
	pw := ""
	if !awsManaged {
		if pw, err = dbinstance.AppliedPassword(ctx, s.kube, cr); err != nil {
			return obs, errors.Wrap(err, dbinstance.ErrGetCachedPassword)
		}
	}
	obs.ConnectionDetails = connectionDetails(cr, obs.ConnectionDetails, pw, awsManaged)
	return obs, nil
}

func createPatch(out *svcsdk.DescribeDBInstancesOutput, target *svcapitypes.DBInstanceParameters) (*svcapitypes.DBInstanceParameters, error) {
	currentParams := &svcapitypes.DBInstanceParameters{}
	err := lateInitialize(currentParams, out)
	if err != nil {
		return nil, err
	}
	setObservedForComparison(currentParams, target, &out.DBInstances[0])
	normalizeObserved(currentParams, target)
	jsonPatch, err := jsonpatch.CreateJSONPatch(currentParams, target)
	if err != nil {
		return nil, err
	}
	patch := &svcapitypes.DBInstanceParameters{}
	if err := json.Unmarshal(jsonPatch, patch); err != nil {
		return nil, err
	}
	return patch, nil
}

func (c *customConnector) Connect(ctx context.Context, cr *svcapitypes.DBInstance) (managed.TypedExternalClient[*svcapitypes.DBInstance], error) {
	cfg, err := awsclient.GetConfig(ctx, c.kube, c.usage, cr, cr.Spec.ForProvider.Region)
	if err != nil {
		return nil, errors.Wrap(err, errCreateSession)
	}
	return newCustomExternal(c.kube, svcsdk.NewFromConfig(cfg)), nil
}

// configureDBInstance replaces the generated connector with one that wires
// the hand-written hooks of the DBInstance controller.
func configureDBInstance(mgr ctrl.Manager, _ controller.Options) ([]option, []managed.ReconcilerOption, error) {
	c := &customConnector{kube: mgr.GetClient(), usage: awsclient.NewUsageTracker(mgr.GetClient())}
	return nil, []managed.ReconcilerOption{managed.WithTypedExternalConnector[*svcapitypes.DBInstance](c)}, nil
}

func init() {
	configure = configureDBInstance
}
