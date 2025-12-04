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

// desiredVPAState captures desired fields for reconciliation.
type desiredVPAState struct {
	Name        string            // Name is the VPA name.
	Profile     string            // Profile is the selected profile.
	Labels      map[string]string // Labels are the VPA labels.
	Annotations map[string]string // Annotations are the VPA annotations.
	Spec        map[string]any    // Spec is the VPA spec.
}

// BaseReconciler contains shared fields for reconcilers.
type BaseReconciler struct {
	KubeClient client.Client        // KubeClient is the Kubernetes API client.
	Logger     *logr.Logger         // Logger is used for logging reconciliation events.
	Recorder   record.EventRecorder // Recorder records Kubernetes events.
	Meta       MetaConfig           // Meta contains the metadata configuration.
	Profiles   ProfileConfig        // Profiles contains the profile configuration.
}

const fieldManager = "autovpa"

// ReconcileWorkload performs the full reconciliation loop for a workload:
// it resolves the desired VPA name/spec from the selected profile, prunes obsolete
// managed VPAs for the workload, and creates or updates the current VPA.
func (b *BaseReconciler) ReconcileWorkload(ctx context.Context, obj client.Object, targetGVK schema.GroupVersionKind) (ctrl.Result, error) {
	name, ns := obj.GetName(), obj.GetNamespace()

	// Look up any existing VPA the operator manages for this workload.
	annotations := obj.GetAnnotations()
	profileName := annotations[b.Meta.ProfileAnnotation]
	if profileName == "" {
		b.Logger.Info("profile annotation missing, skipping VPA", "annotation", b.Meta.ProfileAnnotation)
		b.Recorder.Event(
			obj,
			corev1.EventTypeWarning,
			"ProfileAnnotationMissing",
			fmt.Sprintf("annotation %q missing; skipping VPA", b.Meta.ProfileAnnotation),
		)
		metrics.VPASkipped.WithLabelValues(ns, name, targetGVK.Kind, "annotation_missing").Inc()

		// Profile annotation value was removed, so we do need to cleanup managed VPAs.
		if err := b.purgeManagedVPAsForWorkload(ctx, obj, targetGVK.Kind); err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{}, nil // Do not return an error to avoid requeuing the workload.
	}

	// Select the profile to use.
	selectedProfile := utils.DefaultIfZero(profileName, b.Profiles.DefaultProfile)

	// Validate the selected profile.
	profile, found := b.Profiles.Profiles[selectedProfile]
	if !found {
		// Warn but do not requeue; requeue would thrash until a valid profile exists.
		b.Logger.Info("profile not found", "profile", selectedProfile)
		b.Recorder.Event(obj, corev1.EventTypeWarning, "ProfileMissing", fmt.Sprintf("profile %q not found", selectedProfile))
		metrics.VPASkipped.WithLabelValues(ns, name, targetGVK.Kind, "profile_missing").Inc()
		return ctrl.Result{}, nil // Do not return an error to avoid requeuing the workload.
	}

	// Build the desired VPA template according to the profile.
	desired, err := b.buildDesiredVPA(obj, targetGVK, selectedProfile, profile)
	if err != nil {
		return ctrl.Result{}, err
	}

	// Remove obsolete managed VPAs if the name template or profile changed.
	if err := b.pruneManagedVPAsExcept(ctx, obj, desired.Name); err != nil {
		return ctrl.Result{}, err
	}

	// Try to fetch the existing VPA by name.
	existing, err := b.fetchExistingVPA(ctx, types.NamespacedName{Name: desired.Name, Namespace: ns})
	if err != nil {
		return ctrl.Result{}, err
	}

	// If the VPA does not exist, create it from scratch.
	// Use the desired template for the new VPA.
	if existing == nil {
		if err := b.createVPA(ctx, obj, desired.Name, desired.Labels, desired.Annotations, desired.Spec, selectedProfile); err != nil {
			return ctrl.Result{}, err
		}
		metrics.VPACreated.WithLabelValues(ns, name, targetGVK.Kind, selectedProfile).Inc()
		return ctrl.Result{}, nil
	}

	// VPA exists: merge exsting VPA into desired VPA so later we can compare/apply updates.
	// desired object can contain new or updated fields that are not part of the existing VPA.
	updated, err := b.mergeVPA(existing, desired, obj)
	if err != nil {
		return ctrl.Result{}, err
	}

	// Short-circuit if nothing changed to avoid unnecessary API updates.
	if !vpaNeedsUpdate(existing, updated) {
		return ctrl.Result{}, nil
	}

	// Something changed, update the VPA.
	if err := b.updateVPA(ctx, updated); err != nil {
		return ctrl.Result{}, err
	}

	b.Recorder.Eventf(obj, corev1.EventTypeNormal, "VPAUpdated", "Updated VPA %s to profile %s", desired.Name, selectedProfile)
	metrics.VPAUpdated.WithLabelValues(ns, name, targetGVK.Kind, selectedProfile).Inc()
	b.Logger.Info("updated VPA", "vpa", desired.Name, "profile", selectedProfile)

	return ctrl.Result{}, nil
}

