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

package domain

import (
	"context"
	"encoding/json"
	"reflect"
	"slices"
	"sort"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	svcsdk "github.com/aws/aws-sdk-go-v2/service/opensearch"
	svcsdktypes "github.com/aws/aws-sdk-go-v2/service/opensearch/types"
	"github.com/crossplane/crossplane-runtime/v2/pkg/errors"
	"github.com/crossplane/crossplane-runtime/v2/pkg/meta"
	"github.com/crossplane/crossplane-runtime/v2/pkg/reconciler/managed"
	cpresource "github.com/crossplane/crossplane-runtime/v2/pkg/resource"
	xpv2 "github.com/crossplane/crossplane/apis/v2/core/v2"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	svcapitypes "github.com/swisscom/crossplane-provider-aws-v2/apis/opensearch/v1alpha1"
	awsclient "github.com/swisscom/crossplane-provider-aws-v2/internal/clients/aws"
	"github.com/swisscom/crossplane-provider-aws-v2/internal/utils/compare"
	pointer "github.com/swisscom/crossplane-provider-aws-v2/internal/utils/pointer"
)

const (
	// port is the HTTPS port of every domain endpoint.
	port = "443"
	// vpcEndpointKey is the key of the endpoint of a VPC domain in
	// DomainStatus.Endpoints; such domains have no DomainStatus.Endpoint.
	vpcEndpointKey = "vpc"
	// awsTagPrefix marks tags reserved by AWS; they can't be changed.
	awsTagPrefix = "aws:"

	errGetPasswordSecret   = "cannot get the secret of masterUserPasswordSecretRef"
	errGetConnectionSecret = "cannot get the connection secret"
	errFmtEmptyPassword    = "key %q of the secret of masterUserPasswordSecretRef is empty"
	errListTags            = "cannot list tags"
	errAddTags             = "cannot add tags"
	errRemoveTags          = "cannot remove tags"
	errUpgrade             = "cannot upgrade the engine version"
	errBuildUpdate         = "cannot build the update request"
)

// Fields of DomainParameters (JSON names) that isUpToDate compares by hand
// instead of with DiffUpdatableParameters, or in addition to it.
const (
	fieldAccessPolicies   = "accessPolicies"
	fieldAdvancedOptions  = "advancedOptions"
	fieldAdvancedSecurity = "advancedSecurityOptions"
	fieldAutoTune         = "autoTuneOptions"
	fieldEncryptionAtRest = "encryptionAtRestOptions"
	fieldLogPublishing    = "logPublishingOptions"
	fieldEngineVersion    = "engineVersion"
	fieldPassword         = "masterUserPasswordSecretRef"
	fieldTags             = "tags"
)

// hooks keeps what Observe found for Update: the reconciler calls Update in
// the same reconcile, on the same external client.
type hooks struct {
	client Client
	kube   client.Client

	// diff are the differing fields found by isUpToDate.
	diff []string
	// password is the one of masterUserPasswordSecretRef, passwordChanged
	// whether it differs from the one in the connection secret.
	password        string
	passwordChanged bool
	// tagsAdd and tagsRemove make the domain's tags equal the desired ones.
	tagsAdd    []svcsdktypes.Tag
	tagsRemove []string
}

func preObserve(_ context.Context, cr *svcapitypes.Domain, obj *svcsdk.DescribeDomainInput) error {
	obj.DomainName = aws.String(meta.GetExternalName(cr))
	return nil
}

