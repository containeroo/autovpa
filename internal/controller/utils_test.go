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

package controller

import (
	"testing"

	"github.com/containeroo/autovpa/internal/config"
	"github.com/containeroo/autovpa/internal/flag"
	"github.com/containeroo/autovpa/internal/utils"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	appsv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	vpaautoscaling "k8s.io/autoscaler/vertical-pod-autoscaler/pkg/apis/autoscaling.k8s.io/v1"
)

func TestControllerVpaNeedsUpdate(t *testing.T) {
	t.Parallel()

	t.Run("Returns true when objects differ", func(t *testing.T) {
		t.Parallel()
		a := newVPAObject()
		a.SetLabels(map[string]string{"a": "1"})
		a.Object["spec"] = map[string]any{"foo": "bar"}

		b := a.DeepCopy()
		b.Object["spec"] = map[string]any{"foo": "baz"}
		assert.True(t, vpaNeedsUpdate(a, b))
	})

	t.Run("Returns false when objects equal", func(t *testing.T) {
		t.Parallel()
		a := newVPAObject()
		a.SetLabels(map[string]string{"a": "1"})
		a.SetAnnotations(map[string]string{"note": "x"})
		a.Object["spec"] = map[string]any{"foo": "bar"}

		b := a.DeepCopy()
		assert.False(t, vpaNeedsUpdate(a, b))
	})
}

func TestControllerWithArgoTrackingAnnotation(t *testing.T) {
	t.Parallel()

	t.Run("Returns original annotations when disabled", func(t *testing.T) {
		t.Parallel()
		ann := map[string]string{"argocd.argoproj.io/tracking-id": "demo"}
		out := withArgoTrackingAnnotation(false, flag.ArgoTrackingAnnotation, ann)
		assert.Empty(t, out)
	})

	t.Run("Ignores when workload lacks annotation", func(t *testing.T) {
		t.Parallel()
		ann := map[string]string{"foo": "bar"}
		out := withArgoTrackingAnnotation(true, flag.ArgoTrackingAnnotation, ann)
		assert.Empty(t, out)
	})

	t.Run("Copies tracking annotation when present", func(t *testing.T) {
		t.Parallel()
		ann := map[string]string{"argocd.argoproj.io/tracking-id": "demo"}
		out := withArgoTrackingAnnotation(true, flag.ArgoTrackingAnnotation, ann)
		assert.Equal(t, "demo", out["argocd.argoproj.io/tracking-id"])
		assert.Len(t, out, 1)
	})
}

func TestRenderVPAName(t *testing.T) {
	t.Parallel()

	t.Run("Renders valid name", func(t *testing.T) {
		t.Parallel()
		name, err := RenderVPAName("{{ toLower .WorkloadName }}-{{ dnsLabel .Profile }}", utils.NameTemplateData{
			WorkloadName: "demo",
			Namespace:    "ns1",
			Kind:         "Deployment",
			Profile:      "P1",
		})
		require.NoError(t, err)
		assert.Equal(t, "demo-p1", name)
	})

	t.Run("Errors on invalid rendered name", func(t *testing.T) {
		t.Parallel()
		_, err := RenderVPAName("INVALID", utils.NameTemplateData{
			WorkloadName: "demo",
			Namespace:    "ns1",
			Kind:         "Deployment",
			Profile:      "p1",
		})
		require.Error(t, err)
	})
}

func TestControllerBuildVPASpec(t *testing.T) {
	t.Parallel()

	t.Run("Sets targetRef and merges profile", func(t *testing.T) {
		t.Parallel()
		profile := config.ProfileSpec{
			UpdatePolicy: &vpaautoscaling.PodUpdatePolicy{
				UpdateMode: updateModePtr(t, vpaautoscaling.UpdateModeAuto),
			},
		}
		gvk := appsv1.SchemeGroupVersion.WithKind("Deployment")

		spec, err := buildVPASpec(profile, gvk, "demo")
		require.NoError(t, err)

		target := spec["targetRef"].(map[string]any)
		require.Equal(t, gvk.GroupVersion().String(), target["apiVersion"])
		require.Equal(t, "Deployment", target["kind"])
		require.Equal(t, "demo", target["name"])

		updatePolicy := spec["updatePolicy"].(map[string]any)
		assert.Equal(t, string(vpaautoscaling.UpdateModeAuto), updatePolicy["updateMode"])
	})
}

func TestControllerNewVPAObject(t *testing.T) {
	t.Parallel()

	t.Run("Initializes VPA with GVK", func(t *testing.T) {
		t.Parallel()
		obj := newVPAObject()
		objGVK := obj.GroupVersionKind()
		vpaGVK := schema.GroupVersionKind{
			Group:   objGVK.Group,
			Version: objGVK.Version,
			Kind:    objGVK.Kind,
		}
		assert.Equal(t, objGVK, vpaGVK)
	})
}

func TestControllerOwnerRefsEqual(t *testing.T) {
	t.Parallel()

	t.Run("Matches equal slices", func(t *testing.T) {
		t.Parallel()
		a := []metav1.OwnerReference{
			{APIVersion: "v1", Kind: "Pod", Name: "a"},
		}
		b := []metav1.OwnerReference{
			{APIVersion: "v1", Kind: "Pod", Name: "a"},
		}
		assert.True(t, ownerRefsEqual(a, b))
	})

	t.Run("Detects different slices", func(t *testing.T) {
		t.Parallel()
		a := []metav1.OwnerReference{
			{APIVersion: "v1", Kind: "Pod", Name: "a"},
		}
		b := []metav1.OwnerReference{
			{APIVersion: "v1", Kind: "Pod", Name: "b"},
		}
		assert.False(t, ownerRefsEqual(a, b))
	})
}
