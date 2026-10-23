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

package generator

import (
	"testing"

	awssdkmodel "github.com/aws-controllers-k8s/code-generator/pkg/api"
)

func TestRunRejectsSensitiveFields(t *testing.T) {
	withoutIgnore := removeLine(t, "    - PendingModifiedValues.MasterUserPassword")

	out, err := docdbFixture.run(t, withoutIgnore, "")

	errContains(t, err,
		"DBInstance: status.atProvider.pendingModifiedValues.masterUserPassword looks like a secret but would be a plain API field; ignore it (ignore.field_paths: PendingModifiedValues.MasterUserPassword)",
		"or allow it in apis/docdb/codegen.yaml kinds.DBInstance.allowSensitiveFields with a reason",
	)
	if files := goFiles(t, out); len(files) > 0 {
		t.Errorf("Run wrote files although it failed: %v", files)
	}
}

func TestRunAllowSensitiveFields(t *testing.T) {
	const pmv = "status.atProvider.pendingModifiedValues.masterUserPassword"
	withoutIgnore := removeLine(t, "    - PendingModifiedValues.MasterUserPassword")
	cases := map[string]struct {
		edit    func(string) string
		yaml    string
		wantErr string
	}{
		"allowed with a reason": {
			edit: withoutIgnore,
			yaml: "kinds:\n  DBInstance:\n    allowSensitiveFields:\n    - path: " + pmv + "\n      reason: test\n",
		},
		"allowed without a reason": {
			edit:    withoutIgnore,
			yaml:    "kinds:\n  DBInstance:\n    allowSensitiveFields:\n    - path: " + pmv + "\n      reason: ' '\n",
			wantErr: "DBInstance: allowSensitiveFields entry " + pmv + " needs a reason",
		},
		"stale allowance": {
			yaml:    "kinds:\n  DBInstance:\n    allowSensitiveFields:\n    - path: " + pmv + "\n      reason: test\n",
			wantErr: "DBInstance: allowSensitiveFields entry " + pmv + " matches no sensitive field; remove it",
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := docdbFixture.run(t, tc.edit, tc.yaml)
			if tc.wantErr == "" {
				if err != nil {
					t.Fatalf("Run: %v", err)
				}
				return
			}
			errContains(t, err, tc.wantErr)
		})
	}
}

func TestIsSensitiveLeaf(t *testing.T) {
	str := &awssdkmodel.Shape{Type: shapeString}
	cases := []struct {
		member string
		shape  *awssdkmodel.Shape
		want   bool
	}{
		{"TdeCredentialPassword", str, true},
		{"ClientToken", str, true},
		{"MasterUserPassword", str, true},
		{"AuthPassphrase", str, true},
		{"ManageMasterUserPassword", &awssdkmodel.Shape{Type: "boolean"}, false},
		{"PasswordLength", &awssdkmodel.Shape{Type: "integer"}, false},
		{"SecretType", &awssdkmodel.Shape{Type: shapeString, Enum: []string{"a", "b"}}, false},
		{"SecretArn", str, false},
		{"SecretStatus", str, false},
		{"TdeCredentialArn", str, false},
		{"MasterUserSecretKmsKeyId", str, false},
		{"AccessKeyId", str, false},
		{"MasterUserSecret", &awssdkmodel.Shape{Type: shapeStructure}, false},
		{"DBInstanceClass", str, false},
		{"PrivateKey", &awssdkmodel.Shape{Type: shapeBlob}, true},
		{"ClientSecret", &awssdkmodel.Shape{Type: shapeBlob}, true},
		{"CertificateBody", &awssdkmodel.Shape{Type: shapeBlob}, false},
		{"SecretArn", &awssdkmodel.Shape{Type: shapeBlob}, false},
	}
	for _, tc := range cases {
		if got := isSensitiveLeaf(tc.member, tc.shape); got != tc.want {
			t.Errorf("isSensitiveLeaf(%s, %s) = %v, want %v", tc.member, tc.shape.Type, got, tc.want)
		}
	}
}

func TestSensitiveLeavesLooksThroughBlobCollections(t *testing.T) {
	blob := &awssdkmodel.Shape{Type: shapeBlob}
	cases := map[string]*awssdkmodel.Shape{
		"list of blobs": {Type: "list", MemberRef: awssdkmodel.ShapeRef{Shape: blob}},
		"map of blobs":  {Type: "map", ValueRef: awssdkmodel.ShapeRef{Shape: blob}},
	}
	for name, s := range cases {
		t.Run(name, func(t *testing.T) {
			got := sensitiveLeaves(jsonNames{}, "spec.forProvider.privateKeys", "", "PrivateKeys", &awssdkmodel.ShapeRef{Shape: s}, map[string]bool{})
			if len(got) != 1 || got[0].path != "spec.forProvider.privateKeys" {
				t.Errorf("sensitiveLeaves() = %+v, want the privateKeys leaf", got)
			}
		})
	}
}
