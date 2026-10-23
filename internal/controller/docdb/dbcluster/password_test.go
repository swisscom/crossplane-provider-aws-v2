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
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/aws/aws-sdk-go-v2/service/docdb"
	docdbtypes "github.com/aws/aws-sdk-go-v2/service/docdb/types"
	"github.com/crossplane/crossplane-runtime/v2/pkg/errors"
	"github.com/crossplane/crossplane-runtime/v2/pkg/reconciler/managed"
	"github.com/crossplane/crossplane-runtime/v2/pkg/test"
	xpv2 "github.com/crossplane/crossplane/apis/v2/core/v2"
	"github.com/google/go-cmp/cmp"
	corev1 "k8s.io/api/core/v1"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"

	svcapitypes "github.com/swisscom/crossplane-provider-aws-v2/apis/docdb/v1alpha1"
	awsclient "github.com/swisscom/crossplane-provider-aws-v2/internal/clients/aws"
	fake "github.com/swisscom/crossplane-provider-aws-v2/internal/clients/docdb/fake"
	"github.com/swisscom/crossplane-provider-aws-v2/internal/utils/compare"
)

// updateClient answers Describe with an available DocumentDB cluster and
// accepts every Modify.
func updateClient() *fake.MockDocDBClient {
	cluster := docdbtypes.DBCluster{
		DBClusterIdentifier: ptr.To(testDBClusterIdentifier),
		DBClusterArn:        ptr.To(testDBClusterArn),
		Engine:              ptr.To(svcapitypes.EngineDocDB),
		Status:              ptr.To(svcapitypes.DocDBInstanceStateAvailable),
	}
	return &fake.MockDocDBClient{
		MockDescribeDBClusters: func(context.Context, *docdb.DescribeDBClustersInput, ...func(*docdb.Options)) (*docdb.DescribeDBClustersOutput, error) {
			return &docdb.DescribeDBClustersOutput{DBClusters: []docdbtypes.DBCluster{cluster}}, nil
		},
		MockModifyDBCluster: func(context.Context, *docdb.ModifyDBClusterInput, ...func(*docdb.Options)) (*docdb.ModifyDBClusterOutput, error) {
			return &docdb.ModifyDBClusterOutput{DBCluster: &cluster}, nil
		},
	}
}

// AWS defers a password change to the maintenance window without
// ApplyImmediately, while the connection secret would show it at once.
func TestUpdateAppliesPasswordImmediately(t *testing.T) {
	managedSecret := &svcapitypes.ClusterMasterUserSecret{SecretARN: ptr.To("arn")}
	cases := map[string]struct {
		published string
		apply     *bool
		manage    *bool
		observed  *svcapitypes.ClusterMasterUserSecret
		want      *bool
	}{
		"ChangedApplyUnset":     {published: "old", want: ptr.To(true)},
		"ChangedApplyFalse":     {published: "old", apply: ptr.To(false), want: ptr.To(true)},
		"UnchangedApplyUnset":   {published: testNewPassword},
		"UnchangedApplyFalse":   {published: testNewPassword, apply: ptr.To(false), want: ptr.To(false)},
		"ManageToggledOff":      {published: testNewPassword, apply: ptr.To(false), manage: ptr.To(false), observed: managedSecret, want: ptr.To(true)},
		"ManageToggledOnNoPass": {published: testNewPassword, apply: ptr.To(false), manage: ptr.To(true), want: ptr.To(false)},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			c := updateClient()
			e := newExternal(passwordKube(testNewPassword, tc.published), c, []option{setupExternal})
			cr := passwordCluster()
			cr.Spec.ForProvider.ApplyImmediately = tc.apply
			cr.Spec.ForProvider.ManageMasterUserPassword = tc.manage
			cr.Status.AtProvider.MasterUserSecret = tc.observed

			if _, err := e.Update(context.Background(), cr); err != nil {
				t.Fatalf("Update(...): %v", err)
			}

			if diff := cmp.Diff(tc.want, c.Called.ModifyDBCluster[0].I.ApplyImmediately); diff != "" {
				t.Errorf("ApplyImmediately: -want, +got:\n%s", diff)
			}
		})
	}
}

