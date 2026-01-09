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
	"context"
	"time"

	"github.com/containeroo/autovpa/internal/controller"
	"github.com/containeroo/autovpa/internal/utils"
	"github.com/containeroo/autovpa/test/testutils"

	. "github.com/onsi/ginkgo/v2" // nolint:staticcheck
)

var _ = Describe("Namespaced mode", Serial, Ordered, func() {
	var ns string

	BeforeAll(func() {
		By("Stopping any running operator instance")
		testutils.StopOperator()
		time.Sleep(4 * time.Second)

		By("Resetting log buffer before suite")
		testutils.LogBuffer.Reset()

		By("Creating the watched namespace")
		ns = testutils.NSManager.CreateNamespace(context.Background())

		By("Starting operator in namespaced mode, watching only the created namespace")
		configPath := testutils.WriteProfiles("autovpa-profiles.yaml")
		testutils.StartOperatorWithFlags([]string{
			"--leader-elect=false",
			"--metrics-enabled=false",
			"--profile-annotation=" + profileKey,
			"--managed-label=" + managedLabel,
			"--vpa-name-template=" + VPANameTemplate,
			"--config=" + configPath,
			"--watch-namespace=" + ns,
		})
	})

	AfterAll(func(ctx SpecContext) {
		By("Stopping operator after suite")
		testutils.StopOperator()

		By("Cleaning up watched namespace")
		testutils.NSManager.Cleanup(ctx)
	})

	It("Reconciles workloads within the watched namespace", func(ctx SpecContext) {
		name := testutils.GenerateUniqueName("dep")

		By("Creating an opted-in Deployment inside the watched namespace")
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

	It("Skips workloads outside the watched namespace", func(ctx SpecContext) {
		By("Creating a separate namespace that is not watched by the operator")
		other := testutils.NSManager.CreateNamespace(ctx)

		name := testutils.GenerateUniqueName("dep")

		By("Creating an opted-in Deployment outside the watched namespace")
		dep := testutils.CreateDeployment(ctx, other, name, testutils.WithAnnotation(profileKey, "default"))

		vpaName, _ := controller.RenderVPAName(VPANameTemplate, utils.NameTemplateData{
			WorkloadName: dep.GetName(),
			Namespace:    dep.GetNamespace(),
			Kind:         DeploymentGVK.Kind,
			Profile:      "default",
		})

		By("Ensuring no managed VPA is created outside the watched namespace")
		testutils.ExpectVPANotFound(ctx, dep.GetNamespace(), vpaName)
	})
})