func (h *hooks) postObserve(_ context.Context, cr *svcapitypes.Domain, resp *svcsdk.DescribeDomainOutput, obs managed.ExternalObservation, err error) (managed.ExternalObservation, error) {
	if err != nil {
		return obs, err
	}
	ds := resp.DomainStatus
	if cr.Status.AtProvider.Endpoint == nil {
		if ep, ok := ds.Endpoints[vpcEndpointKey]; ok {
			cr.Status.AtProvider.Endpoint = aws.String(ep)
		}
	}
	switch {
	case aws.ToBool(ds.Deleted):
		cr.SetConditions(xpv2.Deleting())
	case !aws.ToBool(ds.Created) || pointer.StringValue(cr.Status.AtProvider.Endpoint) == "":
		cr.SetConditions(xpv2.Creating())
	default:
		// Also while a configuration change is processed: the domain
		// keeps serving requests during blue/green deployments.
		cr.SetConditions(xpv2.Available())
	}
	obs.ConnectionDetails = connectionDetails(cr)
	// A changed password is published by postUpdate, once AWS has it.
	if h.password != "" && !h.passwordChanged {
		obs.ConnectionDetails[awsclient.ResourceCredentialsSecretPasswordKey] = []byte(h.password)
	}
	return obs, nil
}

// isUpToDate compares the desired spec with the domain. While AWS processes
// a change, the domain is up to date: AWS rejects further changes until
// it's done.
func (h *hooks) isUpToDate(ctx context.Context, cr *svcapitypes.Domain, resp *svcsdk.DescribeDomainOutput) (bool, string, error) {
	ds := resp.DomainStatus
	if !aws.ToBool(ds.Created) || aws.ToBool(ds.Deleted) || aws.ToBool(ds.Processing) || aws.ToBool(ds.UpgradeProcessing) {
		return true, "", nil
	}
	diff := diffConfig(&cr.Spec.ForProvider, resp)

	var err error
	if h.password, h.passwordChanged, err = h.getPasswordFromRef(ctx, cr); err != nil {
		return false, "", err
	}
	if h.passwordChanged {
		diff = append(diff, fieldPassword)
	}

	tags, err := h.client.ListTags(ctx, &svcsdk.ListTagsInput{ARN: ds.ARN})
	if err != nil {
		return false, "", awsclient.Wrap(err, errListTags)
	}
	h.tagsAdd, h.tagsRemove = diffTags(desiredTags(cr), tags.TagList)
	if len(h.tagsAdd)+len(h.tagsRemove) > 0 {
		diff = append(diff, fieldTags)
	}

	h.diff = diff
	return len(diff) == 0, compare.Message(diff), nil
}

// diffConfig returns the JSON names of the fields of desired that differ from
// the domain's configuration.
func diffConfig(desired *svcapitypes.DomainParameters, resp *svcsdk.DescribeDomainOutput) []string {
	ds := resp.DomainStatus
	observed := &GenerateDomain(resp).Spec.ForProvider
	diff := compare.Without(DiffUpdatableParameters(desired, observed),
		fieldAccessPolicies, fieldAdvancedOptions, fieldAdvancedSecurity, fieldAutoTune, fieldEncryptionAtRest, fieldLogPublishing)
	for _, f := range []struct {
		name     string
		upToDate bool
	}{
		{fieldAccessPolicies, policyEqual(desired.AccessPolicies, observed.AccessPolicies)},
		{fieldAdvancedOptions, mapSubset(desired.AdvancedOptions, observed.AdvancedOptions)},
		{fieldAdvancedSecurity, advancedSecurityUpToDate(desired.AdvancedSecurityOptions, observed.AdvancedSecurityOptions)},
		{fieldAutoTune, autoTuneUpToDate(desired.AutoTuneOptions, observed.AutoTuneOptions, ds.AutoTuneOptions)},
		{fieldEncryptionAtRest, encryptionUpToDate(desired.EncryptionAtRestOptions, observed.EncryptionAtRestOptions)},
		{fieldLogPublishing, mapSubset(desired.LogPublishingOptions, observed.LogPublishingOptions)},
		{fieldEngineVersion, desired.EngineVersion == nil || *desired.EngineVersion == pointer.StringValue(ds.EngineVersion)},
	} {
		if !f.upToDate {
			diff = append(diff, f.name)
		}
	}
	return diff
}

