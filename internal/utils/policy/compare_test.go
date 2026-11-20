/*
Copyright 2023 The Crossplane Authors.

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

package policy

import (
	_ "embed"
	"testing"

	"github.com/google/go-cmp/cmp"
)

var (
	//go:embed testdata/Issue1892_a.json
	policyIssue1892a string

	//go:embed testdata/Issue1892_a_min.json
	policyIssue1892aMin string

	//go:embed testdata/Issue1892_b.json
	policyIssue1892b string

	//go:embed testdata/Issue1892_b_min.json
	policyIssue1892bMin string

	//go:embed testdata/PrincipalsOrder_a.json
	policyPrincipalsOrderA string

	//go:embed testdata/PrincipalsOrder_b.json
	policyPrincipalsOrderB string

	//go:embed testdata/PrincipalsOrder_c.json
	policyPrincipalsOrderC string
)

func TestCompareRawPolicies(t *testing.T) {
	type args struct {
		policyA string
		policyB string
	}
	type want struct {
		equals bool
	}
	cases := map[string]struct {
		want
		args
	}{
		"Issue1892_a": {
			args: args{
				policyA: policyIssue1892a,
				policyB: policyIssue1892aMin,
			},
			want: want{
				equals: true,
			},
		},
		"Issue1892_b": {
			args: args{
				policyA: policyIssue1892b,
				policyB: policyIssue1892bMin,
			},
			want: want{
				equals: true,
			},
		},
		"PrincipalsOrder Equal": {
			args: args{
				policyA: policyPrincipalsOrderA,
				policyB: policyPrincipalsOrderB,
			},
			want: want{
				equals: true,
			},
		},
		"PrincipalsOrder NotEqual": {
			args: args{
				policyA: policyPrincipalsOrderA,
				policyB: policyPrincipalsOrderC,
			},
			want: want{
				equals: false,
			},
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			pA, err := ParsePolicyString(tc.args.policyA)
			if err != nil {
				t.Fatal(err)
			}
			pB, err := ParsePolicyString(tc.args.policyB)
			if err != nil {
				t.Fatal(err)
			}

			equal, pDiff := ArePoliciesEqal(&pA, &pB)
			if diff := cmp.Diff(&tc.want.equals, &equal); diff != "" {
				t.Errorf("ArePoliciesEqal(...): -want, +got\n:%s\nDiff: -want +got\n:%s", diff, pDiff)
			}
		})
	}
}
