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
	internalmetrics "github.com/containeroo/autovpa/internal/metrics"
	"github.com/containeroo/autovpa/internal/utils"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/go-logr/logr"
	"github.com/prometheus/client_golang/prometheus"
	io_prometheus_client "github.com/prometheus/client_model/go"
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

func mustGetCounterValue(t *testing.T, g prometheus.Gatherer, metricName string, wantLabels map[string]string) float64 {
	t.Helper()

	mfs, err := g.Gather()
	require.NoError(t, err)

	for _, mf := range mfs {
		if mf.GetName() != metricName {
			continue
		}
		for _, m := range mf.GetMetric() {
			if labelsMatch(m.GetLabel(), wantLabels) {
				// Counter must be present for this metric.
				require.NotNil(t, m.GetCounter())
				return m.GetCounter().GetValue()
			}
		}
		t.Fatalf("metric %q found but no series matched labels: %#v", metricName, wantLabels)
	}

	t.Fatalf("metric %q not found in registry", metricName)
	return 0
}

func labelsMatch(lbls []*io_prometheus_client.LabelPair, want map[string]string) bool {
	if len(want) == 0 {
		return true
	}
	got := make(map[string]string, len(lbls))
	for _, lp := range lbls {
		got[lp.GetName()] = lp.GetValue()
	}
	for k, v := range want {
		if got[k] != v {
			return false
		}
	}
	return true
}

