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
	"fmt"

	"github.com/containeroo/autovpa/internal/config"
	"github.com/containeroo/autovpa/internal/metrics"
	"github.com/containeroo/autovpa/internal/utils"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	"k8s.io/utils/ptr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// desiredVPAState is the fully rendered desired state for a workload's VPA.
// It contains all fields required to create or update the VPA.
type desiredVPAState struct {
	Name    string            // VPA name rendered from the name template.
	Profile string            // Selected profile for the workload.
	Labels  map[string]string // Final merged labels (workload labels + managed/profile markers).
	Spec    map[string]any    // The VPA "spec" rendered from the selected profile.
}

// BaseReconciler contains the shared logic for Deployment/StatefulSet/DaemonSet reconcilers.
// It owns the profile configuration and implements the VPA lifecycle for a single workload.
type BaseReconciler struct {
	KubeClient client.Client
	Logger     *logr.Logger
	Recorder   record.EventRecorder
	Meta       MetaConfig
	Profiles   ProfileConfig
}

const fieldManager = "autovpa"

// ReconcileWorkload executes the full VPA lifecycle state machine for a workload.
//
// Algorithm overview:
//  1. Determine whether the workload opts into VPA management (profile annotation).
//  2. If not opted-in → delete all managed VPAs for this workload.
//  3. Resolve the profile to use.
//  4. Render the desired VPA name, labels, annotations and spec.
//  5. Delete obsolete VPAs (e.g. profile/name-template change).
//  6. Create the desired VPA if missing.
//  7. If it exists, merge and apply changes via server-side apply.
//
// This function NEVER requeues on configuration errors (e.g. profile missing) to
// avoid thrashing. It only returns a non-nil error when an API call fails.
func (b *BaseReconciler) ReconcileWorkload(
	ctx context.Context,
	obj client.Object,
	targetGVK schema.GroupVersionKind,
) (ctrl.Result, error) {
	name, ns := obj.GetName(), obj.GetNamespace()
	log := b.Logger.WithValues("namespace", ns, "workload", name)

	// Check profile annotation (opt-in).
	annotations := obj.GetAnnotations()
	profileName := annotations[b.Meta.ProfileKey]
	if profileName == "" {
		log.Info("profile missing; skipping VPA reconciliation",
			"annotation", b.Meta.ProfileKey,
		)

		b.Recorder.Event(
			obj,
			corev1.EventTypeWarning,
			"ProfileAnnotationMissing",
			fmt.Sprintf("annotation %q missing; skipping VPA", b.Meta.ProfileKey),
		)

		metrics.VPASkipped.WithLabelValues(ns, name, targetGVK.Kind, "annotation_missing").Inc()

		// User opted out → delete all operator-managed VPAs for this workload.
		if err := b.DeleteAllManagedVPAsForWorkload(ctx, obj, targetGVK.Kind); err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{}, nil // Do not return an error to avoid requeuing the workload.
	}

	// Resolve profile (fall back to default if annotation is "default"/empty).
	selectedProfile := utils.DefaultIfZero(profileName, b.Profiles.DefaultProfile)
	profile, found := b.Profiles.Profiles[selectedProfile]
	if !found {
		// Invalid configuration: profile doesn't exist. This is surfaced as an
		// Event and metric, but we do not requeue to avoid hot-looping until
		// someone fixes the profile config.
		log.Info("profile not found; skipping VPA reconciliation",
			"profile", selectedProfile,
		)

		b.Recorder.Eventf(
			obj,
			corev1.EventTypeWarning,
			"ProfileNotFound",
			"profile %q not found", selectedProfile,
		)

		metrics.VPASkipped.WithLabelValues(ns, name, targetGVK.Kind, "profile_missing").Inc()
		return ctrl.Result{}, nil // Do not return an error to avoid requeuing the workload.
	}

	// Build desired VPA state from the profile and workload.
	desired, err := b.buildDesiredVPA(obj, targetGVK, selectedProfile, profile)
	if err != nil {
		return ctrl.Result{}, err
	}

	// Delete obsolete VPAs (e.g. if name template or profile changed).
	if err := b.DeleteObsoleteManagedVPAs(ctx, obj, desired.Name); err != nil {
		return ctrl.Result{}, err
	}

	// Fetch or create the current VPA instance.
	existing, err := b.fetchExistingVPA(ctx, types.NamespacedName{Name: desired.Name, Namespace: ns})
	if err != nil {
		return ctrl.Result{}, err
	}

	// Create a new VPA when none exists yet.
	if existing == nil {
		if err := b.createVPA(ctx, obj, desired.Name, desired.Labels, desired.Spec); err != nil {
			return ctrl.Result{}, err
		}

		log.Info("created VPA",
			"vpa", desired.Name,
			"profile", selectedProfile,
		)

		b.Recorder.Eventf(
			obj,
			corev1.EventTypeNormal,
			"VPACreated",
			"Created VPA %s with profile %s", desired.Name, selectedProfile,
		)

		metrics.VPACreated.WithLabelValues(ns, name, targetGVK.Kind, selectedProfile).Inc()
		return ctrl.Result{}, nil
	}

	// Merge desired state into the existing VPA and apply any changes.
	updated, err := b.mergeVPA(existing, desired, obj)
	if err != nil {
		return ctrl.Result{}, err
	}

	// Short-circuit if nothing changed to avoid unnecessary API updates.
	if !vpaNeedsUpdate(existing, updated) {
		return ctrl.Result{}, nil
	}

	if err := b.updateVPA(ctx, updated); err != nil {
		return ctrl.Result{}, err
	}

	log.Info("updated VPA",
		"vpa", desired.Name,
		"profile", selectedProfile,
	)

	b.Recorder.Eventf(
		obj,
		corev1.EventTypeNormal,
		"VPAUpdated",
		"Updated VPA %s to profile %s", desired.Name, selectedProfile,
	)

	metrics.VPAUpdated.WithLabelValues(ns, name, targetGVK.Kind, selectedProfile).Inc()
	return ctrl.Result{}, nil
}

