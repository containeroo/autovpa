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

	"github.com/go-logr/logr"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	appsv1 "k8s.io/api/apps/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	"k8s.io/utils/ptr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

const (
	profileKey      = "autovpa.containeroo.ch/profile"
	managedLabelKey = "autovpa.containeroo.ch/managed"
)

func TestVPAReconciler_Reconcile(t *testing.T) {
	t.Parallel()

	const namespace = "default"
	const ownerName = "demo"
	vpaName := "demo-vpa"

	t.Run("Deletes orphaned managed VPA (has managed label but no controller owner)", func(t *testing.T) {
		t.Parallel()

		// VPA with managed label but no ownerRef
		vpa := newManagedVPA(t, namespace, vpaName, "default",
			metav1.OwnerReference{}, // not a real controller owner
		)
		vpa.SetOwnerReferences(nil) // ensure no owner

		reconciler := newTestVPAReconciler(t, vpa)

		_, err := reconciler.Reconcile(
			context.Background(),
			ctrl.Request{NamespacedName: types.NamespacedName{Name: vpaName, Namespace: namespace}},
		)
		require.NoError(t, err)

		obj := newVPAObject()
		err = reconciler.KubeClient.Get(
			context.Background(),
			client.ObjectKey{Name: vpaName, Namespace: namespace},
			obj,
		)
		assert.True(t, apierrors.IsNotFound(err))
	})

	t.Run("Skips unmanaged VPA (missing managed label)", func(t *testing.T) {
		t.Parallel()

		vpa := newVPAObject()
		vpa.SetNamespace(namespace)
		vpa.SetName(vpaName)
		vpa.SetLabels(map[string]string{}) // no managed label

		reconciler := newTestVPAReconciler(t, vpa)

		_, err := reconciler.Reconcile(
			context.Background(),
			ctrl.Request{NamespacedName: types.NamespacedName{Name: vpaName, Namespace: namespace}},
		)
		require.NoError(t, err)

		// VPA must remain untouched
		obj := newVPAObject()
		err = reconciler.KubeClient.Get(context.Background(),
			client.ObjectKey{Name: vpaName, Namespace: namespace}, obj)
		require.NoError(t, err)
	})

	t.Run("Deletes VPA when owner exists in ownerRef but not in cluster", func(t *testing.T) {
		t.Parallel()

		// VPA ownerRef points to nonexistent workload
		vpa := newManagedVPA(t, namespace, vpaName, "default", deploymentOwnerRef(t, ownerName))
		reconciler := newTestVPAReconciler(t, vpa)

		_, err := reconciler.Reconcile(
			context.Background(),
			ctrl.Request{NamespacedName: types.NamespacedName{Name: vpaName, Namespace: namespace}},
		)
		require.NoError(t, err)

		obj := newVPAObject()
		err = reconciler.KubeClient.Get(context.Background(),
			client.ObjectKey{Name: vpaName, Namespace: namespace}, obj)
		assert.True(t, apierrors.IsNotFound(err))
	})

	t.Run("Keeps VPA when owner exists and is valid", func(t *testing.T) {
		t.Parallel()

		owner := newDeployment(t, ownerName, namespace, nil)
		vpa := newManagedVPA(t, namespace, vpaName, "default", deploymentOwnerRef(t, ownerName))
		reconciler := newTestVPAReconciler(t, owner, vpa)

		_, err := reconciler.Reconcile(
			context.Background(),
			ctrl.Request{NamespacedName: types.NamespacedName{Name: vpaName, Namespace: namespace}},
		)
		require.NoError(t, err)

		obj := newVPAObject()
		err = reconciler.KubeClient.Get(context.Background(),
			client.ObjectKey{Name: vpaName, Namespace: namespace}, obj)
		require.NoError(t, err)
	})
}

func TestVPAReconciler_skipUnmanaged(t *testing.T) {
	t.Parallel()

	t.Run("Returns false when managed label is true", func(t *testing.T) {
		t.Parallel()

		reconciler := newTestVPAReconciler(t)
		vpa := newManagedVPA(t, "default", "vpa", "profile", deploymentOwnerRef(t, "owner"))

		skip := reconciler.skipUnmanaged(vpa)
		assert.False(t, skip)
	})

	t.Run("Returns true when managed label is missing or not true", func(t *testing.T) {
		t.Parallel()

		reconciler := newTestVPAReconciler(t)

		vpa := newVPAObject()
		vpa.SetNamespace("default")
		vpa.SetName("vpa-without-label")
		vpa.SetLabels(map[string]string{
			managedLabelKey: "false",
		})

		skip := reconciler.skipUnmanaged(vpa)
		assert.True(t, skip)
	})
}

