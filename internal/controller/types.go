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
	"github.com/containeroo/autovpa/internal/config"

	appsv1 "k8s.io/api/apps/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

// MetaConfig holds annotation/label settings shared across reconcilers.
// It controls how workloads opt into profiles and how managed VPAs are marked.
type MetaConfig struct {
	ProfileKey   string // Workload annotation key used to pick a VPA profile.
	ManagedLabel string // Label key applied to VPAs managed by this operator.
}

// ProfileConfig wraps profile data shared across reconcilers.
// It supplies the available profiles, default profile, and default name template.
type ProfileConfig struct {
	NameTemplate string                    // Default VPA name template when a profile does not override.
	Default      string                    // Default profile name to use when annotation selects "default".
	Entries      map[string]config.Profile // All available profiles keyed by name.
}

var (
	vpaGVK = schema.GroupVersionKind{
		Group:   "autoscaling.k8s.io",
		Version: "v1",
		Kind:    "VerticalPodAutoscaler",
	}
	vpaListGVK = schema.GroupVersionKind{
		Group:   "autoscaling.k8s.io",
		Version: "v1",
		Kind:    "VerticalPodAutoscalerList",
	}
	DeploymentGVK  = appsv1.SchemeGroupVersion.WithKind("Deployment")
	StatefulSetGVK = appsv1.SchemeGroupVersion.WithKind("StatefulSet")
	DaemonSetGVK   = appsv1.SchemeGroupVersion.WithKind("DaemonSet")
)