func TestBaseReconciler_ReconcileWorkload(t *testing.T) {
	t.Parallel()

	t.Run("Skips VPA when annotation missing", func(t *testing.T) {
		t.Parallel()
		ctx := context.Background()
		scheme := newScheme(t)
		client := fake.NewClientBuilder().WithScheme(scheme).Build()
		rec := record.NewFakeRecorder(10)
		logger := logr.Discard()

		promReg := prometheus.NewRegistry()
		metricsReg := internalmetrics.NewRegistry(promReg)

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
			Metrics:    metricsReg,
			Meta: MetaConfig{
				ProfileKey:   "vpa/profile",
				ManagedLabel: "vpa/managed",
			},
			Profiles: ProfileConfig{
				Entries:      cfg.Profiles,
				Default:      cfg.DefaultProfile,
				NameTemplate: flag.DefaultNameTemplate,
			},
		}

		dep := &appsv1.Deployment{}
		dep.SetNamespace("ns1")
		dep.SetName("demo")

		_, err := reconciler.ReconcileWorkload(ctx, dep, appsv1.SchemeGroupVersion.WithKind("Deployment"))
		require.NoError(t, err)

		// Assert by gathering from the registry and finding the matching series.
		got := mustGetCounterValue(t, promReg,
			"autovpa_vpa_skipped_total",
			map[string]string{
				"namespace": "ns1",
				"name":      "demo",
				"kind":      "Deployment",
				"reason":    vpaSkipReasonAnnotationMissing,
			},
		)
		assert.Equal(t, float64(1), got)
	})

	t.Run("Skips VPA when profile missing", func(t *testing.T) {
		t.Parallel()
		ctx := context.Background()
		scheme := newScheme(t)
		client := fake.NewClientBuilder().WithScheme(scheme).Build()
		rec := record.NewFakeRecorder(10)
		logger := logr.Discard()

		promReg := prometheus.NewRegistry()
		metricsReg := internalmetrics.NewRegistry(promReg)

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
				ProfileKey:   "vpa/profile",
				ManagedLabel: "vpa/managed",
			},
			Profiles: ProfileConfig{
				Entries:      cfg.Profiles,
				Default:      cfg.DefaultProfile,
				NameTemplate: flag.DefaultNameTemplate,
			},
			Metrics: metricsReg,
		}

		dep := &appsv1.Deployment{}
		dep.SetNamespace("ns1")
		dep.SetName("demo")
		dep.SetAnnotations(map[string]string{"vpa/profile": "unknown"})

		_, err := reconciler.ReconcileWorkload(ctx, dep, appsv1.SchemeGroupVersion.WithKind("Deployment"))
		require.NoError(t, err)

		// Assert by gathering from the registry and finding the matching series.
		got := mustGetCounterValue(t, promReg,
			"autovpa_vpa_skipped_total",
			map[string]string{
				"namespace": "ns1",
				"name":      "demo",
				"kind":      "Deployment",
				"reason":    vpaSkipReasonProfileMissing,
			},
		)
		assert.Equal(t, float64(1), got)
	})

	t.Run("Creates VPA", func(t *testing.T) {
		t.Parallel()
		ctx := context.Background()
		scheme := newScheme(t)
		client := fake.NewClientBuilder().WithScheme(scheme).Build()
		rec := record.NewFakeRecorder(10)
		logger := logr.Discard()

		promReg := prometheus.NewRegistry()
		metricsReg := internalmetrics.NewRegistry(promReg)

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
			Metrics:    metricsReg,
			Meta: MetaConfig{
				ProfileKey:   "vpa/profile",
				ManagedLabel: "vpa/managed",
			},
			Profiles: ProfileConfig{
				Entries:      cfg.Profiles,
				Default:      cfg.DefaultProfile,
				NameTemplate: flag.DefaultNameTemplate,
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
		assert.Equal(t, "p1", labels["vpa/profile"])
		assert.Equal(t, "true", labels["vpa/managed"])

		spec := vpa.Object["spec"].(map[string]any)
		target := spec["targetRef"].(map[string]any)
		assert.Equal(t, "demo", target["name"])
		assert.Equal(t, "Deployment", target["kind"])

		// Assert by gathering from the registry and finding the matching series.
		got := mustGetCounterValue(t, promReg,
			"autovpa_vpa_created_total",
			map[string]string{
				"namespace": "ns1",
				"name":      "demo",
				"kind":      "Deployment",
				"profile":   "p1",
			},
		)
		assert.Equal(t, float64(1), got)
	})

	t.Run("Deletes obsolete managed VPA when name changes", func(t *testing.T) {
		t.Parallel()
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

		promReg := prometheus.NewRegistry()
		metricsReg := internalmetrics.NewRegistry(promReg)

		reconciler := BaseReconciler{
			KubeClient: client,
			Logger:     &logger,
			Recorder:   rec,
			Metrics:    metricsReg,
			Meta: MetaConfig{
				ProfileKey:   "vpa/profile",
				ManagedLabel: "vpa/managed",
			},
			Profiles: ProfileConfig{
				Entries:      cfg.Profiles,
				Default:      cfg.DefaultProfile,
				NameTemplate: flag.DefaultNameTemplate,
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

	t.Run("Updates VPA", func(t *testing.T) {
		t.Parallel()
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

		promReg := prometheus.NewRegistry()
		metricsReg := internalmetrics.NewRegistry(promReg)

		reconciler := BaseReconciler{
			KubeClient: client,
			Logger:     &logger,
			Recorder:   rec,
			Metrics:    metricsReg,
			Meta: MetaConfig{
				ProfileKey:   "vpa/profile",
				ManagedLabel: "vpa/managed",
			},
			Profiles: ProfileConfig{
				Entries:      cfg.Profiles,
				Default:      cfg.DefaultProfile,
				NameTemplate: flag.DefaultNameTemplate,
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

		// Assert by gathering from the registry and finding the matching series.
		got := mustGetCounterValue(t, promReg,
			"autovpa_vpa_updated_total",
			map[string]string{
				"namespace": "ns1",
				"name":      "demo",
				"kind":      "Deployment",
			},
		)
		assert.Equal(t, float64(1), got)
	})

	t.Run("Cleans managed VPAs when annotation is removed", func(t *testing.T) {
		t.Parallel()
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

		promReg := prometheus.NewRegistry()
		metricsReg := internalmetrics.NewRegistry(promReg)

		reconciler := BaseReconciler{
			KubeClient: client,
			Logger:     &logger,
			Recorder:   rec,
			Metrics:    metricsReg,
			Meta: MetaConfig{
				ProfileKey:   "vpa/profile",
				ManagedLabel: "vpa/managed",
			},
			Profiles: ProfileConfig{
				Entries:      cfg.Profiles,
				Default:      cfg.DefaultProfile,
				NameTemplate: flag.DefaultNameTemplate,
			},
		}

		_, err := reconciler.ReconcileWorkload(ctx, dep, appsv1.SchemeGroupVersion.WithKind("Deployment"))
		require.NoError(t, err)

		err = client.Get(ctx, types.NamespacedName{Name: vpaName, Namespace: "ns1"}, vpa)
		assert.True(t, apierrors.IsNotFound(err))

		// Assert by gathering from the registry and finding the matching series.
		got := mustGetCounterValue(t, promReg,
			"autovpa_vpa_skipped_total",
			map[string]string{
				"namespace": "ns1",
				"name":      "demo",
				"kind":      "Deployment",
				"reason":    vpaSkipReasonAnnotationMissing,
			},
		)
		assert.Equal(t, float64(1), got)
	})
}

func TestBaseReconciler_buildDesiredVPA(t *testing.T) {
	t.Parallel()

	scheme := newScheme(t)
	logger := logr.Discard()
	br := BaseReconciler{
		KubeClient: fake.NewClientBuilder().WithScheme(scheme).Build(),
		Logger:     &logger,
		Meta: MetaConfig{
			ProfileKey:   "vpa/profile",
			ManagedLabel: "vpa/managed",
		},
		Profiles: ProfileConfig{
			NameTemplate: flag.DefaultNameTemplate,
		},
	}

	dep := &appsv1.Deployment{}
	dep.SetNamespace("ns1")
	dep.SetName("demo")

	profile := config.Profile{
		Spec: config.ProfileSpec{},
	}

	targetGVK := appsv1.SchemeGroupVersion.WithKind("Deployment")

	desired, err := br.buildDesiredVPA(dep, targetGVK, "p1", profile)
	require.NoError(t, err)

	// Expected name via the same template helper used in production.
	expectedName := renderDeploymentVPAName(t, "ns1", "demo", "p1")
	assert.Equal(t, expectedName, desired.Name)
	assert.Equal(t, "p1", desired.Profile)

	// Managed + profile labels must be present.
	assert.Equal(t, map[string]string{
		"vpa/managed": "true",
		"vpa/profile": "p1",
	}, desired.Labels)

	// Spec should contain a targetRef pointing to the workload.
	spec := desired.Spec
	targetRef, ok := spec["targetRef"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "demo", targetRef["name"])
	assert.Equal(t, "Deployment", targetRef["kind"])
}

func TestBaseReconciler_fetchExistingVPA(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	scheme := newScheme(t)
	logger := logr.Discard()

	// Build a client with no VPAs.
	client := fake.NewClientBuilder().WithScheme(scheme).Build()
	br := BaseReconciler{
		KubeClient: client,
		Logger:     &logger,
	}

	// Not found → nil, nil.
	obj, err := br.fetchExistingVPA(ctx, types.NamespacedName{Name: "missing", Namespace: "ns1"})
	require.NoError(t, err)
	assert.Nil(t, obj)

	// Create a VPA and ensure it is returned.
	existing := newVPAObject()
	existing.SetNamespace("ns1")
	existing.SetName("present")
	existing.Object["spec"] = map[string]any{"foo": "bar"}

	client = fake.NewClientBuilder().WithScheme(scheme).WithObjects(existing).Build()
	br.KubeClient = client

	obj, err = br.fetchExistingVPA(ctx, types.NamespacedName{Name: "present", Namespace: "ns1"})
	require.NoError(t, err)
	require.NotNil(t, obj)
	assert.Equal(t, "present", obj.GetName())
}

func TestBaseReconciler_mergeVPA(t *testing.T) {
	t.Parallel()

	scheme := newScheme(t)
	client := fake.NewClientBuilder().WithScheme(scheme).Build()
	logger := logr.Discard()

	br := BaseReconciler{
		KubeClient: client,
		Logger:     &logger,
	}

	existing := newVPAObject()
	existing.SetNamespace("ns1")
	existing.SetName("demo-vpa")
	existing.SetLabels(map[string]string{
		"keep":     "yes",
		"override": "old",
	})
	existing.Object["spec"] = map[string]any{
		"targetRef": map[string]any{"name": "demo"},
	}

	desired := desiredVPAState{
		Name:    "demo-vpa",
		Profile: "p1",
		Labels: map[string]string{
			"override":    "new",
			"vpa/managed": "true",
			"vpa/profile": "p1",
		},
		Spec: map[string]any{
			"targetRef": map[string]any{"name": "demo"},
			"foo":       "bar",
		},
	}

	owner := &appsv1.Deployment{}
	owner.SetNamespace("ns1")
	owner.SetName("demo")
	owner.SetUID("uid1")

	updated, err := br.mergeVPA(existing, desired, owner)
	require.NoError(t, err)

	// Existing must not be mutated.
	existingSpec := existing.Object["spec"].(map[string]any)
	assert.NotContains(t, existingSpec, "foo")

	// Labels must be merged, with desired overriding existing keys.
	gotLabels := updated.GetLabels()
	assert.Equal(t, "yes", gotLabels["keep"])
	assert.Equal(t, "new", gotLabels["override"])
	assert.Equal(t, "true", gotLabels["vpa/managed"])
	assert.Equal(t, "p1", gotLabels["vpa/profile"])

	// Spec must match desired.
	gotSpec := updated.Object["spec"].(map[string]any)
	assert.Equal(t, "bar", gotSpec["foo"])

	// Owner reference must be set with controller=true.
	owners := updated.GetOwnerReferences()
	require.Len(t, owners, 1)
	assert.Equal(t, "demo", owners[0].Name)
	require.NotNil(t, owners[0].Controller)
	assert.True(t, *owners[0].Controller)
}

func TestBaseReconciler_applyVPA(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	scheme := newScheme(t)
	logger := logr.Discard()

	// Seed cluster with an existing VPA.
	existing := newVPAObject()
	existing.SetNamespace("ns1")
	existing.SetName("demo-vpa")
	existing.Object["spec"] = map[string]any{"field": "old"}

	client := fake.NewClientBuilder().WithScheme(scheme).WithObjects(existing).Build()

	br := BaseReconciler{
		KubeClient: client,
		Logger:     &logger,
	}

	// Prepare an updated object with managedFields set to ensure they are cleared.
	toApply := existing.DeepCopy()
	toApply.Object["spec"] = map[string]any{"field": "new"}
	toApply.SetManagedFields([]metav1.ManagedFieldsEntry{
		{Manager: "test"},
	})

	err := br.applyVPA(ctx, toApply)
	require.NoError(t, err)

	// Ensure managedFields were cleared before/after the call.
	assert.Len(t, toApply.GetManagedFields(), 0)

	// Spec in the cluster should be updated.
	got := newVPAObject()
	err = client.Get(ctx, types.NamespacedName{Name: "demo-vpa", Namespace: "ns1"}, got)
	require.NoError(t, err)

	spec := got.Object["spec"].(map[string]any)
	assert.Equal(t, "new", spec["field"])
}

func TestBaseReconciler_createVPA(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	scheme := newScheme(t)
	logger := logr.Discard()

	client := fake.NewClientBuilder().WithScheme(scheme).Build()
	br := BaseReconciler{
		KubeClient: client,
		Logger:     &logger,
	}

	owner := &appsv1.Deployment{}
	owner.SetNamespace("ns1")
	owner.SetName("demo")
	owner.SetUID("uid1")

	err := br.createVPA(
		ctx,
		owner,
		"demo-vpa",
		map[string]string{"vpa/managed": "true"},
		map[string]any{"foo": "bar"},
	)
	require.NoError(t, err)

	// VPA should exist with expected fields and owner reference.
	got := newVPAObject()
	err = client.Get(ctx, types.NamespacedName{Name: "demo-vpa", Namespace: "ns1"}, got)
	require.NoError(t, err)

	assert.Equal(t, "demo-vpa", got.GetName())
	assert.Equal(t, "true", got.GetLabels()["vpa/managed"])

	spec := got.Object["spec"].(map[string]any)
	assert.Equal(t, "bar", spec["foo"])

	owners := got.GetOwnerReferences()
	require.Len(t, owners, 1)
	assert.Equal(t, "demo", owners[0].Name)
}

func TestBaseReconciler_updateVPA(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	scheme := newScheme(t)
	logger := logr.Discard()

	// Existing VPA in cluster.
	existing := newVPAObject()
	existing.SetNamespace("ns1")
	existing.SetName("demo-vpa")
	existing.Object["spec"] = map[string]any{"field": "old"}

	client := fake.NewClientBuilder().WithScheme(scheme).WithObjects(existing).Build()

	br := BaseReconciler{
		KubeClient: client,
		Logger:     &logger,
	}

	// Updated object to send.
	updated := existing.DeepCopy()
	updated.Object["spec"] = map[string]any{"field": "new"}

	err := br.updateVPA(ctx, updated)
	require.NoError(t, err)

	got := newVPAObject()
	err = client.Get(ctx, types.NamespacedName{Name: "demo-vpa", Namespace: "ns1"}, got)
	require.NoError(t, err)

	spec := got.Object["spec"].(map[string]any)
	assert.Equal(t, "new", spec["field"])
}

func TestBaseReconciler_listManagedVPAs(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	scheme := newScheme(t)
	logger := logr.Discard()

	// Two managed VPAs in ns1, one unmanaged, one managed in another namespace.
	vpa1 := newVPAObject()
	vpa1.SetNamespace("ns1")
	vpa1.SetName("vpa-managed-1")
	vpa1.SetLabels(map[string]string{"vpa/managed": "true"})

	vpa2 := newVPAObject()
	vpa2.SetNamespace("ns1")
	vpa2.SetName("vpa-unmanaged")
	vpa2.SetLabels(map[string]string{"other": "label"})

	vpa3 := newVPAObject()
	vpa3.SetNamespace("ns2")
	vpa3.SetName("vpa-managed-2")
	vpa3.SetLabels(map[string]string{"vpa/managed": "true"})

	client := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(vpa1, vpa2, vpa3).
		Build()

	br := BaseReconciler{
		KubeClient: client,
		Logger:     &logger,
		Meta: MetaConfig{
			ManagedLabel: "vpa/managed",
		},
	}

	list, err := br.listManagedVPAs(ctx, "ns1")
	require.NoError(t, err)

	// Only vpa1 should be returned for ns1.
	require.Len(t, list, 1)
	assert.Equal(t, "vpa-managed-1", list[0].GetName())
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
