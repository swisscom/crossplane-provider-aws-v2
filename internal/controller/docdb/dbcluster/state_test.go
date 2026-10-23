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
	"testing"

	"github.com/aws/aws-sdk-go-v2/service/docdb"
	docdbtypes "github.com/aws/aws-sdk-go-v2/service/docdb/types"
	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"

	svcapitypes "github.com/swisscom/crossplane-provider-aws-v2/apis/docdb/v1alpha1"
	fake "github.com/swisscom/crossplane-provider-aws-v2/internal/clients/docdb/fake"
)

func TestDeleteSnapshotFieldsAndDeletingState(t *testing.T) {
	cases := map[string]struct {
		cr   []docDBModifier
		want []*docdb.DeleteDBClusterInput
	}{
		"AlreadyDeleting": {
			cr: []docDBModifier{withStatus("deleting"), withSkipFinalSnapshot(true)},
		},
		"BothSnapshotFields": {
			cr: []docDBModifier{withStatus("available"), withFinalDBSnapshotIdentifier("final"),
				func(o *svcapitypes.DBCluster) { o.Spec.ForProvider.SkipFinalSnapshot = ptr.To(false) }},
			want: []*docdb.DeleteDBClusterInput{{
				DBClusterIdentifier:       ptr.To("orders"),
				FinalDBSnapshotIdentifier: ptr.To("final"),
				SkipFinalSnapshot:         ptr.To(false),
			}},
		},
		// AWS rejects this call, which fails safe.
		"NoSnapshotFields": {
			cr:   []docDBModifier{withStatus("available")},
			want: []*docdb.DeleteDBClusterInput{{DBClusterIdentifier: ptr.To("orders")}},
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			c := &fake.MockDocDBClient{}
			e := newExternal(nil, c, []option{setupExternal})
			cr := instance(append([]docDBModifier{withExternalName("orders")}, tc.cr...)...)

			if _, err := e.Delete(context.Background(), cr); err != nil {
				t.Fatalf("Delete(...): %v", err)
			}

			got := make([]*docdb.DeleteDBClusterInput, 0, len(c.Called.DeleteDBCluster))
			for _, call := range c.Called.DeleteDBCluster {
				got = append(got, call.I)
			}
			if diff := cmp.Diff(tc.want, got, ignoreUnexported, cmpopts.EquateEmpty()); diff != "" {
				t.Errorf("DeleteDBCluster inputs: -want, +got:\n%s", diff)
			}
		})
	}
}

// AWS rejects ModifyDBCluster in these states; a diff would only produce
// errors until the cluster is available again.
func TestObserveTransitionalStatesWait(t *testing.T) {
	cases := map[string]bool{
		"creating": true, "modifying": true, "upgrading": true, "backing-up": true,
		"resetting-master-credentials": true, "rebooting": true, "renaming": true,
		"starting": true, "stopping": true, "migrating": true, "maintenance": true,
		// A broken cluster stays visible.
		"available": false, "failed": false, "some-new-state": false,
	}
	for state, wait := range cases {
		t.Run(state, func(t *testing.T) {
			c := describeClusters(docdbtypes.DBCluster{
				DBClusterIdentifier: ptr.To(testDBClusterIdentifier),
				Engine:              ptr.To(svcapitypes.EngineDocDB),
				Status:              ptr.To(state),
				Port:                ptr.To[int32](27017),
			})
			refReads := 0
			kube := passwordKube("new", "old")
			get := kube.MockGet
			kube.MockGet = func(ctx context.Context, key client.ObjectKey, obj client.Object) error {
				if key.Name == testMasterPasswordSecretName {
					refReads++
				}
				return get(ctx, key, obj)
			}
			e := newExternal(kube, c, []option{setupExternal})
			cr := passwordCluster(withPort(1234))

			obs, err := e.Observe(context.Background(), cr)
			if err != nil {
				t.Fatalf("Observe(...): %v", err)
			}

			if obs.ResourceUpToDate != wait {
				t.Errorf("Observe(...) ResourceUpToDate = %v, want %v (diff %q)", obs.ResourceUpToDate, wait, obs.Diff)
			}
			// postObserve still reads the password once to publish it.
			if wait && (len(c.Called.ListTagsForResource) != 0 || refReads != 1) {
				t.Errorf("Observe(...) listed tags %d times and read the password %d times while %s", len(c.Called.ListTagsForResource), refReads, state)
			}
		})
	}
}
