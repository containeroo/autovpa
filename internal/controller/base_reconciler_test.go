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
	"context"
	"testing"

	"github.com/containeroo/autovpa/internal/config"
	"github.com/containeroo/autovpa/internal/flag"
	"github.com/containeroo/autovpa/internal/metrics"
	"github.com/containeroo/autovpa/internal/utils"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/go-logr/logr"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
	appsv1 "k8s.io/api/apps/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	vpaautoscaling "k8s.io/autoscaler/vertical-pod-autoscaler/pkg/apis/autoscaling.k8s.io/v1"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestBaseReconciler_ReconcileWorkload(t *testing.T) {
	t.Parallel()

	t.Run("Skips VPA when annotation missing", func(t *testing.T) {
		resetMetrics(t)
		ctx := context.Background()
		scheme := newScheme(t)
		client := fake.NewClientBuilder().WithScheme(scheme).Build()
		rec := record.NewFakeRecorder(10)
		logger := logr.Discard()

		cfg := &config.Config{
			DefaultProfile: "p1",
			Profiles: map[string]config.Profile{
				"p1": {Spec: config.ProfileSpec{}},
			},
		}

		reconciler := BaseReconciler{
			KubeClient: client,
			Logger:     &logger,
			Recorder:   rec,
			Meta: MetaConfig{
				ProfileAnnotation: "vpa/profile",
				ManagedLabel:      "vpa/managed",
			},
			Profiles: ProfileConfig{
				Profiles:       cfg.Profiles,
				DefaultProfile: cfg.DefaultProfile,
				NameTemplate:   flag.DefaultNameTemplate,
			},
		}

		dep := &appsv1.Deployment{}
		dep.SetNamespace("ns1")
		dep.SetName("demo")

		_, err := reconciler.ReconcileWorkload(ctx, dep, appsv1.SchemeGroupVersion.WithKind("Deployment"))
		require.NoError(t, err)

		metric := metrics.VPASkipped.WithLabelValues("ns1", "demo", "Deployment", "annotation_missing")
		got := readCounter(t, metric)
		assert.Equal(t, 1, got)
	})

	t.Run("Skips VPA when profile missing", func(t *testing.T) {
		resetMetrics(t)
		ctx := context.Background()
		scheme := newScheme(t)
		client := fake.NewClientBuilder().WithScheme(scheme).Build()
		rec := record.NewFakeRecorder(10)
		logger := logr.Discard()

		cfg := &config.Config{
			DefaultProfile: "p1",
			Profiles: map[string]config.Profile{
				"p1": {Spec: config.ProfileSpec{}},
			},
		}

		reconciler := BaseReconciler{
			KubeClient: client,
			Logger:     &logger,
			Recorder:   rec,
			Meta: MetaConfig{
				ProfileAnnotation: "vpa/profile",
				ManagedLabel:      "vpa/managed",
			},
			Profiles: ProfileConfig{
				Profiles:       cfg.Profiles,
				DefaultProfile: cfg.DefaultProfile,
				NameTemplate:   flag.DefaultNameTemplate,
			},
		}

		dep := &appsv1.Deployment{}
		dep.SetNamespace("ns1")
		dep.SetName("demo")
		dep.SetAnnotations(map[string]string{"vpa/profile": "unknown"})

		_, err := reconciler.ReconcileWorkload(ctx, dep, appsv1.SchemeGroupVersion.WithKind("Deployment"))
		require.NoError(t, err)

		metric := metrics.VPASkipped.WithLabelValues("ns1", "demo", "Deployment", "profile_missing")
		got := readCounter(t, metric)
		require.Equal(t, 1, got)
		assert.Len(t, rec.Events, 1)
	})
	t.Run("Creates VPA", func(t *testing.T) {
		resetMetrics(t)
		ctx := context.Background()
		scheme := newScheme(t)
		client := fake.NewClientBuilder().WithScheme(scheme).Build()
		rec := record.NewFakeRecorder(10)
		logger := logr.Discard()

		cfg := &config.Config{
			DefaultProfile: "p1",
			Profiles: map[string]config.Profile{
				"p1": {Spec: config.ProfileSpec{}},
			},
		}

		reconciler := BaseReconciler{
			KubeClient: client,
			Logger:     &logger,
			Recorder:   rec,
			Meta: MetaConfig{
				ProfileAnnotation: "vpa/profile",
				ManagedLabel:      "vpa/managed",
			},
			Profiles: ProfileConfig{
				Profiles:       cfg.Profiles,
				DefaultProfile: cfg.DefaultProfile,
				NameTemplate:   flag.DefaultNameTemplate,
			},
		}

		dep := &appsv1.Deployment{}
		dep.SetNamespace("ns1")
		dep.SetName("demo")
		dep.SetAnnotations(map[string]string{"vpa/profile": "p1"})

		_, err := reconciler.ReconcileWorkload(ctx, dep, appsv1.SchemeGroupVersion.WithKind("Deployment"))
		require.NoError(t, err)

		vpaName := renderDeploymentVPAName(t, "ns1", dep.GetName(), "p1")
		vpa := newVPAObject()
		err = client.Get(ctx, types.NamespacedName{Name: vpaName, Namespace: "ns1"}, vpa)
		require.NoError(t, err)

		labels := vpa.GetLabels()
		assert.Equal(t, "true", labels["vpa/managed"])

		spec := vpa.Object["spec"].(map[string]any)
		target := spec["targetRef"].(map[string]any)
		assert.Equal(t, "demo", target["name"])
		assert.Equal(t, "Deployment", target["kind"])

		got := readCounter(t, metrics.VPACreated.WithLabelValues("ns1", "demo", "Deployment", "p1"))
		assert.Equal(t, 1, got)
	})
	t.Run("Deletes obsolete managed VPA when name changes", func(t *testing.T) {
		resetMetrics(t)
		ctx := context.Background()
		scheme := newScheme(t)
		dep := &appsv1.Deployment{}
		dep.SetNamespace("ns1")
		dep.SetName("demo")
		dep.SetUID("uid1")
		dep.SetAnnotations(map[string]string{"vpa/profile": "p2"})

		managed := true
		existing := newVPAObject()
		existing.SetNamespace("ns1")
		legacyName := "legacy-demo"
		existing.SetName(legacyName)
		existing.SetLabels(map[string]string{"vpa/managed": "true"})
		existing.SetOwnerReferences([]metav1.OwnerReference{
			{
				APIVersion: appsv1.SchemeGroupVersion.String(),
				Kind:       "Deployment",
				Name:       dep.GetName(),
				UID:        dep.GetUID(),
				Controller: &managed,
			},
		})
		existing.Object["spec"] = map[string]any{
			"targetRef": map[string]any{
				"apiVersion": appsv1.SchemeGroupVersion.String(),
				"kind":       "Deployment",
				"name":       "demo",
			},
		}

		client := fake.NewClientBuilder().WithScheme(scheme).WithObjects(existing, dep).Build()
		rec := record.NewFakeRecorder(10)
		logger := logr.Discard()

		cfg := &config.Config{
			DefaultProfile: "p2",
			Profiles: map[string]config.Profile{
				"p1": {Spec: config.ProfileSpec{}, NameTemplate: "legacy-{{ .WorkloadName }}"},
				"p2": {Spec: config.ProfileSpec{}},
			},
		}

		reconciler := BaseReconciler{
			KubeClient: client,
			Logger:     &logger,
			Recorder:   rec,
			Meta: MetaConfig{
				ProfileAnnotation: "vpa/profile",
				ManagedLabel:      "vpa/managed",
			},
			Profiles: ProfileConfig{
				Profiles:       cfg.Profiles,
				DefaultProfile: cfg.DefaultProfile,
				NameTemplate:   flag.DefaultNameTemplate,
			},
		}

		_, err := reconciler.ReconcileWorkload(ctx, dep, appsv1.SchemeGroupVersion.WithKind("Deployment"))
		require.NoError(t, err)

		// old VPA should be deleted
		err = client.Get(ctx, types.NamespacedName{Name: legacyName, Namespace: "ns1"}, newVPAObject())
		require.True(t, apierrors.IsNotFound(err))

		newVPAName := renderDeploymentVPAName(t, "ns1", dep.GetName(), "p2")
		// new VPA with desired name should exist
		err = client.Get(ctx, types.NamespacedName{Name: newVPAName, Namespace: "ns1"}, newVPAObject())
		require.NoError(t, err)
	})
	t.Run("Creates VPA with Argo tracking annotation when enabled and present", func(t *testing.T) {
		resetMetrics(t)
		ctx := context.Background()
		scheme := newScheme(t)
		client := fake.NewClientBuilder().WithScheme(scheme).Build()
		rec := record.NewFakeRecorder(10)
		logger := logr.Discard()

		cfg := &config.Config{
			DefaultProfile: "p1",
			Profiles: map[string]config.Profile{
				"p1": {Spec: config.ProfileSpec{}},
			},
		}

		reconciler := BaseReconciler{
			KubeClient: client,
			Logger:     &logger,
			Recorder:   rec,
			Meta: MetaConfig{
				ProfileAnnotation:      "vpa/profile",
				ManagedLabel:           "vpa/managed",
				ArgoManaged:            true,
				ArgoTrackingAnnotation: flag.ArgoTrackingAnnotation,
			},
			Profiles: ProfileConfig{
				Profiles:       cfg.Profiles,
				DefaultProfile: cfg.DefaultProfile,
				NameTemplate:   flag.DefaultNameTemplate,
			},
		}

		dep := &appsv1.Deployment{}
		dep.SetNamespace("ns1")
		dep.SetName("demo")
		dep.SetAnnotations(map[string]string{
			"vpa/profile":                    "p1",
			"argocd.argoproj.io/tracking-id": "myapp",
		})

		_, err := reconciler.ReconcileWorkload(ctx, dep, appsv1.SchemeGroupVersion.WithKind("Deployment"))
		require.NoError(t, err)

		vpaName := renderDeploymentVPAName(t, "ns1", dep.GetName(), "p1")
		vpa := newVPAObject()
		err = client.Get(ctx, types.NamespacedName{Name: vpaName, Namespace: "ns1"}, vpa)
		require.NoError(t, err)

		annotations := vpa.GetAnnotations()
		assert.Equal(t, "myapp", annotations["argocd.argoproj.io/tracking-id"])
	})
	t.Run("Updates VPA", func(t *testing.T) {
		resetMetrics(t)
		ctx := context.Background()
		scheme := newScheme(t)

		existing := newVPAObject()
		existing.SetNamespace("ns1")
		vpaName := renderDeploymentVPAName(t, "ns1", "demo", "p1")
		existing.SetName(vpaName)
		existing.SetLabels(map[string]string{"old": "label"})
		existing.Object["spec"] = map[string]any{
			"targetRef": map[string]any{
				"apiVersion": appsv1.SchemeGroupVersion.String(),
				"kind":       "Deployment",
				"name":       "demo",
			},
		}

		client := fake.NewClientBuilder().WithScheme(scheme).WithObjects(existing).Build()
		rec := record.NewFakeRecorder(10)
		logger := logr.Discard()

		cfg := &config.Config{
			DefaultProfile: "p1",
			Profiles: map[string]config.Profile{
				"p1": {
					Spec: config.ProfileSpec{
						UpdatePolicy: &vpaautoscaling.PodUpdatePolicy{
							UpdateMode: updateModePtr(t, vpaautoscaling.UpdateModeAuto),
						},
					},
				},
			},
		}

		reconciler := BaseReconciler{
			KubeClient: client,
			Logger:     &logger,
			Recorder:   rec,
			Meta: MetaConfig{
				ProfileAnnotation: "vpa/profile",
				ManagedLabel:      "vpa/managed",
			},
			Profiles: ProfileConfig{
				Profiles:       cfg.Profiles,
				DefaultProfile: cfg.DefaultProfile,
				NameTemplate:   flag.DefaultNameTemplate,
			},
		}

		dep := &appsv1.Deployment{}
		dep.SetNamespace("ns1")
		dep.SetName("demo")
		dep.SetAnnotations(map[string]string{"vpa/profile": "p1"})

		_, err := reconciler.ReconcileWorkload(ctx, dep, appsv1.SchemeGroupVersion.WithKind("Deployment"))
		require.NoError(t, err)

		vpa := newVPAObject()
		err = client.Get(ctx, types.NamespacedName{Name: vpaName, Namespace: "ns1"}, vpa)
		require.NoError(t, err)

		spec := vpa.Object["spec"].(map[string]any)
		updatePolicy := spec["updatePolicy"].(map[string]any)
		assert.Equal(t, "Auto", updatePolicy["updateMode"])
		got := readCounter(t, metrics.VPAUpdated.WithLabelValues("ns1", "demo", "Deployment", "p1"))
		assert.Equal(t, 1, got)
	})

	t.Run("Cleans managed VPAs when annotation is removed", func(t *testing.T) {
		resetMetrics(t)
		ctx := context.Background()
		scheme := newScheme(t)

		dep := &appsv1.Deployment{}
		dep.SetNamespace("ns1")
		dep.SetName("demo")
		dep.SetUID("uid1")
		dep.SetAnnotations(map[string]string{}) // annotation removed

		managed := true
		vpa := newVPAObject()
		vpa.SetNamespace("ns1")
		vpaName := renderDeploymentVPAName(t, "ns1", dep.GetName(), "p1")
		vpa.SetName(vpaName)
		vpa.SetLabels(map[string]string{"vpa/managed": "true"})
		vpa.SetOwnerReferences([]metav1.OwnerReference{
			{
				APIVersion: appsv1.SchemeGroupVersion.String(),
				Kind:       "Deployment",
				Name:       dep.GetName(),
				UID:        dep.GetUID(),
				Controller: &managed,
			},
		})
		vpa.Object["spec"] = map[string]any{}

		client := fake.NewClientBuilder().WithScheme(scheme).WithObjects(dep, vpa).Build()
		rec := record.NewFakeRecorder(10)
		logger := logr.Discard()

		cfg := &config.Config{
			DefaultProfile: "p1",
			Profiles:       map[string]config.Profile{"p1": {Spec: config.ProfileSpec{}}},
		}

		reconciler := BaseReconciler{
			KubeClient: client,
			Logger:     &logger,
			Recorder:   rec,
			Meta: MetaConfig{
				ProfileAnnotation: "vpa/profile",
				ManagedLabel:      "vpa/managed",
			},
			Profiles: ProfileConfig{
				Profiles:       cfg.Profiles,
				DefaultProfile: cfg.DefaultProfile,
				NameTemplate:   flag.DefaultNameTemplate,
			},
		}

		_, err := reconciler.ReconcileWorkload(ctx, dep, appsv1.SchemeGroupVersion.WithKind("Deployment"))
		require.NoError(t, err)

		err = client.Get(ctx, types.NamespacedName{Name: vpaName, Namespace: "ns1"}, vpa)
		assert.True(t, apierrors.IsNotFound(err))
	})
}

