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
// This controller is intentionally lightweight and defensive.
// It does NOT manage desired state (spec, labels, naming).
//
// Responsibilities:
//  1. Delete managed VPAs that have no valid controller ownerRef (orphans).
//  2. Delete managed VPAs whose referenced owner object no longer exists.
//
// All desired-state reconciliation (create/update/snap-back) is handled
// exclusively by workload reconcilers (DeploymentReconciler, etc.).
//
// The VPAReconciler acts purely as a *safety net* to prevent resource leaks
// and stale VPAs caused by user tampering or partial cleanup.
type VPAReconciler struct {
	// KubeClient is the Kubernetes API client used for reads and deletes.
	KubeClient client.Client

	// Logger is used for structured reconciliation logging.
	Logger *logr.Logger

	// Recorder emits Kubernetes events for visibility.
	Recorder record.EventRecorder

	// Meta contains operator metadata such as label keys.
	Meta MetaConfig

	// Metrics holds the Metrics
	Metrics *metrics.Registry
}

// Kubernetes event reasons emitted by the VPAReconciler.
const (
	// vpaEventOrphaned is emitted when a managed VPA has no controller ownerRef.
	vpaEventOrphaned = "OrphanedVPA"

	// vpaEventOwnerDeleted is emitted when the owner workload no longer exists.
	vpaEventOwnerDeleted = "OwnerDeleted"
)

// Reconcile validates a managed VPA’s ownership and deletes invalid VPAs.
//
// A VPA is deleted when:
//   - it carries the managed label, AND
//   - it has no controller ownerRef, OR
//   - its controller ownerRef points to a non-existent workload.
//
// The reconciler never creates or updates VPAs.
// It only deletes invalid ones.
//
// Errors are returned only for failed API operations;
// “not found” conditions are treated as terminal and non-fatal.
func (r *VPAReconciler) Reconcile(
	ctx context.Context,
	req ctrl.Request,
) (ctrl.Result, error) {
	log := r.Logger.WithValues(
		"namespace", req.Namespace,
		"vpa", req.Name,
		"controller", vpaGVK.Kind,
	)

	// Load the VPA; if it no longer exists, nothing to do.
	vpa, err := r.fetchExistingVPA(ctx, req.NamespacedName)
	if err != nil {
		r.Metrics.IncReconcileErrors("vpa", vpaGVK.Kind, "get")
		return ctrl.Result{}, err
	}
	if vpa == nil {
		log.Info("managed VPA already deleted")
		return ctrl.Result{}, nil
	}

	// Ignore unmanaged (user-owned) VPAs entirely.
	if r.skipUnmanaged(vpa) {
		log.Info("managed label removed; skipping VPA reconciliation")
		return ctrl.Result{}, nil
	}

	vpaName := vpa.GetName()
	vpaNamespace := vpa.GetNamespace()

	// Validate controller ownerRef.
	gvk, ownerName, found := r.resolveOwnerGVK(vpa)
	if !found {
		// Managed VPA without controller owner → orphan.
		log.Info("orphaned managed VPA has no controller owner")

		r.Recorder.Eventf(
			vpa,
			corev1.EventTypeNormal,
			vpaEventOrphaned,
			"%s/%s has no controller owner", vpaNamespace, vpaName,
		)

		if err := r.deleteManagedVPA(ctx, vpa); err != nil {
			r.Metrics.IncReconcileErrors("vpa", vpaGVK.Kind, "delete")
			return ctrl.Result{}, err
		}

		profile := profileFromLabels(vpa.GetLabels(), r.Meta.ProfileKey)
		r.Metrics.IncVPADeletedOrphaned(vpaNamespace)
		r.Metrics.DecVPAManaged(vpaNamespace, profile)
		return ctrl.Result{}, nil
	}

	// Verify that the referenced owner object still exists.
	owner, err := r.fetchOwner(ctx, gvk, vpaNamespace, ownerName)
	if err != nil {
		if !apierrors.IsNotFound(err) {
			// Transient API error → retry.
			r.Metrics.IncReconcileErrors("vpa", vpaGVK.Kind, "fetch_owner")
			return ctrl.Result{}, err
		}

		// Owner object is gone → delete managed VPA.
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
			r.Metrics.IncReconcileErrors("vpa", vpaGVK.Kind, "delete")
			return ctrl.Result{}, err
		}

		profile := profileFromLabels(vpa.GetLabels(), r.Meta.ProfileKey)
		r.Metrics.IncVPADeletedOwnerGone(vpaNamespace, gvk.Kind)
		r.Metrics.DecVPAManaged(vpaNamespace, profile)
		return ctrl.Result{}, nil
	}

	// Happy path: managed VPA with valid controller owner.
	log.Info("managed VPA has valid controller owner",
		"ownerKind", gvk.Kind,
		"ownerName", owner.GetName(),
	)

	return ctrl.Result{}, nil
}

// SetupWithManager wires the VPAReconciler into the controller manager.
//
// The reconciler watches only VPAs and uses a structural predicate to ensure
// it is triggered exclusively by meaningful lifecycle or ownership changes.
func (r *VPAReconciler) SetupWithManager(mgr ctrl.Manager) error {
	vpa := newVPAObject()

	return ctrl.NewControllerManagedBy(mgr).
		// Primary resource: VPAs.
		For(vpa).
		// Filter to structural transitions only.
		WithEventFilter(
			predicates.ManagedVPAStructuralLifecycle(r.Meta.ManagedLabel),
		).
		Complete(r)
}

// resolveOwnerGVK extracts the controller ownerRef from a VPA and returns
// its GroupVersionKind and name.
//
// Only controller ownerRefs for supported workload types are considered.
// If no valid controller ownerRef is found, found=false is returned.
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

// skipUnmanaged returns true if the VPA does not carry the operator’s
// managed label with value "true".
//
// Such VPAs are treated as user-managed and ignored entirely.
func (r *VPAReconciler) skipUnmanaged(
	vpa *unstructured.Unstructured,
) bool {
	labels := vpa.GetLabels()
	return labels[r.Meta.ManagedLabel] != "true"
}

// fetchOwner retrieves the controller owner object for a VPA.
//
// The GroupVersionKind determines the workload type.
// A NotFound error indicates the owner has been deleted.
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

// fetchExistingVPA loads a VPA by name/namespace.
//
// If the VPA does not exist, (nil, nil) is returned.
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

// deleteManagedVPA deletes the given VPA.
//
// NotFound errors are ignored to make deletion idempotent.
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
