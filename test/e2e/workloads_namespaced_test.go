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
		testutils.StopOperator()
		time.Sleep(4 * time.Second)
		testutils.LogBuffer.Reset()

		ns = testutils.NSManager.CreateNamespace(context.Background())

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
		testutils.StopOperator()
		testutils.NSManager.Cleanup(ctx)
	})

	It("Reconciles workloads within the watched namespace", func(ctx SpecContext) {
		name := testutils.GenerateUniqueName("dep")
		dep := testutils.CreateDeployment(ctx, ns, name, testutils.WithAnnotation(profileKey, "default"))

		vpaName, _ := controller.RenderVPAName(VPANameTemplate, utils.NameTemplateData{
			WorkloadName: dep.GetName(),
			Namespace:    dep.GetNamespace(),
			Kind:         DeploymentGVK.Kind,
			Profile:      "default",
		})

		testutils.ExpectVPA(ctx, dep.GetNamespace(), vpaName, managedLabel)
	})

	It("Skips workloads outside the watched namespace", func(ctx SpecContext) {
		other := testutils.NSManager.CreateNamespace(ctx)
		name := testutils.GenerateUniqueName("dep")
		dep := testutils.CreateDeployment(ctx, other, name, testutils.WithAnnotation(profileKey, "default"))

		vpaName, _ := controller.RenderVPAName(VPANameTemplate, utils.NameTemplateData{
			WorkloadName: dep.GetName(),
			Namespace:    dep.GetNamespace(),
			Kind:         DeploymentGVK.Kind,
			Profile:      "default",
		})

		testutils.ExpectVPANotFound(ctx, dep.GetNamespace(), vpaName)
	})
})