func TestUpdateModifyFailsRecordsNothing(t *testing.T) {
	store := secretStore{}
	store.put(testMasterPasswordSecretName, map[string][]byte{testMasterPasswordSecretKey: []byte(testNewPassword)})
	c := updateClient()
	c.MockModifyDBCluster = func(context.Context, *docdb.ModifyDBClusterInput, ...func(*docdb.Options)) (*docdb.ModifyDBClusterOutput, error) {
		return nil, errors.New("boom")
	}
	e := newExternal(store.kube(), c, []option{setupExternal})

	upd, err := e.Update(context.Background(), statefulCluster())

	if err == nil {
		t.Fatal("Update(...): want error")
	}
	if diff := cmp.Diff(managed.ExternalUpdate{}, upd); diff != "" {
		t.Errorf("Update(...): -want, +got:\n%s", diff)
	}
	if _, ok := store[client.ObjectKey{Namespace: testMasterPasswordSecretNamespace, Name: testStateName}]; ok {
		t.Error("Update(...) recorded a password that AWS rejected")
	}
}

// The password is in AWS after the first Modify; a failing engine upgrade
// must not make the provider send it again.
func TestUpdateEngineUpgradeFailsKeepsRecord(t *testing.T) {
	store := secretStore{}
	store.put(testMasterPasswordSecretName, map[string][]byte{testMasterPasswordSecretKey: []byte(testNewPassword)})
	c := updateClient()
	modify := c.MockModifyDBCluster
	c.MockModifyDBCluster = func(ctx context.Context, in *docdb.ModifyDBClusterInput, o ...func(*docdb.Options)) (*docdb.ModifyDBClusterOutput, error) {
		if in.EngineVersion != nil {
			return nil, errors.New("boom")
		}
		return modify(ctx, in, o...)
	}
	e := newExternal(store.kube(), c, []option{setupExternal})
	cr := statefulCluster(withEngineVersion("5.0.0"))

	if _, err := e.Update(context.Background(), cr); err == nil {
		t.Fatal("Update(...): want error")
	}

	state := store[client.ObjectKey{Namespace: testMasterPasswordSecretNamespace, Name: testStateName}]
	if state == nil || !bytes.Equal(state.Data[stateDigestKey], digest(testNewPassword)) {
		t.Fatal("Update(...) did not record the password AWS accepted")
	}
	obs, err := e.Observe(context.Background(), cr)
	if err != nil {
		t.Fatalf("Observe(...): %v", err)
	}
	if got := string(obs.ConnectionDetails[awsclient.ResourceCredentialsSecretPasswordKey]); got != testNewPassword {
		t.Errorf("Observe(...) published %q, want the applied password", got)
	}
	if obs.Diff != compare.Message([]string{"engineVersion"}) {
		t.Errorf("Observe(...) diff = %q, want only the engine version", obs.Diff)
	}
}

const (
	testUID         = types.UID("0b1e5a7c-uid")
	testNewPassword = "new"
)

// testStateName is the name of the password state Secret of a test cluster.
var testStateName = "docdb-dbcluster." + string(testUID) + ".password-state"

// secretStore is an in-memory Secret API: Get, Create and merge Patch.
type secretStore map[client.ObjectKey]*corev1.Secret

func (s secretStore) put(name string, data map[string][]byte) {
	key := client.ObjectKey{Namespace: testMasterPasswordSecretNamespace, Name: name}
	s[key] = &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: key.Namespace}, Data: data}
}

