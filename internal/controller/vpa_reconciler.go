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

	"github.com/containeroo/autovpa/internal/metrics"
	"github.com/containeroo/autovpa/internal/predicates"
	"github.com/go-logr/logr"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// VPAReconciler enforces the *structural correctness* of managed VPAs.
//
// This controller is intentionally lightweight. It performs exactly two tasks:
//
//  1. Delete orphaned VPAs
//     - VPAs with the managed label
//     - but no valid controller ownerRef
//
//  2. Delete VPAs whose owner object has been deleted
//
// All *desired state* (name, labels, annotations, spec) and lifecycle (create/update)
// is handled exclusively by the workload reconcilers.
//
// The VPA reconciler therefore operates as a safety net: it ensures that only
// valid, workload-owned VPAs remain in the cluster, avoiding resource leaks or
// stale VPAs left behind by user modification or partial cleanup.
type VPAReconciler struct {
	KubeClient client.Client        // Kubernetes API client.
	Logger     *logr.Logger         // Logger for reconciliation events.
	Recorder   record.EventRecorder // Event recorder for Kubernetes events.
	Meta       MetaConfig           // Operator metadata (managed label, profile label, etc.).
}

// Event types.
const (
	vpaEventOrphaned     = "OrphanedVPA"
	vpaEventOwnerDeleted = "OwnerDeleted"
)

// Reconcile validates a VPA's ownership and deletes VPAs that:
//   - are marked as managed, but
//   - have no valid controller ownerRef, or
//   - reference an owner object that no longer exists.
//
// The reconciler does *not* attempt to recreate or update VPAs -- that is delegated
// entirely to workload reconcilers (DeploymentReconciler, StatefulSetReconciler, etc.).
func (r *VPAReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := r.Logger.WithValues(
		"namespace", req.Namespace,
		"vpa", req.Name,
		"controller", vpaGVK,
	)

	// Load the VPA; if missing, nothing to do.
	vpa, err := r.fetchExistingVPA(ctx, req.NamespacedName)
	if err != nil {
		metrics.ReconcileErrors.WithLabelValues("vpa", vpaGVK.Kind, "get").Inc()
		return ctrl.Result{}, err
	}
	if vpa == nil {
		log.Info("managed VPA already deleted")
		return ctrl.Result{}, nil
	}

	// Skip unmanaged VPAs (these are effectively user-owned VPAs).
	if r.skipUnmanaged(vpa) {
		log.Info("managed label removed; skipping VPA reconciliation")
		return ctrl.Result{}, nil
	}

	vpaName, vpaNamespace := vpa.GetName(), vpa.GetNamespace()

	// Validate controller ownerRef.
	gvk, ownerName, found := r.resolveOwnerGVK(vpa)
	if !found {
		log.Info("orphaned managed VPA has no controller owner")
		r.Recorder.Eventf(
			vpa,
			corev1.EventTypeNormal,
			vpaEventOrphaned,
			"%s/%s has no controller owner", vpaNamespace, vpaName,
		)

		if err := r.deleteManagedVPA(ctx, vpa); err != nil {
			metrics.ReconcileErrors.WithLabelValues("vpa", vpaGVK.Kind, "delete").Inc()
			return ctrl.Result{}, err
		}

		profile := profileFromLabels(vpa.GetLabels(), r.Meta.ProfileKey)
		metrics.VPADeletedOrphaned.WithLabelValues(vpaNamespace).Inc()
		metrics.VPAManaged.WithLabelValues(vpaNamespace, profile).Dec()
		return ctrl.Result{}, nil
	}

	// Confirm the referenced owner object still exists.
	owner, err := r.fetchOwner(ctx, gvk, vpaNamespace, ownerName)
	if err != nil {
		if !apierrors.IsNotFound(err) {
			// Error is not "not found" → return to retry.
			metrics.ReconcileErrors.WithLabelValues("vpa", vpaGVK.Kind, "fetch_owner").Inc()
			return ctrl.Result{}, err
		}

		// Owner object really is gone → delete VPA.
		log.Info("owner gone; deleting VPA",
			"ownerKind", gvk.Kind,
			"ownerName", ownerName,
		)
		r.Recorder.Eventf(
			vpa,
			corev1.EventTypeNormal,
			vpaEventOwnerDeleted,
			"owner %s %s/%s gone; deleting VPA %s", gvk.Kind, vpaNamespace, ownerName, vpaName,
		)

		if err := r.deleteManagedVPA(ctx, vpa); err != nil {
			metrics.ReconcileErrors.WithLabelValues("vpa", vpaGVK.Kind, "delete").Inc()
			return ctrl.Result{}, err
		}

		profile := profileFromLabels(vpa.GetLabels(), r.Meta.ProfileKey)
		metrics.VPADeletedOwnerGone.WithLabelValues(vpaNamespace, gvk.Kind).Inc()
		metrics.VPAManaged.WithLabelValues(vpaNamespace, profile).Dec()
		return ctrl.Result{}, nil
	}

	// Happy path -- the VPA is valid and managed.
	log.Info("managed VPA has valid controller owner",
		"ownerKind", gvk.Kind,
		"ownerName", owner.GetName(),
	)
	return ctrl.Result{}, nil
}

