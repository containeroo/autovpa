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
	"errors"

	"github.com/containeroo/autovpa/internal/predicates"

	appsv1 "k8s.io/api/apps/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

// DeploymentReconciler reconciles Deployments to detect restarts and target reloads.
type DeploymentReconciler struct {
	BaseReconciler
}

// Reconcile handles the reconciliation logic when a Deployment is updated.
func (r *DeploymentReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	// Fetch the Deployment instance
	dep := &appsv1.Deployment{}
	if err := r.KubeClient.Get(ctx, req.NamespacedName, dep); err != nil {
		if apierrors.IsNotFound(err) {
			logger.Info("Deployment not found; cleaning managed VPAs if any")
			if err := r.purgeManagedVPAsForWorkload(ctx, &appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: req.Namespace,
					Name:      req.Name,
				},
			}, DeploymentGVK.Kind); err != nil {
				return ctrl.Result{}, err
			}
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, errors.New("failed to fetch Deployment")
	}

	return r.ReconcileWorkload(ctx, dep, DeploymentGVK)
}

// SetupWithManager sets up the controller with the Manager.
func (r *DeploymentReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&appsv1.Deployment{}).
		WithEventFilter(predicates.AnnotationLifecycle(r.Meta.ProfileAnnotation)).
		Complete(r)
}
