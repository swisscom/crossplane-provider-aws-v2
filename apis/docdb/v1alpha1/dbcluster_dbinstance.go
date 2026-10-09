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

// ResolveReferences of this DBInstance
func (mg *DBInstance) ResolveReferences(ctx context.Context, c client.Reader) error {
	r := reference.NewAPINamespacedResolver(c, mg)

	// Resolve spec.forProvider.dbClusterIdentifier
	rsp, err := r.Resolve(ctx, reference.NamespacedResolutionRequest{
		CurrentValue: reference.FromPtrValue(mg.Spec.ForProvider.DBClusterIdentifier),
		Reference:    mg.Spec.ForProvider.DBClusterIdentifierRef,
		Selector:     mg.Spec.ForProvider.DBClusterIdentifierSelector,
		To:           reference.To{Managed: &DBCluster{}, List: &DBClusterList{}},
		Extract:      reference.ExternalName(),
		Namespace:    mg.GetNamespace()})
	if err != nil {
		return errors.Wrap(err, "spec.forProvider.dbClusterIdentifier")
	}
	mg.Spec.ForProvider.DBClusterIdentifier = reference.ToPtrValue(rsp.ResolvedValue)
	mg.Spec.ForProvider.DBClusterIdentifierRef = rsp.ResolvedReference

	return nil
}
