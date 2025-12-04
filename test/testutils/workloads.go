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
	"context"
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2" // nolint:staticcheck
	. "github.com/onsi/gomega"    // nolint:staticcheck

	"github.com/google/uuid"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	DefaultTestImage     string = "nginx:1.29"
	DefaultTestImageName string = "nginx"
)

var vpaGVK = schema.GroupVersionKind{
	Group:   "autoscaling.k8s.io",
	Version: "v1",
	Kind:    "VerticalPodAutoscaler",
}

// K8sClient is the shared Kubernetes client used in e2e tests.
var K8sClient client.Client

// CreateDeployment creates and applies a Deployment in the specified namespace.
func CreateDeployment(ctx context.Context, namespace, name string, opts ...Option) *appsv1.Deployment { // nolint:dupl
	meta := metav1.ObjectMeta{
		Name:        name,
		Namespace:   namespace,
		Annotations: map[string]string{},
		Labels:      map[string]string{"app": name},
	}
	spec := appsv1.DeploymentSpec{
		Replicas: Int32Ptr(1),
		Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"app": name}},
		Template: corev1.PodTemplateSpec{
			ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"app": name}},
			Spec: corev1.PodSpec{
				Containers: []corev1.Container{{Name: DefaultTestImageName, Image: DefaultTestImage}},
			},
		},
	}
	deployment := &appsv1.Deployment{ObjectMeta: meta, Spec: spec}
	applyOptions(deployment, opts...)
	Expect(K8sClient.Create(ctx, deployment)).To(Succeed())

	CheckResourceReadiness(ctx, deployment)

	deployment.TypeMeta = metav1.TypeMeta{Kind: "Deployment", APIVersion: "apps/v1"}
	return deployment
}

// CreateStatefulSet creates and applies a StatefulSet in the specified namespace.
func CreateStatefulSet(ctx context.Context, namespace, name string, opts ...Option) *appsv1.StatefulSet { // nolint:dupl
	meta := metav1.ObjectMeta{
		Name:        name,
		Namespace:   namespace,
		Annotations: map[string]string{},
		Labels:      map[string]string{"app": name},
	}
	spec := appsv1.StatefulSetSpec{
		Replicas: Int32Ptr(1),
		Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"app": name}},
		Template: corev1.PodTemplateSpec{
			ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"app": name}},
			Spec: corev1.PodSpec{
				Containers: []corev1.Container{{Name: DefaultTestImageName, Image: DefaultTestImage}},
			},
		},
	}
	statefulSet := &appsv1.StatefulSet{ObjectMeta: meta, Spec: spec}
	applyOptions(statefulSet, opts...)
	Expect(K8sClient.Create(ctx, statefulSet)).To(Succeed())

	CheckResourceReadiness(ctx, statefulSet)

	statefulSet.TypeMeta = metav1.TypeMeta{Kind: "StatefulSet", APIVersion: "apps/v1"}
	return statefulSet
}

// CreateDaemonSet creates and applies a DaemonSet in the specified namespace.
func CreateDaemonSet(ctx context.Context, namespace, name string, opts ...Option) *appsv1.DaemonSet { // nolint:dupl
	meta := metav1.ObjectMeta{
		Name:        name,
		Namespace:   namespace,
		Annotations: map[string]string{},
		Labels:      map[string]string{"app": name},
	}
	spec := appsv1.DaemonSetSpec{
		Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"app": name}},
		Template: corev1.PodTemplateSpec{
			ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"app": name}},
			Spec: corev1.PodSpec{
				Containers: []corev1.Container{{Name: DefaultTestImageName, Image: DefaultTestImage}},
			},
		},
	}
	daemonSet := &appsv1.DaemonSet{ObjectMeta: meta, Spec: spec}
	applyOptions(daemonSet, opts...)
	Expect(K8sClient.Create(ctx, daemonSet)).To(Succeed())

	CheckResourceReadiness(ctx, daemonSet)

	daemonSet.TypeMeta = metav1.TypeMeta{Kind: "DaemonSet", APIVersion: "apps/v1"}
	return daemonSet
}

// ExpectVPA asserts that a VPA with the given name exists and is marked as managed.
func ExpectVPA(ctx context.Context, namespace, name, managedLabel string) {
	Eventually(func(g Gomega) {
		vpa, err := GetVPA(ctx, namespace, name)
		g.Expect(err).ShouldNot(HaveOccurred())
		g.Expect(vpa.GetName()).To(Equal(name))
		g.Expect(vpa.GetLabels()).To(HaveKeyWithValue(managedLabel, "true"))
	}).WithContext(ctx).Within(30 * time.Second).ProbeEvery(1 * time.Second).Should(Succeed())
}

// GetVPA fetches a VPA as unstructured.
func GetVPA(ctx context.Context, namespace, name string) (*unstructured.Unstructured, error) {
	vpa := &unstructured.Unstructured{}
	vpa.SetGroupVersionKind(schema.GroupVersionKind{
		Group:   "autoscaling.k8s.io",
		Version: "v1",
		Kind:    "VerticalPodAutoscaler",
	})
	if err := K8sClient.Get(ctx, client.ObjectKey{Namespace: namespace, Name: name}, vpa); err != nil {
		return nil, err
	}
	return vpa, nil
}