// policyEqual compares two IAM policy documents as JSON, where a single value
// equals a list with only that value. Documents that aren't JSON are compared
// as strings.
func policyEqual(desired, observed *string) bool {
	if desired == nil {
		return true
	}
	o := pointer.StringValue(observed)
	if *desired == o {
		return true
	}
	var d, ob any
	if json.Unmarshal([]byte(*desired), &d) != nil || json.Unmarshal([]byte(o), &ob) != nil {
		return false
	}
	return reflect.DeepEqual(normalizePolicy(d), normalizePolicy(ob))
}

func normalizePolicy(v any) any {
	switch t := v.(type) {
	case []any:
		if len(t) == 1 {
			return normalizePolicy(t[0])
		}
		for i := range t {
			t[i] = normalizePolicy(t[i])
		}
	case map[string]any:
		for k := range t {
			t[k] = normalizePolicy(t[k])
		}
	}
	return v
}

// mapSubset reports whether observed has the desired value of every desired
// key. AWS adds keys with default values (advancedOptions) and keeps log
// types that aren't desired (logPublishingOptions).
func mapSubset[V any](desired, observed map[string]V) bool {
	for k, v := range desired {
		ov, ok := observed[k]
		if !ok || !compare.IsSubset(v, ov) {
			return false
		}
	}
	return true
}

// advancedSecurityUpToDate compares fine-grained access control without the
// master user, which AWS doesn't return.
func advancedSecurityUpToDate(desired, observed *svcapitypes.AdvancedSecurityOptionsInput) bool {
	if desired == nil {
		return true
	}
	d := desired.DeepCopy()
	d.MasterUserOptions = nil
	if d.SAMLOptions != nil {
		d.SAMLOptions.MasterBackendRole = nil
		d.SAMLOptions.MasterUserName = nil
	}
	return compare.IsSubset(d, observed)
}

// autoTuneUpToDate compares the desired Auto-Tune state with the observed
// state. rollbackOnDisable and maintenanceSchedules aren't returned by
// DescribeDomain; they are sent with every update of autoTuneOptions.
func autoTuneUpToDate(desired, observed *svcapitypes.AutoTuneOptions, status *svcsdktypes.AutoTuneOptionsOutput) bool {
	if desired == nil {
		return true
	}
	d := desired.DeepCopy()
	d.DesiredState, d.RollbackOnDisable, d.MaintenanceSchedules = nil, nil, nil
	if observed == nil {
		observed = &svcapitypes.AutoTuneOptions{}
	}
	if !compare.IsSubset(d, observed) {
		return false
	}
	if desired.DesiredState == nil {
		return true
	}
	if status == nil {
		// Domains that don't support Auto-Tune (e.g. t3 instances) have
		// no Auto-Tune status.
		return *desired.DesiredState == string(svcsdktypes.AutoTuneDesiredStateDisabled)
	}
	switch status.State {
	case svcsdktypes.AutoTuneStateError:
		// Sending the desired state again doesn't fix an error; AWS
		// reports it in the domain's Auto-Tune error message.
		return true
	case svcsdktypes.AutoTuneStateEnabled, svcsdktypes.AutoTuneStateEnableInProgress:
		return *desired.DesiredState == string(svcsdktypes.AutoTuneDesiredStateEnabled)
	case svcsdktypes.AutoTuneStateDisabled, svcsdktypes.AutoTuneStateDisableInProgress,
		svcsdktypes.AutoTuneStateDisabledAndRollbackScheduled, svcsdktypes.AutoTuneStateDisabledAndRollbackInProgress,
		svcsdktypes.AutoTuneStateDisabledAndRollbackComplete, svcsdktypes.AutoTuneStateDisabledAndRollbackError:
		return *desired.DesiredState == string(svcsdktypes.AutoTuneDesiredStateDisabled)
	}
	// Unknown (newer) states: don't send updates that may be rejected.
	return true
}

