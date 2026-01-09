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
	"sigs.k8s.io/controller-runtime/pkg/event"
)

func TestProfileAnnotationLifecycle(t *testing.T) {
	t.Parallel()

	pred := ProfileAnnotationLifecycle("a")

	objWith := &unstructured.Unstructured{}
	objWith.SetAnnotations(map[string]string{"a": "b"})

	objWithDifferent := &unstructured.Unstructured{}
	objWithDifferent.SetAnnotations(map[string]string{"a": "c"})

	objEmptyValue := &unstructured.Unstructured{}
	objEmptyValue.SetAnnotations(map[string]string{"a": ""})

	objWithout := &unstructured.Unstructured{}

	t.Run("Create allowed when annotation exists and non-empty", func(t *testing.T) {
		t.Parallel()
		e := event.CreateEvent{Object: objWith}
		assert.True(t, pred.Create(e))
	})

	t.Run("Create denied when annotation missing", func(t *testing.T) {
		t.Parallel()
		e := event.CreateEvent{Object: objWithout}
		assert.False(t, pred.Create(e))
	})

	t.Run("Create denied when annotation exists but empty", func(t *testing.T) {
		t.Parallel()
		e := event.CreateEvent{Object: objEmptyValue}
		assert.False(t, pred.Create(e))
	})

	t.Run("Update allowed when opt-in added", func(t *testing.T) {
		t.Parallel()
		e := event.UpdateEvent{ObjectOld: objWithout, ObjectNew: objWith}
		assert.True(t, pred.Update(e))
	})

	t.Run("Update allowed when opt-in removed", func(t *testing.T) {
		t.Parallel()
		e := event.UpdateEvent{ObjectOld: objWith, ObjectNew: objWithout}
		assert.True(t, pred.Update(e))
	})

	t.Run("Update allowed when value changes while opted-in", func(t *testing.T) {
		t.Parallel()
		e := event.UpdateEvent{ObjectOld: objWith, ObjectNew: objWithDifferent}
		assert.True(t, pred.Update(e))
	})

	t.Run("Update denied when opted-in and value unchanged", func(t *testing.T) {
		t.Parallel()
		objSame := &unstructured.Unstructured{}
		objSame.SetAnnotations(map[string]string{"a": "b"})
		e := event.UpdateEvent{ObjectOld: objWith, ObjectNew: objSame}
		assert.False(t, pred.Update(e))
	})

	t.Run("Update allowed when deletion just started", func(t *testing.T) {
		t.Parallel()
		oldObj := &unstructured.Unstructured{}
		oldObj.SetAnnotations(map[string]string{"a": "b"})

		newObj := oldObj.DeepCopy()
		now := metav1.NewTime(time.Now())
		newObj.SetDeletionTimestamp(&now)

		e := event.UpdateEvent{ObjectOld: oldObj, ObjectNew: newObj}
		assert.True(t, pred.Update(e))
	})

	t.Run("Delete allowed when annotation exists and non-empty", func(t *testing.T) {
		t.Parallel()
		e := event.DeleteEvent{Object: objWith}
		assert.True(t, pred.Delete(e))
	})

	t.Run("Delete denied when annotation missing", func(t *testing.T) {
		t.Parallel()
		e := event.DeleteEvent{Object: objWithout}
		assert.False(t, pred.Delete(e))
	})

	t.Run("Generic ignored", func(t *testing.T) {
		t.Parallel()
		e := event.GenericEvent{Object: objWith}
		assert.False(t, pred.Generic(e))
	})
}

