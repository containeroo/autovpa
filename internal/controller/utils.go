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
	"fmt"
	"maps"

	"github.com/containeroo/autovpa/internal/config"
	"github.com/containeroo/autovpa/internal/utils"

	k8sautoscalingv1 "k8s.io/api/autoscaling/v1"
	apiequality "k8s.io/apimachinery/pkg/api/equality"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	vpaautoscaling "k8s.io/autoscaler/vertical-pod-autoscaler/pkg/apis/autoscaling.k8s.io/v1"
)

// vpaNeedsUpdate reports whether the relevant managed fields of the two VPAs differ.
func vpaNeedsUpdate(a, b *unstructured.Unstructured) bool {
	if a == nil || b == nil {
		return a != b
	}

	return !apiequality.Semantic.DeepEqual(a.Object["spec"], b.Object["spec"]) ||
		!maps.Equal(a.GetLabels(), b.GetLabels()) ||
		!ownerRefsEqual(a.GetOwnerReferences(), b.GetOwnerReferences())
}

// RenderVPAName renders and validates the VPA name using the provided template and data.
func RenderVPAName(tmpl string, data utils.NameTemplateData) (string, error) {
	return utils.RenderNameTemplate(tmpl, data)
}

// newVPAObject returns an empty VPA object with the correct GVK set.
func newVPAObject() *unstructured.Unstructured {
	obj := &unstructured.Unstructured{Object: map[string]any{}}
	obj.SetGroupVersionKind(vpaGVK)
	return obj
}

// buildVPASpec creates a VPA spec from the profile and plugs in the workload targetRef,
// returning it as an unstructured map for use in unstructured VPAs.
func buildVPASpec(
	profile config.ProfileSpec,
	targetGVK schema.GroupVersionKind,
	workloadName string,
) (unstructuredSpec map[string]any, err error) {
	spec := vpaautoscaling.VerticalPodAutoscalerSpec(profile)
	spec.TargetRef = &k8sautoscalingv1.CrossVersionObjectReference{
		APIVersion: targetGVK.GroupVersion().String(),
		Kind:       targetGVK.Kind,
		Name:       workloadName,
	}

	// Unstructured objects are easier to work with than the typed ones.
	unstructuredSpec, err = runtime.DefaultUnstructuredConverter.ToUnstructured(&spec)
	if err != nil {
		return nil, fmt.Errorf("convert VPA spec to unstructured: %w", err)
	}

	return unstructuredSpec, nil
}

// ownerRefsEqual compares owner reference slices.
func ownerRefsEqual(a, b []metav1.OwnerReference) bool {
	return apiequality.Semantic.DeepEqual(a, b)
}

// profileFromLabels returns the profile label value or "unknown" if absent.
func profileFromLabels(labels map[string]string, key string) string {
	if labels == nil {
		return "unknown"
	}
	if v, ok := labels[key]; ok && v != "" {
		return v
	}
	return "unknown"
}
