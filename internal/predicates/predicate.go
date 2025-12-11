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

// ProfileAnnotationLifecycle returns a predicate that reacts to
// add/remove/delete lifecycle events of the operator’s profile-annotation
// on workload resources.
//
// Semantics:
//   - Create:  only if the annotation exists (workload opts in).
//   - Update:  if the annotation was added/removed OR still present (opt-in/out transitions and opted-in updates).
//   - Delete:  if the annotation existed (final cleanup for deleted workload).
//   - Generic: disabled to avoid noisy resyncs.
func ProfileAnnotationLifecycle(annotation string) predicate.Predicate {
	return predicate.Funcs{
		CreateFunc: func(e event.CreateEvent) bool {
			return hasAnnotation(e.Object, annotation)
		},
		UpdateFunc: func(e event.UpdateEvent) bool {
			oldHas := hasAnnotation(e.ObjectOld, annotation)
			newHas := hasAnnotation(e.ObjectNew, annotation)
			return oldHas != newHas || newHas
		},
		DeleteFunc: func(e event.DeleteEvent) bool {
			return hasAnnotation(e.Object, annotation)
		},
		GenericFunc: func(event.GenericEvent) bool {
			return false
		},
	}
}

// ManagedVPALifecycle returns a predicate that reacts to
// add/remove/delete lifecycle events of the operator’s managed-label
// on VerticalPodAutoscaler objects.
//
// Semantics:
//   - Create:  only if the managed label exists (we reconcile only managed VPAs).
//   - Update:  if the label was added or removed, or if the new object still has it;
//     this ensures the controller observes a label-removal event once.
//   - Delete:  only if the deleted VPA had the label (cleanup for managed VPAs).
//   - Generic: disabled to avoid noisy resyncs.
func ManagedVPALifecycle(label string) predicate.Predicate {
	return predicate.Funcs{
		CreateFunc: func(e event.CreateEvent) bool {
			return hasLabel(e.Object, label)
		},
		UpdateFunc: func(e event.UpdateEvent) bool {
			oldHas := hasLabel(e.ObjectOld, label)
			newHas := hasLabel(e.ObjectNew, label)
			return oldHas != newHas || newHas
		},
		DeleteFunc: func(e event.DeleteEvent) bool {
			return hasLabel(e.Object, label)
		},
		GenericFunc: func(event.GenericEvent) bool {
			return false
		},
	}
}
