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

import "sigs.k8s.io/controller-runtime/pkg/client"

// hasAnnotation returns true if obj contains any of the specified annotations.
func hasAnnotation(obj client.Object, annotation string) bool {
	if obj == nil {
		return false
	}
	objAnnots := obj.GetAnnotations()
	if objAnnots == nil {
		return false
	}
	if _, ok := objAnnots[annotation]; ok {
		return true
	}
	return false
}
