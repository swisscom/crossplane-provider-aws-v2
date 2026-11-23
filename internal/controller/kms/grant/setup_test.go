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

package grant

import (
	"context"
	"errors"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	svcsdk "github.com/aws/aws-sdk-go-v2/service/kms"
	svcsdktypes "github.com/aws/aws-sdk-go-v2/service/kms/types"
	"github.com/crossplane/crossplane-runtime/v2/pkg/meta"

	svcapitypes "github.com/swisscom/crossplane-provider-aws-v2/apis/kms/v1alpha1"
)

type revokeClient struct {
	Client
	in  *svcsdk.RevokeGrantInput
	err error
}

func (c *revokeClient) RevokeGrant(_ context.Context, in *svcsdk.RevokeGrantInput, _ ...func(*svcsdk.Options)) (*svcsdk.RevokeGrantOutput, error) {
	c.in = in
	return &svcsdk.RevokeGrantOutput{}, c.err
}

func TestDelete(t *testing.T) {
	cases := map[string]struct {
		err     error
		wantErr bool
	}{
		"Revoked":  {},
		"NotFound": {err: &svcsdktypes.NotFoundException{Message: aws.String("gone")}},
		"Error":    {err: errors.New("boom"), wantErr: true},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			cr := &svcapitypes.Grant{}
			meta.SetExternalName(cr, "grant-id")
			cr.Spec.ForProvider.KeyID = aws.String("key-id")
			c := &revokeClient{err: tc.err}

			_, err := (&hooks{client: c}).delete(context.Background(), cr)
			if (err != nil) != tc.wantErr {
				t.Fatalf("wantErr %v, got %v", tc.wantErr, err)
			}
			if aws.ToString(c.in.GrantId) != "grant-id" || aws.ToString(c.in.KeyId) != "key-id" {
				t.Errorf("unexpected input: %s, %s", aws.ToString(c.in.GrantId), aws.ToString(c.in.KeyId))
			}
		})
	}
}
