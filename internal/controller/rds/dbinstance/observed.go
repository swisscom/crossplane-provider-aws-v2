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

package dbinstance

import (
	"fmt"
	"slices"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	svcsdktypes "github.com/aws/aws-sdk-go-v2/service/rds/types"
	"k8s.io/utils/ptr"

	svcapitypes "github.com/swisscom/crossplane-provider-aws-v2/apis/rds/v1alpha1"
	"github.com/swisscom/crossplane-provider-aws-v2/internal/controller/rds/utils"
	"github.com/swisscom/crossplane-provider-aws-v2/internal/utils/compare"
	"github.com/swisscom/crossplane-provider-aws-v2/internal/utils/pointer"
)

// setObservedForComparison fills modifiable fields that DescribeDBInstances
// returns but that are not late-initialized into the spec. Late-initializing
// them would resend them with every ModifyDBInstance (e.g. multiTenant: false
// on engines that do not support it), so they are only filled into the
// observed side of the comparison in isUpToDate. Fields that are unset in the
// spec never cause a diff, because the patch is computed spec-relative.
func setObservedForComparison(current, target *svcapitypes.DBInstanceParameters, db *svcsdktypes.DBInstance) {
	if db.DatabaseInsightsMode != "" {
		current.DatabaseInsightsMode = aws.String(string(db.DatabaseInsightsMode))
	}
	current.EngineLifecycleSupport = db.EngineLifecycleSupport
	current.NetworkType = db.NetworkType
	current.MultiTenant = db.MultiTenant
	current.DedicatedLogVolume = db.DedicatedLogVolume
	current.EnableCustomerOwnedIP = db.CustomerOwnedIpEnabled
	current.TDECredentialARN = db.TdeCredentialArn

	// lateInitialize skips a MonitoringInterval of 0 (monitoring off) so that
	// it is not written into the spec. Compare with the value AWS reports,
	// unset meaning 0, so that a spec of 0 matches disabled monitoring.
	current.MonitoringInterval = aws.Int64(int64(pointer.Int32Value(db.MonitoringInterval)))
	// MasterUserSecretKMSKeyID is not compared: AWS does not allow changing it
	// once the password is managed in Secrets Manager.
	current.ManageMasterUserPassword = aws.Bool(db.MasterUserSecret != nil)

	if m := activeDomainMembership(db); m != nil {
		current.Domain = m.Domain
		current.DomainAuthSecretARN = m.AuthSecretArn
		current.DomainFqdn = m.FQDN
		current.DomainIAMRoleName = m.IAMRoleName
		current.DomainOu = m.OU
		// AWS may return the DNS IPs in a different order.
		if utils.AreSameElements(target.DomainDNSIPs, m.DnsIps) {
			current.DomainDNSIPs = target.DomainDNSIPs
		} else {
			current.DomainDNSIPs = aws.StringSlice(m.DnsIps)
		}
	}
}

// activeDomainMembership returns the domain membership of the instance that is
// not being removed. An instance can be joined to at most one domain.
func activeDomainMembership(db *svcsdktypes.DBInstance) *svcsdktypes.DomainMembership {
	for i := range db.DomainMemberships {
		switch pointer.StringValue(db.DomainMemberships[i].Status) {
		case "removing-membership", "removed":
			continue
		}
		return &db.DomainMemberships[i]
	}
	return nil
}

// diffAdditionalStorageVolumes compares the desired additional storage volumes
// with the observed ones by volume name. Only fields that are set in the spec
// are compared. Observed volumes that are missing in the spec are not
// reported: deleting a volume (and its data) requires an explicit change.
func diffAdditionalStorageVolumes(desired []*svcapitypes.AdditionalStorageVolume, observed []svcsdktypes.AdditionalStorageVolumeOutput) string {
	diff := ""
	for _, d := range desired {
		if d == nil {
			continue
		}
		name := pointer.StringValue(d.VolumeName)
		i := slices.IndexFunc(observed, func(o svcsdktypes.AdditionalStorageVolumeOutput) bool {
			return pointer.StringValue(o.VolumeName) == name
		})
		if i < 0 {
			diff += fmt.Sprintf("\nadditionalStorageVolume %q: not found", name)
			continue
		}
		o := observed[i]
		diff += diffInt(name, "allocatedStorage", d.AllocatedStorage, o.AllocatedStorage)
		diff += diffInt(name, "iops", d.IOPS, o.IOPS)
		diff += diffInt(name, "maxAllocatedStorage", d.MaxAllocatedStorage, o.MaxAllocatedStorage)
		diff += diffInt(name, "storageThroughput", d.StorageThroughput, o.StorageThroughput)
		if d.StorageType != nil && pointer.StringValue(d.StorageType) != pointer.StringValue(o.StorageType) {
			diff += fmt.Sprintf("\nadditionalStorageVolume %q: desired storageType: %s, observed: %s", name, *d.StorageType, pointer.StringValue(o.StorageType))
		}
	}
	return diff
}

func diffInt(volume, field string, desired *int64, observed *int32) string {
	if desired == nil || *desired == int64(pointer.Int32Value(observed)) {
		return ""
	}
	return fmt.Sprintf("\nadditionalStorageVolume %q: desired %s: %d, observed: %d", volume, field, *desired, pointer.Int32Value(observed))
}

// normalizeObserved counts AWS's form of a value as the user's: AWS returns
// some names in lowercase and KMS keys as key ARNs. Create no longer copies
// AWS's form into the spec, so without this every poll would send a modify.
func normalizeObserved(current, target *svcapitypes.DBInstanceParameters) {
	for _, f := range []struct{ observed, desired **string }{
		{&current.Engine, &target.Engine},
		{&current.DBInstanceClass, &target.DBInstanceClass},
		{&current.StorageType, &target.StorageType},
		{&current.LicenseModel, &target.LicenseModel},
		{&current.CACertificateIdentifier, &target.CACertificateIdentifier},
	} {
		if *f.desired != nil && compare.EqualIgnoreCase(*f.desired, *f.observed) {
			*f.observed = ptr.To(**f.desired)
		}
	}
	if kmsKeyMatches(target.KMSKeyID, current.KMSKeyID) {
		current.KMSKeyID = ptr.To(*target.KMSKeyID)
	}
	if kmsKeyMatches(target.PerformanceInsightsKMSKeyID, current.PerformanceInsightsKMSKeyID) {
		current.PerformanceInsightsKMSKeyID = ptr.To(*target.PerformanceInsightsKMSKeyID)
	}
}

// kmsKeyMatches reports whether the desired KMS key can be the observed one.
// An alias can't be resolved without a KMS call, so it matches any key; an
// alias ARN at least has to be in the key's partition, region and account.
func kmsKeyMatches(desired, observed *string) bool {
	if desired == nil || observed == nil {
		return false
	}
	d, o := *desired, *observed
	switch {
	case compare.KMSKeyIDEqual(d, o):
		return true
	case strings.HasPrefix(d, "alias/"):
		return true
	case strings.Contains(d, ":alias/"):
		return !isARN(o) || arnScope(d) == arnScope(o)
	}
	return false
}

func isARN(s string) bool { return strings.HasPrefix(s, "arn:") }

// arnScope is the "arn:partition:service:region:account" part of an ARN.
func arnScope(arn string) string {
	parts := strings.SplitN(arn, ":", 6)
	if len(parts) < 6 {
		return arn
	}
	return strings.Join(parts[:5], ":")
}
