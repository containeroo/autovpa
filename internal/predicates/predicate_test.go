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

	"github.com/stretchr/testify/assert"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/controller-runtime/pkg/event"
)

func TestPredicatesAnnotationExists(t *testing.T) {
	t.Parallel()

	objWith := &unstructured.Unstructured{}
	objWith.SetAnnotations(map[string]string{"a": "b"})
	objWithout := &unstructured.Unstructured{}

	pred := AnnotationLifecycle("a")

	t.Run("Create event allowed when annotation exists", func(t *testing.T) {
		t.Parallel()
		e := event.CreateEvent{Object: objWith}
		assert.True(t, pred.Create(e))
	})

	t.Run("Create event denied when annotation missing", func(t *testing.T) {
		t.Parallel()
		e := event.CreateEvent{Object: objWithout}
		assert.False(t, pred.Create(e))
	})

	t.Run("Update event allowed when new object has annotation", func(t *testing.T) {
		t.Parallel()
		e := event.UpdateEvent{ObjectNew: objWith, ObjectOld: objWithout}
		assert.True(t, pred.Update(e))
	})

	t.Run("Delete event allowed when annotation exists", func(t *testing.T) {
		t.Parallel()
		e := event.DeleteEvent{Object: objWith}
		assert.True(t, pred.Delete(e))
	})

	t.Run("Update event allowed when annotation removed", func(t *testing.T) {
		t.Parallel()
		e := event.UpdateEvent{ObjectNew: objWithout, ObjectOld: objWith}
		assert.True(t, pred.Update(e))
	})

	t.Run("Generic event ignored", func(t *testing.T) {
		t.Parallel()
		e := event.GenericEvent{Object: objWith}
		assert.False(t, pred.Generic(e))
	})
}
