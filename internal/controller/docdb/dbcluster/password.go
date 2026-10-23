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

package dbcluster

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"

	"github.com/crossplane/crossplane-runtime/v2/pkg/errors"
	"github.com/crossplane/crossplane-runtime/v2/pkg/meta"
	"github.com/crossplane/crossplane-runtime/v2/pkg/password"
	cpresource "github.com/crossplane/crossplane-runtime/v2/pkg/resource"
	corev1 "k8s.io/api/core/v1"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"

	svcapitypes "github.com/swisscom/crossplane-provider-aws-v2/apis/docdb/v1alpha1"
	awsclient "github.com/swisscom/crossplane-provider-aws-v2/internal/clients/aws"
)

const (
	errGetPasswordSecretFailed = "cannot get password secret"
	errSaveSecretFailed        = "failed to save generated password to Kubernetes secret"
	errGetStateSecret          = "cannot get password state secret"
	errGetConnectionSecret     = "cannot get connection secret"
	errRecordPassword          = "cannot record the applied master password"
	errStateNotControlled      = "password state secret %q exists but is not controlled by this DBCluster"

	stateDigestKey = "sha256"
)

// stateSecretName has a fixed length and contains the UID, so it fits the
// Secret name limit and cannot collide with a user's Secret.
func stateSecretName(cr *svcapitypes.DBCluster) string {
	return "docdb-dbcluster." + string(cr.GetUID()) + ".password-state"
}

func hexDigest(pw string) []byte {
	sum := sha256.Sum256([]byte(pw))
	return []byte(hex.EncodeToString(sum[:]))
}

// refPassword returns the password of masterUserPasswordSecretRef, which is
// always in the namespace of cr.
func (e *hooks) refPassword(ctx context.Context, cr *svcapitypes.DBCluster) (string, error) {
	ref := cr.Spec.ForProvider.MasterUserPasswordSecretRef
	if ref == nil {
		return "", nil
	}
	s := &corev1.Secret{}
	if err := e.kube.Get(ctx, types.NamespacedName{Name: ref.Name, Namespace: cr.GetNamespace()}, s); err != nil {
		return "", errors.Wrap(err, errGetPasswordSecretFailed)
	}
	return string(s.Data[ref.Key]), nil
}

// desiredPassword returns the referenced password and whether AWS does not
// have it yet.
func (e *hooks) desiredPassword(ctx context.Context, cr *svcapitypes.DBCluster) (pw string, changed bool, err error) {
	pw, err = e.refPassword(ctx, cr)
	if err != nil || pw == "" {
		return pw, false, err
	}
	applied, err := e.isApplied(ctx, cr, pw)
	if err != nil {
		return "", false, err
	}
	return pw, !applied, nil
}

// isApplied reports whether pw is the password the provider last applied.
func (e *hooks) isApplied(ctx context.Context, cr *svcapitypes.DBCluster, pw string) (bool, error) {
	state := &corev1.Secret{}
	err := e.kube.Get(ctx, types.NamespacedName{Name: stateSecretName(cr), Namespace: cr.GetNamespace()}, state)
	switch {
	case kerrors.IsNotFound(err):
		// Nothing recorded yet, e.g. an MR from before the state Secret.
		return e.isPublished(ctx, cr, pw)
	case err != nil:
		return false, errors.Wrap(err, errGetStateSecret)
	}
	if c := metav1.GetControllerOf(state); c != nil && c.UID != cr.GetUID() {
		return false, errors.Errorf(errStateNotControlled, state.GetName())
	}
	return subtle.ConstantTimeCompare(state.Data[stateDigestKey], hexDigest(pw)) == 1, nil
}

// isPublished reports whether the connection secret already has pw, which
// it only gets after AWS accepted it.
func (e *hooks) isPublished(ctx context.Context, cr *svcapitypes.DBCluster, pw string) (bool, error) {
	out := cr.Spec.WriteConnectionSecretToReference
	if out == nil {
		return false, nil
	}
	s := &corev1.Secret{}
	if err := e.kube.Get(ctx, types.NamespacedName{Name: out.Name, Namespace: cr.GetNamespace()}, s); cpresource.IgnoreNotFound(err) != nil {
		return false, errors.Wrap(err, errGetConnectionSecret)
	}
	return string(s.Data[awsclient.ResourceCredentialsSecretPasswordKey]) == pw, nil
}

// recordAppliedPassword stores the digest of pw, never pw itself, in a
// Secret that is garbage collected with cr.
func (e *hooks) recordAppliedPassword(ctx context.Context, cr *svcapitypes.DBCluster, pw string) error {
	owner := meta.AsController(meta.TypedReferenceTo(cr, svcapitypes.DBClusterGroupVersionKind))
	// Blocking owner deletion would need RBAC on DBCluster finalizers.
	owner.BlockOwnerDeletion = ptr.To(false)
	s := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:            stateSecretName(cr),
			Namespace:       cr.GetNamespace(),
			OwnerReferences: []metav1.OwnerReference{owner},
		},
		Data: map[string][]byte{stateDigestKey: hexDigest(pw)},
	}
	err := cpresource.NewAPIPatchingApplicator(e.kube).Apply(ctx, s, cpresource.MustBeControllableBy(cr.GetUID()))
	if cpresource.IsNotControllable(err) {
		return errors.Errorf(errStateNotControlled, s.GetName())
	}
	return errors.Wrap(err, errRecordPassword)
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

// masterPassword returns the password of masterUserPasswordSecretRef. With
// autogeneratePassword and no password yet, it generates one and saves it
// to that secret.
func (e *hooks) masterPassword(ctx context.Context, cr *svcapitypes.DBCluster) (string, error) {
	pw, err := e.refPassword(ctx, cr)
	if cpresource.IgnoreNotFound(err) != nil {
		return "", err
	}
	if pw != "" || !cr.Spec.ForProvider.AutogeneratePassword {
		return pw, nil
	}
	pw, err = password.Generate()
	if err != nil {
		return "", errors.Wrap(err, "unable to generate a password")
	}
	if err := e.savePasswordSecret(ctx, cr, pw); err != nil {
		return "", errors.Wrap(err, errSaveSecretFailed)
	}
	return pw, nil
}
