package v1alpha1

import (
	resource "github.com/crossplane/crossplane-runtime/v2/pkg/resource"
	xpv2 "github.com/crossplane/crossplane/apis/v2/core/v2"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// interface checks to ensure our types conform to the crossplane-runtime interfaces
var (
	_ resource.ProviderConfig           = &ProviderConfig{}
	_ resource.ProviderConfig           = &ClusterProviderConfig{}
	_ resource.TypedProviderConfigUsage = &ProviderConfigUsage{}
	_ resource.ProviderConfigUsageList  = &ProviderConfigUsageList{}
)

// A ProviderConfigStatus defines the status of a Provider.
type ProviderConfigStatus struct {
	xpv2.ProviderConfigStatus `json:",inline"`
}

// ProviderCredentials required to authenticate.
type ProviderCredentials struct {
	// Source of the provider credentials.
	// Secret: an AWS shared credentials file (INI) stored in a Secret.
	// IRSA and PodIdentity: credentials of the provider pod, resolved by the
	// AWS SDK default credential chain (IRSA, EKS Pod Identity, environment).
	// WebIdentity: exchange the pod's web identity token for the role in
	// assumeRoleWithWebIdentity.
	// +kubebuilder:validation:Enum=Secret;IRSA;PodIdentity;WebIdentity
	Source CredentialsSource `json:"source"`

	// SecretRef to the AWS shared credentials file. Required when source is
	// Secret. For a ProviderConfig the Secret must be in the ProviderConfig's
	// namespace.
	// +optional
	SecretRef *xpv2.SecretKeySelector `json:"secretRef,omitempty"`
}

// CredentialsSource is the source of the AWS credentials.
type CredentialsSource string

// Supported credentials sources.
const (
	CredentialsSourceSecret      CredentialsSource = "Secret"
	CredentialsSourceIRSA        CredentialsSource = "IRSA"
	CredentialsSourcePodIdentity CredentialsSource = "PodIdentity"
	CredentialsSourceWebIdentity CredentialsSource = "WebIdentity"
)

// Tag is a session tag passed to sts:AssumeRole.
type Tag struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

// AssumeRoleOptions configures sts:AssumeRole on top of the base credentials.
type AssumeRoleOptions struct {
	// RoleARN of the role to assume.
	RoleARN string `json:"roleARN"`

	// ExternalID passed to sts:AssumeRole.
	// +optional
	ExternalID *string `json:"externalID,omitempty"`

	// RoleSessionName passed to sts:AssumeRole.
	// +optional
	RoleSessionName *string `json:"roleSessionName,omitempty"`

	// Tags are session tags passed to sts:AssumeRole.
	// +optional
	Tags []Tag `json:"tags,omitempty"`

	// TransitiveTagKeys passed to sts:AssumeRole.
	// +optional
	TransitiveTagKeys []string `json:"transitiveTagKeys,omitempty"`
}

// AssumeRoleWithWebIdentityOptions configures sts:AssumeRoleWithWebIdentity.
type AssumeRoleWithWebIdentityOptions struct {
	// RoleARN of the role to assume.
	RoleARN string `json:"roleARN"`

	// RoleSessionName passed to sts:AssumeRoleWithWebIdentity.
	// +optional
	RoleSessionName *string `json:"roleSessionName,omitempty"`

	// TokenFile is the path of the web identity token. Defaults to
	// $AWS_WEB_IDENTITY_TOKEN_FILE, then the EKS default path.
	// +optional
	TokenFile *string `json:"tokenFile,omitempty"`
}

// EndpointConfig overrides the AWS API endpoint, e.g. for LocalStack.
type EndpointConfig struct {
	// URL used as the base endpoint for every AWS service client.
	URL string `json:"url"`
}

// A ProviderConfigSpec defines the desired state of a ProviderConfig.
// +kubebuilder:validation:XValidation:rule="self.credentials.source != 'WebIdentity' || has(self.assumeRoleWithWebIdentity)",message="assumeRoleWithWebIdentity is required when credentials.source is WebIdentity"
// +kubebuilder:validation:XValidation:rule="self.credentials.source != 'Secret' || has(self.credentials.secretRef)",message="credentials.secretRef is required when credentials.source is Secret"
type ProviderConfigSpec struct {
	// Credentials required to authenticate to this provider.
	Credentials ProviderCredentials `json:"credentials"`

	// AssumeRole is applied on top of the credentials from credentials.source.
	// +optional
	AssumeRole *AssumeRoleOptions `json:"assumeRole,omitempty"`

	// AssumeRoleWithWebIdentity configures the WebIdentity credentials source.
	// +optional
	AssumeRoleWithWebIdentity *AssumeRoleWithWebIdentityOptions `json:"assumeRoleWithWebIdentity,omitempty"`

	// Endpoint overrides the AWS API endpoint.
	// +optional
	Endpoint *EndpointConfig `json:"endpoint,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:storageversion

// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="AGE",type="date",JSONPath=".metadata.creationTimestamp"
// +kubebuilder:printcolumn:name="SECRET-NAME",type="string",JSONPath=".spec.credentials.secretRef.name",priority=1
// +kubebuilder:resource:scope=Namespaced,categories={crossplane,provider,aws2}
// A ProviderConfig configures a Helm 'provider', i.e. a connection to a particular
type ProviderConfig struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ProviderConfigSpec   `json:"spec"`
	Status ProviderConfigStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// ProviderConfigList contains a list of Provider
type ProviderConfigList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ProviderConfig `json:"items"`
}

// +kubebuilder:object:root=true
// +kubebuilder:storageversion

// +kubebuilder:printcolumn:name="AGE",type="date",JSONPath=".metadata.creationTimestamp"
// +kubebuilder:printcolumn:name="CONFIG-NAME",type="string",JSONPath=".providerConfigRef.name"
// +kubebuilder:printcolumn:name="RESOURCE-KIND",type="string",JSONPath=".resourceRef.kind"
// +kubebuilder:printcolumn:name="RESOURCE-NAME",type="string",JSONPath=".resourceRef.name"
// +kubebuilder:resource:scope=Namespaced,categories={crossplane,provider,aws2}
// A ProviderConfigUsage indicates that a resource is using a ProviderConfig or a
// ClusterProviderConfig. There is deliberately no cluster scoped usage type, Usages always live in
// the namespace of the MR that created them, and record which kind of config they refer to.
type ProviderConfigUsage struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	xpv2.TypedProviderConfigUsage `json:",inline"`
}

// +kubebuilder:object:root=true

// ProviderConfigUsageList contains a list of ProviderConfigUsage
type ProviderConfigUsageList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ProviderConfigUsage `json:"items"`
}

// +kubebuilder:object:root=true

// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="AGE",type="date",JSONPath=".metadata.creationTimestamp"
// +kubebuilder:printcolumn:name="SECRET-NAME",type="string",JSONPath=".spec.credentials.secretRef.name",priority=1
// +kubebuilder:resource:scope=Cluster,categories={crossplane,provider,aws2}
// A ClusterProviderConfig configures a AWS v2 provider.
type ClusterProviderConfig struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ProviderConfigSpec   `json:"spec"`
	Status ProviderConfigStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// ClusterProviderConfigList contains a list of ProviderConfig.
type ClusterProviderConfigList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ClusterProviderConfig `json:"items"`
}