// DeleteObsoleteManagedVPAs deletes all managed VPAs owned by `owner` except
// the one named keepName. This handles profile/name-template changes.
func (b *BaseReconciler) DeleteObsoleteManagedVPAs(ctx context.Context, owner client.Object, keepName string) error {
	vpas, err := b.listManagedVPAs(ctx, owner.GetNamespace())
	if err != nil {
		return err
	}

	for _, vpa := range vpas {
		if vpa.GetName() == keepName {
			continue
		}
		// Only consider VPAs actually owned by this workload.
		if !metav1.IsControlledBy(vpa, owner) {
			continue
		}

		// When here, we know that the VPA is owned by the workload and the VPA name
		// has changed. Most likely the profile or name template changed, so the VPA
		// is obsolete and should be removed.
		if err := b.KubeClient.Delete(ctx, vpa); err != nil {
			return fmt.Errorf("delete obsolete VPA %s: %w", vpa.GetName(), err)
		}

		b.Logger.Info("deleted obsolete VPA",
			"vpa", vpa.GetName(),
			"namespace", owner.GetNamespace(),
			"workload", owner.GetName(),
		)

		b.Recorder.Eventf(
			owner,
			corev1.EventTypeNormal,
			"DeletedObsoleteVPA",
			"Deleted obsolete VPA %s", vpa.GetName(),
		)
	}

	return nil
}

// DeleteAllManagedVPAsForWorkload deletes every operator-managed VPA that is
// owned by the specified workload. This is used when a workload:
//   - is deleted
//   - removes its profile annotation
//   - or otherwise opts out of VPA management
func (b *BaseReconciler) DeleteAllManagedVPAsForWorkload(ctx context.Context, owner client.Object, workloadKind string) error {
	vpas, err := b.listManagedVPAs(ctx, owner.GetNamespace())
	if err != nil {
		return err
	}

	for _, vpa := range vpas {
		for _, ref := range vpa.GetOwnerReferences() {
			if ref.Kind != workloadKind || ref.Name != owner.GetName() {
				continue
			}

			if err := b.KubeClient.Delete(ctx, vpa); err != nil && !apierrors.IsNotFound(err) {
				return fmt.Errorf("delete VPA %s: %w", vpa.GetName(), err)
			}

			b.Logger.Info("deleted managed VPA for workload",
				"vpa", vpa.GetName(),
				"namespace", owner.GetNamespace(),
				"workload", owner.GetName(),
			)

			b.Recorder.Eventf(
				owner,
				corev1.EventTypeNormal,
				"DeletedManagedVPA",
				"Deleted managed VPA %s for workload %s", vpa.GetName(), owner.GetName(),
			)
		}
	}

	return nil
}

