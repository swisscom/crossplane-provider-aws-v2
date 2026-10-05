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

package utils

import (
	"testing"

	svcsdktypes "github.com/aws/aws-sdk-go-v2/service/rds/types"
	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"

	"github.com/teeverr/provider-aws-v2/internal/utils/pointer"
)

func strPtrs(vals ...string) []*string {
	out := make([]*string, 0, len(vals))
	for _, v := range vals {
		out = append(out, pointer.ToOrNilIfZeroValue(v))
	}
	return out
}

func TestAreSameElements(t *testing.T) {
	cases := map[string]struct {
		a1   []*string
		a2   []string
		want bool
	}{
		"SameOrder":        {strPtrs("audit", "error"), []string{"audit", "error"}, true},
		"DifferentOrder":   {strPtrs("audit", "error"), []string{"error", "audit"}, true},
		"DifferentLength":  {strPtrs("audit"), []string{"audit", "error"}, false},
		"DifferentElement": {strPtrs("audit", "error"), []string{"audit", "general"}, false},
		"BothEmpty":        {nil, nil, true},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			if got := AreSameElements(tc.a1, tc.a2); got != tc.want {
				t.Errorf("AreSameElements(...): want %v, got %v", tc.want, got)
			}
		})
	}
}

func TestGenerateCloudWatchExportConfiguration(t *testing.T) {
	cases := map[string]struct {
		spec    []*string
		current []string
		want    *svcsdktypes.CloudwatchLogsExportConfiguration
	}{
		"NoChange": {
			spec:    strPtrs("audit", "error"),
			current: []string{"error", "audit"},
		},
		"EnableOnly": {
			spec:    strPtrs("audit", "error"),
			current: []string{"audit"},
			want:    &svcsdktypes.CloudwatchLogsExportConfiguration{EnableLogTypes: []string{"error"}},
		},
		"DisableOnly": {
			spec:    strPtrs("audit"),
			current: []string{"audit", "error"},
			want:    &svcsdktypes.CloudwatchLogsExportConfiguration{DisableLogTypes: []string{"error"}},
		},
		"EnableAndDisable": {
			spec:    strPtrs("audit", "general"),
			current: []string{"audit", "error"},
			want: &svcsdktypes.CloudwatchLogsExportConfiguration{
				EnableLogTypes:  []string{"general"},
				DisableLogTypes: []string{"error"},
			},
		},
		"BothEmpty": {},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			got := GenerateCloudWatchExportConfiguration(tc.spec, tc.current)
			if diff := cmp.Diff(tc.want, got, cmpopts.EquateEmpty(), cmpopts.IgnoreUnexported(svcsdktypes.CloudwatchLogsExportConfiguration{})); diff != "" {
				t.Errorf("GenerateCloudWatchExportConfiguration(...): -want, +got:\n%s", diff)
			}
		})
	}
}
