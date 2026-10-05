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
	"slices"

	svcsdktypes "github.com/aws/aws-sdk-go-v2/service/rds/types"

	"github.com/teeverr/provider-aws-v2/internal/utils/pointer"
)

// AreSameElements reports whether the desired and observed slices contain
// the same set of elements, ignoring order and duplicates.
func AreSameElements(desired []*string, observed []string) bool {
	if len(desired) != len(observed) {
		return false
	}
	for _, d := range desired {
		if !slices.Contains(observed, pointer.StringValue(d)) {
			return false
		}
	}
	return true
}

// GenerateCloudWatchExportConfiguration computes the CloudwatchLogsExportConfiguration
// (log types to enable/disable) needed to reconcile the currently enabled CloudWatch
// log exports on AWS towards the desired spec. It returns nil when no change is
// required, so callers can leave the field unset on the modify request in that case.
func GenerateCloudWatchExportConfiguration(spec []*string, current []string) *svcsdktypes.CloudwatchLogsExportConfiguration {
	var toEnable, toDisable []string
	desired := make([]string, 0, len(spec))
	for _, s := range spec {
		v := pointer.StringValue(s)
		desired = append(desired, v)
		if !slices.Contains(current, v) {
			toEnable = append(toEnable, v)
		}
	}
	for _, c := range current {
		if !slices.Contains(desired, c) {
			toDisable = append(toDisable, c)
		}
	}
	if len(toEnable) == 0 && len(toDisable) == 0 {
		return nil
	}
	return &svcsdktypes.CloudwatchLogsExportConfiguration{
		EnableLogTypes:  toEnable,
		DisableLogTypes: toDisable,
	}
}