func TestManagedVPAStructuralLifecycle(t *testing.T) {
	t.Parallel()

	pred := ManagedVPAStructuralLifecycle("m")

	objManaged := &unstructured.Unstructured{}
	objManaged.SetLabels(map[string]string{"m": "true"})

	objUnmanaged := &unstructured.Unstructured{} // no label

	t.Run("Create allowed when managed label is true", func(t *testing.T) {
		t.Parallel()
		e := event.CreateEvent{Object: objManaged}
		assert.True(t, pred.Create(e))
	})

	t.Run("Create denied when label missing", func(t *testing.T) {
		t.Parallel()
		e := event.CreateEvent{Object: objUnmanaged}
		assert.False(t, pred.Create(e))
	})

	t.Run("Update allowed when managed label toggled (added)", func(t *testing.T) {
		t.Parallel()
		e := event.UpdateEvent{ObjectOld: objUnmanaged, ObjectNew: objManaged}
		assert.True(t, pred.Update(e))
	})

	t.Run("Update allowed when managed label toggled (removed)", func(t *testing.T) {
		t.Parallel()
		e := event.UpdateEvent{ObjectOld: objManaged, ObjectNew: objUnmanaged}
		assert.True(t, pred.Update(e))
	})

	t.Run("Update allowed when deletion just started (managed)", func(t *testing.T) {
		t.Parallel()
		oldObj := &unstructured.Unstructured{}
		oldObj.SetLabels(map[string]string{"m": "true"})

		newObj := oldObj.DeepCopy()
		now := metav1.NewTime(time.Now())
		newObj.SetDeletionTimestamp(&now)

		e := event.UpdateEvent{ObjectOld: oldObj, ObjectNew: newObj}
		assert.True(t, pred.Update(e))
	})

	t.Run("Update allowed when controller ownerRef changed (managed)", func(t *testing.T) {
		t.Parallel()
		tval := true

		oldObj := &unstructured.Unstructured{}
		oldObj.SetLabels(map[string]string{"m": "true"})
		oldObj.SetOwnerReferences([]metav1.OwnerReference{
			{Kind: "Deployment", Name: "dep1", Controller: &tval},
		})

		newObj := oldObj.DeepCopy()
		newObj.SetOwnerReferences([]metav1.OwnerReference{
			{Kind: "Deployment", Name: "dep2", Controller: &tval},
		})

		e := event.UpdateEvent{ObjectOld: oldObj, ObjectNew: newObj}
		assert.True(t, pred.Update(e))
	})

	t.Run("Update denied on managed VPA when only spec changes", func(t *testing.T) {
		t.Parallel()

		oldObj := &unstructured.Unstructured{}
		oldObj.SetLabels(map[string]string{"m": "true"})
		oldObj.Object["spec"] = map[string]any{"a": float64(1)}

		newObj := oldObj.DeepCopy()
		newObj.Object["spec"] = map[string]any{"a": float64(2)}

		e := event.UpdateEvent{ObjectOld: oldObj, ObjectNew: newObj}
		assert.False(t, pred.Update(e))
	})

	t.Run("Delete allowed when managed label is true", func(t *testing.T) {
		t.Parallel()
		e := event.DeleteEvent{Object: objManaged}
		assert.True(t, pred.Delete(e))
	})

	t.Run("Delete denied when unmanaged", func(t *testing.T) {
		t.Parallel()
		e := event.DeleteEvent{Object: objUnmanaged}
		assert.False(t, pred.Delete(e))
	})

	t.Run("Generic ignored", func(t *testing.T) {
		t.Parallel()
		e := event.GenericEvent{Object: objManaged}
		assert.False(t, pred.Generic(e))
	})
}

func TestManagedVPALifecycle(t *testing.T) {
	t.Parallel()

	pred := ManagedVPALifecycle("m", "k")

	t.Run("Update allowed when spec changes on managed VPA", func(t *testing.T) {
		t.Parallel()

		oldObj := &unstructured.Unstructured{
			Object: map[string]any{
				"spec": map[string]any{"a": float64(1)},
			},
		}

		oldObj.SetLabels(map[string]string{"m": "true"})

		newObj := oldObj.DeepCopy()
		newObj.Object["spec"] = map[string]any{"a": 2}

		e := event.UpdateEvent{ObjectOld: oldObj, ObjectNew: newObj}
		assert.True(t, pred.Update(e))
	})

	t.Run("Update denied when unmanaged even if spec changes", func(t *testing.T) {
		t.Parallel()

		oldObj := &unstructured.Unstructured{
			Object: map[string]any{
				"spec": map[string]any{"a": float64(1)},
			},
		}

		newObj := oldObj.DeepCopy()
		newObj.Object["spec"] = map[string]any{"a": float64(2)}

		e := event.UpdateEvent{ObjectOld: oldObj, ObjectNew: newObj}
		assert.False(t, pred.Update(e))
	})

	t.Run("Update denied when managed and nothing relevant changed", func(t *testing.T) {
		t.Parallel()

		tval := true
		oldObj := &unstructured.Unstructured{}
		oldObj.SetLabels(map[string]string{"m": "true"})
		oldObj.Object["spec"] = map[string]any{"a": float64(1)}
		oldObj.SetOwnerReferences([]metav1.OwnerReference{
			{Kind: "Deployment", Name: "dep1", Controller: &tval},
		})

		newObj := oldObj.DeepCopy()

		e := event.UpdateEvent{ObjectOld: oldObj, ObjectNew: newObj}
		assert.False(t, pred.Update(e))
	})
}