func (s secretStore) kube() *test.MockClient {
	return &test.MockClient{
		MockGet: func(_ context.Context, key client.ObjectKey, obj client.Object) error {
			sec, ok := s[key]
			if !ok {
				return kerrors.NewNotFound(schema.GroupResource{Resource: "secrets"}, key.Name)
			}
			sec.DeepCopyInto(obj.(*corev1.Secret))
			return nil
		},
		MockCreate: func(_ context.Context, obj client.Object, _ ...client.CreateOption) error {
			s[client.ObjectKeyFromObject(obj)] = obj.(*corev1.Secret).DeepCopy()
			return nil
		},
		MockPatch: func(_ context.Context, obj client.Object, p client.Patch, _ ...client.PatchOption) error {
			data, err := p.Data(obj)
			if err != nil {
				return err
			}
			cur := s[client.ObjectKeyFromObject(obj)]
			if err := json.Unmarshal(data, cur); err != nil {
				return err
			}
			cur.DeepCopyInto(obj.(*corev1.Secret))
			return nil
		},
	}
}

func digest(pw string) []byte {
	sum := sha256.Sum256([]byte(pw))
	return []byte(hex.EncodeToString(sum[:]))
}

// statefulCluster is a password cluster without connection secret.
func statefulCluster(m ...docDBModifier) *svcapitypes.DBCluster {
	cr := passwordCluster(m...)
	cr.SetUID(testUID)
	cr.SetName("orders")
	cr.Spec.WriteConnectionSecretToReference = nil
	return cr
}

func TestPasswordRotationWithoutConnectionSecret(t *testing.T) {
	store := secretStore{}
	store.put(testMasterPasswordSecretName, map[string][]byte{testMasterPasswordSecretKey: []byte("rotated")})
	store.put(testStateName, map[string][]byte{stateDigestKey: digest("old")})
	c := updateClient()
	e := newExternal(store.kube(), c, []option{setupExternal})
	cr := statefulCluster()

	obs, err := e.Observe(context.Background(), cr)
	if err != nil {
		t.Fatalf("Observe(...): %v", err)
	}
	if obs.ResourceUpToDate || obs.Diff != compare.Message([]string{"masterUserPasswordSecretRef"}) {
		t.Fatalf("Observe(...) = up to date %v, diff %q; want the password diff", obs.ResourceUpToDate, obs.Diff)
	}
	if _, ok := obs.ConnectionDetails[awsclient.ResourceCredentialsSecretPasswordKey]; ok {
		t.Errorf("Observe(...) published the password before AWS has it")
	}

	upd, err := e.Update(context.Background(), cr)
	if err != nil {
		t.Fatalf("Update(...): %v", err)
	}
	in := c.Called.ModifyDBCluster[0].I
	if ptr.Deref(in.MasterUserPassword, "") != "rotated" || !ptr.Deref(in.ApplyImmediately, false) {
		t.Errorf("ModifyDBCluster sent password %v, ApplyImmediately %v", in.MasterUserPassword != nil, in.ApplyImmediately)
	}
	if got := string(upd.ConnectionDetails[awsclient.ResourceCredentialsSecretPasswordKey]); got != "rotated" {
		t.Errorf("Update(...) published password %q, want the rotated one", got)
	}
	state := store[client.ObjectKey{Namespace: testMasterPasswordSecretNamespace, Name: testStateName}]
	if diff := cmp.Diff(digest("rotated"), state.Data[stateDigestKey]); diff != "" {
		t.Errorf("state digest: -want, +got:\n%s", diff)
	}
	wantOwner := []metav1.OwnerReference{{
		APIVersion: svcapitypes.GroupVersion.String(), Kind: svcapitypes.DBClusterKind, Name: "orders", UID: testUID,
		Controller: ptr.To(true), BlockOwnerDeletion: ptr.To(false),
	}}
	if diff := cmp.Diff(wantOwner, state.OwnerReferences); diff != "" {
		t.Errorf("state owner: -want, +got:\n%s", diff)
	}

	// The next Observe sees the password as applied.
	obs, err = e.Observe(context.Background(), cr)
	if err != nil || !obs.ResourceUpToDate {
		t.Errorf("Observe(...) after Update = up to date %v, diff %q, %v", obs.ResourceUpToDate, obs.Diff, err)
	}
}

