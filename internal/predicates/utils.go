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
	"reflect"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// annotationValue returns the annotation value and whether it is considered "present".
// "Present" means the key exists AND the value is non-empty (matches controller opt-in logic).
func annotationValue(obj client.Object, key string) (value string, present bool) {
	if obj == nil {
		return "", false
	}
	ann := obj.GetAnnotations()
	if ann == nil {
		return "", false
	}
	v, ok := ann[key]
	if !ok || v == "" {
		return "", false
	}
	return v, true
}

// hasNonEmptyAnnotation returns true if obj contains the annotation key with a non-empty value.
func hasNonEmptyAnnotation(obj client.Object, key string) bool {
	_, ok := annotationValue(obj, key)
	return ok
}

// hasTrueLabel returns true if obj contains the label key with value "true".
// This matches controller behavior where "managed" is label == "true", not just presence.
func hasTrueLabel(obj client.Object, key string) bool {
	if obj == nil {
		return false
	}
	labels := obj.GetLabels()
	if labels == nil {
		return false
	}
	return labels[key] == "true"
}

// deletionJustStarted returns true if deletion was requested on the new object
// but not on the old one.
func deletionJustStarted(oldObj, newObj client.Object) bool {
	return oldObj.GetDeletionTimestamp().IsZero() &&
		!newObj.GetDeletionTimestamp().IsZero()
}

// controllerOwnerRef returns the single controller ownerRef (controller=true),
// or nil if none exists.
func controllerOwnerRef(obj client.Object) *metav1.OwnerReference {
	if obj == nil {
		return nil
	}
	for _, ref := range obj.GetOwnerReferences() {
		if ref.Controller != nil && *ref.Controller {
			r := ref // copy so the pointer is stable
			return &r
		}
	}
	return nil
}

// controllerOwnerRefChanged returns true if the controller ownerRef changed
// (including added/removed).
func controllerOwnerRefChanged(oldObj, newObj client.Object) bool {
	return !reflect.DeepEqual(controllerOwnerRef(oldObj), controllerOwnerRef(newObj))
}

// unstructuredObject attempts to cast obj to *unstructured.Unstructured.
func unstructuredObject(obj client.Object) (*unstructured.Unstructured, bool) {
	u, ok := obj.(*unstructured.Unstructured)
	return u, ok
}

// specChanged returns true if the unstructured "spec" field changed.
// If the objects are not unstructured, it returns false (conservative).
func specChanged(oldObj, newObj client.Object) bool {
	oldU, ok1 := unstructuredObject(oldObj)
	newU, ok2 := unstructuredObject(newObj)
	if !ok1 || !ok2 {
		return false
	}

	return !reflect.DeepEqual(oldU.Object["spec"], newU.Object["spec"])
}

// operatorLabelsChanged returns true if any operator-owned labels differ.
// This avoids requeueing on user-added labels while still allowing “snap back”.
func operatorLabelsChanged(oldObj, newObj client.Object, keys ...string) bool {
	oldL := oldObj.GetLabels()
	newL := newObj.GetLabels()

	for _, k := range keys {
		if oldL[k] != newL[k] {
			return true
		}
	}
	return false
}
