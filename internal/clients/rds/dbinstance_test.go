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
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/swisscom/crossplane-provider-aws-v2/apis/rds/v1alpha1"
)

func restoreParams() *v1alpha1.DBInstanceParameters {
	when := metav1.NewTime(time.Date(2026, 1, 2, 3, 4, 5, 0, time.FixedZone("CET", 3600)))
	p := &v1alpha1.DBInstanceParameters{
		Engine:                   aws.String("mysql"),
		AllocatedStorage:         aws.Int64(20),
		Port:                     aws.Int64(3306),
		ManageMasterUserPassword: aws.Bool(true),
		MasterUserSecretKMSKeyID: aws.String("alias/secrets"),
		Tags:                     []*v1alpha1.Tag{{Key: aws.String("k"), Value: aws.String("v")}, nil},
		ProcessorFeatures:        []*v1alpha1.ProcessorFeature{{Name: aws.String("coreCount"), Value: aws.String("2")}, nil},
	}
	p.RestoreFrom = &v1alpha1.RestoreDBInstanceBackupConfiguration{
		S3:          &v1alpha1.S3RestoreBackupConfiguration{BucketName: aws.String("bucket"), SourceEngine: aws.String("mysql")},
		Snapshot:    &v1alpha1.SnapshotRestoreBackupConfiguration{SnapshotIdentifier: aws.String("snap")},
		PointInTime: &v1alpha1.PointInTimeRestoreBackupConfiguration{RestoreTime: &when, SourceDBInstanceIdentifier: aws.String("src")},
	}
	return p
}

func TestRestoreInputs(t *testing.T) {
	p := restoreParams()

	s3 := GenerateRestoreDBInstanceFromS3Input("db", "pw", p)
	if aws.ToString(s3.DBInstanceIdentifier) != "db" || aws.ToString(s3.MasterUserPassword) != "pw" || aws.ToString(s3.S3BucketName) != "bucket" {
		t.Errorf("S3 input = %+v", s3)
	}
	if !aws.ToBool(s3.ManageMasterUserPassword) || aws.ToString(s3.MasterUserSecretKmsKeyId) != "alias/secrets" {
		t.Error("S3 input must carry the managed-password settings")
	}
	if len(s3.Tags) != 1 || len(s3.ProcessorFeatures) != 1 || aws.ToInt32(s3.AllocatedStorage) != 20 {
		t.Errorf("S3 input tags %v, features %v, storage %v", s3.Tags, s3.ProcessorFeatures, s3.AllocatedStorage)
	}
	if GenerateRestoreDBInstanceFromS3Input("db", "", p).MasterUserPassword != nil {
		t.Error("an empty password must not be sent")
	}

	snap := GenerateRestoreDBInstanceFromSnapshotInput("db", p)
	if aws.ToString(snap.DBSnapshotIdentifier) != "snap" || !aws.ToBool(snap.ManageMasterUserPassword) || snap.TdeCredentialPassword != nil {
		t.Errorf("snapshot input = %+v", snap)
	}

	pitr := GenerateRestoreDBInstanceToPointInTimeInput("db", p)
	if aws.ToString(pitr.TargetDBInstanceIdentifier) != "db" || aws.ToString(pitr.SourceDBInstanceIdentifier) != "src" || !aws.ToBool(pitr.ManageMasterUserPassword) {
		t.Errorf("point-in-time input = %+v", pitr)
	}
	if pitr.RestoreTime == nil || pitr.RestoreTime.Location() != time.UTC || pitr.RestoreTime.Hour() != 2 {
		t.Errorf("restore time = %v, want it in UTC", pitr.RestoreTime)
	}
	p.RestoreFrom.PointInTime.RestoreTime = nil
	if GenerateRestoreDBInstanceToPointInTimeInput("db", p).RestoreTime != nil {
		t.Error("no restore time must stay nil")
	}
}

func TestEmptyListsConvertToNil(t *testing.T) {
	p := &v1alpha1.DBInstanceParameters{}
	if Tags(p) != nil || ProcessorFeatures(p) != nil {
		t.Error("empty tags and processor features must convert to nil")
	}
}

func TestReadReplicaInput(t *testing.T) {
	cr := &v1alpha1.DBInstance{}
	cr.Spec.ForProvider.SourceDBInstanceID = aws.String("primary")
	cr.Spec.ForProvider.Port = aws.Int64(5432)
	in := GenerateCreateDBInstanceReadReplicaInput(cr)
	if aws.ToString(in.SourceDBInstanceIdentifier) != "primary" || aws.ToInt32(in.Port) != 5432 {
		t.Errorf("read replica input = %+v", in)
	}
}
