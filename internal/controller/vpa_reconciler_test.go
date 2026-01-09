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

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
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
	const vpaName = "demo-vpa"

	t.Run("Returns nil when VPA is already deleted", func(t *testing.T) {
		t.Parallel()

		r := newTestVPAReconciler(t /* no objects */)

		_, err := r.Reconcile(
			context.Background(),
			ctrl.Request{NamespacedName: types.NamespacedName{Name: vpaName, Namespace: namespace}},
		)
		require.NoError(t, err)
	})

	t.Run("Skips unmanaged VPA (missing managed label)", func(t *testing.T) {
		t.Parallel()

		vpa := newVPAObject()
		vpa.SetNamespace(namespace)
		vpa.SetName(vpaName)
		vpa.SetLabels(map[string]string{}) // no managed label

		r := newTestVPAReconciler(t, vpa)

		_, err := r.Reconcile(
			context.Background(),
			ctrl.Request{NamespacedName: types.NamespacedName{Name: vpaName, Namespace: namespace}},
		)
		require.NoError(t, err)

		// VPA must still exist.
		got := newVPAObject()
		err = r.KubeClient.Get(context.Background(), client.ObjectKeyFromObject(vpa), got)
		require.NoError(t, err)
	})

	t.Run("Deletes orphaned managed VPA (no controller ownerRef)", func(t *testing.T) {
		t.Parallel()

		vpa := newManagedVPA(t, namespace, vpaName, "default")
		vpa.SetOwnerReferences(nil) // no ownerRefs

		r := newTestVPAReconciler(t, vpa)

		_, err := r.Reconcile(
			context.Background(),
			ctrl.Request{NamespacedName: types.NamespacedName{Name: vpaName, Namespace: namespace}},
		)
		require.NoError(t, err)

		got := newVPAObject()
		err = r.KubeClient.Get(context.Background(), client.ObjectKeyFromObject(vpa), got)
		assert.True(t, apierrors.IsNotFound(err))
	})

	t.Run("Deletes managed VPA when only non-controller ownerRefs exist", func(t *testing.T) {
		t.Parallel()

		// ownerRef exists but Controller is nil/false => resolveOwnerGVK should ignore it.
		vpa := newManagedVPA(t, namespace, vpaName, "default")
		vpa.SetOwnerReferences([]metav1.OwnerReference{
			{
				APIVersion: DeploymentGVK.GroupVersion().String(),
				Kind:       DeploymentGVK.Kind,
				Name:       ownerName,
				// Controller intentionally omitted
			},
		})

		r := newTestVPAReconciler(t, vpa)

		_, err := r.Reconcile(
			context.Background(),
			ctrl.Request{NamespacedName: types.NamespacedName{Name: vpaName, Namespace: namespace}},
		)
		require.NoError(t, err)

		got := newVPAObject()
		err = r.KubeClient.Get(context.Background(), client.ObjectKeyFromObject(vpa), got)
		assert.True(t, apierrors.IsNotFound(err))
	})

	t.Run("Deletes managed VPA when controller owner kind is unsupported", func(t *testing.T) {
		t.Parallel()

		vpa := newManagedVPA(t, namespace, vpaName, "default")
		vpa.SetOwnerReferences([]metav1.OwnerReference{
			{
				APIVersion: "batch/v1",
				Kind:       "Job",
				Name:       "job-owner",
				Controller: ptr.To(true),
			},
		})

		r := newTestVPAReconciler(t, vpa)

		_, err := r.Reconcile(
			context.Background(),
			ctrl.Request{NamespacedName: types.NamespacedName{Name: vpaName, Namespace: namespace}},
		)
		require.NoError(t, err)

		got := newVPAObject()
		err = r.KubeClient.Get(context.Background(), client.ObjectKeyFromObject(vpa), got)
		assert.True(t, apierrors.IsNotFound(err))
	})

	t.Run("Deletes managed VPA when controller owner does not exist in cluster", func(t *testing.T) {
		t.Parallel()

		vpa := newManagedVPA(t, namespace, vpaName, "default")
		vpa.SetOwnerReferences([]metav1.OwnerReference{deploymentOwnerRef(ownerName)})

		r := newTestVPAReconciler(t, vpa /* owner not created */)

		_, err := r.Reconcile(
			context.Background(),
			ctrl.Request{NamespacedName: types.NamespacedName{Name: vpaName, Namespace: namespace}},
		)
		require.NoError(t, err)

		got := newVPAObject()
		err = r.KubeClient.Get(context.Background(), client.ObjectKeyFromObject(vpa), got)
		assert.True(t, apierrors.IsNotFound(err))
	})

	t.Run("Keeps managed VPA when controller owner exists and is valid", func(t *testing.T) {
		t.Parallel()

		owner := newOwnerUnstructuredDeployment(t, namespace, ownerName)

		vpa := newManagedVPA(t, namespace, vpaName, "default")
		vpa.SetOwnerReferences([]metav1.OwnerReference{deploymentOwnerRef(ownerName)})

		r := newTestVPAReconciler(t, owner, vpa)

		_, err := r.Reconcile(
			context.Background(),
			ctrl.Request{NamespacedName: types.NamespacedName{Name: vpaName, Namespace: namespace}},
		)
		require.NoError(t, err)

		got := newVPAObject()
		err = r.KubeClient.Get(context.Background(), client.ObjectKeyFromObject(vpa), got)
		require.NoError(t, err)
	})
}

