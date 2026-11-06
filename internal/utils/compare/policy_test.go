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

package compare

import (
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
)

func TestPolicyEqual(t *testing.T) {
	const (
		policy = `{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Principal":{"AWS":"*"},"Action":"es:*","Resource":"arn:aws:es:eu-central-1:123456789012:domain/logs/*"}]}`
		// policyAWS is policy as AWS returns it: other formatting, and
		// lists with one element replaced by the element.
		policyAWS = `{"Statement": {"Action": ["es:*"], "Effect": "Allow", "Principal": {"AWS": "*"}, "Resource": "arn:aws:es:eu-central-1:123456789012:domain/logs/*"}, "Version": "2012-10-17"}`
	)
	cases := map[string]struct {
		desired, observed *string
		want              bool
	}{
		"NoDesired":         {observed: aws.String(policy), want: true},
		"Same":              {desired: aws.String(policy), observed: aws.String(policy), want: true},
		"FormattedByAWS":    {desired: aws.String(policy), observed: aws.String(policyAWS), want: true},
		"NotObserved":       {desired: aws.String(policy), want: false},
		"Different":         {desired: aws.String(policy), observed: aws.String(`{"Version":"2012-10-17","Statement":[]}`), want: false},
		"NotJSON":           {desired: aws.String("x"), observed: aws.String("y"), want: false},
		"ListOfTwoNotEqual": {desired: aws.String(`{"a":["x","y"]}`), observed: aws.String(`{"a":"x"}`), want: false},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			if got := PolicyEqual(tc.desired, tc.observed); got != tc.want {
				t.Errorf("PolicyEqual(...) = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestCompactPolicy(t *testing.T) {
	cases := map[string]struct {
		policy, want *string
	}{
		"Nil":                      {},
		"MultiLine":                {policy: aws.String("{\n  \"Version\": \"2012-10-17\",\n  \"Statement\": [{\"Action\": \"es:*\"}]\n}\n"), want: aws.String(`{"Version":"2012-10-17","Statement":[{"Action":"es:*"}]}`)},
		"Compact":                  {policy: aws.String(`{"Version":"2012-10-17"}`), want: aws.String(`{"Version":"2012-10-17"}`)},
		"KeepsWhitespaceInStrings": {policy: aws.String(`{ "Sid": "a b\nc" }`), want: aws.String(`{"Sid":"a b\nc"}`)},
		"NotJSON":                  {policy: aws.String("{\n  not json\n}"), want: aws.String("{\n  not json\n}")},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			got := CompactPolicy(tc.policy)
			if (got == nil) != (tc.want == nil) || (got != nil && *got != *tc.want) {
				t.Errorf("CompactPolicy(...) = %v, want %v", aws.ToString(got), aws.ToString(tc.want))
			}
		})
	}
}