func TestObservePasswordState(t *testing.T) {
	cases := map[string]struct {
		state        []byte
		connSecret   bool
		published    string
		wantUpToDate bool
	}{
		"RecordedAndEqual":           {state: digest("pw"), wantUpToDate: true},
		"RecordedAndDifferent":       {state: digest("old"), connSecret: true, published: "pw"},
		"MigrationConnSecretEqual":   {connSecret: true, published: "pw", wantUpToDate: true},
		"MigrationConnSecretOld":     {connSecret: true, published: "old"},
		"MigrationNoConnSecret":      {},
		"MigrationConnSecretMissing": {connSecret: true},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			store := secretStore{}
			store.put(testMasterPasswordSecretName, map[string][]byte{testMasterPasswordSecretKey: []byte("pw")})
			if tc.state != nil {
				store.put(testStateName, map[string][]byte{stateDigestKey: tc.state})
			}
			cr := statefulCluster()
			if tc.connSecret {
				cr.Spec.WriteConnectionSecretToReference = &xpv2.LocalSecretReference{Name: "conn"}
			}
			if tc.published != "" {
				store.put("conn", map[string][]byte{awsclient.ResourceCredentialsSecretPasswordKey: []byte(tc.published)})
			}
			c := updateClient()
			e := newExternal(store.kube(), c, []option{setupExternal})

			obs, err := e.Observe(context.Background(), cr)
			if err != nil {
				t.Fatalf("Observe(...): %v", err)
			}

			if obs.ResourceUpToDate != tc.wantUpToDate {
				t.Errorf("Observe(...) ResourceUpToDate = %v, want %v (diff %q)", obs.ResourceUpToDate, tc.wantUpToDate, obs.Diff)
			}
			_, published := obs.ConnectionDetails[awsclient.ResourceCredentialsSecretPasswordKey]
			if published != tc.wantUpToDate {
				t.Errorf("Observe(...) published the password = %v, want %v", published, tc.wantUpToDate)
			}
		})
	}
}

// A restored cluster keeps the snapshot's password, so the generated one must
// still be applied by the first Update.
func TestRestoreAppliesGeneratedPassword(t *testing.T) {
	store := secretStore{}
	c := updateClient()
	c.MockRestoreDBClusterFromSnapshot = func(context.Context, *docdb.RestoreDBClusterFromSnapshotInput, ...func(*docdb.Options)) (*docdb.RestoreDBClusterFromSnapshotOutput, error) {
		return &docdb.RestoreDBClusterFromSnapshotOutput{DBCluster: &docdbtypes.DBCluster{DBClusterIdentifier: ptr.To(testDBClusterIdentifier)}}, nil
	}
	ext := newExternal(store.kube(), c, []option{setupExternal})
	cr := statefulCluster(withRestoreFromSnapshot(&svcapitypes.RestoreSnapshotConfiguration{SnapshotIdentifier: "snap"}))
	cr.Spec.ForProvider.AutogeneratePassword = true

	if _, err := wrap(ext).Create(context.Background(), cr); err != nil {
		t.Fatalf("Create(...): %v", err)
	}
	ref := store[client.ObjectKey{Namespace: testMasterPasswordSecretNamespace, Name: testMasterPasswordSecretName}]
	generated := string(ref.Data[testMasterPasswordSecretKey])
	if generated == "" {
		t.Fatal("Create(...) saved no generated password")
	}
	if len(c.Called.RestoreDBClusterFromSnapshot) != 1 || len(c.Called.CreateDBCluster) != 0 {
		t.Errorf("Create(...) calls = %+v, want one restore", c.Called)
	}
	if _, ok := store[client.ObjectKey{Namespace: testMasterPasswordSecretNamespace, Name: testStateName}]; ok {
		t.Error("Create(...) recorded the password as applied, but the restored cluster has the snapshot's")
	}

	obs, err := ext.Observe(context.Background(), cr)
	if err != nil || obs.ResourceUpToDate {
		t.Fatalf("Observe(...) = up to date %v, %v; want the password diff", obs.ResourceUpToDate, err)
	}
	if _, err := ext.Update(context.Background(), cr); err != nil {
		t.Fatalf("Update(...): %v", err)
	}
	if got := ptr.Deref(c.Called.ModifyDBCluster[0].I.MasterUserPassword, ""); got != generated {
		t.Error("Update(...) did not send the generated password")
	}
}