// encryptionUpToDate compares encryption at rest; AWS returns the ARN of the
// KMS key, which matches a desired key ID.
func encryptionUpToDate(desired, observed *svcapitypes.EncryptionAtRestOptions) bool {
	if desired == nil {
		return true
	}
	d := desired.DeepCopy()
	d.KMSKeyID = nil
	if !compare.IsSubset(d, observed) {
		return false
	}
	if desired.KMSKeyID == nil {
		return true
	}
	return observed != nil && compare.KMSKeyIDEqual(*desired.KMSKeyID, pointer.StringValue(observed.KMSKeyID))
}

// desiredTags returns the spec tags plus the external tags Crossplane sets on
// every resource (crossplane-kind, crossplane-name, crossplane-providerconfig).
// Spec tags win.
func desiredTags(cr *svcapitypes.Domain) map[string]string {
	tags := cpresource.GetExternalTags(cr)
	for _, t := range cr.Spec.ForProvider.Tags {
		tags[pointer.StringValue(t.Key)] = pointer.StringValue(t.Value)
	}
	return tags
}

// diffTags returns the tags to add (new or changed values) and the keys to
// remove so that current equals desired. Tags reserved by AWS are left alone.
func diffTags(desired map[string]string, current []svcsdktypes.Tag) (add []svcsdktypes.Tag, remove []string) {
	cur := make(map[string]string, len(current))
	for _, t := range current {
		if k := pointer.StringValue(t.Key); !strings.HasPrefix(k, awsTagPrefix) {
			cur[k] = pointer.StringValue(t.Value)
		}
	}
	toAdd := map[string]string{}
	for k, v := range desired {
		if cv, ok := cur[k]; !ok || cv != v {
			toAdd[k] = v
		}
	}
	for k := range cur {
		if _, ok := desired[k]; !ok {
			remove = append(remove, k)
		}
	}
	sort.Strings(remove)
	return sdkTags(toAdd), remove
}

func sdkTags(tags map[string]string) []svcsdktypes.Tag {
	if len(tags) == 0 {
		return nil
	}
	res := make([]svcsdktypes.Tag, 0, len(tags))
	for k, v := range tags {
		res = append(res, svcsdktypes.Tag{Key: aws.String(k), Value: aws.String(v)})
	}
	sort.Slice(res, func(i, j int) bool { return *res[i].Key < *res[j].Key })
	return res
}

func (h *hooks) preCreate(ctx context.Context, cr *svcapitypes.Domain, obj *svcsdk.CreateDomainInput) error {
	obj.DomainName = aws.String(meta.GetExternalName(cr))
	obj.TagList = sdkTags(desiredTags(cr))

	// spec.autoTuneOptions has the shape of the update input.
	if cr.Spec.ForProvider.AutoTuneOptions != nil {
		u, err := GenerateUpdateDomainConfigInput(cr)
		if err != nil {
			return err
		}
		obj.AutoTuneOptions = &svcsdktypes.AutoTuneOptionsInput{
			DesiredState:         u.AutoTuneOptions.DesiredState,
			MaintenanceSchedules: u.AutoTuneOptions.MaintenanceSchedules,
			UseOffPeakWindow:     u.AutoTuneOptions.UseOffPeakWindow,
		}
	}

	pw, _, err := h.getPasswordFromRef(ctx, cr)
	if err != nil || pw == "" {
		return err
	}
	h.password = pw
	if obj.AdvancedSecurityOptions == nil {
		obj.AdvancedSecurityOptions = &svcsdktypes.AdvancedSecurityOptionsInput{}
	}
	if obj.AdvancedSecurityOptions.MasterUserOptions == nil {
		obj.AdvancedSecurityOptions.MasterUserOptions = &svcsdktypes.MasterUserOptions{}
	}
	obj.AdvancedSecurityOptions.MasterUserOptions.MasterUserPassword = aws.String(pw)
	return nil
}

