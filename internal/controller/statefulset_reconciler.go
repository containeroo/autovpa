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

// StatefulSetReconciler reconciles StatefulSets and manages their VPAs.
//
// Its responsibilities are:
//
//   - Drive the desired VPA state for StatefulSets by delegating to
//     BaseReconciler.ReconcileWorkload.
//   - Perform cleanup of managed VPAs when a StatefulSet is deleted.
//
// Actual VPA content (name, labels, spec) is resolved by the shared
// BaseReconciler logic; this controller only wires that logic to the
// StatefulSet API type.
type StatefulSetReconciler struct {
	BaseReconciler
}

// Reconcile ensures that the StatefulSet's opted-in state (profile annotation)
// is reflected in its managed VPAs.
//
// High-level flow:
//
//  1. Try to load the StatefulSet.
//     - If it does not exist anymore, proactively delete any managed VPAs
//     that still point at this StatefulSet (best-effort cleanup).
//  2. If it exists, delegate to ReconcileWorkload to create/update/delete
//     the associated VPA based on the selected profile.
func (r *StatefulSetReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	// Fetch the current StatefulSet object from the cache/API server.
	dep := &appsv1.StatefulSet{}
	if err := r.KubeClient.Get(ctx, req.NamespacedName, dep); err != nil {
		if apierrors.IsNotFound(err) {
			// The StatefulSet has been deleted. We may still have managed VPAs
			// with an ownerRef pointing at this name/namespace; clean them up.
			logger.Info("StatefulSet not found; cleaning managed VPAs if any")

			if err := r.DeleteManagedVPAsForGoneWorkload(
				ctx,
				&appsv1.StatefulSet{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: req.Namespace,
						Name:      req.Name,
					},
				},
				StatefulSetGVK.Kind,
			); err != nil {
				return ctrl.Result{}, err
			}
			return ctrl.Result{}, nil
		}

		// Any non-NotFound error should be retried by controller-runtime.
		return ctrl.Result{}, errors.New("failed to fetch StatefulSet")
	}

	// StatefulSet exists: reconcile its VPA according to the selected profile.
	return r.ReconcileWorkload(ctx, dep, StatefulSetGVK)
}

// SetupWithManager wires the StatefulSet controller into the manager.
//
//   - StatefulSet events are filtered by the profile annotation lifecycle.
//   - Owned VPA events are filtered by ManagedVPALifecycle, so spec/label drift
//     requeues the owning StatefulSet ("snap back" behavior) while still ignoring
//     status churn.
func (r *StatefulSetReconciler) SetupWithManager(mgr ctrl.Manager) error {
	vpa := newVPAObject()

	return ctrl.NewControllerManagedBy(mgr).
		// Primary resource: only react when the profile annotation is added/removed/present.
		For(&appsv1.StatefulSet{}, builder.WithPredicates(
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
