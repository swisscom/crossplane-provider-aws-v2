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

// Package aws builds AWS SDK v2 configurations from ProviderConfigs.
package aws

import (
	"context"
	"os"
	"sync"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/credentials/stscreds"
	"github.com/aws/aws-sdk-go-v2/service/sts"
	ststypes "github.com/aws/aws-sdk-go-v2/service/sts/types"
	"github.com/crossplane/crossplane-runtime/v2/pkg/errors"
	"github.com/crossplane/crossplane-runtime/v2/pkg/resource"
	xpv2 "github.com/crossplane/crossplane/apis/v2/core/v2"
	"gopkg.in/ini.v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/teeverr/provider-aws-v2/apis/v1alpha1"
)

const (
	// DefaultProfile is the profile read from the shared credentials file.
	DefaultProfile = "default"

	webIdentityTokenFileEnv         = "AWS_WEB_IDENTITY_TOKEN_FILE"
	webIdentityTokenFileDefaultPath = "/var/run/secrets/eks.amazonaws.com/serviceaccount/token"

	errTrackUsage          = "cannot track ProviderConfig usage"
	errGetPC               = "cannot get ProviderConfig"
	errGetCPC              = "cannot get ClusterProviderConfig"
	errUnknownPCKind       = "unsupported provider config kind"
	errNoPCRef             = "managed resource has no provider config reference"
	errNoSecretRef         = "credentials.secretRef is required when credentials.source is Secret"
	errSecretOtherNS       = "a ProviderConfig can only reference a Secret in its own namespace"
	errGetSecret           = "cannot get credentials secret"
	errParseCreds          = "cannot parse credentials secret"
	errNoWebIdentity       = "assumeRoleWithWebIdentity is required when credentials.source is WebIdentity"
	errUnknownSource       = "unsupported credentials source"
	errLoadDefault         = "cannot load default AWS config"
	errMissingAccessKeyIDs = "aws_access_key_id and aws_secret_access_key are required"
)

// A Tracker tracks the usage of a ProviderConfig by a managed resource.
type Tracker interface {
	Track(ctx context.Context, mg resource.ModernManaged) error
}

// A ProviderConfigRef identifies the (Cluster)ProviderConfig of a managed
// resource.
type ProviderConfigRef struct {
	Kind      string
	Name      string
	Namespace string
}

// GetConfig tracks the ProviderConfig usage of mg and returns an AWS config
// for the given region, built from the ProviderConfig or ClusterProviderConfig
// that mg references.
func GetConfig(ctx context.Context, kube client.Client, t Tracker, mg resource.ModernManaged, region string) (aws.Config, error) {
	if err := t.Track(ctx, mg); err != nil {
		return aws.Config{}, errors.Wrap(err, errTrackUsage)
	}
	ref := mg.GetProviderConfigReference()
	if ref == nil {
		return aws.Config{}, errors.New(errNoPCRef)
	}
	spec, err := GetProviderConfigSpec(ctx, kube, ProviderConfigRef{Kind: ref.Kind, Name: ref.Name, Namespace: mg.GetNamespace()})
	if err != nil {
		return aws.Config{}, err
	}
	// Only a namespaced ProviderConfig is restricted to its own namespace.
	ns := ""
	if ref.Kind == v1alpha1.ProviderConfigKind {
		ns = mg.GetNamespace()
	}
	return NewConfig(ctx, kube, spec, ns, region)
}

// GetProviderConfigSpec returns the spec of the referenced ProviderConfig or
// ClusterProviderConfig.
func GetProviderConfigSpec(ctx context.Context, kube client.Client, ref ProviderConfigRef) (*v1alpha1.ProviderConfigSpec, error) {
	switch ref.Kind {
	case v1alpha1.ProviderConfigKind:
		pc := &v1alpha1.ProviderConfig{}
		if err := kube.Get(ctx, types.NamespacedName{Name: ref.Name, Namespace: ref.Namespace}, pc); err != nil {
			return nil, errors.Wrap(err, errGetPC)
		}
		return &pc.Spec, nil
	case v1alpha1.ClusterProviderConfigKind:
		cpc := &v1alpha1.ClusterProviderConfig{}
		if err := kube.Get(ctx, types.NamespacedName{Name: ref.Name}, cpc); err != nil {
			return nil, errors.Wrap(err, errGetCPC)
		}
		return &cpc.Spec, nil
	default:
		return nil, errors.Errorf("%s: %q", errUnknownPCKind, ref.Kind)
	}
}

