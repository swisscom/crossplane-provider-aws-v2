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
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/crossplane/crossplane-runtime/v2/pkg/errors"

	svcapitypes "github.com/swisscom/crossplane-provider-aws-v2/apis/docdb/v1alpha1"
)

const errForeignEngine = "%s %q has engine %q, not %q; refusing to manage it"

// CheckEngine returns an error unless engine is DocumentDB. The DocumentDB
// API also returns RDS and Neptune resources, which share its name space.
func CheckEngine(kind, id string, engine *string) error {
	if aws.ToString(engine) == svcapitypes.EngineDocDB {
		return nil
	}
	return errors.Errorf(errForeignEngine, kind, id, aws.ToString(engine), svcapitypes.EngineDocDB)
}

const errForeignFamily = "DBClusterParameterGroup %q has family %q, not a DocumentDB (%s*) family; refusing to manage it"

// CheckParameterGroupFamily returns an error unless family is a DocumentDB
// family such as docdb5.0; Aurora and Neptune groups share the name space.
func CheckParameterGroupFamily(name string, family *string) error {
	if strings.HasPrefix(aws.ToString(family), svcapitypes.EngineDocDB) {
		return nil
	}
	return errors.Errorf(errForeignFamily, name, aws.ToString(family), svcapitypes.EngineDocDB)
}
