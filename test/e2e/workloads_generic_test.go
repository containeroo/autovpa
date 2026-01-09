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

	vpaautoscaling "k8s.io/autoscaler/vertical-pod-autoscaler/pkg/apis/autoscaling.k8s.io/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var _ = Describe("Generic", Serial, Ordered, func() {
	var ns string

	BeforeAll(func() {
		By("Stopping any running operator instance")
		testutils.StopOperator()
		time.Sleep(4 * time.Second) // wait for operator to stop

		By("Resetting log buffer before test suite")
		testutils.LogBuffer.Reset()

		By("Starting operator with test configuration")
		configPath := testutils.WriteProfiles("autovpa-profiles.yaml")
		testutils.StartOperatorWithFlags([]string{
			"--leader-elect=false",
			"--metrics-enabled=false",
			"--profile-annotation=" + profileKey,
			"--managed-label=" + managedLabel,
			"--vpa-name-template=" + VPANameTemplate,
			"--config=" + configPath,
		})
	})

	AfterAll(func() {
		By("Stopping operator after test suite")
		testutils.StopOperator()
	})

	BeforeEach(func(ctx SpecContext) {
		By("Creating a fresh namespace for the test")
		ns = testutils.NSManager.CreateNamespace(ctx)
	})

	AfterEach(func(ctx SpecContext) {
		By("Cleaning up namespace and resetting logs")
		testutils.NSManager.Cleanup(ctx)
		testutils.LogBuffer.Reset()
	})

	It("Creates a VPA for a Deployment", func(ctx SpecContext) {
		name := testutils.GenerateUniqueName("dep")

		By("Creating an opted-in Deployment")
		dep := testutils.CreateDeployment(ctx, ns, name, testutils.WithAnnotation(profileKey, "default"))

		vpaName, _ := controller.RenderVPAName(VPANameTemplate, utils.NameTemplateData{
			WorkloadName: dep.GetName(),
			Namespace:    dep.GetNamespace(),
			Kind:         DeploymentGVK.Kind,
			Profile:      "default",
		})

		By("Waiting for the managed VPA to be created")
		testutils.ExpectVPA(ctx, dep.GetNamespace(), vpaName, managedLabel)
	})

	It("Creates a VPA for a StatefulSet", func(ctx SpecContext) {
		name := testutils.GenerateUniqueName("sts")

		By("Creating an opted-in StatefulSet")
		sts := testutils.CreateStatefulSet(ctx, ns, name, testutils.WithAnnotation(profileKey, "default"))

		vpaName, _ := controller.RenderVPAName(VPANameTemplate, utils.NameTemplateData{
			WorkloadName: sts.GetName(),
			Namespace:    sts.GetNamespace(),
			Kind:         StatefulSetGVK.Kind,
			Profile:      "default",
		})

		By("Waiting for the managed VPA to be created")
		testutils.ExpectVPA(ctx, sts.GetNamespace(), vpaName, managedLabel)
	})

	It("Creates a VPA for a DaemonSet", func(ctx SpecContext) {
		name := testutils.GenerateUniqueName("ds")

		By("Creating an opted-in DaemonSet")
		ds := testutils.CreateDaemonSet(ctx, ns, name, testutils.WithAnnotation(profileKey, "default"))

		vpaName, _ := controller.RenderVPAName(VPANameTemplate, utils.NameTemplateData{
			WorkloadName: ds.GetName(),
			Namespace:    ds.GetNamespace(),
			Kind:         DaemonSetGVK.Kind,
			Profile:      "default",
		})

		By("Waiting for the managed VPA to be created")
		testutils.ExpectVPA(ctx, ds.GetNamespace(), vpaName, managedLabel)
	})

	It("Uses profile name template overrides", func(ctx SpecContext) {
		name := testutils.GenerateUniqueName("dep")

		By("Creating a Deployment using a profile that overrides nameTemplate")
		dep := testutils.CreateDeployment(ctx, ns, name, testutils.WithAnnotation(profileKey, "auto"))

		By("Waiting for the managed VPA to be created with the overridden name")
		// The "auto" profile in the test config sets nameTemplate to "{{ .WorkloadName }}-vpa".
		expectedName := dep.GetName() + "-vpa"
		testutils.ExpectVPA(ctx, dep.GetNamespace(), expectedName, managedLabel)
	})

	It("Skips workloads without profile annotation", func(ctx SpecContext) {
		name := testutils.GenerateUniqueName("dep")

		By("Creating a Deployment without the profile annotation (opt-out)")
		dep := testutils.CreateDeployment(ctx, ns, name)

		vpaName, _ := controller.RenderVPAName(VPANameTemplate, utils.NameTemplateData{
			WorkloadName: dep.GetName(),
			Namespace:    dep.GetNamespace(),
			Kind:         DeploymentGVK.Kind,
			Profile:      "default",
		})

		By("Ensuring no managed VPA is created")
		testutils.ExpectVPANotFound(ctx, dep.GetNamespace(), vpaName)
	})

	It("Deletes VPA when workload removes the profile annotation (opt-out)", func(ctx SpecContext) {
		name := testutils.GenerateUniqueName("dep")

		By("Creating an opted-in Deployment")
		dep := testutils.CreateDeployment(ctx, ns, name, testutils.WithAnnotation(profileKey, "default"))

		vpaName, _ := controller.RenderVPAName(VPANameTemplate, utils.NameTemplateData{
			WorkloadName: dep.GetName(),
			Namespace:    dep.GetNamespace(),
			Kind:         DeploymentGVK.Kind,
			Profile:      "default",
		})

		By("Waiting for the managed VPA to be created")
		testutils.ExpectVPA(ctx, dep.GetNamespace(), vpaName, managedLabel)

		By("Removing the profile annotation to opt out")
		patch := client.MergeFrom(dep.DeepCopy())
		delete(dep.Annotations, profileKey)
		Expect(testutils.K8sClient.Patch(ctx, dep, patch)).To(Succeed())

		By("Waiting for the managed VPA to be deleted")
		testutils.ExpectVPANotFound(ctx, dep.GetNamespace(), vpaName)
	})

	It("Leaves an unmanaged VPA alone after opt-out and managed-label removal", func(ctx SpecContext) {
		name := testutils.GenerateUniqueName("dep")

		By("Creating an opted-in Deployment")
		dep := testutils.CreateDeployment(ctx, ns, name, testutils.WithAnnotation(profileKey, "default"))

		vpaName, _ := controller.RenderVPAName(VPANameTemplate, utils.NameTemplateData{
			WorkloadName: dep.GetName(),
			Namespace:    dep.GetNamespace(),
			Kind:         DeploymentGVK.Kind,
			Profile:      "default",
		})

		By("Waiting for the managed VPA to be created")
		testutils.ExpectVPA(ctx, dep.GetNamespace(), vpaName, managedLabel)

		By("Opting out by removing the profile annotation")
		patch := client.MergeFrom(dep.DeepCopy())
		delete(dep.Annotations, profileKey)
		Expect(testutils.K8sClient.Patch(ctx, dep, patch)).To(Succeed())

		By("Waiting for the managed VPA to be deleted")
		testutils.ExpectVPANotFound(ctx, dep.GetNamespace(), vpaName)

		By("Recreating the VPA without the managed label; operator should ignore it")
		spec := map[string]any{
			"targetRef": map[string]any{
				"apiVersion": DeploymentGVK.GroupVersion().String(),
				"kind":       DeploymentGVK.Kind,
				"name":       dep.GetName(),
			},
		}
		testutils.CreateVPA(
			ctx,
			dep.GetNamespace(),
			vpaName,
			spec,
			map[string]string{}, // no managed label
			map[string]string{},
			dep,
		)

		By("Ensuring the operator does not re-add the managed label")
		Consistently(func(g Gomega) {
			vpa, err := testutils.GetVPA(ctx, dep.GetNamespace(), vpaName)
			g.Expect(err).ToNot(HaveOccurred())
			g.Expect(vpa.GetLabels()).ToNot(HaveKey(managedLabel))
		}).WithContext(ctx).Within(6 * time.Second).ProbeEvery(1 * time.Second).Should(Succeed())
	})

	It("Replaces VPA when name template changes", func(ctx SpecContext) {
		name := testutils.GenerateUniqueName("dep")

		By("Creating an opted-in Deployment")
		dep := testutils.CreateDeployment(ctx, ns, name, testutils.WithAnnotation(profileKey, "default"))

		vpaName, _ := controller.RenderVPAName(VPANameTemplate, utils.NameTemplateData{
			WorkloadName: dep.GetName(),
			Namespace:    dep.GetNamespace(),
			Kind:         DeploymentGVK.Kind,
			Profile:      "default",
		})

		By("Waiting for the managed VPA to be created")
		testutils.ExpectVPA(ctx, dep.GetNamespace(), vpaName, managedLabel)

		By("Changing the profile so the name template renders a different name")
		patch := client.MergeFrom(dep.DeepCopy())
		dep.Annotations[profileKey] = "auto"
		Expect(testutils.K8sClient.Patch(ctx, dep, patch)).To(Succeed())

		By("Waiting for the new VPA to be created and the old one to be deleted")
		// Profile "auto" overrides the name template to "{{ .WorkloadName }}-vpa".
		newVPAName := dep.GetName() + "-vpa"
		testutils.ExpectVPA(ctx, dep.GetNamespace(), newVPAName, managedLabel)
		testutils.ExpectVPANotFound(ctx, dep.GetNamespace(), vpaName)

		By("Verifying the expected 'deleted obsolete VPA' log line was emitted")
		testutils.ContainsLogs(
			fmt.Sprintf("\"deleted obsolete VPA\",\"vpa\":%q,\"namespace\":%q,\"workload\":%q", vpaName, ns, dep.Name),
			4*time.Second,
			1*time.Second,
		)
	})

	It("Matches VPA spec to profile fields", func(ctx SpecContext) {
		name := testutils.GenerateUniqueName("dep")

		By("Creating an opted-in Deployment with the 'auto' profile")
		dep := testutils.CreateDeployment(ctx, ns, name, testutils.WithAnnotation(profileKey, "auto"))

		By("Waiting for the managed VPA to exist")
		// auto profile uses nameTemplate "{{ .WorkloadName }}-vpa"
		vpaName := dep.GetName() + "-vpa"
		testutils.ExpectVPA(ctx, dep.GetNamespace(), vpaName, managedLabel)

		expected := map[string]any{
			"updatePolicy": map[string]any{
				"updateMode": string(vpaautoscaling.UpdateModeAuto),
			},
			"resourcePolicy": map[string]any{
				"containerPolicies": []any{
					map[string]any{
						"containerName": "*",
						"controlledResources": []any{
							"cpu",
							"memory",
						},
						"minAllowed": map[string]any{
							"cpu":    "20m",
							"memory": "64Mi",
						},
					},
				},
			},
		}

		By("Verifying the VPA spec matches the expected profile rendering")
		testutils.ExpectVPASpec(ctx, dep.GetNamespace(), vpaName, expected)
	})

	It("Deployment restart does not trigger a VPA update", func(ctx SpecContext) {
		name := testutils.GenerateUniqueName("dep")

		By("Creating an opted-in Deployment")
		dep := testutils.CreateDeployment(ctx, ns, name, testutils.WithAnnotation(profileKey, "default"))

		vpaName, _ := controller.RenderVPAName(VPANameTemplate, utils.NameTemplateData{
			WorkloadName: dep.GetName(),
			Namespace:    dep.GetNamespace(),
			Kind:         DeploymentGVK.Kind,
			Profile:      "default",
		})

		By("Waiting for the managed VPA to be created")
		testutils.ExpectVPA(ctx, dep.GetNamespace(), vpaName, managedLabel)

		By("Resetting logs to only observe effects of the restart")
		testutils.LogBuffer.Reset()

		By("Restarting the Deployment (rollout restart)")
		testutils.RestartResource(ctx, dep)

		By("Ensuring no VPA update/create logs are emitted after restart-only changes")
		testutils.ContainsNotLogs(
			fmt.Sprintf("\"updated VPA\",\"vpa\":%q,\"profile\":\"default\"", vpaName),
			4*time.Second,
			1*time.Second,
		)
		testutils.ContainsNotLogs(
			fmt.Sprintf("\"created VPA\",\"vpa\":%q,\"profile\":\"default\"", vpaName),
			4*time.Second,
			1*time.Second,
		)

		By("Ensuring the managed VPA still exists")
		testutils.ExpectVPA(ctx, dep.GetNamespace(), vpaName, managedLabel)
	})

	It("Delete statefulset removes VPA", func(ctx SpecContext) {
		name := testutils.GenerateUniqueName("dep")

		By("Creating an opted-in Deployment")
		dep := testutils.CreateDeployment(ctx, ns, name, testutils.WithAnnotation(profileKey, "default"))

		// NOTE: This test name says StatefulSet, but it creates a Deployment and uses StatefulSetGVK.Kind.
		// Keeping your original logic unchanged; only adding By().
		vpaName, _ := controller.RenderVPAName(VPANameTemplate, utils.NameTemplateData{
			WorkloadName: dep.GetName(),
			Namespace:    dep.GetNamespace(),
			Kind:         StatefulSetGVK.Kind,
			Profile:      "default",
		})

		By("Waiting for the managed VPA to be created")
		testutils.ExpectVPA(ctx, dep.GetNamespace(), vpaName, managedLabel)

		By("Resetting logs to only observe effects of the delete")
		testutils.LogBuffer.Reset()

		By("Deleting the workload")
		Expect(testutils.K8sClient.Delete(ctx, dep)).To(Succeed())

		By("Ensuring no VPA update/create logs are emitted after delete-only changes")
		testutils.ContainsNotLogs(
			fmt.Sprintf("\"updated VPA\",\"vpa\":%q,\"profile\":\"default\"", vpaName),
			4*time.Second,
			1*time.Second,
		)
		testutils.ContainsNotLogs(
			fmt.Sprintf("\"created VPA\",\"vpa\":%q,\"profile\":\"default\"", vpaName),
			4*time.Second,
			1*time.Second,
		)

		By("Waiting for the managed VPA to be deleted")
		testutils.ExpectVPANotFound(ctx, dep.GetNamespace(), vpaName)
	})

	It("Scaling deployment up does not trigger a VPA update", func(ctx SpecContext) {
		name := testutils.GenerateUniqueName("dep")

		By("Creating an opted-in Deployment")
		dep := testutils.CreateDeployment(ctx, ns, name, testutils.WithAnnotation(profileKey, "default"))

		vpaName, _ := controller.RenderVPAName(VPANameTemplate, utils.NameTemplateData{
			WorkloadName: dep.GetName(),
			Namespace:    dep.GetNamespace(),
			Kind:         DeploymentGVK.Kind,
			Profile:      "default",
		})

		By("Waiting for the managed VPA to be created")
		testutils.ExpectVPA(ctx, dep.GetNamespace(), vpaName, managedLabel)

		By("Resetting logs to only observe effects of scaling")
		testutils.LogBuffer.Reset()

		By("Scaling the Deployment")
		testutils.ScaleResource(ctx, dep, 2)

		By("Ensuring no VPA update/create logs are emitted after scaling-only changes")
		testutils.ContainsNotLogs(
			fmt.Sprintf("\"updated VPA\",\"vpa\":%q,\"profile\":\"default\"", vpaName),
			4*time.Second,
			1*time.Second,
		)
		testutils.ContainsNotLogs(
			fmt.Sprintf("\"created VPA\",\"vpa\":%q,\"profile\":\"default\"", vpaName),
			4*time.Second,
			1*time.Second,
		)
	})

	It("Unknown profile", func(ctx SpecContext) {
		name := testutils.GenerateUniqueName("dep")

		By("Creating a Deployment with an unknown profile")
		dep := testutils.CreateDeployment(ctx, ns, name, testutils.WithAnnotation(profileKey, "unknown"))

		vpaName, _ := controller.RenderVPAName(VPANameTemplate, utils.NameTemplateData{
			WorkloadName: dep.GetName(),
			Namespace:    dep.GetNamespace(),
			Kind:         DeploymentGVK.Kind,
			Profile:      "unknown",
		})

		By("Ensuring no managed VPA is created")
		testutils.ExpectVPANotFound(ctx, dep.GetNamespace(), vpaName)

		By("Verifying the expected 'profile not found' log line was emitted")
		testutils.ContainsLogs(
			fmt.Sprintf(
				"\"profile not found; skipping VPA reconciliation\",\"namespace\":%q,\"workload\":%q,\"kind\":%q,\"controller\":\"Deployment\",\"profile\":\"unknown\"",
				ns,
				dep.Name,
				dep.GroupVersionKind().Kind,
			),
			4*time.Second,
			1*time.Second,
		)
	})

	It("Cleanup obsolete VPAs", func(ctx SpecContext) {
		name := testutils.GenerateUniqueName("dep")

		By("Creating a workload without the profile annotation")
		dep := testutils.CreateDeployment(ctx, ns, name)

		vpaName := fmt.Sprintf("%s-obsolete-vpa", dep.GetName())

		spec := map[string]any{
			"targetRef": map[string]any{
				"apiVersion": DeploymentGVK.GroupVersion().String(),
				"kind":       DeploymentGVK.Kind,
				"name":       dep.GetName(),
			},
		}

		By("Creating an obsolete managed VPA owned by the workload")
		testutils.CreateVPA(
			ctx,
			dep.GetNamespace(),
			vpaName,
			spec,
			map[string]string{managedLabel: "true"},
			map[string]string{},
			dep,
		)

		By("Ensuring the obsolete VPA exists")
		testutils.ExpectVPA(ctx, dep.GetNamespace(), vpaName, managedLabel)

		By("Opting the workload in to trigger desired VPA creation and obsolete cleanup")
		patch := client.MergeFrom(dep.DeepCopy())
		dep.Annotations = map[string]string{profileKey: "default"}
		Expect(testutils.K8sClient.Patch(ctx, dep, patch)).To(Succeed())

		expectedNewName, _ := controller.RenderVPAName(VPANameTemplate, utils.NameTemplateData{
			WorkloadName: dep.GetName(),
			Namespace:    dep.GetNamespace(),
			Kind:         DeploymentGVK.Kind,
			Profile:      "default",
		})

		By("Waiting for the desired VPA to exist")
		testutils.ExpectVPA(ctx, dep.GetNamespace(), expectedNewName, managedLabel)

		By("Waiting for the obsolete VPA to be deleted")
		testutils.ExpectVPANotFound(ctx, dep.GetNamespace(), vpaName)
	})
})
