/*
Copyright 2019 The Crossplane Authors.

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
	"strings"

	"github.com/crossplane/crossplane-runtime/v2/pkg/errors"
	"github.com/crossplane/crossplane-runtime/v2/pkg/resource"
	xpv2 "github.com/crossplane/crossplane/apis/v2/core/v2"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"

	svcapitypes "github.com/swisscom/crossplane-provider-aws-v2/apis/rds/v1alpha1"
)

// Publicly usable variables
const (
	PasswordCacheKey    = "dbMasterUserPassword"
	RestoreFlagCacheKay = "dbRestoreState"

	// RestoreStateRestored marks the cached password as not applied yet: after
	// a snapshot restore, or while a modify that sends it is unconfirmed.
	RestoreStateRestored restoreSate = "RestoreStateRestored"
	RestoreStateNormal   restoreSate = "RestoreStateNormal"

	ErrNoRetrievePasswordOrGenerate = "cannot retrieve password form masterUserPasswordSecretRef or generate a password"
	ErrNoMasterPasswordSource       = "no master password: set masterUserPasswordSecretRef, autogeneratePassword or manageMasterUserPassword"
	ErrManagedPasswordConflict      = "manageMasterUserPassword cannot be combined with masterUserPasswordSecretRef or autogeneratePassword"
	ErrEmptyMasterPassword          = "masterUserPasswordSecretRef points to a missing secret or an empty value"
	ErrManagedOffNeedsPassword      = "cannot turn off manageMasterUserPassword: set masterUserPasswordSecretRef or autogeneratePassword for the new master password"
	ErrCachePassword                = "cannot cache password"
	ErrNoPasswordUpToDate           = "cannot determine password up to date status"
	ErrGetCachedPassword            = "cannot get cached password"
	ErrRetrievePasswordForUpdate    = "cannot retrieve password for update"
	ErrDescribe                     = "cannot describe dbinstance"
)

const (
	errGetSecret            = "cannot get secret"
	errDeleteSecretF        = "cannot delete secret %q in namespace %q"
	errGetCachedPassword    = "cannot get cached password"
	errGetCachedRestoreInfo = "cannot get cached restore info"
	errGetMasterPassword    = "cannot get master password"
)

type restoreSate string

// cacheSecretKey returns the key of the secret that caches the password and
// restore state of mg. It lives in the namespace of mg.
func cacheSecretKey(mg resource.Managed) types.NamespacedName {
	name := strings.ToLower(mg.GetObjectKind().GroupVersionKind().Kind + "." + string(mg.GetUID()))
	return types.NamespacedName{Namespace: mg.GetNamespace(), Name: name}
}

// DeleteCache removes the (secret) cache
func DeleteCache(ctx context.Context, kube client.Client, mg resource.Managed) error {
	key := cacheSecretKey(mg)
	secret := &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: key.Name, Namespace: key.Namespace}}
	if err := kube.Delete(ctx, secret); resource.IgnoreNotFound(err) != nil {
		return errors.Wrapf(err, errDeleteSecretF, key.Name, key.Namespace)
	}
	return nil
}

// GetSecretValue fetches the value of the referenced key of a secret in the
// given namespace. A missing secret yields an empty value.
func GetSecretValue(ctx context.Context, kube client.Client, namespace string, ref *xpv2.LocalSecretKeySelector) (string, error) {
	return getSecretValue(ctx, kube, types.NamespacedName{Namespace: namespace, Name: ref.Name}, ref.Key)
}

func getSecretValue(ctx context.Context, kube client.Client, key types.NamespacedName, dataKey string) (string, error) {
	secret := &corev1.Secret{}
	if err := kube.Get(ctx, key, secret); err != nil {
		return "", errors.Wrap(resource.IgnoreNotFound(err), errGetSecret)
	}
	return string(secret.Data[dataKey]), nil
}

func getCachedRestoreInfo(ctx context.Context, kube client.Client, mg resource.Managed) (restoreSate, error) {
	restoreInfo, err := getSecretValue(ctx, kube, cacheSecretKey(mg), RestoreFlagCacheKay)
	if restoreInfo == string(RestoreStateRestored) {
		return RestoreStateRestored, err
	}
	return RestoreStateNormal, err
}

// GetCachedPassword returns the cached password of mg.
func GetCachedPassword(ctx context.Context, kube client.Client, mg resource.Managed) (string, error) {
	return getSecretValue(ctx, kube, cacheSecretKey(mg), PasswordCacheKey)
}

// AppliedPassword returns the master password AWS is known to have: the
// cached one, or "" while a restore has not applied it yet.
func AppliedPassword(ctx context.Context, kube client.Client, mg resource.Managed) (string, error) {
	restoreInfo, err := getCachedRestoreInfo(ctx, kube, mg)
	if err != nil {
		return "", errors.Wrap(err, errGetCachedRestoreInfo)
	}
	if restoreInfo == RestoreStateRestored {
		return "", nil
	}
	pw, err := GetCachedPassword(ctx, kube, mg)
	return pw, errors.Wrap(err, errGetCachedPassword)
}

// Cache stores kv in the cache Secret of mg. An empty password never replaces
// a stored one. The Secret is owned by mg, so it is garbage collected with it.
func Cache(ctx context.Context, kube client.Client, mg resource.Managed, kv map[string]string) (*corev1.Secret, error) {
	key := cacheSecretKey(mg)
	data := make(map[string][]byte, len(kv))
	for k, v := range kv {
		// The stored password may be the only copy of what AWS has.
		if k == PasswordCacheKey && v == "" {
			continue
		}
		data[k] = []byte(v)
	}
	sc := &corev1.Secret{
		Data:       data,
		ObjectMeta: metav1.ObjectMeta{Name: key.Name, Namespace: key.Namespace, OwnerReferences: ownerReferences(mg)},
	}
	err := resource.NewAPIPatchingApplicator(kube).Apply(ctx, sc)
	return sc, err
}

// ownerReferences makes mg the controller of its cache Secret. It doesn't
// block owner deletion, so the provider needs no permission on finalizers.
func ownerReferences(mg resource.Managed) []metav1.OwnerReference {
	gvk := mg.GetObjectKind().GroupVersionKind()
	if gvk.Kind == "" || mg.GetUID() == "" {
		return nil
	}
	return []metav1.OwnerReference{{
		APIVersion:         gvk.GroupVersion().String(),
		Kind:               gvk.Kind,
		Name:               mg.GetName(),
		UID:                mg.GetUID(),
		Controller:         ptr.To(true),
		BlockOwnerDeletion: ptr.To(false),
	}}
}

func masterPassword(ctx context.Context, kube client.Client, cr svcapitypes.RDSClusterOrInstance) (string, error) {
	ref := cr.GetMasterUserPasswordSecretRef()
	if ref == nil {
		return "", nil
	}
	pw, err := GetSecretValue(ctx, kube, cr.GetNamespace(), ref)
	return pw, errors.Wrap(err, errGetMasterPassword)
}

// PasswordUpToDate reports whether AWS has the desired master password. The
// desired password is the value of masterUserPasswordSecretRef, else the
// cached one: after a restore the cached password still has to be applied.
func PasswordUpToDate(ctx context.Context, kube client.Client, cr svcapitypes.RDSClusterOrInstance) (upToDate bool, desiredPassword string, err error) {
	restoreInfo, err := getCachedRestoreInfo(ctx, kube, cr)
	if err != nil {
		return false, "", errors.Wrap(err, errGetCachedRestoreInfo)
	}
	cachedPassword, err := GetCachedPassword(ctx, kube, cr)
	if err != nil {
		return false, "", errors.Wrap(err, errGetCachedPassword)
	}
	fromSecret, err := masterPassword(ctx, kube, cr)
	if err != nil {
		return false, "", err
	}

	desiredPassword = fromSecret
	if desiredPassword == "" {
		desiredPassword = cachedPassword
	}
	secretChanged := fromSecret != "" && fromSecret != cachedPassword
	restorePending := restoreInfo == RestoreStateRestored && desiredPassword != ""
	return !secretChanged && !restorePending, desiredPassword, nil
}