func TestCreateWithGeneratedPassword(t *testing.T) {
	cases := map[string]struct {
		existing string
	}{
		"Generates":     {},
		"KeepsExisting": {existing: "chosen"},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			store := secretStore{}
			if tc.existing != "" {
				store.put(testMasterPasswordSecretName, map[string][]byte{testMasterPasswordSecretKey: []byte(tc.existing)})
			}
			kube := store.kube()
			writes := 0
			create, patch := kube.MockCreate, kube.MockPatch
			kube.MockCreate = func(ctx context.Context, obj client.Object, o ...client.CreateOption) error {
				if obj.GetName() == testMasterPasswordSecretName {
					writes++
				}
				return create(ctx, obj, o...)
			}
			kube.MockPatch = func(ctx context.Context, obj client.Object, p client.Patch, o ...client.PatchOption) error {
				if obj.GetName() == testMasterPasswordSecretName {
					writes++
				}
				return patch(ctx, obj, p, o...)
			}
			c := &fake.MockDocDBClient{}
			cr := statefulCluster()
			cr.Spec.ForProvider.AutogeneratePassword = true

			cre, err := wrap(newExternal(kube, c, []option{setupExternal})).Create(context.Background(), cr)
			if err != nil {
				t.Fatalf("Create(...): %v", err)
			}

			pw := string(store[client.ObjectKey{Namespace: testMasterPasswordSecretNamespace, Name: testMasterPasswordSecretName}].Data[testMasterPasswordSecretKey])
			if tc.existing != "" && (pw != tc.existing || writes != 0) {
				t.Errorf("Create(...) replaced the existing password (%d writes)", writes)
			}
			if tc.existing == "" && (pw == "" || writes != 1) {
				t.Errorf("Create(...) wrote the generated password %d times, want once", writes)
			}
			if got := ptr.Deref(c.Called.CreateDBCluster[0].I.MasterUserPassword, ""); got != pw {
				t.Error("CreateDBCluster did not carry the saved password")
			}
			if got := string(cre.ConnectionDetails[awsclient.ResourceCredentialsSecretPasswordKey]); got != pw {
				t.Error("Create(...) did not publish the saved password")
			}
			state := store[client.ObjectKey{Namespace: testMasterPasswordSecretNamespace, Name: testStateName}]
			if state == nil || !bytes.Equal(state.Data[stateDigestKey], digest(pw)) {
				t.Error("Create(...) did not record the applied password")
			}
		})
	}
}

func TestStateSecretOfAnotherOwner(t *testing.T) {
	store := secretStore{}
	store.put(testMasterPasswordSecretName, map[string][]byte{testMasterPasswordSecretKey: []byte(testNewPassword)})
	store.put(testStateName, map[string][]byte{stateDigestKey: digest(testNewPassword)})
	key := client.ObjectKey{Namespace: testMasterPasswordSecretNamespace, Name: testStateName}
	store[key].OwnerReferences = []metav1.OwnerReference{{Name: "intruder", UID: "other", Controller: ptr.To(true)}}
	e := newExternal(store.kube(), updateClient(), []option{setupExternal})
	cr := statefulCluster()
	want := `password state secret "` + testStateName + `" exists but is not controlled by this DBCluster`

	_, err := e.Observe(context.Background(), cr)
	if err == nil || !strings.HasSuffix(err.Error(), want) {
		t.Errorf("Observe(...) error = %v, want %q", err, want)
	}

	// Writing it must not take it over either.
	cr.Spec.ForProvider.ManageMasterUserPassword = ptr.To(false)
	cr.Status.AtProvider.MasterUserSecret = &svcapitypes.ClusterMasterUserSecret{SecretARN: ptr.To("arn")}
	_, err = e.Update(context.Background(), cr)
	if err == nil || !strings.HasSuffix(err.Error(), want) {
		t.Errorf("Update(...) error = %v, want %q", err, want)
	}
	if got := store[key].OwnerReferences[0].Name; got != "intruder" {
		t.Errorf("Update(...) changed the owner to %q", got)
	}
}

