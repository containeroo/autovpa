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
	"time"

	"github.com/containeroo/autovpa/internal/controller"
	"github.com/containeroo/autovpa/internal/utils"
	"github.com/containeroo/autovpa/test/testutils"

	. "github.com/onsi/ginkgo/v2" // nolint:staticcheck
	. "github.com/onsi/gomega"    // nolint:staticcheck
)

var _ = Describe("Argo tracking disabled", Serial, Ordered, func() {
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
			"--argo-managed=false",
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

	It("Does not propagate Argo tracking annotation when disabled", func(ctx SpecContext) {
		name := testutils.GenerateUniqueName("dep")
		dep := testutils.CreateDeployment(ctx, ns, name,
			testutils.WithAnnotation(profileAnnotation, "default"),
			testutils.WithAnnotation(argoTracking, "argo-managed"),
		)

		vpaName, _ := controller.RenderVPAName(VPANameTemplate, utils.NameTemplateData{
			WorkloadName: dep.GetName(),
			Namespace:    dep.GetNamespace(),
			Kind:         DeploymentGVK.Kind,
			Profile:      "default",
		})

		testutils.ExpectVPA(ctx, dep.GetNamespace(), vpaName, managedLabel)

		// Annotation should not be copied when Argo is disabled.
		Eventually(func(g Gomega) {
			vpa, err := testutils.GetVPA(ctx, dep.GetNamespace(), vpaName)
			g.Expect(err).To(Succeed())
			g.Expect(vpa.GetAnnotations()).NotTo(HaveKey(argoTracking))
		}).Should(Succeed())
	})
})
