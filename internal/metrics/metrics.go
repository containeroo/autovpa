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

import "github.com/prometheus/client_golang/prometheus"

// Registry provides a typed façade for recording AutoVPA Prometheus metrics.
type Registry struct {
	reg                    prometheus.Registerer
	vpaCreated             *prometheus.CounterVec
	vpaUpdated             *prometheus.CounterVec
	vpaSkipped             *prometheus.CounterVec
	vpaDeletedObsolete     *prometheus.CounterVec
	vpaDeletedOptOut       *prometheus.CounterVec
	vpaDeletedWorkloadGone *prometheus.CounterVec
	vpaDeletedOwnerGone    *prometheus.CounterVec
	vpaDeletedOrphaned     *prometheus.CounterVec
	vpaManaged             *prometheus.GaugeVec
	vpaReconcileErrors     *prometheus.CounterVec
}

// NewRegistry creates and registers all AutoVPA metrics with the provided
// Prometheus registerer, allowing the metrics server to expose them automatically.
func NewRegistry(reg prometheus.Registerer) *Registry {
	if reg == nil {
		reg = prometheus.DefaultRegisterer
	}

	vpaCreated := prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "autovpa_vpa_created_total",
			Help: "Total number of VPAs created by the operator.",
		},
		[]string{"namespace", "name", "kind", "profile"},
	)

	vpaUpdated := prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "autovpa_vpa_updated_total",
			Help: "Total number of VPAs updated by the operator.",
		},
		[]string{"namespace", "name", "kind", "profile"},
	)

	vpaSkipped := prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "autovpa_vpa_skipped_total",
			Help: "Total number of workload reconciliations skipped (reason label indicates why).",
		},
		[]string{"namespace", "name", "kind", "reason"},
	)

	vpaDeletedObsolete := prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "autovpa_vpa_deleted_obsolete_total",
			Help: "Total number of managed VPAs deleted because they became obsolete (name/profile change).",
		},
		[]string{"namespace", "kind"},
	)

	vpaDeletedOptOut := prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "autovpa_vpa_deleted_opt_out_total",
			Help: "Total number of managed VPAs deleted because the workload opted out.",
		},
		[]string{"namespace", "kind"},
	)

	vpaDeletedWorkloadGone := prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "autovpa_vpa_deleted_workload_gone_total",
			Help: "Total number of managed VPAs deleted because the workload no longer exists.",
		},
		[]string{"namespace", "kind"},
	)

	vpaDeletedOwnerGone := prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "autovpa_vpa_deleted_owner_gone_total",
			Help: "Total number of managed VPAs deleted because the referenced owner is missing.",
		},
		[]string{"namespace", "kind"},
	)

	vpaDeletedOrphaned := prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "autovpa_vpa_deleted_orphaned_total",
			Help: "Total number of managed VPAs deleted because they lacked a controller owner reference.",
		},
		[]string{"namespace"},
	)

	vpaManaged := prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "autovpa_managed_vpa",
			Help: "Current number of managed VPAs by namespace and profile.",
		},
		[]string{"namespace", "profile"},
	)

	vpaReconcileErrors := prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "autovpa_reconcile_errors_total",
			Help: "Total number of reconciliation errors, labeled by controller, kind, and reason.",
		},
		[]string{"controller", "kind", "reason"},
	)

	reg.MustRegister(
		vpaCreated,
		vpaUpdated,
		vpaSkipped,
		vpaDeletedObsolete,
		vpaDeletedOptOut,
		vpaDeletedWorkloadGone,
		vpaDeletedOwnerGone,
		vpaDeletedOrphaned,
		vpaManaged,
		vpaReconcileErrors,
	)

	return &Registry{
		reg:                    reg,
		vpaCreated:             vpaCreated,
		vpaUpdated:             vpaUpdated,
		vpaSkipped:             vpaSkipped,
		vpaDeletedObsolete:     vpaDeletedObsolete,
		vpaDeletedOptOut:       vpaDeletedOptOut,
		vpaDeletedWorkloadGone: vpaDeletedWorkloadGone,
		vpaDeletedOwnerGone:    vpaDeletedOwnerGone,
		vpaDeletedOrphaned:     vpaDeletedOrphaned,
		vpaManaged:             vpaManaged,
		vpaReconcileErrors:     vpaReconcileErrors,
	}
}

// IncVPACreated increments the counter for created VPAs.
func (r *Registry) IncVPACreated(namespace, name, kind, profile string) {
	r.vpaCreated.WithLabelValues(namespace, name, kind, profile).Inc()
}

// IncVPAUpdated increments the counter for updated VPAs.
func (r *Registry) IncVPAUpdated(namespace, name, kind, profile string) {
	r.vpaUpdated.WithLabelValues(namespace, name, kind, profile).Inc()
}

// IncVPASkipped increments the counter for skipped reconciliations.
func (r *Registry) IncVPASkipped(namespace, name, kind, reason string) {
	r.vpaSkipped.WithLabelValues(namespace, name, kind, reason).Inc()
}

// IncVPADeletedObsolete increments the counter for obsolete VPAs.
func (r *Registry) IncVPADeletedObsolete(namespace, kind string) {
	r.vpaDeletedObsolete.WithLabelValues(namespace, kind).Inc()
}

// IncVPADeletedOptOut increments the counter for opt-out deletions.
func (r *Registry) IncVPADeletedOptOut(namespace, kind string) {
	r.vpaDeletedOptOut.WithLabelValues(namespace, kind).Inc()
}

// IncVPADeletedWorkloadGone increments the counter for missing workload deletions.
func (r *Registry) IncVPADeletedWorkloadGone(namespace, kind string) {
	r.vpaDeletedWorkloadGone.WithLabelValues(namespace, kind).Inc()
}

// IncVPADeletedOwnerGone increments the counter for missing owner deletions.
func (r *Registry) IncVPADeletedOwnerGone(namespace, kind string) {
	r.vpaDeletedOwnerGone.WithLabelValues(namespace, kind).Inc()
}

// IncVPADeletedOrphaned increments the counter for orphaned VPAs.
func (r *Registry) IncVPADeletedOrphaned(namespace string) {
	r.vpaDeletedOrphaned.WithLabelValues(namespace).Inc()
}

// IncVPAManaged increments the gauge tracking managed VPAs.
func (r *Registry) IncVPAManaged(namespace, profile string) {
	r.vpaManaged.WithLabelValues(namespace, profile).Inc()
}

// DecVPAManaged decrements the gauge tracking managed VPAs.
func (r *Registry) DecVPAManaged(namespace, profile string) {
	r.vpaManaged.WithLabelValues(namespace, profile).Dec()
}

// IncReconcileErrors increments the counter for reconciliation errors.
func (r *Registry) IncReconcileErrors(controller, kind, reason string) {
	r.vpaReconcileErrors.WithLabelValues(controller, kind, reason).Inc()
}
