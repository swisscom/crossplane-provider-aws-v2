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
	"context"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	svcsdk "github.com/aws/aws-sdk-go-v2/service/rds"
	svcsdktypes "github.com/aws/aws-sdk-go-v2/service/rds/types"
	"github.com/crossplane/crossplane-runtime/v2/pkg/test"
	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"

	svcapitypes "github.com/teeverr/provider-aws-v2/apis/rds/v1alpha1"
)

// observedDB is a DBInstance as returned by DescribeDBInstances with all the
// fields that are compared via setObservedForComparison.
func observedDB() svcsdktypes.DBInstance {
	return svcsdktypes.DBInstance{
		DBInstanceStatus:       aws.String("available"),
		DatabaseInsightsMode:   svcsdktypes.DatabaseInsightsModeStandard,
		EngineLifecycleSupport: aws.String("open-source-rds-extended-support-disabled"),
		NetworkType:            aws.String("IPV4"),
		MultiTenant:            aws.Bool(false),
		DedicatedLogVolume:     aws.Bool(false),
		CustomerOwnedIpEnabled: aws.Bool(false),
		TdeCredentialArn:       aws.String("arn:tde"),
		MasterUserSecret:       &svcsdktypes.MasterUserSecret{KmsKeyId: aws.String("arn:aws:kms:eu-central-1:123:key/abc")},
		DomainMemberships: []svcsdktypes.DomainMembership{
			{Domain: aws.String("d-old"), Status: aws.String("removing-membership")},
			{
				Domain:        aws.String("d-123"),
				Status:        aws.String("joined"),
				AuthSecretArn: aws.String("arn:secret"),
				DnsIps:        []string{"10.0.1.10", "10.0.0.10"},
				FQDN:          aws.String("corp.example.com"),
				IAMRoleName:   aws.String("rds-ad"),
				OU:            aws.String("OU=RDS"),
			},
		},
		AdditionalStorageVolumes: []svcsdktypes.AdditionalStorageVolumeOutput{{
			VolumeName:       aws.String("rdsdbdata2"),
			AllocatedStorage: aws.Int32(200),
			IOPS:             aws.Int32(3000),
			StorageType:      aws.String("gp3"),
		}},
	}
}

// desiredParams matches observedDB, including fields that are not returned by
// AWS (create-only or write-only); those must not cause a diff either.
func desiredParams() svcapitypes.DBInstanceParameters {
	return svcapitypes.DBInstanceParameters{
		DatabaseInsightsMode:     aws.String("standard"),
		EngineLifecycleSupport:   aws.String("open-source-rds-extended-support-disabled"),
		NetworkType:              aws.String("IPV4"),
		MultiTenant:              aws.Bool(false),
		DedicatedLogVolume:       aws.Bool(false),
		EnableCustomerOwnedIP:    aws.Bool(false),
		TDECredentialARN:         aws.String("arn:tde"),
		ManageMasterUserPassword: aws.Bool(true),
		MasterUserSecretKMSKeyID: aws.String("alias/other"),
		Domain:                   aws.String("d-123"),
		DomainAuthSecretARN:      aws.String("arn:secret"),
		DomainDNSIPs:             []*string{aws.String("10.0.0.10"), aws.String("10.0.1.10")},
		DomainFqdn:               aws.String("corp.example.com"),
		DomainIAMRoleName:        aws.String("rds-ad"),
		DomainOu:                 aws.String("OU=RDS"),
		AdditionalStorageVolumes: []*svcapitypes.AdditionalStorageVolume{{
			VolumeName:       aws.String("rdsdbdata2"),
			AllocatedStorage: aws.Int64(200),
			StorageType:      aws.String("gp3"),
		}},
		BackupTarget:                 aws.String("region"),
		CustomIAMInstanceProfile:     aws.String("profile"),
		DBSystemID:                   aws.String("ORCL"),
		NcharCharacterSetName:        aws.String("AL16UTF16"),
		TagSpecifications:            []*svcapitypes.TagSpecification{{ResourceType: aws.String("auto-backup")}},
		MasterUserAuthenticationType: aws.String("password"),
		TDECredentialPassword:        aws.String("secret"),
	}
}