func TestVPAReconciler_resolveOwnerGVK(t *testing.T) {
	t.Parallel()

	t.Run("Returns matching controller owner ref", func(t *testing.T) {
		t.Parallel()

		vpa := newManagedVPA(t, "ns", "vpa", "p", deploymentOwnerRef(t, "demo"))
		r := newTestVPAReconciler(t)

		gvk, name, found := r.resolveOwnerGVK(vpa)

		assert.True(t, found)
		assert.Equal(t, DeploymentGVK.Kind, gvk.Kind)
		assert.Equal(t, "demo", name)
	})

	t.Run("Returns not found when no controller ownerRef", func(t *testing.T) {
		t.Parallel()

		r := newTestVPAReconciler(t)

		vpa := newVPAObject()
		vpa.SetNamespace("ns")
		vpa.SetName("vpa")

		gvk, name, found := r.resolveOwnerGVK(vpa)

		assert.False(t, found)
		assert.Empty(t, name)
		assert.Empty(t, gvk.Kind)
	})
}

func TestVPAReconciler_deleteManagedVPA(t *testing.T) {
	t.Parallel()

	t.Run("Deletes existing VPA", func(t *testing.T) {
		t.Parallel()

		vpa := newManagedVPA(t, "ns", "vpa", "p", deploymentOwnerRef(t, "o"))
		r := newTestVPAReconciler(t, vpa)

		err := r.deleteManagedVPA(context.Background(), vpa)
		require.NoError(t, err)

		obj := newVPAObject()
		err = r.KubeClient.Get(context.Background(),
			client.ObjectKey{Name: "vpa", Namespace: "ns"}, obj)
		assert.True(t, apierrors.IsNotFound(err))
	})

	t.Run("Ignores NotFound error", func(t *testing.T) {
		t.Parallel()

		r := newTestVPAReconciler(t)

		vpa := newManagedVPA(t, "ns", "missing", "p", deploymentOwnerRef(t, "o"))
		err := r.deleteManagedVPA(context.Background(), vpa)

		require.NoError(t, err)
	})
}

func TestVPAReconciler_fetchExistingVPA(t *testing.T) {
	t.Parallel()

	t.Run("Returns nil when VPA not found", func(t *testing.T) {
		t.Parallel()

		r := newTestVPAReconciler(t)

		obj, err := r.fetchExistingVPA(context.Background(),
			types.NamespacedName{Name: "none", Namespace: "ns"})

		require.NoError(t, err)
		assert.Nil(t, obj)
	})

	t.Run("Returns VPA when found", func(t *testing.T) {
		t.Parallel()

		vpa := newManagedVPA(t, "ns", "vpa", "profile", deploymentOwnerRef(t, "o"))
		r := newTestVPAReconciler(t, vpa)

		obj, err := r.fetchExistingVPA(context.Background(),
			types.NamespacedName{Name: "vpa", Namespace: "ns"})

		require.NoError(t, err)
		assert.NotNil(t, obj)
	})
}

// -----------------------------------------------------------------------------
// Helpers
// -----------------------------------------------------------------------------

func newTestVPAReconciler(t *testing.T, objs ...client.Object) *VPAReconciler {
	t.Helper()

	scheme := newScheme(t)
	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(objs...).Build()
	logger := logr.Discard()

	return &VPAReconciler{
		KubeClient: c,
		Logger:     &logger,
		Recorder:   record.NewFakeRecorder(10),
		Meta: MetaConfig{
			ProfileKey:   profileKey,
			ManagedLabel: managedLabelKey,
		},
	}
}

func newManagedVPA(t *testing.T, namespace, name, profile string, ownerRef metav1.OwnerReference) *unstructured.Unstructured {
	t.Helper()

	vpa := newVPAObject()
	vpa.SetNamespace(namespace)
	vpa.SetName(name)
	vpa.SetLabels(map[string]string{
		managedLabelKey: "true",
		profileKey:      profile,
	})
	vpa.SetOwnerReferences([]metav1.OwnerReference{ownerRef})
	return vpa
}

func deploymentOwnerRef(t *testing.T, name string) metav1.OwnerReference {
	t.Helper()

	return metav1.OwnerReference{
		APIVersion: DeploymentGVK.GroupVersion().String(),
		Kind:       DeploymentGVK.Kind,
		Name:       name,
		Controller: ptr.To(true),
	}
}

func newDeployment(t *testing.T, name, namespace string, annotations map[string]string) *appsv1.Deployment {
	t.Helper()

	return &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:        name,
			Namespace:   namespace,
			Annotations: annotations,
		},
	}
}
