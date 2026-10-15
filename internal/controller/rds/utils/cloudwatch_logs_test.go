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

	"github.com/swisscom/crossplane-provider-aws-v2/internal/utils/pointer"
)

const (
	logAudit = "audit"
	logError = "error"
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
		"SameOrder":        {strPtrs(logAudit, logError), []string{logAudit, logError}, true},
		"DifferentOrder":   {strPtrs(logAudit, logError), []string{logError, logAudit}, true},
		"DifferentLength":  {strPtrs(logAudit), []string{logAudit, logError}, false},
		"DifferentElement": {strPtrs(logAudit, logError), []string{logAudit, "general"}, false},
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
			spec:    strPtrs(logAudit, logError),
			current: []string{logError, logAudit},
		},
		"EnableOnly": {
			spec:    strPtrs(logAudit, logError),
			current: []string{logAudit},
			want:    &svcsdktypes.CloudwatchLogsExportConfiguration{EnableLogTypes: []string{logError}},
		},
		"DisableOnly": {
			spec:    strPtrs(logAudit),
			current: []string{logAudit, logError},
			want:    &svcsdktypes.CloudwatchLogsExportConfiguration{DisableLogTypes: []string{logError}},
		},
		"EnableAndDisable": {
			spec:    strPtrs(logAudit, "general"),
			current: []string{logAudit, logError},
			want: &svcsdktypes.CloudwatchLogsExportConfiguration{
				EnableLogTypes:  []string{"general"},
				DisableLogTypes: []string{logError},
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
