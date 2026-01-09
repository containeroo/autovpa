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
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func TestAnnotationValue(t *testing.T) {
	t.Parallel()

	t.Run("Returns not present on nil object", func(t *testing.T) {
		t.Parallel()
		val, ok := annotationValue(nil, "a")
		assert.Equal(t, "", val)
		assert.False(t, ok)
	})

	t.Run("Returns not present when annotations are nil", func(t *testing.T) {
		t.Parallel()
		obj := &unstructured.Unstructured{}
		val, ok := annotationValue(obj, "a")
		assert.Equal(t, "", val)
		assert.False(t, ok)
	})

	t.Run("Returns not present when annotation missing", func(t *testing.T) {
		t.Parallel()
		obj := &unstructured.Unstructured{}
		obj.SetAnnotations(map[string]string{"b": "c"})
		val, ok := annotationValue(obj, "a")
		assert.Equal(t, "", val)
		assert.False(t, ok)
	})

	t.Run("Returns not present when annotation value is empty", func(t *testing.T) {
		t.Parallel()
		obj := &unstructured.Unstructured{}
		obj.SetAnnotations(map[string]string{"a": ""})
		val, ok := annotationValue(obj, "a")
		assert.Equal(t, "", val)
		assert.False(t, ok)
	})

	t.Run("Returns present when annotation exists and non-empty", func(t *testing.T) {
		t.Parallel()
		obj := &unstructured.Unstructured{}
		obj.SetAnnotations(map[string]string{"a": "b"})
		val, ok := annotationValue(obj, "a")
		assert.Equal(t, "b", val)
		assert.True(t, ok)
	})
}

func TestHasNonEmptyAnnotation(t *testing.T) {
	t.Parallel()

	t.Run("Returns false on nil object", func(t *testing.T) {
		t.Parallel()
		assert.False(t, hasNonEmptyAnnotation(nil, "a"))
	})

	t.Run("Returns false when annotation missing", func(t *testing.T) {
		t.Parallel()
		obj := &unstructured.Unstructured{}
		obj.SetAnnotations(map[string]string{"b": "c"})
		assert.False(t, hasNonEmptyAnnotation(obj, "a"))
	})

	t.Run("Returns false when annotation exists but empty", func(t *testing.T) {
		t.Parallel()
		obj := &unstructured.Unstructured{}
		obj.SetAnnotations(map[string]string{"a": ""})
		assert.False(t, hasNonEmptyAnnotation(obj, "a"))
	})

	t.Run("Returns true when annotation exists and non-empty", func(t *testing.T) {
		t.Parallel()
		obj := &unstructured.Unstructured{}
		obj.SetAnnotations(map[string]string{"a": "b"})
		assert.True(t, hasNonEmptyAnnotation(obj, "a"))
	})
}

func TestHasTrueLabel(t *testing.T) {
	t.Parallel()

	t.Run("Returns false on nil object", func(t *testing.T) {
		t.Parallel()
		assert.False(t, hasTrueLabel(nil, "managed"))
	})

	t.Run("Returns false when labels are nil", func(t *testing.T) {
		t.Parallel()
		obj := &unstructured.Unstructured{}
		assert.False(t, hasTrueLabel(obj, "managed"))
	})

	t.Run("Returns false when label missing", func(t *testing.T) {
		t.Parallel()
		obj := &unstructured.Unstructured{}
		obj.SetLabels(map[string]string{"other": "true"})
		assert.False(t, hasTrueLabel(obj, "managed"))
	})

	t.Run("Returns false when label exists but not true", func(t *testing.T) {
		t.Parallel()
		obj := &unstructured.Unstructured{}
		obj.SetLabels(map[string]string{"managed": "false"})
		assert.False(t, hasTrueLabel(obj, "managed"))
	})

	t.Run(`Returns true when label value is "true"`, func(t *testing.T) {
		t.Parallel()
		obj := &unstructured.Unstructured{}
		obj.SetLabels(map[string]string{"managed": "true"})
		assert.True(t, hasTrueLabel(obj, "managed"))
	})
}

func TestDeletionJustStarted(t *testing.T) {
	t.Parallel()

	t.Run("Returns false when neither object is deleting", func(t *testing.T) {
		t.Parallel()
		oldObj := &unstructured.Unstructured{}
		newObj := &unstructured.Unstructured{}
		assert.False(t, deletionJustStarted(oldObj, newObj))
	})

	t.Run("Returns true when deletion timestamp appears", func(t *testing.T) {
		t.Parallel()
		oldObj := &unstructured.Unstructured{}
		newObj := &unstructured.Unstructured{}
		now := metav1.NewTime(time.Now())
		newObj.SetDeletionTimestamp(&now)

		assert.True(t, deletionJustStarted(oldObj, newObj))
	})

	t.Run("Returns false when both already have deletion timestamp", func(t *testing.T) {
		t.Parallel()
		oldObj := &unstructured.Unstructured{}
		newObj := &unstructured.Unstructured{}
		now := metav1.NewTime(time.Now())
		oldObj.SetDeletionTimestamp(&now)
		newObj.SetDeletionTimestamp(&now)

		assert.False(t, deletionJustStarted(oldObj, newObj))
	})
}

