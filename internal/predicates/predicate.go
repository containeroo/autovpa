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

package predicates

import (
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
)

// ProfileAnnotationLifecycle returns a predicate that reacts to lifecycle events
// relevant to the operator’s profile annotation on workload resources
// (Deployments, StatefulSets, DaemonSets).
//
// This predicate determines *when a workload should be reconciled* based on
// opt-in semantics controlled via an annotation.
//
// Semantics:
//   - Create: enqueue only if the workload is opted-in
//     (annotation present and non-empty).
//   - Update: enqueue if:
//   - opt-in was added or removed,
//   - the profile value changed, or
//   - deletion has just started (for cleanup).
//   - Delete: enqueue only if the workload was opted-in, so managed VPAs
//     can be cleaned up.
//   - Generic: disabled to avoid noisy resyncs.
func ProfileAnnotationLifecycle(annotation string) predicate.Predicate {
	return predicate.Funcs{
		CreateFunc: func(e event.CreateEvent) bool {
			return hasNonEmptyAnnotation(e.Object, annotation)
		},

		UpdateFunc: func(e event.UpdateEvent) bool {
			oldVal, oldHas := annotationValue(e.ObjectOld, annotation)
			newVal, newHas := annotationValue(e.ObjectNew, annotation)

			// Opt-in added or removed.
			if oldHas != newHas {
				return true
			}

			// Not opted-in → ignore everything else.
			if !newHas {
				return false
			}

			// Profile value changed (e.g. "gold" → "silver").
			if oldVal != newVal {
				return true
			}

			// Deletion started → allow cleanup.
			if deletionJustStarted(e.ObjectOld, e.ObjectNew) {
				return true
			}

			return false
		},

		DeleteFunc: func(e event.DeleteEvent) bool {
			return hasNonEmptyAnnotation(e.Object, annotation)
		},

		GenericFunc: func(event.GenericEvent) bool {
			return false
		},
	}
}

// ManagedVPAStructuralLifecycle returns a predicate that reacts only to
// *structural lifecycle events* of managed VPAs.
//
// This predicate is intended for the VPAReconciler, which acts as a
// safety net and enforces ownership correctness.
//
// It deliberately ignores spec, label drift, and status churn.
//
// Semantics:
//   - Create: enqueue only if the VPA is managed (label == "true").
//   - Update: enqueue if:
//   - managed label toggled,
//   - deletion started, or
//   - controller ownerRef changed.
//   - Delete: enqueue only if the deleted VPA was managed.
//   - Generic: disabled to avoid noisy resyncs.
func ManagedVPAStructuralLifecycle(managedLabel string) predicate.Predicate {
	return predicate.Funcs{
		CreateFunc: func(e event.CreateEvent) bool {
			return hasTrueLabel(e.Object, managedLabel)
		},

		UpdateFunc: func(e event.UpdateEvent) bool {
			oldHas := hasTrueLabel(e.ObjectOld, managedLabel)
			newHas := hasTrueLabel(e.ObjectNew, managedLabel)

			// Managed label toggled.
			if oldHas != newHas {
				return true
			}

			// Unmanaged → ignore everything else.
			if !newHas {
				return false
			}

			// Deletion started.
			if deletionJustStarted(e.ObjectOld, e.ObjectNew) {
				return true
			}

			// Controller owner changed/added/removed → orphan detection / safety net.
			if controllerOwnerRefChanged(e.ObjectOld, e.ObjectNew) {
				return true
			}

			return false
		},

		DeleteFunc: func(e event.DeleteEvent) bool {
			return hasTrueLabel(e.Object, managedLabel)
		},

		GenericFunc: func(event.GenericEvent) bool {
			return false
		},
	}
}

// ManagedVPALifecycle returns a predicate that requeues the owning workload
// when a managed VPA diverges from the operator’s desired state.
//
// This predicate is intended for workload reconcilers (e.g. DeploymentReconciler)
// to provide “snap-back” behavior when users manually tamper with managed VPAs.
//
// It reacts to *operator-owned drift*, not arbitrary churn.
//
// Semantics:
//   - Create: enqueue only if the VPA is managed.
//   - Update: enqueue if:
//   - managed label toggled,
//   - deletion started,
//   - controller ownerRef changed,
//   - operator-owned labels changed (managed/profile), or
//   - spec changed.
//   - Delete: enqueue only if the deleted VPA was managed.
//   - Generic: disabled to avoid noisy resyncs.
func ManagedVPALifecycle(managedLabel, profileKey string) predicate.Predicate {
	return predicate.Funcs{
		CreateFunc: func(e event.CreateEvent) bool {
			return hasTrueLabel(e.Object, managedLabel)
		},

		UpdateFunc: func(e event.UpdateEvent) bool {
			oldHas := hasTrueLabel(e.ObjectOld, managedLabel)
			newHas := hasTrueLabel(e.ObjectNew, managedLabel)

			// Managed label toggled.
			if oldHas != newHas {
				return true
			}

			// Unmanaged → ignore everything else.
			if !newHas {
				return false
			}

			// Deletion started → allow the owner reconciler to react quickly.
			if deletionJustStarted(e.ObjectOld, e.ObjectNew) {
				return true
			}

			// Controller ownership changed.
			if controllerOwnerRefChanged(e.ObjectOld, e.ObjectNew) {
				return true
			}

			// Operator-owned label drift (managed/profile).
			if operatorLabelsChanged(e.ObjectOld, e.ObjectNew, managedLabel, profileKey) {
				return true
			}

			// Spec drift.
			if specChanged(e.ObjectOld, e.ObjectNew) {
				return true
			}

			return false
		},

		DeleteFunc: func(e event.DeleteEvent) bool {
			return hasTrueLabel(e.Object, managedLabel)
		},

		GenericFunc: func(event.GenericEvent) bool {
			return false
		},
	}
}