func TestIsUpToDateObservedFields(t *testing.T) {
	cases := map[string]struct {
		spec     func(*svcapitypes.DBInstanceParameters)
		observed func(*svcsdktypes.DBInstance)
		upToDate bool
	}{
		"AllFieldsMatch":              {upToDate: true},
		"UnsetFieldsAreIgnored":       {spec: func(p *svcapitypes.DBInstanceParameters) { *p = svcapitypes.DBInstanceParameters{} }, upToDate: true},
		"DatabaseInsightsModeChanged": {spec: func(p *svcapitypes.DBInstanceParameters) { p.DatabaseInsightsMode = aws.String("advanced") }},
		"EngineLifecycleSupportChanged": {spec: func(p *svcapitypes.DBInstanceParameters) {
			p.EngineLifecycleSupport = aws.String("open-source-rds-extended-support")
		}},
		"NetworkTypeChanged":          {spec: func(p *svcapitypes.DBInstanceParameters) { p.NetworkType = aws.String("DUAL") }},
		"MultiTenantChanged":          {spec: func(p *svcapitypes.DBInstanceParameters) { p.MultiTenant = aws.Bool(true) }},
		"DedicatedLogVolumeChanged":   {spec: func(p *svcapitypes.DBInstanceParameters) { p.DedicatedLogVolume = aws.Bool(true) }},
		"CustomerOwnedIPChanged":      {spec: func(p *svcapitypes.DBInstanceParameters) { p.EnableCustomerOwnedIP = aws.Bool(true) }},
		"TDECredentialARNChanged":     {spec: func(p *svcapitypes.DBInstanceParameters) { p.TDECredentialARN = aws.String("arn:other") }},
		"ManageMasterUserPasswordOff": {spec: func(p *svcapitypes.DBInstanceParameters) { p.ManageMasterUserPassword = aws.Bool(false) }},
		"ManageMasterUserPasswordOn": {
			observed: func(db *svcsdktypes.DBInstance) { db.MasterUserSecret = nil },
		},
		"DomainChanged":        {spec: func(p *svcapitypes.DBInstanceParameters) { p.Domain = aws.String("d-456") }},
		"DomainDNSIPsChanged":  {spec: func(p *svcapitypes.DBInstanceParameters) { p.DomainDNSIPs = []*string{aws.String("10.0.0.10")} }},
		"DomainOuChanged":      {spec: func(p *svcapitypes.DBInstanceParameters) { p.DomainOu = aws.String("OU=Other") }},
		"DomainLeftByInstance": {observed: func(db *svcsdktypes.DBInstance) { db.DomainMemberships = nil }},
		"VolumeStorageChanged": {spec: func(p *svcapitypes.DBInstanceParameters) {
			p.AdditionalStorageVolumes[0].AllocatedStorage = aws.Int64(300)
		}},
		"VolumeTypeChanged": {spec: func(p *svcapitypes.DBInstanceParameters) {
			p.AdditionalStorageVolumes[0].StorageType = aws.String("io2")
		}},
		"VolumeMissingInAWS": {spec: func(p *svcapitypes.DBInstanceParameters) {
			p.AdditionalStorageVolumes[0].VolumeName = aws.String("rdsdbdata3")
		}},
		"ExtraVolumeInAWSIsKept": {spec: func(p *svcapitypes.DBInstanceParameters) { p.AdditionalStorageVolumes = nil }, upToDate: true},
		"PendingVolumeChangeIsUpToDate": {
			spec: func(p *svcapitypes.DBInstanceParameters) {
				p.AdditionalStorageVolumes[0].AllocatedStorage = aws.Int64(300)
			},
			observed: func(db *svcsdktypes.DBInstance) {
				db.PendingModifiedValues = &svcsdktypes.PendingModifiedValues{AdditionalStorageVolumes: []svcsdktypes.AdditionalStorageVolume{
					{VolumeName: aws.String("rdsdbdata2"), AllocatedStorage: aws.Int32(300)},
				}}
			},
			upToDate: true,
		},
		"PendingMultiTenantIsUpToDate": {
			spec: func(p *svcapitypes.DBInstanceParameters) { p.MultiTenant = aws.Bool(true) },
			observed: func(db *svcsdktypes.DBInstance) {
				db.PendingModifiedValues = &svcsdktypes.PendingModifiedValues{MultiTenant: aws.Bool(true)}
			},
			upToDate: true,
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			p := desiredParams()
			if tc.spec != nil {
				tc.spec(&p)
			}
			db := observedDB()
			if tc.observed != nil {
				tc.observed(&db)
			}
			cr := &svcapitypes.DBInstance{Spec: svcapitypes.DBInstanceSpec{ForProvider: p}}
			out := &svcsdk.DescribeDBInstancesOutput{DBInstances: []svcsdktypes.DBInstance{db}}
			upToDate, diff, err := newCustomExternal(test.NewMockClient(), nil).isUpToDate(context.TODO(), cr, out)
			if err != nil {
				t.Fatalf("isUpToDate: %v", err)
			}
			if upToDate != tc.upToDate {
				t.Errorf("upToDate: want %t, got %t\ndiff: %s", tc.upToDate, upToDate, diff)
			}
		})
	}
}

func TestPreUpdateManageMasterUserPassword(t *testing.T) {
	cases := map[string]struct {
		manage  *bool
		managed bool
		want    *svcsdk.ModifyDBInstanceInput
	}{
		"UnsetIsNotSent": {
			want: &svcsdk.ModifyDBInstanceInput{},
		},
		"UnchangedIsNotSent": {
			manage: aws.Bool(true), managed: true,
			want: &svcsdk.ModifyDBInstanceInput{},
		},
		"TurnOnSendsKMSKeyWithoutPassword": {
			manage: aws.Bool(true),
			want:   &svcsdk.ModifyDBInstanceInput{ManageMasterUserPassword: aws.Bool(true), MasterUserSecretKmsKeyId: aws.String("key")},
		},
		"TurnOffSendsPassword": {
			manage: aws.Bool(false), managed: true,
			want: &svcsdk.ModifyDBInstanceInput{ManageMasterUserPassword: aws.Bool(false), MasterUserPassword: aws.String("pw")},
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			cr := &svcapitypes.DBInstance{Spec: svcapitypes.DBInstanceSpec{ForProvider: svcapitypes.DBInstanceParameters{
				ManageMasterUserPassword: tc.manage,
			}}}
			// Simulate the generated input: spec fields are always copied.
			obj := &svcsdk.ModifyDBInstanceInput{
				ManageMasterUserPassword: tc.manage,
				MasterUserSecretKmsKeyId: aws.String("key"),
			}
			s := &shared{cache: &cache{masterUserPasswordManaged: tc.managed, desiredPassword: "pw"}}
			if err := s.preUpdate(context.TODO(), cr, obj); err != nil {
				t.Fatal(err)
			}
			opts := []cmp.Option{
				cmpopts.IgnoreUnexported(svcsdk.ModifyDBInstanceInput{}),
				cmpopts.IgnoreFields(svcsdk.ModifyDBInstanceInput{}, "DBPortNumber", "CloudwatchLogsExportConfiguration"),
			}
			if diff := cmp.Diff(tc.want, obj, opts...); diff != "" {
				t.Errorf("ModifyDBInstanceInput: -want, +got:\n%s", diff)
			}
		})
	}
}
