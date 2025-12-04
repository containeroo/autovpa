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

package testutils

import (
	appsv1 "k8s.io/api/apps/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// Option defines a functional option for customizing resources.
type Option func(resource client.Object)

// applyOptions applies functional options to a Kubernetes resource.
func applyOptions(resource client.Object, opts ...Option) {
	for _, opt := range opts {
		opt(resource)
	}
}

// WithAnnotation sets a annotation for a resource.
func WithAnnotation(key, value string) Option {
	return func(resource client.Object) {
		annotations := resource.GetAnnotations()
		if annotations == nil {
			annotations = make(map[string]string)
		}
		annotations[key] = value
		resource.SetAnnotations(annotations)
	}
}

// WithLabel sets a label for a resource.
func WithLabel(key, value string) Option {
	return func(resource client.Object) {
		labels := resource.GetLabels()
		if labels == nil {
			labels = make(map[string]string)
		}
		labels[key] = value
		resource.SetLabels(labels)
	}
}

// WithReplicas sets the replicas for a resource.
func WithReplicas(replicas int32) Option {
	return func(resource client.Object) {
		switch obj := resource.(type) {
		case *appsv1.Deployment:
			obj.Spec.Replicas = &replicas
		case *appsv1.StatefulSet:
			obj.Spec.Replicas = &replicas
		}
	}
}