func newScheme(t *testing.T) *runtime.Scheme {
	t.Helper()
	s := runtime.NewScheme()
	err := appsv1.AddToScheme(s)
	require.NoError(t, err)

	// Register VPA unstructured types.
	s.AddKnownTypeWithName(vpaGVK, &unstructured.Unstructured{})
	s.AddKnownTypeWithName(schema.GroupVersionKind{
		Group:   vpaGVK.Group,
		Version: vpaGVK.Version,
		Kind:    vpaGVK.Kind + "List",
	}, &unstructured.UnstructuredList{})
	return s
}

func resetMetrics(t *testing.T) {
	t.Helper()
	metrics.VPACreated.Reset()
	metrics.VPAUpdated.Reset()
	metrics.VPASkipped.Reset()
}

func readCounter(t *testing.T, c prometheus.Collector) int {
	t.Helper()
	return int(testutil.ToFloat64(c))
}

func updateModePtr(t *testing.T, mode vpaautoscaling.UpdateMode) *vpaautoscaling.UpdateMode {
	t.Helper()
	return &mode
}

func renderDeploymentVPAName(t *testing.T, namespace, workloadName, profile string) string {
	t.Helper()
	vpaName, err := RenderVPAName(flag.DefaultNameTemplate, utils.NameTemplateData{
		WorkloadName: workloadName,
		Namespace:    namespace,
		Kind:         "Deployment",
		Profile:      profile,
	})
	require.NoError(t, err)
	return vpaName
}