func (h *hooks) postCreate(_ context.Context, cr *svcapitypes.Domain, _ *svcsdk.CreateDomainOutput, cre managed.ExternalCreation, err error) (managed.ExternalCreation, error) {
	if err != nil {
		return cre, err
	}
	cre.ConnectionDetails = connectionDetails(cr)
	if h.password != "" {
		cre.ConnectionDetails[awsclient.ResourceCredentialsSecretPasswordKey] = []byte(h.password)
	}
	return cre, nil
}

// preUpdate sends only the differing fields: AWS applies every field it
// gets, and some (the master user) need values that aren't in the spec.
func (h *hooks) preUpdate(_ context.Context, cr *svcapitypes.Domain, obj *svcsdk.UpdateDomainConfigInput) error {
	fields := h.diff
	if h.passwordChanged {
		fields = append(slices.Clone(fields), fieldAdvancedSecurity)
	}
	p, err := onlyFields(cr.Spec.ForProvider, fields)
	if err != nil {
		return errors.Wrap(err, errBuildUpdate)
	}
	only := cr.DeepCopy()
	only.Spec.ForProvider = p
	in, err := GenerateUpdateDomainConfigInput(only)
	if err != nil {
		return errors.Wrap(err, errBuildUpdate)
	}
	*obj = *in //nolint:govet // copies the SDK's unexported serde marker, which has no state
	obj.DomainName = aws.String(meta.GetExternalName(cr))
	if aso := obj.AdvancedSecurityOptions; aso != nil {
		if !h.passwordChanged {
			// The master user can only be sent with its password.
			aso.MasterUserOptions = nil
		} else {
			if aso.MasterUserOptions == nil {
				aso.MasterUserOptions = &svcsdktypes.MasterUserOptions{}
			}
			aso.MasterUserOptions.MasterUserPassword = aws.String(h.password)
		}
	}
	return nil
}

// onlyFields returns the fields of p with the given JSON names.
func onlyFields(p svcapitypes.DomainParameters, names []string) (svcapitypes.DomainParameters, error) {
	var out svcapitypes.DomainParameters
	b, err := json.Marshal(p)
	if err != nil {
		return out, err
	}
	all := map[string]json.RawMessage{}
	if err := json.Unmarshal(b, &all); err != nil {
		return out, err
	}
	keep := map[string]json.RawMessage{}
	for _, n := range names {
		if v, ok := all[n]; ok {
			keep[n] = v
		}
	}
	if b, err = json.Marshal(keep); err != nil {
		return out, err
	}
	return out, json.Unmarshal(b, &out)
}

func (h *hooks) postUpdate(_ context.Context, cr *svcapitypes.Domain, _ *svcsdk.UpdateDomainConfigOutput, upd managed.ExternalUpdate, err error) (managed.ExternalUpdate, error) {
	if err != nil || !h.passwordChanged {
		return upd, err
	}
	upd.ConnectionDetails = connectionDetails(cr)
	upd.ConnectionDetails[awsclient.ResourceCredentialsSecretPasswordKey] = []byte(h.password)
	return upd, nil
}

func preDelete(_ context.Context, cr *svcapitypes.Domain, obj *svcsdk.DeleteDomainInput) (bool, error) {
	obj.DomainName = aws.String(meta.GetExternalName(cr))
	return aws.ToBool(cr.Status.AtProvider.Deleted), nil
}

// connectionDetails returns the endpoint, port and master user name.
func connectionDetails(cr *svcapitypes.Domain) managed.ConnectionDetails {
	cd := managed.ConnectionDetails{
		awsclient.ResourceCredentialsSecretEndpointKey: []byte(pointer.StringValue(cr.Status.AtProvider.Endpoint)),
		awsclient.ResourceCredentialsSecretPortKey:     []byte(port),
	}
	if aso := cr.Spec.ForProvider.AdvancedSecurityOptions; aso != nil && aso.MasterUserOptions != nil && aso.MasterUserOptions.MasterUserName != nil {
		cd[awsclient.ResourceCredentialsSecretUserKey] = []byte(*aso.MasterUserOptions.MasterUserName)
	}
	return cd
}