func TestControllerOwnerRef(t *testing.T) {
	t.Parallel()

	t.Run("Returns nil on nil object", func(t *testing.T) {
		t.Parallel()
		assert.Nil(t, controllerOwnerRef(nil))
	})

	t.Run("Returns nil when no ownerRefs exist", func(t *testing.T) {
		t.Parallel()
		obj := &unstructured.Unstructured{}
		assert.Nil(t, controllerOwnerRef(obj))
	})

	t.Run("Returns nil when only non-controller ownerRefs exist", func(t *testing.T) {
		t.Parallel()
		obj := &unstructured.Unstructured{}
		f := false
		obj.SetOwnerReferences([]metav1.OwnerReference{
			{Kind: "Deployment", Name: "a", Controller: &f},
		})
		assert.Nil(t, controllerOwnerRef(obj))
	})

	t.Run("Returns controller ownerRef when present", func(t *testing.T) {
		t.Parallel()
		obj := &unstructured.Unstructured{}
		tval := true
		obj.SetOwnerReferences([]metav1.OwnerReference{
			{Kind: "ReplicaSet", Name: "rs1"},
			{Kind: "Deployment", Name: "dep1", Controller: &tval},
		})

		ref := controllerOwnerRef(obj)
		if assert.NotNil(t, ref) {
			assert.Equal(t, "Deployment", ref.Kind)
			assert.Equal(t, "dep1", ref.Name)
			assert.NotNil(t, ref.Controller)
			assert.True(t, *ref.Controller)
		}
	})
}

func TestControllerOwnerRefChanged(t *testing.T) {
	t.Parallel()

	t.Run("Returns false when both have no controller ownerRef", func(t *testing.T) {
		t.Parallel()
		oldObj := &unstructured.Unstructured{}
		newObj := &unstructured.Unstructured{}
		assert.False(t, controllerOwnerRefChanged(oldObj, newObj))
	})

	t.Run("Returns true when controller ownerRef is added", func(t *testing.T) {
		t.Parallel()
		oldObj := &unstructured.Unstructured{}
		newObj := &unstructured.Unstructured{}
		tval := true
		newObj.SetOwnerReferences([]metav1.OwnerReference{
			{Kind: "Deployment", Name: "dep1", Controller: &tval},
		})

		assert.True(t, controllerOwnerRefChanged(oldObj, newObj))
	})

	t.Run("Returns true when controller ownerRef is removed", func(t *testing.T) {
		t.Parallel()
		oldObj := &unstructured.Unstructured{}
		newObj := &unstructured.Unstructured{}
		tval := true
		oldObj.SetOwnerReferences([]metav1.OwnerReference{
			{Kind: "Deployment", Name: "dep1", Controller: &tval},
		})

		assert.True(t, controllerOwnerRefChanged(oldObj, newObj))
	})

	t.Run("Returns true when controller ownerRef name changes", func(t *testing.T) {
		t.Parallel()
		oldObj := &unstructured.Unstructured{}
		newObj := &unstructured.Unstructured{}
		tval := true
		oldObj.SetOwnerReferences([]metav1.OwnerReference{
			{Kind: "Deployment", Name: "dep1", Controller: &tval},
		})
		newObj.SetOwnerReferences([]metav1.OwnerReference{
			{Kind: "Deployment", Name: "dep2", Controller: &tval},
		})

		assert.True(t, controllerOwnerRefChanged(oldObj, newObj))
	})

	t.Run("Returns false when controller ownerRef is identical", func(t *testing.T) {
		t.Parallel()
		oldObj := &unstructured.Unstructured{}
		newObj := &unstructured.Unstructured{}
		tval := true
		oldObj.SetOwnerReferences([]metav1.OwnerReference{
			{Kind: "Deployment", Name: "dep1", Controller: &tval},
		})
		newObj.SetOwnerReferences([]metav1.OwnerReference{
			{Kind: "Deployment", Name: "dep1", Controller: &tval},
		})

		assert.False(t, controllerOwnerRefChanged(oldObj, newObj))
	})

	t.Run("Ignores non-controller ownerRef changes", func(t *testing.T) {
		t.Parallel()
		oldObj := &unstructured.Unstructured{}
		newObj := &unstructured.Unstructured{}
		tval := true

		// Same controller ownerRef in both.
		oldObj.SetOwnerReferences([]metav1.OwnerReference{
			{Kind: "Deployment", Name: "dep1", Controller: &tval},
			{Kind: "Foo", Name: "x"},
		})
		newObj.SetOwnerReferences([]metav1.OwnerReference{
			{Kind: "Deployment", Name: "dep1", Controller: &tval},
			{Kind: "Bar", Name: "y"}, // changed non-controller ref
		})

		assert.False(t, controllerOwnerRefChanged(oldObj, newObj))
	})
}
