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

package utils

import (
	"testing"

	"k8s.io/utils/ptr"
)

func TestCheckEngine(t *testing.T) {
	cases := map[string]struct {
		engine *string
		want   string
	}{
		"DocDB":  {engine: ptr.To("docdb")},
		"Aurora": {engine: ptr.To("aurora-postgresql"), want: `DBCluster "orders" has engine "aurora-postgresql", not "docdb"; refusing to manage it`},
		"Prefix": {engine: ptr.To("docdb-elastic"), want: `DBCluster "orders" has engine "docdb-elastic", not "docdb"; refusing to manage it`},
		"Unset":  {want: `DBCluster "orders" has engine "", not "docdb"; refusing to manage it`},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			err := CheckEngine("DBCluster", "orders", tc.engine)
			if got := errString(err); got != tc.want {
				t.Errorf("CheckEngine(...) = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestCheckParameterGroupFamily(t *testing.T) {
	cases := map[string]struct {
		family *string
		want   string
	}{
		"DocDB5": {family: ptr.To("docdb5.0")},
		"DocDB4": {family: ptr.To("docdb4.0")},
		"Aurora": {family: ptr.To("aurora-mysql8.0"), want: `DBClusterParameterGroup "p" has family "aurora-mysql8.0", not a DocumentDB (docdb*) family; refusing to manage it`},
		"Unset":  {want: `DBClusterParameterGroup "p" has family "", not a DocumentDB (docdb*) family; refusing to manage it`},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			if got := errString(CheckParameterGroupFamily("p", tc.family)); got != tc.want {
				t.Errorf("CheckParameterGroupFamily(...) = %q, want %q", got, tc.want)
			}
		})
	}
}

func errString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}