// getPasswordFromRef returns the password of masterUserPasswordSecretRef and
// whether it differs from the one in the connection secret. Both secrets are
// in the namespace of cr. Without a connection secret, a change can't be
// detected.
func (h *hooks) getPasswordFromRef(ctx context.Context, cr *svcapitypes.Domain) (string, bool, error) {
	ref := cr.Spec.ForProvider.MasterUserPasswordSecretRef
	if ref == nil {
		return "", false, nil
	}
	s := &corev1.Secret{}
	if err := h.kube.Get(ctx, types.NamespacedName{Namespace: cr.GetNamespace(), Name: ref.Name}, s); err != nil {
		return "", false, errors.Wrap(err, errGetPasswordSecret)
	}
	pw := string(s.Data[ref.Key])
	if pw == "" {
		return "", false, errors.Errorf(errFmtEmptyPassword, ref.Key)
	}
	out := cr.Spec.WriteConnectionSecretToReference
	if out == nil {
		return pw, false, nil
	}
	cs := &corev1.Secret{}
	if err := h.kube.Get(ctx, types.NamespacedName{Namespace: cr.GetNamespace(), Name: out.Name}, cs); cpresource.IgnoreNotFound(err) != nil {
		return "", false, errors.Wrap(err, errGetConnectionSecret)
	}
	return pw, pw != string(cs.Data[awsclient.ResourceCredentialsSecretPasswordKey]), nil
}

// domainExternal updates tags and the engine version, which
// UpdateDomainConfig can't change.
type domainExternal struct {
	*external
	hooks *hooks
}

// Update makes the tags equal the desired ones, then sends either the
// configuration changes or, once there are none, the engine version
// upgrade: AWS processes one change at a time.
func (e *domainExternal) Update(ctx context.Context, cr *svcapitypes.Domain) (managed.ExternalUpdate, error) {
	h := e.hooks
	arn := cr.Status.AtProvider.ARN
	if len(h.tagsRemove) > 0 {
		if _, err := h.client.RemoveTags(ctx, &svcsdk.RemoveTagsInput{ARN: arn, TagKeys: h.tagsRemove}); err != nil {
			return managed.ExternalUpdate{}, awsclient.Wrap(err, errRemoveTags)
		}
	}
	if len(h.tagsAdd) > 0 {
		if _, err := h.client.AddTags(ctx, &svcsdk.AddTagsInput{ARN: arn, TagList: h.tagsAdd}); err != nil {
			return managed.ExternalUpdate{}, awsclient.Wrap(err, errAddTags)
		}
	}
	if len(compare.Without(slices.Clone(h.diff), fieldTags, fieldEngineVersion)) > 0 {
		return e.external.Update(ctx, cr)
	}
	if slices.Contains(h.diff, fieldEngineVersion) {
		_, err := h.client.UpgradeDomain(ctx, &svcsdk.UpgradeDomainInput{
			DomainName:    aws.String(meta.GetExternalName(cr)),
			TargetVersion: cr.Spec.ForProvider.EngineVersion,
		})
		return managed.ExternalUpdate{}, awsclient.Wrap(err, errUpgrade)
	}
	return managed.ExternalUpdate{}, nil
}

func wrap(e *external) managed.TypedExternalClient[*svcapitypes.Domain] {
	h := &hooks{client: e.client, kube: e.kube}
	e.preObserve = preObserve
	e.postObserve = h.postObserve
	e.isUpToDate = h.isUpToDate
	e.preCreate = h.preCreate
	e.postCreate = h.postCreate
	e.preUpdate = h.preUpdate
	e.postUpdate = h.postUpdate
	e.preDelete = preDelete
	return &domainExternal{external: e, hooks: h}
}

func init() {
	wrapExternal = wrap
}