func TestPasswordNeverInErrorsOrDiff(t *testing.T) {
	const pw = "s3cr3t-Passw0rd"
	failWrites := func(k *test.MockClient) {
		k.MockCreate = func(context.Context, client.Object, ...client.CreateOption) error { return errors.New("denied") }
		k.MockPatch = func(context.Context, client.Object, client.Patch, ...client.PatchOption) error {
			return errors.New("denied")
		}
	}
	failStateRead := func(k *test.MockClient) {
		get := k.MockGet
		k.MockGet = func(ctx context.Context, key client.ObjectKey, obj client.Object) error {
			if key.Name == testStateName {
				return errors.New("denied")
			}
			return get(ctx, key, obj)
		}
	}
	for name, breakKube := range map[string]func(*test.MockClient){"WriteFails": failWrites, "StateReadFails": failStateRead} {
		t.Run(name, func(t *testing.T) {
			store := secretStore{}
			store.put(testMasterPasswordSecretName, map[string][]byte{testMasterPasswordSecretKey: []byte(pw)})
			kube := store.kube()
			breakKube(kube)
			e := newExternal(kube, updateClient(), []option{setupExternal})
			cr := statefulCluster()

			obs, oerr := e.Observe(context.Background(), cr)
			_, uerr := e.Update(context.Background(), cr)

			texts := make([]string, 0, 3+len(cr.Status.Conditions))
			texts = append(texts, obs.Diff, fmt.Sprint(oerr), fmt.Sprint(uerr))
			for _, c := range cr.Status.Conditions {
				texts = append(texts, c.Message)
			}
			for _, s := range texts {
				if strings.Contains(s, pw) {
					t.Errorf("password leaked into %q", s)
				}
			}
			if uerr == nil {
				t.Error("Update(...): want an error")
			}
		})
	}
}

// AWS owns the password while it manages it, and rejects MasterUserPassword.
func TestManagedPasswordIgnoresRef(t *testing.T) {
	store := secretStore{}
	store.put(testMasterPasswordSecretName, map[string][]byte{testMasterPasswordSecretKey: []byte("pw")})
	c := describeClusters(docdbtypes.DBCluster{
		DBClusterIdentifier: ptr.To(testDBClusterIdentifier),
		Engine:              ptr.To(svcapitypes.EngineDocDB),
		Status:              ptr.To(svcapitypes.DocDBInstanceStateAvailable),
		MasterUserSecret:    &docdbtypes.ClusterMasterUserSecret{SecretArn: ptr.To("arn")},
	})
	e := newExternal(store.kube(), c, []option{setupExternal})
	cr := statefulCluster()
	cr.Spec.ForProvider.ManageMasterUserPassword = ptr.To(true)

	obs, err := e.Observe(context.Background(), cr)

	if err != nil || !obs.ResourceUpToDate {
		t.Errorf("Observe(...) = up to date %v, diff %q, %v; want up to date", obs.ResourceUpToDate, obs.Diff, err)
	}
}