// ExpectVPANotFound asserts that a VPA with the given name does not exist.
func ExpectVPANotFound(ctx context.Context, namespace, name string) {
	vpa := &unstructured.Unstructured{}
	vpa.SetGroupVersionKind(schema.GroupVersionKind{
		Group:   "autoscaling.k8s.io",
		Version: "v1",
		Kind:    "VerticalPodAutoscaler",
	})

	Eventually(func(g Gomega) {
		err := K8sClient.Get(ctx, client.ObjectKey{Namespace: namespace, Name: name}, vpa)
		g.Expect(client.IgnoreNotFound(err)).To(Succeed())
		g.Expect(apierrors.IsNotFound(err)).To(BeTrue())
	}).WithContext(ctx).Within(30 * time.Second).ProbeEvery(1 * time.Second).Should(Succeed())
}

// CreateVPA creates a VerticalPodAutoscaler object in the given namespace.
func CreateVPA(
	ctx context.Context,
	namespace, name string,
	spec map[string]any,
	labels map[string]string,
	annotations map[string]string,
	owner client.Object,
) *unstructured.Unstructured {
	vpa := &unstructured.Unstructured{
		Object: map[string]any{},
	}
	vpa.SetGroupVersionKind(vpaGVK)
	vpa.SetNamespace(namespace)
	vpa.SetName(name)
	vpa.SetLabels(labels)
	vpa.SetAnnotations(annotations)
	vpa.Object["spec"] = spec

	if owner != nil {
		gvk := owner.GetObjectKind().GroupVersionKind()
		controller := true
		blockOwnerDeletion := true
		vpa.SetOwnerReferences([]metav1.OwnerReference{
			{
				APIVersion:         gvk.GroupVersion().String(),
				Kind:               gvk.Kind,
				Name:               owner.GetName(),
				UID:                owner.GetUID(),
				Controller:         &controller,
				BlockOwnerDeletion: &blockOwnerDeletion,
			},
		})
	}

	Expect(K8sClient.Create(ctx, vpa)).To(Succeed())
	return vpa
}

// GenerateUniqueName returns a unique name based on the given base string and a truncated UUID.
func GenerateUniqueName(base string) string {
	return fmt.Sprintf("%s-%s", base, uuid.New().String()[:5])
}

// DeleteNamespaceIfExists deletes the given namespace if it exists.
func DeleteNamespaceIfExists(namespace string) {
	ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: namespace}}
	err := K8sClient.Delete(context.Background(), ns)
	if err != nil && !apierrors.IsNotFound(err) {
		Expect(err).ToNot(HaveOccurred(), "failed to delete namespace %q", namespace)
	}
}

// CheckResourceReadiness waits until a Deployment, StatefulSet, or DaemonSet is ready.
func CheckResourceReadiness(ctx context.Context, resource client.Object) {
	By(fmt.Sprintf("Checking readiness of %T %s/%s", resource, resource.GetNamespace(), resource.GetName()))

	Eventually(func() bool {
		if err := K8sClient.Get(ctx, client.ObjectKeyFromObject(resource), resource); err != nil {
			return false
		}

		switch obj := resource.(type) {
		case *appsv1.Deployment:
			replicas := int32(-1)
			if obj.Spec.Replicas != nil {
				replicas = *obj.Spec.Replicas
			}
			return obj.Status.ReadyReplicas == replicas

		case *appsv1.StatefulSet:
			replicas := int32(-1)
			if obj.Spec.Replicas != nil {
				replicas = *obj.Spec.Replicas
			}
			return obj.Status.ReadyReplicas == replicas

		case *appsv1.DaemonSet:
			return obj.Status.NumberReady == obj.Status.DesiredNumberScheduled

		default:
			return false // unsupported type
		}
	}, 1*time.Minute, 1*time.Second).Should(BeTrue(),
		fmt.Sprintf("resource %T %s/%s did not become ready", resource, resource.GetNamespace(), resource.GetName()))
}

// ScaleResource updates the replica count of a Deployment or StatefulSet.
func ScaleResource(ctx context.Context, resource client.Object, replicas int32) {
	patch := client.MergeFrom(resource.DeepCopyObject().(client.Object))

	switch res := resource.(type) {
	case *appsv1.Deployment:
		res.Spec.Replicas = &replicas
	case *appsv1.StatefulSet:
		res.Spec.Replicas = &replicas
	case *appsv1.DaemonSet:
		panic("cannot scale a DaemonSet using replicas")
	default:
		panic(fmt.Sprintf("unsupported resource type: %T", res))
	}

	Expect(K8sClient.Patch(ctx, resource, patch)).To(Succeed())
}

// RestartResource sets the restart annotation on the PodTemplateSpec of a resource.
func RestartResource(ctx context.Context, resource client.Object) {
	By(fmt.Sprintf("Restarting %s %s/%s", resource.GetObjectKind().GroupVersionKind().Kind, resource.GetNamespace(), resource.GetName()))

	patch := client.MergeFrom(resource.DeepCopyObject().(client.Object))

	var template *corev1.PodTemplateSpec

	switch res := resource.(type) {
	case *appsv1.Deployment:
		template = &res.Spec.Template
	case *appsv1.StatefulSet:
		template = &res.Spec.Template
	case *appsv1.DaemonSet:
		template = &res.Spec.Template
	default:
		panic(fmt.Sprintf("unsupported resource type: %T", res))
	}

	if template.Annotations == nil {
		template.Annotations = map[string]string{}
	}
	template.Annotations["kubectl.kubernetes.io/restartedAt"] = time.Now().Format(time.RFC3339Nano)

	Expect(K8sClient.Patch(ctx, resource, patch)).To(Succeed())
}