// buildDesiredVPA resolves the target VPA name, labels, annotations, and spec for a workload/profile.
// It applies profile overrides (name template, spec) and managed markers/annotations.
func (b *BaseReconciler) buildDesiredVPA(
	obj client.Object,
	targetGVK schema.GroupVersionKind,
	selectedProfile string,
	profile config.Profile,
) (desiredVPAState, error) {
	// Compute the desired VPA name from the template (profile override or default).
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

	// Merge labels/annotations so managed markers and workload labels propagate to the VPA.
	labels := utils.MergeMaps(obj.GetLabels(), map[string]string{
		b.Meta.ManagedLabel: "true",
	})

	annotations := withArgoTrackingAnnotation(
		b.Meta.ArgoManaged,
		b.Meta.ArgoTrackingAnnotation,
		obj.GetAnnotations(),
	)

	return desiredVPAState{
		Name:        vpaName,
		Profile:     selectedProfile,
		Labels:      labels,
		Annotations: annotations,
		Spec:        spec,
	}, nil
}

// fetchExistingVPA returns the existing VPA for the given key or nil if not found.
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

// mergeVPA merges desired labels/annotations/spec into an existing VPA and sets ownership.
// The returned object is safe to mutate without touching the cache.
func (b *BaseReconciler) mergeVPA(
	existing *unstructured.Unstructured,
	desired desiredVPAState,
	owner client.Object,
) (*unstructured.Unstructured, error) {
	// Start from a copy to avoid mutating cache objects.
	updated := existing.DeepCopy()
	updated.SetLabels(utils.MergeMaps(updated.GetLabels(), desired.Labels))
	updated.SetAnnotations(utils.MergeMaps(updated.GetAnnotations(), desired.Annotations))
	updated.Object["spec"] = desired.Spec

	if err := ctrl.SetControllerReference(owner, updated, b.KubeClient.Scheme()); err != nil {
		return nil, err
	}
	return updated, nil
}

// applyVPA applies the VPA via server-side apply with a consistent field manager.
// This keeps managedFields stable and avoids double-create/update log spam.
func (b *BaseReconciler) applyVPA(ctx context.Context, vpa *unstructured.Unstructured) error {
	return b.KubeClient.Patch(ctx, vpa, client.Apply, &client.PatchOptions{
		FieldManager: fieldManager,
		Force:        ptr.To(true),
	})
}

