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
	"bytes"
	"context"
	"errors"
	"log"
	"os"
	"strings"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	svcsdk "github.com/aws/aws-sdk-go-v2/service/rds"
	svcsdktypes "github.com/aws/aws-sdk-go-v2/service/rds/types"

	svcapitypes "github.com/swisscom/crossplane-provider-aws-v2/apis/rds/v1alpha1"
	rds "github.com/swisscom/crossplane-provider-aws-v2/internal/clients/rds"
	"github.com/swisscom/crossplane-provider-aws-v2/internal/clients/rds/fake"
)

const passwordKey = "password"

func TestObservePublishesAppliedPassword(t *testing.T) {
	cr := newTestCR(withPasswordRef)
	kube := newTestKube(t, pwSecret(newPassword))
	seedCache(t, kube, cr, map[string]string{rds.PasswordCacheKey: oldPassword})
	rdsc := &fake.MockRDSClient{MockDescribeDBInstances: describeFn(describeOut("available"), nil)}

	obs, err := newCustomExternal(kube, rdsc).Observe(context.Background(), cr)
	if err != nil {
		t.Fatal(err)
	}
	if obs.ResourceUpToDate {
		t.Fatal("want the rotated password detected")
	}
	if got := string(obs.ConnectionDetails[passwordKey]); got != oldPassword {
		t.Errorf("Observe publishes %q before ModifyDBInstance ran; want the applied %q", got, oldPassword)
	}
}

func TestUpdatePublishesPasswordAfterModify(t *testing.T) {
	cases := map[string]struct {
		modifyErr     error
		cacheErr      error
		wantErr       string
		wantPublished string
		wantCache     string
		wantPending   bool
		wantNoModify  bool
		wantTagCalls  int
	}{
		"ModifySucceeds": {wantPublished: newPassword, wantCache: newPassword, wantTagCalls: 1},
		// The new password stays recorded as not applied, so it is neither
		// published nor lost; the next round re-sends it.
		"ModifyFails": {
			modifyErr:   errors.New("InvalidParameterValue"),
			wantErr:     errUpdate + ": InvalidParameterValue",
			wantCache:   newPassword,
			wantPending: true,
		},
		// No password goes to AWS that the provider could not record first.
		"CacheWriteFails": {
			cacheErr:     errors.New("forbidden"),
			wantErr:      "pre-update failed: " + rds.ErrCachePassword + ": cannot patch object: forbidden",
			wantCache:    oldPassword,
			wantNoModify: true,
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			cr := newTestCR(withPasswordRef, func(cr *svcapitypes.DBInstance) {
				cr.Spec.ForProvider.Tags = []*svcapitypes.Tag{{Key: aws.String("team"), Value: aws.String("a")}}
			})
			kube := newTestKube(t, pwSecret(newPassword))
			seedCache(t, kube, cr, map[string]string{rds.PasswordCacheKey: oldPassword})
			var sent *svcsdk.ModifyDBInstanceInput
			tagCalls := 0
			rdsc := &fake.MockRDSClient{
				MockDescribeDBInstances: describeFn(describeOut("available"), nil),
				MockModifyDBInstance: func(_ context.Context, in *svcsdk.ModifyDBInstanceInput, _ ...func(*svcsdk.Options)) (*svcsdk.ModifyDBInstanceOutput, error) {
					sent = in
					return &svcsdk.ModifyDBInstanceOutput{}, tc.modifyErr
				},
				MockAddTagsToResource: func(context.Context, *svcsdk.AddTagsToResourceInput, ...func(*svcsdk.Options)) (*svcsdk.AddTagsToResourceOutput, error) {
					tagCalls++
					return &svcsdk.AddTagsToResourceOutput{}, nil
				},
			}
			e := newCustomExternal(kube, rdsc)
			if _, err := e.Observe(context.Background(), cr); err != nil {
				t.Fatal(err)
			}
			kube.failWrites = tc.cacheErr
			upd, err := e.Update(context.Background(), cr)
			kube.failWrites = nil

			if got := errString(err); got != tc.wantErr {
				t.Fatalf("Update error = %q, want %q", got, tc.wantErr)
			}
			switch {
			case tc.wantNoModify && sent != nil:
				t.Errorf("ModifyDBInstance sent %+v, want none", sent)
			case !tc.wantNoModify && aws.ToString(sent.MasterUserPassword) != newPassword:
				t.Errorf("ModifyDBInstance password = %q, want the new one", aws.ToString(sent.MasterUserPassword))
			}
			pw, published := upd.ConnectionDetails[passwordKey]
			if tc.wantPublished == "" && published {
				t.Errorf("Update published password %q, want none", pw)
			}
			if tc.wantPublished != "" && string(pw) != tc.wantPublished {
				t.Errorf("Update published password %q, want %q", pw, tc.wantPublished)
			}
			c := cacheData(t, kube, cr)
			if got := c[rds.PasswordCacheKey]; got != tc.wantCache {
				t.Errorf("cached password = %q, want %q", got, tc.wantCache)
			}
			if pending := c[rds.RestoreFlagCacheKay] != ""; pending != tc.wantPending {
				t.Errorf("cached password pending = %v, want %v", pending, tc.wantPending)
			}
			if tagCalls != tc.wantTagCalls {
				t.Errorf("AddTagsToResource calls = %d, want %d", tagCalls, tc.wantTagCalls)
			}
		})
	}
}

