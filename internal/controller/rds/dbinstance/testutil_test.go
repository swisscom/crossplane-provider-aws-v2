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
	"sync"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	svcsdk "github.com/aws/aws-sdk-go-v2/service/rds"
	svcsdktypes "github.com/aws/aws-sdk-go-v2/service/rds/types"
	"github.com/crossplane/crossplane-runtime/v2/pkg/meta"
	xpv2 "github.com/crossplane/crossplane/apis/v2/core/v2"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	cfake "sigs.k8s.io/controller-runtime/pkg/client/fake"

	svcapitypes "github.com/swisscom/crossplane-provider-aws-v2/apis/rds/v1alpha1"
	rds "github.com/swisscom/crossplane-provider-aws-v2/internal/clients/rds"
)

const (
	testNS       = "ns"
	testName     = "db1"
	testARN      = "arn:aws:rds:eu-central-1:123456789012:db:db1"
	testUser     = "admin"
	testPWSecret = "pw"
	testPWKey    = "password"

	oldValue    = "old"
	oldPassword = "old-password"
	newPassword = "new-password"
	refPassword = "from-ref"
	sgA         = "sg-a"
)

// testKube is a controller-runtime fake that counts writes, so tests can
// assert that Observe never writes a Secret.
type testKube struct {
	client.Client
	mu     sync.Mutex
	writes int
	// failWrites, if set, is returned by every write.
	failWrites error
}

func (k *testKube) count() error {
	k.mu.Lock()
	defer k.mu.Unlock()
	k.writes++
	return k.failWrites
}

func (k *testKube) Writes() int {
	k.mu.Lock()
	defer k.mu.Unlock()
	return k.writes
}

func (k *testKube) Create(ctx context.Context, obj client.Object, opts ...client.CreateOption) error {
	if err := k.count(); err != nil {
		return err
	}
	return k.Client.Create(ctx, obj, opts...)
}

func (k *testKube) Update(ctx context.Context, obj client.Object, opts ...client.UpdateOption) error {
	if err := k.count(); err != nil {
		return err
	}
	return k.Client.Update(ctx, obj, opts...)
}

func (k *testKube) Patch(ctx context.Context, obj client.Object, patch client.Patch, opts ...client.PatchOption) error {
	if err := k.count(); err != nil {
		return err
	}
	return k.Client.Patch(ctx, obj, patch, opts...)
}

func (k *testKube) Delete(ctx context.Context, obj client.Object, opts ...client.DeleteOption) error {
	if err := k.count(); err != nil {
		return err
	}
	return k.Client.Delete(ctx, obj, opts...)
}

func newTestKube(t *testing.T, objs ...client.Object) *testKube {
	t.Helper()
	s := runtime.NewScheme()
	if err := corev1.AddToScheme(s); err != nil {
		t.Fatal(err)
	}
	return &testKube{Client: cfake.NewClientBuilder().WithScheme(s).WithObjects(objs...).Build()}
}

// newTestCR returns a standalone postgres DBInstance with external name db1.
func newTestCR(opts ...func(*svcapitypes.DBInstance)) *svcapitypes.DBInstance {
	cr := &svcapitypes.DBInstance{ObjectMeta: metav1.ObjectMeta{Name: testName, Namespace: testNS, UID: "uid-1"}}
	cr.SetGroupVersionKind(svcapitypes.DBInstanceGroupVersionKind)
	meta.SetExternalName(cr, testName)
	cr.Spec.ForProvider.Engine = aws.String("postgres")
	cr.Spec.ForProvider.DBInstanceClass = aws.String("db.t3.micro")
	cr.Spec.ForProvider.MasterUsername = aws.String(testUser)
	for _, o := range opts {
		o(cr)
	}
	return cr
}

func withPasswordRef(cr *svcapitypes.DBInstance) {
	cr.Spec.ForProvider.MasterUserPasswordSecretRef = &xpv2.LocalSecretKeySelector{
		LocalSecretReference: xpv2.LocalSecretReference{Name: testPWSecret}, Key: testPWKey,
	}
}

func withAutogenerate(cr *svcapitypes.DBInstance) { cr.Spec.ForProvider.AutogeneratePassword = true }

func pwSecret(value string) *corev1.Secret {
	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: testPWSecret, Namespace: testNS},
		Data:       map[string][]byte{testPWKey: []byte(value)},
	}
}

// dbOut is an available standalone instance as DescribeDBInstances returns it.
func dbOut(status string, opts ...func(*svcsdktypes.DBInstance)) svcsdktypes.DBInstance {
	db := svcsdktypes.DBInstance{
		DBInstanceIdentifier: aws.String(testName),
		DBInstanceArn:        aws.String(testARN),
		DBInstanceStatus:     aws.String(status),
		Engine:               aws.String("postgres"),
		EngineVersion:        aws.String("16.4"),
		DBInstanceClass:      aws.String("db.t3.micro"),
		MasterUsername:       aws.String(testUser),
		Endpoint:             &svcsdktypes.Endpoint{Address: aws.String("db1.example"), Port: aws.Int32(5432)},
	}
	for _, o := range opts {
		o(&db)
	}
	return db
}

func describeOut(status string, opts ...func(*svcsdktypes.DBInstance)) *svcsdk.DescribeDBInstancesOutput {
	return &svcsdk.DescribeDBInstancesOutput{DBInstances: []svcsdktypes.DBInstance{dbOut(status, opts...)}}
}

func describeFn(out *svcsdk.DescribeDBInstancesOutput, err error) func(context.Context, *svcsdk.DescribeDBInstancesInput, ...func(*svcsdk.Options)) (*svcsdk.DescribeDBInstancesOutput, error) {
	return func(context.Context, *svcsdk.DescribeDBInstancesInput, ...func(*svcsdk.Options)) (*svcsdk.DescribeDBInstancesOutput, error) {
		return out, err
	}
}

// cacheData returns the password cache Secret of cr, nil if there is none.
func cacheData(t *testing.T, kube client.Client, cr *svcapitypes.DBInstance) map[string]string {
	t.Helper()
	return cacheDataCtx(context.Background(), t, kube, cr)
}

func cacheDataCtx(ctx context.Context, t *testing.T, kube client.Client, cr *svcapitypes.DBInstance) map[string]string {
	t.Helper()
	s := &corev1.Secret{}
	key := types.NamespacedName{Namespace: cr.GetNamespace(), Name: "dbinstance." + string(cr.GetUID())}
	if err := kube.Get(ctx, key, s); err != nil {
		return nil
	}
	out := map[string]string{}
	for k, v := range s.Data {
		out[k] = string(v)
	}
	return out
}

func errString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

func seedCache(t *testing.T, kube client.Client, cr *svcapitypes.DBInstance, kv map[string]string) {
	t.Helper()
	if _, err := rds.Cache(context.Background(), kube, cr, kv); err != nil {
		t.Fatal(err)
	}
}
