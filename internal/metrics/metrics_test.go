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

package metrics

import (
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stretchr/testify/assert"
)

func withIsolatedPrometheusRegistry(t *testing.T, fn func()) {
	t.Helper()

	origReg := prometheus.DefaultRegisterer
	origGather := prometheus.DefaultGatherer

	// New isolated registry for this test.
	reg := prometheus.NewRegistry()
	prometheus.DefaultRegisterer = reg
	prometheus.DefaultGatherer = reg

	t.Cleanup(func() {
		prometheus.DefaultRegisterer = origReg
		prometheus.DefaultGatherer = origGather
	})

	fn()
}

func resetAll(r *Registry) {
	r.vpaCreated.Reset()
	r.vpaUpdated.Reset()
	r.vpaSkipped.Reset()
	r.vpaDeletedObsolete.Reset()
	r.vpaDeletedOptOut.Reset()
	r.vpaDeletedWorkloadGone.Reset()
	r.vpaDeletedOwnerGone.Reset()
	r.vpaDeletedOrphaned.Reset()
	r.vpaManaged.Reset()
	r.vpaReconcileErrors.Reset()
}

func TestRegistryMetrics_AllMethods(t *testing.T) {
	withIsolatedPrometheusRegistry(t, func() {
		r := NewRegistry(nil)
		resetAll(r)
		t.Cleanup(func() { resetAll(r) })

		t.Run("IncVPACreated increments", func(t *testing.T) {
			resetAll(r)

			r.IncVPACreated("ns1", "demo", "Deployment", "p1")
			val := testutil.ToFloat64(r.vpaCreated.WithLabelValues("ns1", "demo", "Deployment", "p1"))
			assert.Equal(t, float64(1), val)
		})

		t.Run("IncVPAUpdated increments", func(t *testing.T) {
			resetAll(r)

			r.IncVPAUpdated("ns1", "demo", "Deployment", "p1")
			val := testutil.ToFloat64(r.vpaUpdated.WithLabelValues("ns1", "demo", "Deployment", "p1"))
			assert.Equal(t, float64(1), val)
		})

		t.Run("IncVPASkipped increments", func(t *testing.T) {
			resetAll(r)

			r.IncVPASkipped("ns1", "demo", "Deployment", "annotation_missing")
			val := testutil.ToFloat64(r.vpaSkipped.WithLabelValues("ns1", "demo", "Deployment", "annotation_missing"))
			assert.Equal(t, float64(1), val)
		})

		t.Run("IncVPADeletedObsolete increments", func(t *testing.T) {
			resetAll(r)

			// NOTE: label is (namespace, kind)
			r.IncVPADeletedObsolete("ns1", "Deployment")
			val := testutil.ToFloat64(r.vpaDeletedObsolete.WithLabelValues("ns1", "Deployment"))
			assert.Equal(t, float64(1), val)
		})

		t.Run("IncVPADeletedOptOut increments", func(t *testing.T) {
			resetAll(r)

			r.IncVPADeletedOptOut("ns1", "Deployment")
			val := testutil.ToFloat64(r.vpaDeletedOptOut.WithLabelValues("ns1", "Deployment"))
			assert.Equal(t, float64(1), val)
		})

		t.Run("IncVPADeletedWorkloadGone increments", func(t *testing.T) {
			resetAll(r)

			r.IncVPADeletedWorkloadGone("ns1", "Deployment")
			val := testutil.ToFloat64(r.vpaDeletedWorkloadGone.WithLabelValues("ns1", "Deployment"))
			assert.Equal(t, float64(1), val)
		})

		t.Run("IncVPADeletedOwnerGone increments", func(t *testing.T) {
			resetAll(r)

			r.IncVPADeletedOwnerGone("ns1", "Deployment")
			val := testutil.ToFloat64(r.vpaDeletedOwnerGone.WithLabelValues("ns1", "Deployment"))
			assert.Equal(t, float64(1), val)
		})

		t.Run("IncVPADeletedOrphaned increments", func(t *testing.T) {
			resetAll(r)

			r.IncVPADeletedOrphaned("ns1")
			val := testutil.ToFloat64(r.vpaDeletedOrphaned.WithLabelValues("ns1"))
			assert.Equal(t, float64(1), val)
		})

		t.Run("IncVPAManaged increments gauge", func(t *testing.T) {
			resetAll(r)

			r.IncVPAManaged("ns1", "p1")
			val := testutil.ToFloat64(r.vpaManaged.WithLabelValues("ns1", "p1"))
			assert.Equal(t, float64(1), val)
		})

		t.Run("DecVPAManaged decrements gauge", func(t *testing.T) {
			resetAll(r)

			// ensure we're not going below 0 without meaning to
			r.IncVPAManaged("ns1", "p1")
			r.DecVPAManaged("ns1", "p1")

			val := testutil.ToFloat64(r.vpaManaged.WithLabelValues("ns1", "p1"))
			assert.Equal(t, float64(0), val)
		})

		t.Run("IncReconcileErrors increments", func(t *testing.T) {
			resetAll(r)

			r.IncReconcileErrors("autovpa", "Deployment", "api_error")
			val := testutil.ToFloat64(r.vpaReconcileErrors.WithLabelValues("autovpa", "Deployment", "api_error"))
			assert.Equal(t, float64(1), val)
		})
	})
}
