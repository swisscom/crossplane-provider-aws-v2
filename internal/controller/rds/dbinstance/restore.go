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

	svcsdk "github.com/aws/aws-sdk-go-v2/service/rds"
	svcsdktypes "github.com/aws/aws-sdk-go-v2/service/rds/types"
	"github.com/crossplane/crossplane-runtime/v2/pkg/errors"
	"github.com/crossplane/crossplane-runtime/v2/pkg/meta"
	"github.com/crossplane/crossplane-runtime/v2/pkg/reconciler/managed"
	xpv2 "github.com/crossplane/crossplane/apis/v2/core/v2"
	"k8s.io/utils/ptr"

	svcapitypes "github.com/swisscom/crossplane-provider-aws-v2/apis/rds/v1alpha1"
	awsclient "github.com/swisscom/crossplane-provider-aws-v2/internal/clients/aws"
	dbinstance "github.com/swisscom/crossplane-provider-aws-v2/internal/clients/rds"
)

const (
	restoreS3          = "S3"
	restoreSnapshot    = "Snapshot"
	restorePointInTime = "PointInTime"

	errNoRestoreSource      = "restoreFrom.source is not set"
	errRestoreSectionMissed = "restoreFrom.source %s needs restoreFrom.%s"
)

// restore creates the instance from a backup instead of CreateDBInstance.
// The password is cached before the AWS call so it can't get lost.
func (s *shared) restore(ctx context.Context, cr *svcapitypes.DBInstance) (managed.ExternalCreation, error) {
	cr.Status.SetConditions(xpv2.Creating())
	rf := cr.Spec.ForProvider.RestoreFrom
	if err := validateRestore(rf); err != nil {
		return managed.ExternalCreation{}, err
	}
	pw, err := s.sourcePassword(ctx, cr, true)
	if err != nil {
		return managed.ExternalCreation{}, err
	}
	if pw != "" {
		// A snapshot keeps its old password, so it must be applied after the
		// restore; an S3 restore sends it right away.
		state := string(dbinstance.RestoreStateRestored)
		if *rf.Source == restoreS3 {
			state = ""
		}
		if err := s.cachePassword(ctx, cr, pw, state); err != nil {
			return managed.ExternalCreation{}, err
		}
	}

	db, err := s.callRestore(ctx, cr, pw)
	if err != nil {
		return managed.ExternalCreation{}, err
	}
	if db != nil {
		observed := GenerateDBInstance(&svcsdk.DescribeDBInstancesOutput{DBInstances: []svcsdktypes.DBInstance{*db}}).Status.AtProvider
		observed.CustomDBInstanceObservation = cr.Status.AtProvider.CustomDBInstanceObservation
		observed.DeepCopyInto(&cr.Status.AtProvider)
	}
	applied, err := dbinstance.AppliedPassword(ctx, s.kube, cr)
	if err != nil {
		return managed.ExternalCreation{}, errors.Wrap(err, dbinstance.ErrGetCachedPassword)
	}
	return managed.ExternalCreation{
		ConnectionDetails: connectionDetails(cr, nil, applied, ptr.Deref(cr.Spec.ForProvider.ManageMasterUserPassword, false)),
	}, nil
}

func validateRestore(rf *svcapitypes.RestoreDBInstanceBackupConfiguration) error {
	if rf.Source == nil {
		return errors.New(errNoRestoreSource)
	}
	switch *rf.Source {
	case restoreS3:
		if rf.S3 == nil {
			return errors.Errorf(errRestoreSectionMissed, restoreS3, "s3")
		}
	case restoreSnapshot:
		if rf.Snapshot == nil {
			return errors.Errorf(errRestoreSectionMissed, restoreSnapshot, "snapshot")
		}
	case restorePointInTime:
		if rf.PointInTime == nil {
			return errors.Errorf(errRestoreSectionMissed, restorePointInTime, "pointInTime")
		}
	default:
		return errors.New(errUnknownRestoreSource)
	}
	return nil
}

// callRestore sends the restore call; a nil output is no error (status follows
// with the next observe).
func (s *shared) callRestore(ctx context.Context, cr *svcapitypes.DBInstance, pw string) (*svcsdktypes.DBInstance, error) {
	name := meta.GetExternalName(cr)
	p := &cr.Spec.ForProvider
	switch *p.RestoreFrom.Source {
	case restoreS3:
		out, err := s.client.RestoreDBInstanceFromS3(ctx, dbinstance.GenerateRestoreDBInstanceFromS3Input(name, pw, p))
		if err != nil || out == nil {
			return nil, awsclient.Wrap(err, errS3RestoreFailed)
		}
		return out.DBInstance, nil
	case restoreSnapshot:
		out, err := s.client.RestoreDBInstanceFromDBSnapshot(ctx, dbinstance.GenerateRestoreDBInstanceFromSnapshotInput(name, p))
		if err != nil || out == nil {
			return nil, awsclient.Wrap(err, errSnapshotRestoreFailed)
		}
		return out.DBInstance, nil
	default:
		out, err := s.client.RestoreDBInstanceToPointInTime(ctx, dbinstance.GenerateRestoreDBInstanceToPointInTimeInput(name, p))
		if err != nil || out == nil {
			return nil, awsclient.Wrap(err, errPointInTimeRestoreFailed)
		}
		return out.DBInstance, nil
	}
}