// NewConfig builds an AWS config from a ProviderConfigSpec. If secretNamespace
// is not empty, the credentials Secret must be in that namespace.
func NewConfig(ctx context.Context, kube client.Client, spec *v1alpha1.ProviderConfigSpec, secretNamespace, region string) (aws.Config, error) {
	var cfg aws.Config
	var err error

	switch spec.Credentials.Source {
	case v1alpha1.CredentialsSourceSecret:
		cfg, err = secretConfig(ctx, kube, spec.Credentials.SecretRef, secretNamespace, region)
	case v1alpha1.CredentialsSourceIRSA, v1alpha1.CredentialsSourcePodIdentity:
		cfg, err = defaultConfig(ctx, region)
	case v1alpha1.CredentialsSourceWebIdentity:
		cfg, err = webIdentityConfig(ctx, spec.AssumeRoleWithWebIdentity, region)
	default:
		return aws.Config{}, errors.Errorf("%s: %q", errUnknownSource, spec.Credentials.Source)
	}
	if err != nil {
		return aws.Config{}, err
	}

	if spec.AssumeRole != nil {
		cfg.Credentials = aws.NewCredentialsCache(stscreds.NewAssumeRoleProvider(sts.NewFromConfig(cfg), spec.AssumeRole.RoleARN, assumeRoleOptions(spec.AssumeRole)))
	}
	if spec.Endpoint != nil && spec.Endpoint.URL != "" {
		cfg.BaseEndpoint = aws.String(spec.Endpoint.URL)
	}
	return cfg, nil
}

func secretConfig(ctx context.Context, kube client.Client, ref *xpv2.SecretKeySelector, namespace, region string) (aws.Config, error) {
	if ref == nil {
		return aws.Config{}, errors.New(errNoSecretRef)
	}
	if namespace != "" && ref.Namespace != namespace {
		return aws.Config{}, errors.New(errSecretOtherNS)
	}
	s := &corev1.Secret{}
	if err := kube.Get(ctx, types.NamespacedName{Name: ref.Name, Namespace: ref.Namespace}, s); err != nil {
		return aws.Config{}, errors.Wrap(err, errGetSecret)
	}
	creds, err := ParseCredentials(s.Data[ref.Key], DefaultProfile)
	if err != nil {
		return aws.Config{}, errors.Wrap(err, errParseCreds)
	}
	return config.LoadDefaultConfig(ctx,
		config.WithRegion(region),
		config.WithCredentialsProvider(credentials.StaticCredentialsProvider{Value: creds}),
	)
}

func webIdentityConfig(ctx context.Context, o *v1alpha1.AssumeRoleWithWebIdentityOptions, region string) (aws.Config, error) {
	if o == nil {
		return aws.Config{}, errors.New(errNoWebIdentity)
	}
	cfg, err := defaultConfig(ctx, region)
	if err != nil {
		return aws.Config{}, err
	}
	tokenFile := webIdentityTokenFileDefaultPath
	if p := os.Getenv(webIdentityTokenFileEnv); p != "" {
		tokenFile = p
	}
	if o.TokenFile != nil && *o.TokenFile != "" {
		tokenFile = *o.TokenFile
	}
	cfg.Credentials = aws.NewCredentialsCache(stscreds.NewWebIdentityRoleProvider(
		sts.NewFromConfig(cfg), o.RoleARN, stscreds.IdentityTokenFile(tokenFile),
		func(wo *stscreds.WebIdentityRoleOptions) {
			if o.RoleSessionName != nil {
				wo.RoleSessionName = *o.RoleSessionName
			}
		}))
	return cfg, nil
}

func assumeRoleOptions(o *v1alpha1.AssumeRoleOptions) func(*stscreds.AssumeRoleOptions) {
	return func(ao *stscreds.AssumeRoleOptions) {
		ao.ExternalID = o.ExternalID
		if o.RoleSessionName != nil {
			ao.RoleSessionName = *o.RoleSessionName
		}
		for _, t := range o.Tags {
			ao.Tags = append(ao.Tags, ststypes.Tag{Key: aws.String(t.Key), Value: aws.String(t.Value)})
		}
		ao.TransitiveTagKeys = o.TransitiveTagKeys
	}
}

var (
	defaultMu  sync.Mutex
	defaultCfg *aws.Config
)

// defaultConfig returns a copy of the SDK default config, which is loaded once
// so that the pod credentials cache is shared by all managed resources.
func defaultConfig(ctx context.Context, region string) (aws.Config, error) {
	defaultMu.Lock()
	defer defaultMu.Unlock()
	if defaultCfg == nil {
		cfg, err := config.LoadDefaultConfig(ctx)
		if err != nil {
			return aws.Config{}, errors.Wrap(err, errLoadDefault)
		}
		defaultCfg = &cfg
	}
	cfg := defaultCfg.Copy()
	cfg.Region = region
	return cfg, nil
}

// ParseCredentials reads static credentials of a profile from an AWS shared
// credentials file:
//
//	[default]
//	aws_access_key_id = <id>
//	aws_secret_access_key = <secret>
//	aws_session_token = <optional token>
func ParseCredentials(data []byte, profile string) (aws.Credentials, error) {
	f, err := ini.InsensitiveLoad(data)
	if err != nil {
		return aws.Credentials{}, err
	}
	sec, err := f.GetSection(profile)
	if err != nil {
		return aws.Credentials{}, err
	}
	c := aws.Credentials{
		AccessKeyID:     sec.Key("aws_access_key_id").String(),
		SecretAccessKey: sec.Key("aws_secret_access_key").String(),
		SessionToken:    sec.Key("aws_session_token").String(),
	}
	if c.AccessKeyID == "" || c.SecretAccessKey == "" {
		return aws.Credentials{}, errors.New(errMissingAccessKeyIDs)
	}
	return c, nil
}

var _ Tracker = &resource.ProviderConfigUsageTracker{}