func TestObserveModifyingPublishesCachedPassword(t *testing.T) {
	cases := map[string]struct {
		cache map[string]string
		want  *string
	}{
		"Cached":  {cache: map[string]string{rds.PasswordCacheKey: "applied"}, want: aws.String("applied")},
		"NoCache": {},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			cr := newTestCR(withPasswordRef)
			kube := newTestKube(t, pwSecret("rotated"))
			if tc.cache != nil {
				seedCache(t, kube, cr, tc.cache)
			}
			rdsc := &fake.MockRDSClient{MockDescribeDBInstances: describeFn(describeOut("modifying"), nil)}
			obs, err := newCustomExternal(kube, rdsc).Observe(context.Background(), cr)
			if err != nil {
				t.Fatal(err)
			}
			pw, ok := obs.ConnectionDetails[passwordKey]
			switch {
			case tc.want == nil && ok:
				t.Errorf("password = %q, want the key left out", pw)
			case tc.want != nil && string(pw) != *tc.want:
				t.Errorf("password = %q, want %q", pw, *tc.want)
			}
		})
	}
}

func TestUpdatePasswordOnlyKeepsApplyImmediatelyOff(t *testing.T) {
	cr := newTestCR(withPasswordRef, func(cr *svcapitypes.DBInstance) { cr.Spec.ForProvider.ApplyImmediately = aws.Bool(false) })
	kube := newTestKube(t, pwSecret(newPassword))
	seedCache(t, kube, cr, map[string]string{rds.PasswordCacheKey: oldPassword})
	var sent *svcsdk.ModifyDBInstanceInput
	rdsc := &fake.MockRDSClient{
		MockDescribeDBInstances: describeFn(describeOut("available"), nil),
		MockModifyDBInstance: func(_ context.Context, in *svcsdk.ModifyDBInstanceInput, _ ...func(*svcsdk.Options)) (*svcsdk.ModifyDBInstanceOutput, error) {
			sent = in
			return &svcsdk.ModifyDBInstanceOutput{}, nil
		},
	}
	e := newCustomExternal(kube, rdsc)
	if _, err := e.Observe(context.Background(), cr); err != nil {
		t.Fatal(err)
	}
	upd, err := e.Update(context.Background(), cr)
	if err != nil {
		t.Fatal(err)
	}
	// AWS applies a new master password asynchronously regardless of ApplyImmediately.
	if sent.ApplyImmediately == nil || *sent.ApplyImmediately {
		t.Errorf("ApplyImmediately = %v, want false as in the spec", sent.ApplyImmediately)
	}
	if got := string(upd.ConnectionDetails[passwordKey]); got != newPassword {
		t.Errorf("published password = %q, want the new one after the modify", got)
	}
}

func TestObserveWritesNoSecret(t *testing.T) {
	cases := map[string]struct {
		cache map[string]string
		db    func(*svcsdktypes.DBInstance)
	}{
		"Normal":         {cache: map[string]string{rds.PasswordCacheKey: oldValue}},
		"RestorePending": {cache: map[string]string{rds.PasswordCacheKey: "gen", rds.RestoreFlagCacheKay: "RestoreStateRestored"}},
		"Managed": {db: func(db *svcsdktypes.DBInstance) {
			db.MasterUserSecret = &svcsdktypes.MasterUserSecret{SecretArn: aws.String("arn:secret")}
		}},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			cr := newTestCR(withPasswordRef)
			kube := newTestKube(t, pwSecret("new"))
			if tc.cache != nil {
				seedCache(t, kube, cr, tc.cache)
			}
			before := kube.Writes()
			out := describeOut("available")
			if tc.db != nil {
				tc.db(&out.DBInstances[0])
			}
			if _, err := newCustomExternal(kube, &fake.MockRDSClient{MockDescribeDBInstances: describeFn(out, nil)}).Observe(context.Background(), cr); err != nil {
				t.Fatal(err)
			}
			if w := kube.Writes() - before; w != 0 {
				t.Errorf("Observe wrote %d Kubernetes objects, want none", w)
			}
		})
	}
}

func TestObserveDiffHasNoPasswordAndNoLog(t *testing.T) {
	var logged bytes.Buffer
	log.SetOutput(&logged)
	t.Cleanup(func() { log.SetOutput(os.Stderr) })

	cr := newTestCR(withPasswordRef, func(cr *svcapitypes.DBInstance) { cr.Spec.ForProvider.MultiAZ = aws.Bool(true) })
	kube := newTestKube(t, pwSecret("n3w-s3cret"))
	seedCache(t, kube, cr, map[string]string{rds.PasswordCacheKey: "0ld-s3cret"})
	obs, err := newCustomExternal(kube, &fake.MockRDSClient{MockDescribeDBInstances: describeFn(describeOut("available"), nil)}).Observe(context.Background(), cr)
	if err != nil {
		t.Fatal(err)
	}
	if obs.ResourceUpToDate || !strings.Contains(obs.Diff, "master user password changed") {
		t.Fatalf("want a diff for the password and multiAZ, got %q", obs.Diff)
	}
	if strings.Contains(obs.Diff, "s3cret") {
		t.Errorf("diff contains a password: %q", obs.Diff)
	}
	if logged.Len() != 0 {
		t.Errorf("Observe logged to stderr: %q", logged.String())
	}
}
