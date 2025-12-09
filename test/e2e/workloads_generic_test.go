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

package e2e

import (
	"fmt"
	"time"

	"github.com/containeroo/autovpa/internal/controller"
	"github.com/containeroo/autovpa/internal/utils"
	"github.com/containeroo/autovpa/test/testutils"

	. "github.com/onsi/ginkgo/v2" // nolint:staticcheck
	. "github.com/onsi/gomega"    // nolint:staticcheck

	"sigs.k8s.io/controller-runtime/pkg/client"
)

var _ = Describe("Generic", Serial, Ordered, func() {
	var ns string

	BeforeAll(func() {
		testutils.StopOperator()
		time.Sleep(4 * time.Second) // wait for operator to stop
		testutils.LogBuffer.Reset()
		configPath := testutils.WriteProfiles("autovpa-profiles.yaml")
		testutils.StartOperatorWithFlags([]string{
			"--leader-elect=false",
			"--metrics-enabled=false",
			"--profile-annotation=" + profileAnnotation,
			"--managed-label=" + managedLabel,
			"--vpa-name-template=" + VPANameTemplate,
			"--config=" + configPath,
		})
	})

	AfterAll(func() {
		testutils.StopOperator()
	})

	BeforeEach(func(ctx SpecContext) {
		ns = testutils.NSManager.CreateNamespace(ctx)
	})

	AfterEach(func(ctx SpecContext) {
		testutils.NSManager.Cleanup(ctx)
		testutils.LogBuffer.Reset()
	})

	It("Creates a VPA for a Deployment", func(ctx SpecContext) {
		name := testutils.GenerateUniqueName("dep")
		dep := testutils.CreateDeployment(ctx, ns, name, testutils.WithAnnotation(profileAnnotation, "default"))

		vpaName, _ := controller.RenderVPAName(VPANameTemplate, utils.NameTemplateData{
			WorkloadName: dep.GetName(),
			Namespace:    dep.GetNamespace(),
			Kind:         DeploymentGVK.Kind,
			Profile:      "default",
		})

		testutils.ExpectVPA(ctx, dep.GetNamespace(), vpaName, managedLabel)
	})

	It("Creates a VPA for a StatefulSet", func(ctx SpecContext) {
		name := testutils.GenerateUniqueName("sts")
		sts := testutils.CreateStatefulSet(ctx, ns, name, testutils.WithAnnotation(profileAnnotation, "default"))

		vpaName, _ := controller.RenderVPAName(VPANameTemplate, utils.NameTemplateData{
			WorkloadName: sts.GetName(),
			Namespace:    sts.GetNamespace(),
			Kind:         StatefulSetGVK.Kind,
			Profile:      "default",
		})

		testutils.ExpectVPA(ctx, sts.GetNamespace(), vpaName, managedLabel)
	})

	It("Creates a VPA for a DaemonSet", func(ctx SpecContext) {
		name := testutils.GenerateUniqueName("ds")
		ds := testutils.CreateDaemonSet(ctx, ns, name, testutils.WithAnnotation(profileAnnotation, "default"))

		vpaName, _ := controller.RenderVPAName(VPANameTemplate, utils.NameTemplateData{
			WorkloadName: ds.GetName(),
			Namespace:    ds.GetNamespace(),
			Kind:         DaemonSetGVK.Kind,
			Profile:      "default",
		})

		testutils.ExpectVPA(ctx, ds.GetNamespace(), vpaName, managedLabel)
	})

	It("Skips workloads without profile annotation", func(ctx SpecContext) {
		name := testutils.GenerateUniqueName("dep")
		dep := testutils.CreateDeployment(ctx, ns, name)

		vpaName, _ := controller.RenderVPAName(VPANameTemplate, utils.NameTemplateData{
			WorkloadName: dep.GetName(),
			Namespace:    dep.GetNamespace(),
			Kind:         DeploymentGVK.Kind,
			Profile:      "default",
		})

		testutils.ExpectVPANotFound(ctx, dep.GetNamespace(), vpaName)
	})

	It("Replaces VPA when name template changes", func(ctx SpecContext) {
		name := testutils.GenerateUniqueName("dep")
		dep := testutils.CreateDeployment(ctx, ns, name, testutils.WithAnnotation(profileAnnotation, "default"))

		vpaName, _ := controller.RenderVPAName(VPANameTemplate, utils.NameTemplateData{
			WorkloadName: dep.GetName(),
			Namespace:    dep.GetNamespace(),
			Kind:         DeploymentGVK.Kind,
			Profile:      "default",
		})

		testutils.ExpectVPA(ctx, dep.GetNamespace(), vpaName, managedLabel)

		By("Changing the profile so the name template renders a different name")
		patch := client.MergeFrom(dep.DeepCopy())
		dep.Annotations[profileAnnotation] = "auto"
		Expect(testutils.K8sClient.Patch(ctx, dep, patch)).To(Succeed())

		By("Waiting for the new VPA is created and the old one to be gone")
		newVPAName, _ := controller.RenderVPAName(VPANameTemplate, utils.NameTemplateData{
			WorkloadName: dep.GetName(),
			Namespace:    dep.GetNamespace(),
			Kind:         DeploymentGVK.Kind,
			Profile:      "auto",
		})
		testutils.ExpectVPA(ctx, dep.GetNamespace(), newVPAName, managedLabel)
		testutils.ExpectVPANotFound(ctx, dep.GetNamespace(), vpaName)

		testutils.ContainsLogs(
			fmt.Sprintf("\"deleted obsolete VPA\",\"vpa\":%q,\"namespace\":%q,\"workload\":%q", vpaName, ns, dep.Name),
			4*time.Second,
			1*time.Second)
	})

	It("Deployment restart does not trigger a VPA update", func(ctx SpecContext) {
		name := testutils.GenerateUniqueName("dep")
		dep := testutils.CreateDeployment(ctx, ns, name, testutils.WithAnnotation(profileAnnotation, "default"))

		vpaName, _ := controller.RenderVPAName(VPANameTemplate, utils.NameTemplateData{
			WorkloadName: dep.GetName(),
			Namespace:    dep.GetNamespace(),
			Kind:         DeploymentGVK.Kind,
			Profile:      "default",
		})

		testutils.ExpectVPA(ctx, dep.GetNamespace(), vpaName, managedLabel)

		// Clear creation logs so we only observe effects of the restart.
		testutils.LogBuffer.Reset()

		testutils.RestartResource(ctx, dep)

		// Ensure no VPA updates/creations are logged after a restart-only change.
		testutils.ContainsNotLogs(
			fmt.Sprintf("\"updated VPA\",\"vpa\":%q,\"profile\":\"default\"", vpaName),
			4*time.Second,
			1*time.Second)
		testutils.ContainsNotLogs(
			fmt.Sprintf("\"created VPA\",\"vpa\":%q,\"profile\":\"default\"", vpaName),
			4*time.Second,
			1*time.Second)

		// VPA still exists
		testutils.ExpectVPA(ctx, dep.GetNamespace(), vpaName, managedLabel)
	})

	It("Delete statefulset removes VPA", func(ctx SpecContext) {
		name := testutils.GenerateUniqueName("dep")
		dep := testutils.CreateDeployment(ctx, ns, name, testutils.WithAnnotation(profileAnnotation, "default"))

		vpaName, _ := controller.RenderVPAName(VPANameTemplate, utils.NameTemplateData{
			WorkloadName: dep.GetName(),
			Namespace:    dep.GetNamespace(),
			Kind:         StatefulSetGVK.Kind,
			Profile:      "default",
		})
		testutils.ExpectVPA(ctx, dep.GetNamespace(), vpaName, managedLabel)

		// Clear creation logs so we only observe effects of the restart.
		testutils.LogBuffer.Reset()

		Expect(testutils.K8sClient.Delete(ctx, dep)).To(Succeed())

		// Ensure no VPA updates/creations are logged after a restart-only change.
		testutils.ContainsNotLogs(
			fmt.Sprintf("\"updated VPA\",\"vpa\":%q,\"profile\":\"default\"", vpaName),
			4*time.Second,
			1*time.Second)
		testutils.ContainsNotLogs(
			fmt.Sprintf("\"created VPA\",\"vpa\":%q,\"profile\":\"default\"", vpaName),
			4*time.Second,
			1*time.Second)

		// VPA still exists
		testutils.ExpectVPANotFound(ctx, dep.GetNamespace(), vpaName)
	})

	It("Scaling deployment up does not trigger a VPA update", func(ctx SpecContext) {
		name := testutils.GenerateUniqueName("dep")
		dep := testutils.CreateDeployment(ctx, ns, name, testutils.WithAnnotation(profileAnnotation, "default"))

		vpaName, _ := controller.RenderVPAName(VPANameTemplate, utils.NameTemplateData{
			WorkloadName: dep.GetName(),
			Namespace:    dep.GetNamespace(),
			Kind:         DeploymentGVK.Kind,
			Profile:      "default",
		})

		testutils.ExpectVPA(ctx, dep.GetNamespace(), vpaName, managedLabel)

		// Clear creation logs so we only observe effects of the restart.
		testutils.LogBuffer.Reset()

		testutils.ScaleResource(ctx, dep, 2)

		// Ensure no VPA updates/creations are logged after a restart-only change.
		testutils.ContainsNotLogs(
			fmt.Sprintf("\"updated VPA\",\"vpa\":%q,\"profile\":\"default\"", vpaName),
			4*time.Second,
			1*time.Second)
		testutils.ContainsNotLogs(
			fmt.Sprintf("\"created VPA\",\"vpa\":%q,\"profile\":\"default\"", vpaName),
			4*time.Second,
			1*time.Second)
	})

	It("Unknown profile", func(ctx SpecContext) {
		name := testutils.GenerateUniqueName("dep")
		dep := testutils.CreateDeployment(ctx, ns, name, testutils.WithAnnotation(profileAnnotation, "unknown"))

		vpaName, _ := controller.RenderVPAName(VPANameTemplate, utils.NameTemplateData{
			WorkloadName: dep.GetName(),
			Namespace:    dep.GetNamespace(),
			Kind:         DeploymentGVK.Kind,
			Profile:      "unknown",
		})

		testutils.ExpectVPANotFound(ctx, dep.GetNamespace(), vpaName)

		testutils.ContainsLogs(
			fmt.Sprintf("\"profile not found; skipping VPA reconciliation\",\"namespace\":%q,\"workload\":%q,\"profile\":\"unknown\"", ns, dep.Name),
			4*time.Second,
			1*time.Second)
	})

	It("Cleanup obsolete VPAs", func(ctx SpecContext) {
		// Create workload without VPA annotation
		name := testutils.GenerateUniqueName("dep")
		dep := testutils.CreateDeployment(ctx, ns, name)

		vpaName := fmt.Sprintf("%s-obsolete-vpa", dep.GetName())

		// create a default VPA spec
		spec := map[string]any{
			"targetRef": map[string]any{
				"apiVersion": DeploymentGVK.GroupVersion().String(),
				"kind":       DeploymentGVK.Kind,
				"name":       dep.GetName(),
			},
		}

		// Create an obsolete VPA.
		testutils.CreateVPA(
			ctx,
			dep.GetNamespace(),
			vpaName,
			spec,
			map[string]string{managedLabel: "true"},
			map[string]string{},
			dep,
		)
		testutils.ExpectVPA(ctx, dep.GetNamespace(), vpaName, managedLabel)

		// Set profile to trigger creation of a new VPA and cleanup of the obsolete one.
		patch := client.MergeFrom(dep.DeepCopy())
		dep.Annotations = map[string]string{profileAnnotation: "default"}
		Expect(testutils.K8sClient.Patch(ctx, dep, patch)).To(Succeed())

		expectedNewName, _ := controller.RenderVPAName(VPANameTemplate, utils.NameTemplateData{
			WorkloadName: dep.GetName(),
			Namespace:    dep.GetNamespace(),
			Kind:         DeploymentGVK.Kind,
			Profile:      "default",
		})

		testutils.ExpectVPA(ctx, dep.GetNamespace(), expectedNewName, managedLabel)
		testutils.ExpectVPANotFound(ctx, dep.GetNamespace(), vpaName)
	})
})
