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
)

func TestPredicatesHasAnnotation(t *testing.T) {
	t.Parallel()

	t.Run("Returns false on nil object", func(t *testing.T) {
		t.Parallel()
		assert.False(t, hasAnnotation(nil, "a"))
	})

	t.Run("Returns false when annotation missing", func(t *testing.T) {
		t.Parallel()
		obj := &unstructured.Unstructured{}
		assert.False(t, hasAnnotation(obj, "a"))
	})

	t.Run("Returns true when annotation exists", func(t *testing.T) {
		t.Parallel()
		obj := &unstructured.Unstructured{}
		obj.SetAnnotations(map[string]string{"a": "b"})
		assert.True(t, hasAnnotation(obj, "a"))
	})

	t.Run("Return false when annotation does not exists.", func(t *testing.T) {
		t.Parallel()
		obj := &unstructured.Unstructured{}
		obj.SetAnnotations(map[string]string{"a": "b"})
		assert.False(t, hasAnnotation(obj, "c"))
	})
}
