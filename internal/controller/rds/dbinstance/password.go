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
	"maps"
	"strconv"

	"github.com/crossplane/crossplane-runtime/v2/pkg/errors"
	"github.com/crossplane/crossplane-runtime/v2/pkg/password"
	"github.com/crossplane/crossplane-runtime/v2/pkg/reconciler/managed"
	"k8s.io/utils/ptr"

	svcapitypes "github.com/swisscom/crossplane-provider-aws-v2/apis/rds/v1alpha1"
	awsclient "github.com/swisscom/crossplane-provider-aws-v2/internal/clients/aws"
	dbinstance "github.com/swisscom/crossplane-provider-aws-v2/internal/clients/rds"
	"github.com/swisscom/crossplane-provider-aws-v2/internal/utils/pointer"
)

// sourcePassword returns the master password for a new instance. "" means
// AWS manages it, or a restore keeps the snapshot's password.
func (s *shared) sourcePassword(ctx context.Context, cr *svcapitypes.DBInstance, isRestore bool) (string, error) {
	p := cr.Spec.ForProvider
	ref := p.MasterUserPasswordSecretRef
	if ptr.Deref(p.ManageMasterUserPassword, false) {
		// AWS rejects a password together with ManageMasterUserPassword.
		if ref != nil || p.AutogeneratePassword {
			return "", errors.New(dbinstance.ErrManagedPasswordConflict)
		}
		return "", nil
	}
	if ref != nil {
		pw, err := dbinstance.GetSecretValue(ctx, s.kube, cr.GetNamespace(), ref)
		if err != nil {
			return "", errors.Wrap(err, dbinstance.ErrNoRetrievePasswordOrGenerate)
		}
		if pw != "" {
			return pw, nil
		}
		if !p.AutogeneratePassword {
			return "", errors.New(dbinstance.ErrEmptyMasterPassword)
		}
	}
	if p.AutogeneratePassword {
		return s.generatedPassword(ctx, cr)
	}
	if isRestore {
		return "", nil
	}
	return "", errors.New(dbinstance.ErrNoMasterPasswordSource)
}

// generatedPassword returns the cached password, else a new one: a retry
// after a failed create must send the password that is already cached.
func (s *shared) generatedPassword(ctx context.Context, cr *svcapitypes.DBInstance) (string, error) {
	cached, err := dbinstance.GetCachedPassword(ctx, s.kube, cr)
	if err != nil {
		return "", errors.Wrap(err, dbinstance.ErrGetCachedPassword)
	}
	if cached != "" {
		return cached, nil
	}
	pw, err := password.Generate()
	return pw, errors.Wrap(err, dbinstance.ErrNoRetrievePasswordOrGenerate)
}

// cachePassword records pw as the password AWS has (or gets with the next
// call) and the restore state; an empty pw writes nothing.
func (s *shared) cachePassword(ctx context.Context, cr *svcapitypes.DBInstance, pw, restoreState string) error {
	if pw == "" {
		return nil
	}
	_, err := dbinstance.Cache(ctx, s.kube, cr, map[string]string{
		dbinstance.PasswordCacheKey:    pw,
		dbinstance.RestoreFlagCacheKay: restoreState,
	})
	return errors.Wrap(err, dbinstance.ErrCachePassword)
}

// connectionDetails returns base plus username, password, endpoint and port.
// An empty password is left out so an applied one in the connection Secret
// stays; an AWS-managed password is published empty to clear a stale one.
func connectionDetails(cr *svcapitypes.DBInstance, base managed.ConnectionDetails, pw string, awsManaged bool) managed.ConnectionDetails {
	cd := managed.ConnectionDetails{}
	maps.Copy(cd, base)
	cd[awsclient.ResourceCredentialsSecretUserKey] = []byte(pointer.StringValue(cr.Spec.ForProvider.MasterUsername))
	switch {
	case awsManaged:
		cd[awsclient.ResourceCredentialsSecretPasswordKey] = []byte{}
	case pw != "":
		cd[awsclient.ResourceCredentialsSecretPasswordKey] = []byte(pw)
	}
	ep := cr.Status.AtProvider.Endpoint
	if ep == nil {
		return cd
	}
	if addr := pointer.StringValue(ep.Address); addr != "" {
		cd[awsclient.ResourceCredentialsSecretEndpointKey] = []byte(addr)
	}
	if port := pointer.Int64Value(ep.Port); port > 0 {
		cd[awsclient.ResourceCredentialsSecretPortKey] = []byte(strconv.FormatInt(port, 10))
	}
	return cd
}

// isReadReplica reports whether cr is created as a read replica.
func isReadReplica(cr *svcapitypes.DBInstance) bool {
	p := cr.Spec.ForProvider
	return p.SourceDBInstanceID != nil || p.SourceDBInstanceIDRef != nil ||
		p.SourceDBInstanceIDSelector != nil || p.SourceDBClusterID != nil
}

// passwordForManagedOff returns the new master password when Secrets Manager
// management is turned off: the ref's value, else a newly generated one.
func (s *shared) passwordForManagedOff(ctx context.Context, cr *svcapitypes.DBInstance) (string, error) {
	if ref := cr.Spec.ForProvider.MasterUserPasswordSecretRef; ref != nil {
		pw, err := dbinstance.GetSecretValue(ctx, s.kube, cr.GetNamespace(), ref)
		if err != nil {
			return "", errors.Wrap(err, dbinstance.ErrRetrievePasswordForUpdate)
		}
		if pw != "" {
			return pw, nil
		}
	}
	if !cr.Spec.ForProvider.AutogeneratePassword {
		return "", errors.New(dbinstance.ErrManagedOffNeedsPassword)
	}
	// A retry must re-send the password a previous attempt recorded as pending,
	// never the stale one from before management was turned on.
	upToDate, pending, err := dbinstance.PasswordUpToDate(ctx, s.kube, cr)
	if err != nil {
		return "", errors.Wrap(err, dbinstance.ErrNoPasswordUpToDate)
	}
	if !upToDate && pending != "" {
		return pending, nil
	}
	pw, err := password.Generate()
	return pw, errors.Wrap(err, dbinstance.ErrNoRetrievePasswordOrGenerate)
}

// tdePassword returns the Oracle TDE credential password from its Secret, nil
// without a ref. The error never contains the value.
func (s *shared) tdePassword(ctx context.Context, cr *svcapitypes.DBInstance) (*string, error) {
	ref := cr.Spec.ForProvider.TDECredentialPasswordSecretRef
	if ref == nil {
		return nil, nil
	}
	pw, err := dbinstance.GetSecretValue(ctx, s.kube, cr.GetNamespace(), ref)
	if err != nil {
		return nil, errors.Wrap(err, errTDEPassword)
	}
	if pw == "" {
		return nil, errors.Wrap(errors.New("secret or key not found, or empty"), errTDEPassword)
	}
	return &pw, nil
}
