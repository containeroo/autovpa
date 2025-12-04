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

	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stretchr/testify/assert"
)

func TestMetricsCounters(t *testing.T) {
	// Global metrics: reset before/after each subtest.
	reset := func() {
		VPACreated.Reset()
		VPAUpdated.Reset()
		VPASkipped.Reset()
	}
	reset()
	t.Cleanup(reset)

	t.Run("VPACreated increments", func(t *testing.T) {
		VPACreated.WithLabelValues("ns1", "demo", "Deployment", "p1").Inc()
		val := testutil.ToFloat64(VPACreated.WithLabelValues("ns1", "demo", "Deployment", "p1"))
		assert.Equal(t, float64(1), val)
	})

	t.Run("VPAUpdated increments", func(t *testing.T) {
		VPAUpdated.WithLabelValues("ns1", "demo", "Deployment", "p1").Inc()
		val := testutil.ToFloat64(VPAUpdated.WithLabelValues("ns1", "demo", "Deployment", "p1"))
		assert.Equal(t, float64(1), val)
	})

	t.Run("VPASkipped increments", func(t *testing.T) {
		VPASkipped.WithLabelValues("ns1", "demo", "Deployment", "annotation_missing").Inc()
		val := testutil.ToFloat64(VPASkipped.WithLabelValues("ns1", "demo", "Deployment", "annotation_missing"))
		assert.Equal(t, float64(1), val)
	})
}
