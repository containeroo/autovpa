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

// AnnotationLifecycle returns a predicate that watches annotation add/remove/delete transitions.
// - Create:  only if the annotation exists (to create VPAs).
// - Update:  if the annotation was added or removed (to create or clean up VPAs).
// - Delete:  if the annotation existed (to clean up VPAs for deleted workloads).
// - Generic: is disabled to avoid noisy resyncs; we only care about actual object changes.
func AnnotationLifecycle(annotation string) predicate.Predicate {
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
