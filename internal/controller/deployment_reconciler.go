/*
Copyright 2025 containeroo.ch

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

package controller

import (
	"context"
	"errors"

	"github.com/containeroo/autovpa/internal/predicates"

	appsv1 "k8s.io/api/apps/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

// DeploymentReconciler reconciles Deployments and manages their VPAs.
//
// Its responsibilities are:
//
//   - Drive the desired VPA state for Deployments by delegating to
//     BaseReconciler.ReconcileWorkload.
//   - Perform cleanup of managed VPAs when a Deployment is deleted.
//
// Actual VPA content (name, labels, spec) is resolved by the shared
// BaseReconciler logic; this controller only wires that logic to the
// Deployment API type.
type DeploymentReconciler struct {
	BaseReconciler
}

// Reconcile ensures that the Deployment's opted-in state (profile annotation)
// is reflected in its managed VPAs.
//
// High-level flow:
//
//  1. Try to load the Deployment.
//     - If it does not exist anymore, proactively delete any managed VPAs
//     that still point at this Deployment (best-effort cleanup).
//  2. If it exists, delegate to ReconcileWorkload to create/update/delete
//     the associated VPA based on the selected profile.
func (r *DeploymentReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	// Fetch the current Deployment object from the cache/API server.
	dep := &appsv1.Deployment{}
	if err := r.KubeClient.Get(ctx, req.NamespacedName, dep); err != nil {
		if apierrors.IsNotFound(err) {
			// The Deployment has been deleted. We may still have managed VPAs
			// with an ownerRef pointing at this name/namespace; clean them up.
			logger.Info("Deployment not found; cleaning managed VPAs if any")

			if err := r.DeleteManagedVPAsForGoneWorkload(
				ctx,
				&appsv1.Deployment{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: req.Namespace,
						Name:      req.Name,
					},
				},
				DeploymentGVK.Kind,
			); err != nil {
				return ctrl.Result{}, err
			}
			return ctrl.Result{}, nil
		}

		// Any non-NotFound error should be retried by controller-runtime.
		return ctrl.Result{}, errors.New("failed to fetch Deployment")
	}

	// Deployment exists: reconcile its VPA according to the selected profile.
	return r.ReconcileWorkload(ctx, dep, DeploymentGVK)
}

// SetupWithManager wires the Deployment controller into the manager.
//
//   - Deployment events are filtered by the profile annotation lifecycle.
//   - Owned VPA events are filtered by ManagedVPALifecycle, so spec/label drift
//     requeues the owning Deployment ("snap back" behavior) while still ignoring
//     status churn.
func (r *DeploymentReconciler) SetupWithManager(mgr ctrl.Manager) error {
	vpa := newVPAObject()

	return ctrl.NewControllerManagedBy(mgr).
		// Primary resource: only react when the profile annotation is added/removed/present.
		For(&appsv1.Deployment{}, builder.WithPredicates(
			predicates.ProfileAnnotationLifecycle(r.Meta.ProfileKey),
		)).
		// Secondary resource: any change to a managed VPA should requeue the owner.
		// We use a label-based predicate here so only VPAs with the managed label
		// generate events for this controller.
		Owns(vpa, builder.WithPredicates(
			predicates.ManagedVPALifecycle(r.Meta.ManagedLabel, r.Meta.ProfileKey),
		)).
		Complete(r)
}
