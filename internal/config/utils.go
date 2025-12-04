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

package config

import (
	"fmt"

	vpaautoscaling "k8s.io/autoscaler/vertical-pod-autoscaler/pkg/apis/autoscaling.k8s.io/v1"
)

// copyProfileSpec returns a deep copy of the provided VPA profile spec.
func copyProfileSpec(spec ProfileSpec) ProfileSpec {
	s := vpaautoscaling.VerticalPodAutoscalerSpec(spec)
	out := s.DeepCopy()
	return ProfileSpec(*out)
}

// validateProfileSpec ensures targetRef is not set.
func validateProfileSpec(spec *ProfileSpec) error {
	typed := vpaautoscaling.VerticalPodAutoscalerSpec(*spec)
	if typed.TargetRef != nil {
		return fmt.Errorf("targetRef must not be set in profile")
	}
	// Clear targetRef explicitly to avoid accidental reuse.
	typed.TargetRef = nil
	*spec = ProfileSpec(typed)
	return nil
}