func TestVPAReconciler_skipUnmanaged(t *testing.T) {
	t.Parallel()

	t.Run("Returns false when managed label is true", func(t *testing.T) {
		t.Parallel()

		r := newTestVPAReconciler(t)

		vpa := newManagedVPA(t, "default", "vpa", "p")
		vpa.SetLabels(map[string]string{
			managedLabelKey: "true",
		})

		assert.False(t, r.skipUnmanaged(vpa))
	})

	t.Run("Returns true when managed label missing or not true", func(t *testing.T) {
		t.Parallel()

		r := newTestVPAReconciler(t)

		vpa := newVPAObject()
		vpa.SetNamespace("default")
		vpa.SetName("vpa")
		vpa.SetLabels(map[string]string{
			managedLabelKey: "false",
		})

		assert.True(t, r.skipUnmanaged(vpa))
	})
}

func TestVPAReconciler_resolveOwnerGVK(t *testing.T) {
	t.Parallel()

	t.Run("Returns matching controller owner ref for Deployment", func(t *testing.T) {
		t.Parallel()

		r := newTestVPAReconciler(t)

		vpa := newManagedVPA(t, "ns", "vpa", "p")
		vpa.SetOwnerReferences([]metav1.OwnerReference{
			deploymentOwnerRef("demo"),
		})

		gvk, name, found := r.resolveOwnerGVK(vpa)

		assert.True(t, found)
		assert.Equal(t, DeploymentGVK.Kind, gvk.Kind)
		assert.Equal(t, "demo", name)
	})

	t.Run("Returns not found when no controller ownerRef exists", func(t *testing.T) {
		t.Parallel()

		r := newTestVPAReconciler(t)

		vpa := newManagedVPA(t, "ns", "vpa", "p")
		vpa.SetOwnerReferences([]metav1.OwnerReference{
			{
				APIVersion: DeploymentGVK.GroupVersion().String(),
				Kind:       DeploymentGVK.Kind,
				Name:       "demo",
				Controller: ptr.To(false),
			},
		})

		gvk, name, found := r.resolveOwnerGVK(vpa)

		assert.False(t, found)
		assert.Empty(t, name)
		assert.Empty(t, gvk.Kind)
	})

	t.Run("Returns not found for unsupported controller kind", func(t *testing.T) {
		t.Parallel()

		r := newTestVPAReconciler(t)

		vpa := newManagedVPA(t, "ns", "vpa", "p")
		vpa.SetOwnerReferences([]metav1.OwnerReference{
			{
				APIVersion: "batch/v1",
				Kind:       "Job",
				Name:       "job",
				Controller: ptr.To(true),
			},
		})

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

		vpa := newManagedVPA(t, "ns", "vpa", "p")
		r := newTestVPAReconciler(t, vpa)

		err := r.deleteManagedVPA(context.Background(), vpa)
		require.NoError(t, err)

		got := newVPAObject()
		err = r.KubeClient.Get(context.Background(), client.ObjectKeyFromObject(vpa), got)
		assert.True(t, apierrors.IsNotFound(err))
	})

	t.Run("Ignores NotFound error", func(t *testing.T) {
		t.Parallel()

		r := newTestVPAReconciler(t)

		vpa := newManagedVPA(t, "ns", "missing", "p")
		// Not created in client.

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
			types.NamespacedName{Name: "none", Namespace: "ns"},
		)

		require.NoError(t, err)
		assert.Nil(t, obj)
	})

	t.Run("Returns VPA when found", func(t *testing.T) {
		t.Parallel()

		vpa := newManagedVPA(t, "ns", "vpa", "profile")
		r := newTestVPAReconciler(t, vpa)

		obj, err := r.fetchExistingVPA(context.Background(),
			types.NamespacedName{Name: "vpa", Namespace: "ns"},
		)

		require.NoError(t, err)
		require.NotNil(t, obj)
		assert.Equal(t, "vpa", obj.GetName())
	})
}

// -----------------------------------------------------------------------------
// Helpers
// -----------------------------------------------------------------------------

func newTestVPAReconciler(t *testing.T, objs ...client.Object) *VPAReconciler {
	t.Helper()

	scheme := runtime.NewScheme()

	// Register the GVKs we use as unstructured so the fake client can store/get them.
	scheme.AddKnownTypeWithName(vpaGVK, &unstructured.Unstructured{})
	scheme.AddKnownTypeWithName(vpaListGVK, &unstructured.UnstructuredList{})

	// Also allow unstructured reads for supported owner kinds (we Get into Unstructured).
	scheme.AddKnownTypeWithName(DeploymentGVK, &unstructured.Unstructured{})
	scheme.AddKnownTypeWithName(StatefulSetGVK, &unstructured.Unstructured{})
	scheme.AddKnownTypeWithName(DaemonSetGVK, &unstructured.Unstructured{})

	c := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(objs...).
		Build()

	logger := logr.Discard()

	return &VPAReconciler{
		KubeClient: c,
		Logger:     &logger,
		Recorder:   record.NewFakeRecorder(32),
		Meta: MetaConfig{
			ProfileKey:   profileKey,
			ManagedLabel: managedLabelKey,
		},
	}
}

func newManagedVPA(t *testing.T, namespace, name, profile string) *unstructured.Unstructured {
	t.Helper()

	vpa := newVPAObject()
	vpa.SetNamespace(namespace)
	vpa.SetName(name)
	vpa.SetLabels(map[string]string{
		managedLabelKey: "true",
		profileKey:      profile,
	})
	return vpa
}

func deploymentOwnerRef(name string) metav1.OwnerReference {
	return metav1.OwnerReference{
		APIVersion: DeploymentGVK.GroupVersion().String(),
		Kind:       DeploymentGVK.Kind,
		Name:       name,
		Controller: ptr.To(true),
	}
}

func newOwnerUnstructuredDeployment(t *testing.T, namespace, name string) *unstructured.Unstructured {
	t.Helper()

	u := &unstructured.Unstructured{}
	u.SetGroupVersionKind(DeploymentGVK)
	u.SetNamespace(namespace)
	u.SetName(name)
	return u
}