// AWS rejects MasterUserPassword while it manages the password, so another
// drift must not send the referenced one.
func TestManagedPasswordNotSentOnOtherDrift(t *testing.T) {
	store := secretStore{}
	store.put(testMasterPasswordSecretName, map[string][]byte{testMasterPasswordSecretKey: []byte("refpw")})
	store.put(testStateName, map[string][]byte{stateDigestKey: digest("old")})
	c := updateClient()
	e := newExternal(store.kube(), c, []option{setupExternal})
	cr := statefulCluster()
	cr.Spec.ForProvider.ManageMasterUserPassword = ptr.To(true)
	cr.Status.AtProvider.MasterUserSecret = &svcapitypes.ClusterMasterUserSecret{SecretARN: ptr.To("arn")}
	cr.Spec.ForProvider.BackupRetentionPeriod = ptr.To(int64(7))

	upd, err := e.Update(context.Background(), cr)
	if err != nil {
		t.Fatal(err)
	}

	if c.Called.ModifyDBCluster[0].I.MasterUserPassword != nil {
		t.Error("password sent while AWS manages it")
	}
	if _, ok := upd.ConnectionDetails[awsclient.ResourceCredentialsSecretPasswordKey]; ok {
		t.Error("unapplied password published")
	}
	state := store[client.ObjectKey{Namespace: testMasterPasswordSecretNamespace, Name: testStateName}]
	if !bytes.Equal(state.Data[stateDigestKey], digest("old")) {
		t.Error("Update(...) recorded a password it did not send")
	}
}

// Switching to a managed password sends no password, so none is published
// or recorded.
func TestToggleToManagedPublishesNoPassword(t *testing.T) {
	store := secretStore{}
	store.put(testMasterPasswordSecretName, map[string][]byte{testMasterPasswordSecretKey: []byte("refpw")})
	c := updateClient()
	e := newExternal(store.kube(), c, []option{setupExternal})
	cr := statefulCluster()
	cr.Spec.ForProvider.ManageMasterUserPassword = ptr.To(true)

	upd, err := e.Update(context.Background(), cr)
	if err != nil {
		t.Fatal(err)
	}

	in := c.Called.ModifyDBCluster[0].I
	if !ptr.Deref(in.ManageMasterUserPassword, false) || in.MasterUserPassword != nil {
		t.Errorf("ModifyDBCluster manage = %v, password sent = %v", in.ManageMasterUserPassword, in.MasterUserPassword != nil)
	}
	if _, ok := upd.ConnectionDetails[awsclient.ResourceCredentialsSecretPasswordKey]; ok {
		t.Error("Update(...) published a password AWS never received")
	}
	if _, ok := store[client.ObjectKey{Namespace: testMasterPasswordSecretNamespace, Name: testStateName}]; ok {
		t.Error("Update(...) recorded a password it did not send")
	}
}

// While AWS manages the password, the referenced one is not the cluster's.
func TestObserveManagedPasswordPublishesNoRef(t *testing.T) {
	store := secretStore{}
	store.put(testMasterPasswordSecretName, map[string][]byte{testMasterPasswordSecretKey: []byte("refpw")})
	store.put(testStateName, map[string][]byte{stateDigestKey: digest("refpw")})
	c := describeClusters(docdbtypes.DBCluster{
		DBClusterIdentifier: ptr.To(testDBClusterIdentifier),
		Engine:              ptr.To(svcapitypes.EngineDocDB),
		Status:              ptr.To(svcapitypes.DocDBInstanceStateAvailable),
		MasterUserSecret:    &docdbtypes.ClusterMasterUserSecret{SecretArn: ptr.To("arn")},
	})
	cr := statefulCluster()
	cr.Spec.ForProvider.ManageMasterUserPassword = ptr.To(true)

	obs, err := newExternal(store.kube(), c, []option{setupExternal}).Observe(context.Background(), cr)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := obs.ConnectionDetails[awsclient.ResourceCredentialsSecretPasswordKey]; ok {
		t.Error("Observe(...) published the referenced password while AWS manages it")
	}
}
