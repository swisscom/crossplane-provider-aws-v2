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

package v1alpha1

import (
	"context"

	"github.com/crossplane/crossplane-runtime/v2/pkg/errors"
	"github.com/crossplane/crossplane-runtime/v2/pkg/reference"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// ResolveReferences of this DBCluster
func (mg *DBCluster) ResolveReferences(ctx context.Context, c client.Reader) error {
	r := reference.NewAPINamespacedResolver(c, mg)

	// Resolve spec.forProvider.dbClusterParameterGroupName
	rsp, err := r.Resolve(ctx, reference.NamespacedResolutionRequest{
		CurrentValue: reference.FromPtrValue(mg.Spec.ForProvider.DBClusterParameterGroupName),
		Reference:    mg.Spec.ForProvider.DBClusterParameterGroupNameRef,
		Selector:     mg.Spec.ForProvider.DBClusterParameterGroupNameSelector,
		To:           reference.To{Managed: &DBClusterParameterGroup{}, List: &DBClusterParameterGroupList{}},
		Extract:      reference.ExternalName(),
		Namespace:    mg.GetNamespace()})
	if err != nil {
		return errors.Wrap(err, "spec.forProvider.dbClusterParameterGroupName")
	}
	mg.Spec.ForProvider.DBClusterParameterGroupName = reference.ToPtrValue(rsp.ResolvedValue)
	mg.Spec.ForProvider.DBClusterParameterGroupNameRef = rsp.ResolvedReference

	// Resolve spec.forProvider.dbSubnetGroupName
	rsp, err = r.Resolve(ctx, reference.NamespacedResolutionRequest{
		CurrentValue: reference.FromPtrValue(mg.Spec.ForProvider.DBSubnetGroupName),
		Reference:    mg.Spec.ForProvider.DBSubnetGroupNameRef,
		Selector:     mg.Spec.ForProvider.DBSubnetGroupNameSelector,
		To:           reference.To{Managed: &DBSubnetGroup{}, List: &DBSubnetGroupList{}},
		Extract:      reference.ExternalName(),
		Namespace:    mg.GetNamespace()})
	if err != nil {
		return errors.Wrap(err, "spec.forProvider.dbSubnetGroupName")
	}
	mg.Spec.ForProvider.DBSubnetGroupName = reference.ToPtrValue(rsp.ResolvedValue)
	mg.Spec.ForProvider.DBSubnetGroupNameRef = rsp.ResolvedReference

	return nil
}
