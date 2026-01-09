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

	"k8s.io/apimachinery/pkg/types"

	. "github.com/onsi/ginkgo/v2" // nolint:staticcheck
	. "github.com/onsi/gomega"    // nolint:staticcheck

	"sigs.k8s.io/controller-runtime/pkg/client"
)

var _ = Describe("VPA Generic", Serial, Ordered, func() {
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

	It("Deletes a managed VPA whose owner workload does not exist", func(ctx SpecContext) {
		// We create a managed VPA with a controllerRef pointing at a Deployment
		// that does NOT exist in the cluster. The VPA reconciler should detect
		// the missing owner and delete the VPA.
		vpaName := testutils.GenerateUniqueName("orphan-vpa")
		ownerName := testutils.GenerateUniqueName("missing-dep")

		spec := map[string]any{
			"targetRef": map[string]any{
				"apiVersion": DeploymentGVK.GroupVersion().String(),
				"kind":       DeploymentGVK.Kind,
				"name":       ownerName,
			},
		}

		By("Creating a managed VPA with a controllerRef pointing to a non-existing Deployment")
		testutils.CreateManagedVPAWithOwnerRef(
			ctx,
			ns,
			vpaName,
			managedLabel,
			DeploymentGVK,
			ownerName,
			types.UID("missing-owner-uid"),
			spec,
		)

		By("Ensuring the VPA exists before the reconciler deletes it")
		testutils.ExpectVPA(ctx, ns, vpaName, managedLabel)

		By("Resetting logs so we only assert logs produced by this reconciliation")
		testutils.LogBuffer.Reset()

		By("Waiting for the VPAReconciler to delete the orphaned VPA")
		testutils.ExpectVPANotFound(ctx, ns, vpaName)

		By("Verifying the expected log line was emitted")
		testutils.ContainsLogs(
			fmt.Sprintf("\"owner gone; deleting VPA\",\"namespace\":%q,\"vpa\":%q,\"controller\":\"VerticalPodAutoscaler\",\"ownerKind\":\"Deployment\",\"ownerName\":%q", ns, vpaName, ownerName),
			4*time.Second,
			1*time.Second,
		)
	})

	It("Restores the managed label after manual removal when the workload has a profile", func(ctx SpecContext) {
		name := testutils.GenerateUniqueName("dep")

		By("Creating an opted-in Deployment")
		dep := testutils.CreateDeployment(ctx, ns, name, testutils.WithAnnotation(profileKey, "default"))

		vpaName, _ := controller.RenderVPAName(VPANameTemplate, utils.NameTemplateData{
			WorkloadName: dep.GetName(),
			Namespace:    dep.GetNamespace(),
			Kind:         DeploymentGVK.Kind,
			Profile:      "default",
		})

		By("Ensuring the managed VPA exists")
		testutils.ExpectVPA(ctx, dep.GetNamespace(), vpaName, managedLabel)

		By("Manually removing the managed label from the VPA")
		vpa, err := testutils.GetVPA(ctx, dep.GetNamespace(), vpaName)
		Expect(err).ToNot(HaveOccurred())

		patch := client.MergeFrom(vpa.DeepCopy())
		labels := vpa.GetLabels()
		Expect(labels).To(HaveKeyWithValue(managedLabel, "true"))
		delete(labels, managedLabel)
		vpa.SetLabels(labels)
		Expect(testutils.K8sClient.Patch(ctx, vpa, patch)).To(Succeed())

		By("Waiting for the workload reconciler to restore the managed label")
		Eventually(func(g Gomega) {
			vpa, err := testutils.GetVPA(ctx, dep.GetNamespace(), vpaName)
			g.Expect(err).ToNot(HaveOccurred())
			g.Expect(vpa.GetLabels()).To(HaveKeyWithValue(managedLabel, "true"))
		}).WithContext(ctx).Within(30 * time.Second).ProbeEvery(1 * time.Second).Should(Succeed())
	})

	It("Restores the profile label after manual tampering when the workload has a profile", func(ctx SpecContext) {
		name := testutils.GenerateUniqueName("dep")

		By("Creating an opted-in Deployment")
		dep := testutils.CreateDeployment(ctx, ns, name, testutils.WithAnnotation(profileKey, "default"))

		vpaName, _ := controller.RenderVPAName(VPANameTemplate, utils.NameTemplateData{
			WorkloadName: dep.GetName(),
			Namespace:    dep.GetNamespace(),
			Kind:         DeploymentGVK.Kind,
			Profile:      "default",
		})

		By("Ensuring the managed VPA exists")
		testutils.ExpectVPA(ctx, dep.GetNamespace(), vpaName, managedLabel)

		By("Manually changing the profile label on the VPA")
		vpa, err := testutils.GetVPA(ctx, dep.GetNamespace(), vpaName)
		Expect(err).ToNot(HaveOccurred())

		patch := client.MergeFrom(vpa.DeepCopy())
		labels := vpa.GetLabels()
		labels[profileKey] = "tampered"
		vpa.SetLabels(labels)
		Expect(testutils.K8sClient.Patch(ctx, vpa, patch)).To(Succeed())

		By("Waiting for the workload reconciler to restore the profile label")
		Eventually(func(g Gomega) {
			vpa, err := testutils.GetVPA(ctx, dep.GetNamespace(), vpaName)
			g.Expect(err).ToNot(HaveOccurred())
			g.Expect(vpa.GetLabels()).To(HaveKeyWithValue(profileKey, "default"))
		}).WithContext(ctx).Within(30 * time.Second).ProbeEvery(1 * time.Second).Should(Succeed())
	})

	It("Deletes a managed VPA whose ownerRef kind does not match any existing workload", func(ctx SpecContext) {
		By("Creating a Deployment")
		dep := testutils.CreateDeployment(ctx, ns, testutils.GenerateUniqueName("dep"))

		vpaName := testutils.GenerateUniqueName("kind-mismatch-vpa")
		spec := map[string]any{
			"targetRef": map[string]any{
				"apiVersion": controller.StatefulSetGVK.GroupVersion().String(),
				"kind":       controller.StatefulSetGVK.Kind,
				"name":       dep.GetName(),
			},
		}

		By("Creating a managed VPA with a mismatched controllerRef kind")
		testutils.CreateManagedVPAWithOwnerRef(
			ctx,
			ns,
			vpaName,
			managedLabel,
			controller.StatefulSetGVK, // mismatched kind
			dep.GetName(),
			dep.GetUID(),
			spec,
		)

		By("Ensuring the VPA exists before deletion")
		testutils.ExpectVPA(ctx, ns, vpaName, managedLabel)

		By("Resetting logs to isolate reconciliation output")
		testutils.LogBuffer.Reset()

		By("Waiting for the VPAReconciler to delete the VPA")
		testutils.ExpectVPANotFound(ctx, ns, vpaName)

		By("Verifying the expected log line was emitted")
		testutils.ContainsLogs(
			fmt.Sprintf("\"owner gone; deleting VPA\",\"namespace\":%q,\"vpa\":%q,\"controller\":\"VerticalPodAutoscaler\",\"ownerKind\":\"StatefulSet\",\"ownerName\":%q", ns, vpaName, dep.GetName()),
			4*time.Second,
			1*time.Second,
		)
	})

	It("does not spam reconcile logs when a managed VPA is already in desired state", func(ctx SpecContext) {
		timeout := 30 * time.Second
		interval := 250 * time.Millisecond
		quietWindow := 10 * time.Second

		depName := testutils.GenerateUniqueName("dep-nosspam")

		By("Creating an opted-in Deployment")
		dep := testutils.CreateDeployment(
			ctx,
			ns,
			depName,
			testutils.WithAnnotation(profileKey, "default"),
		)

		vpaName, err := controller.RenderVPAName(VPANameTemplate, utils.NameTemplateData{
			WorkloadName: dep.GetName(),
			Namespace:    dep.GetNamespace(),
			Kind:         DeploymentGVK.Kind,
			Profile:      "default",
		})
		Expect(err).NotTo(HaveOccurred())

		By("Waiting for the VPA to exist")
		testutils.ExpectVPA(ctx, dep.GetNamespace(), vpaName, managedLabel)

		By("Waiting until the operator has settled")
		testutils.ContainsLogs(`"created VPA"`, timeout, interval)

		time.Sleep(2 * time.Second)

		By("Resetting the log buffer to measure the quiet window")
		testutils.LogBuffer.Reset()

		By("Ensuring no update spam during steady state")
		testutils.ContainsNotLogs(`"updated VPA"`, quietWindow, interval)

		By("Ensuring the VPA safety-net reconciler stays quiet on the happy path")
		testutils.ContainsNotLogs(`managed VPA has valid controller owner`, quietWindow, interval)

		By("Ensuring no repeated update logs occurred")
		testutils.CountLogOccurrences(`"updated VPA"`, 0, timeout, interval)
	})
})