// buildDesiredVPA resolves the target VPA name, labels, and spec
// according to the selected profile and operator configuration.
func (b *BaseReconciler) buildDesiredVPA(
	obj client.Object,
	targetGVK schema.GroupVersionKind,
	selectedProfile string,
	profile config.Profile,
) (desiredVPAState, error) {
	// Select the name template: profile override or global default.
	templateStr := utils.DefaultIfZero(profile.NameTemplate, b.Profiles.NameTemplate)

	vpaName, err := RenderVPAName(templateStr, utils.NameTemplateData{
		WorkloadName: obj.GetName(),
		Namespace:    obj.GetNamespace(),
		Kind:         targetGVK.Kind,
		Profile:      selectedProfile,
	})
	if err != nil {
		return desiredVPAState{}, err
	}

	spec, err := buildVPASpec(profile.Spec, targetGVK, obj.GetName())
	if err != nil {
		return desiredVPAState{}, err
	}

	labels := map[string]string{
		b.Meta.ManagedLabel: "true",
		b.Meta.ProfileKey:   selectedProfile,
	}

	return desiredVPAState{
		Name:    vpaName,
		Profile: selectedProfile,
		Labels:  labels,
		Spec:    spec,
	}, nil
}

// fetchExistingVPA returns the VPA for the key or nil if not found.
func (b *BaseReconciler) fetchExistingVPA(ctx context.Context, key types.NamespacedName) (*unstructured.Unstructured, error) {
	obj := newVPAObject()
	if err := b.KubeClient.Get(ctx, key, obj); err != nil {
		if apierrors.IsNotFound(err) {
			return nil, nil
		}
		return nil, err
	}
	return obj, nil
}

// mergeVPA merges desired state into an existing VPA and sets the controller reference.
// The returned object is a deep copy and safe to mutate without affecting the cache.
func (b *BaseReconciler) mergeVPA(
	existing *unstructured.Unstructured,
	desired desiredVPAState,
	owner client.Object,
) (*unstructured.Unstructured, error) {
	updated := existing.DeepCopy() // never mutate cache objects
	updated.SetLabels(utils.MergeMaps(updated.GetLabels(), desired.Labels))
	updated.Object["spec"] = desired.Spec

	if err := ctrl.SetControllerReference(owner, updated, b.KubeClient.Scheme()); err != nil {
		return nil, err
	}
	return updated, nil
}

// applyVPA applies a VPA via server-side apply.
// managedFields must be stripped before sending the object, otherwise the API
// server rejects the request.
func (b *BaseReconciler) applyVPA(ctx context.Context, vpa *unstructured.Unstructured) error {
	// Avoid sending stale managedFields back to the API server on Apply.
	vpa.SetManagedFields(nil)

	return b.KubeClient.Patch(ctx, vpa, client.Apply, &client.PatchOptions{
		FieldManager: fieldManager,
		Force:        ptr.To(true),
	})
}

// createVPA builds and creates a new VPA owned by the workload.
func (b *BaseReconciler) createVPA(
	ctx context.Context,
	owner client.Object,
	name string,
	labels map[string]string,
	spec map[string]any,
) error {
	vpa := newVPAObject()
	vpa.SetName(name)
	vpa.SetNamespace(owner.GetNamespace())
	vpa.SetLabels(labels)
	vpa.Object["spec"] = spec

	// Ensure the workload owns the VPA for garbage collection and intent tracking.
	if err := ctrl.SetControllerReference(owner, vpa, b.KubeClient.Scheme()); err != nil {
		return err
	}

	return b.applyVPA(ctx, vpa)
}

// updateVPA updates the given VPA via server-side apply.
func (b *BaseReconciler) updateVPA(ctx context.Context, updated *unstructured.Unstructured) error {
	return b.applyVPA(ctx, updated)
}

// listManagedVPAs returns all VPA resources in the namespace that carry the
// operator's managed label. This is the basis for cleanup logic.
func (b *BaseReconciler) listManagedVPAs(ctx context.Context, namespace string) ([]*unstructured.Unstructured, error) {
	list := &unstructured.UnstructuredList{}
	list.SetGroupVersionKind(vpaListGVK)

	if err := b.KubeClient.List(
		ctx,
		list,
		client.InNamespace(namespace),
		client.MatchingLabels{b.Meta.ManagedLabel: "true"},
	); err != nil {
		return nil, fmt.Errorf("list managed VPAs: %w", err)
	}

	res := make([]*unstructured.Unstructured, len(list.Items))
	for i := range list.Items {
		res[i] = &list.Items[i]
	}
	return res, nil
}