// SetupWithManager configures this controller to watch VPAs with the managed label.
//
// We filter events using ManagedVPALifecycle so the reconciler only receives
// meaningful transitions (creation, deletion, or managed-label changes) and avoids
// unnecessary noise.
func (r *VPAReconciler) SetupWithManager(mgr ctrl.Manager) error {
	vpa := newVPAObject()

	return ctrl.NewControllerManagedBy(mgr).
		// Primary resource.
		For(vpa).
		// Only care about VPAs we mark as managed.
		WithEventFilter(predicates.ManagedVPALifecycle(r.Meta.ManagedLabel)).
		Complete(r)
}

// resolveOwnerGVK inspects controller ownerRefs and returns the GVK + name of
// the owner workload. Only Deployment/StatefulSet/DaemonSet owners are supported.
func (r *VPAReconciler) resolveOwnerGVK(
	vpa *unstructured.Unstructured,
) (gvk schema.GroupVersionKind, ownerName string, found bool) {
	for _, owner := range vpa.GetOwnerReferences() {
		if owner.Controller == nil || !*owner.Controller {
			continue
		}

		switch owner.Kind {
		case DeploymentGVK.Kind:
			return DeploymentGVK, owner.Name, true
		case StatefulSetGVK.Kind:
			return StatefulSetGVK, owner.Name, true
		case DaemonSetGVK.Kind:
			return DaemonSetGVK, owner.Name, true
		}
	}

	return schema.GroupVersionKind{}, "", false
}

// skipUnmanaged returns true if the VPA does *not* carry the operator's
// managed label. Such VPAs are ignored; they are treated as user-managed.
func (r *VPAReconciler) skipUnmanaged(vpa *unstructured.Unstructured) bool {
	labels := vpa.GetLabels()
	return labels[r.Meta.ManagedLabel] != "true"
}

// fetchOwner loads the controller owner object by its GroupVersionKind.
func (r *VPAReconciler) fetchOwner(
	ctx context.Context,
	gvk schema.GroupVersionKind,
	namespace, name string,
) (*unstructured.Unstructured, error) {
	owner := &unstructured.Unstructured{}
	owner.SetGroupVersionKind(gvk)

	if err := r.KubeClient.Get(ctx, types.NamespacedName{
		Namespace: namespace,
		Name:      name,
	}, owner); err != nil {
		return nil, err
	}

	return owner, nil
}

// fetchExistingVPA loads a VPA or returns nil if it does not exist.
func (r *VPAReconciler) fetchExistingVPA(
	ctx context.Context,
	key types.NamespacedName,
) (*unstructured.Unstructured, error) {
	obj := newVPAObject()
	if err := r.KubeClient.Get(ctx, key, obj); err != nil {
		if apierrors.IsNotFound(err) {
			return nil, nil
		}
		return nil, err
	}
	return obj, nil
}

// deleteManagedVPA deletes a VPA, ignoring "not found" errors.
func (r *VPAReconciler) deleteManagedVPA(
	ctx context.Context,
	vpa client.Object,
) error {
	if err := r.KubeClient.Delete(ctx, vpa); err != nil {
		if apierrors.IsNotFound(err) {
			return nil
		}
		return err
	}
	return nil
}
