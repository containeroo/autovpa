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
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	k8sautoscalingv1 "k8s.io/api/autoscaling/v1"
	vpaautoscaling "k8s.io/autoscaler/vertical-pod-autoscaler/pkg/apis/autoscaling.k8s.io/v1"
)

// updateModePtr is a small helper for tests.
func updateModePtr(t *testing.T, mode vpaautoscaling.UpdateMode) *vpaautoscaling.UpdateMode {
	t.Helper()
	return &mode
}

func TestValidateProfileSpec(t *testing.T) {
	t.Parallel()

	t.Run("Allows targetRef nil", func(t *testing.T) {
		t.Parallel()
		spec := ProfileSpec{}
		require.NoError(t, validateProfileSpec(&spec))
	})

	t.Run("Rejects targetRef set", func(t *testing.T) {
		t.Parallel()
		spec := ProfileSpec{
			TargetRef: &k8sautoscalingv1.CrossVersionObjectReference{Name: "bad"},
		}
		assert.Error(t, validateProfileSpec(&spec))
	})
}

func TestCopyProfileSpec(t *testing.T) {
	t.Parallel()

	t.Run("Deep copies profile spec", func(t *testing.T) {
		t.Parallel()
		orig := ProfileSpec{
			UpdatePolicy: &vpaautoscaling.PodUpdatePolicy{
				UpdateMode: updateModePtr(t, vpaautoscaling.UpdateModeAuto),
			},
		}
		cp := copyProfileSpec(orig)
		orig.UpdatePolicy.UpdateMode = updateModePtr(t, vpaautoscaling.UpdateModeOff)

		assert.Equal(t, vpaautoscaling.UpdateModeAuto, *cp.UpdatePolicy.UpdateMode)
	})
}