// createVPA builds and creates a new VPA owned by the workload, with labels/spec applied.
func (b *BaseReconciler) createVPA(
	ctx context.Context,
	owner client.Object,
	name string,
	labels map[string]string,
	annotations map[string]string,
	spec map[string]any,
	profile string,
) error {
	vpa := newVPAObject()
	vpa.SetName(name)
	vpa.SetNamespace(owner.GetNamespace())
	vpa.SetLabels(labels)
	vpa.SetAnnotations(annotations)
	vpa.Object["spec"] = spec

	// Ensure the workload owns the VPA for garbage collection and intent tracking.
	if err := ctrl.SetControllerReference(owner, vpa, b.KubeClient.Scheme()); err != nil {
		return err
	}

	if err := b.applyVPA(ctx, vpa); err != nil {
		return err
	}

	b.Recorder.Eventf(owner, corev1.EventTypeNormal, "VPACreated", "Created VPA %s with profile %s", name, profile)
	b.Logger.Info("created VPA", "vpa", name, "profile", profile)
	return nil
}

// updateVPA updates the given VPA.
func (b *BaseReconciler) updateVPA(ctx context.Context, updated *unstructured.Unstructured) error {
	return b.applyVPA(ctx, updated)
}

// pruneManagedVPAsExcept deletes managed VPAs owned by the workload that do not match the desired name.
// This is used when the profile or name template changes and the desired VPA name shifts.
func (b *BaseReconciler) pruneManagedVPAsExcept(ctx context.Context, owner client.Object, exceptName string) error {
	list := &unstructured.UnstructuredList{}
	list.SetGroupVersionKind(vpaListGVK)

	// Fetch all managed VPAs in the namespace.
	if err := b.KubeClient.List(ctx, list, client.InNamespace(owner.GetNamespace()), client.MatchingLabels{b.Meta.ManagedLabel: "true"}); err != nil {
		return fmt.Errorf("list managed VPAs: %w", err)
	}

	for i := range list.Items {
		vpa := &list.Items[i] // Copy to avoid mutating cache objects.
		if vpa.GetName() == exceptName {
			continue
		}

		// Check if the VPA is owned by the workload.
		if !metav1.IsControlledBy(vpa, owner) {
			continue
		}

		// When here, we know that the VPA is owned by the workload and the VPA name has changed.
		// Most likely the profile has changed, so we delete the obsolete VPA.
		if err := b.KubeClient.Delete(ctx, vpa); err != nil {
			return fmt.Errorf("delete obsolete VPA %s: %w", vpa.GetName(), err)
		}

		b.Logger.Info("deleted obsolete VPA", "vpa", vpa.GetName())
		b.Recorder.Eventf(owner, corev1.EventTypeNormal, "DeletedObsoleteVPA", "Deleted obsolete VPA %s", vpa.GetName())
	}

	return nil
}

// purgeManagedVPAsForWorkload removes all managed VPAs owned by the workload name/kind in a namespace.
// Used when the workload is deleted or its profile annotation is removed; everything managed for it must go.
func (b *BaseReconciler) purgeManagedVPAsForWorkload(ctx context.Context, owner client.Object, workloadKind string) error {
	list := &unstructured.UnstructuredList{}
	list.SetGroupVersionKind(vpaListGVK)

	if err := b.KubeClient.List(ctx, list, client.InNamespace(owner.GetNamespace()), client.MatchingLabels{b.Meta.ManagedLabel: "true"}); err != nil {
		return fmt.Errorf("list managed VPAs for cleanup: %w", err)
	}

	for i := range list.Items {
		vpa := &list.Items[i] // copy to avoid mutating cache objects
		for _, ref := range vpa.GetOwnerReferences() {
			if ref.Kind != workloadKind {
				continue
			}
			if ref.Name != owner.GetName() {
				continue
			}

			if err := b.KubeClient.Delete(ctx, vpa); err != nil && !apierrors.IsNotFound(err) {
				return fmt.Errorf("delete VPA %s: %w", vpa.GetName(), err)
			}
			b.Logger.Info("deleted managed VPA for removed workload", "vpa", vpa.GetName(), "workload", owner.GetName())
			b.Recorder.Eventf(owner, corev1.EventTypeNormal, "DeletedManagedVPA", "Deleted managed VPA %s for workload %s", vpa.GetName(), owner.GetName())
		}
	}
	return nil
}
