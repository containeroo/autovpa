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
	"github.com/prometheus/client_golang/prometheus"
	"sigs.k8s.io/controller-runtime/pkg/metrics"
)

var (
	// VPACreated counts VPAs created by the operator.
	VPACreated = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "autovpa_vpa_created_total",
			Help: "Total number of VPAs created by the operator.",
		},
		[]string{"namespace", "name", "kind", "profile"},
	)

	// VPAUpdated counts VPAs updated by the operator.
	VPAUpdated = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "autovpa_vpa_updated_total",
			Help: "Total number of VPAs updated by the operator.",
		},
		[]string{"namespace", "name", "kind", "profile"},
	)

	// VPASkipped counts workloads skipped due to missing annotation/profile.
	VPASkipped = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "autovpa_vpa_skipped_total",
			Help: "Total number of workload reconciliations skipped (reason label indicates why).",
		},
		[]string{"namespace", "name", "kind", "reason"},
	)

	// VPADeleted... counters track cleanup events by cause.
	VPADeletedObsolete = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "autovpa_vpa_deleted_obsolete_total",
			Help: "Total number of managed VPAs deleted because they became obsolete (name/profile change).",
		},
		[]string{"namespace", "kind"},
	)
	VPADeletedOptOut = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "autovpa_vpa_deleted_opt_out_total",
			Help: "Total number of managed VPAs deleted because the workload opted out.",
		},
		[]string{"namespace", "kind"},
	)
	VPADeletedWorkloadGone = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "autovpa_vpa_deleted_workload_gone_total",
			Help: "Total number of managed VPAs deleted because the workload no longer exists.",
		},
		[]string{"namespace", "kind"},
	)
	VPADeletedOwnerGone = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "autovpa_vpa_deleted_owner_gone_total",
			Help: "Total number of managed VPAs deleted because the referenced owner is missing.",
		},
		[]string{"namespace", "kind"},
	)
	VPADeletedOrphaned = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "autovpa_vpa_deleted_orphaned_total",
			Help: "Total number of managed VPAs deleted because they lacked a controller owner reference.",
		},
		[]string{"namespace"},
	)

	// VPAManaged tracks the current inventory of managed VPAs by namespace/profile.
	VPAManaged = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "autovpa_managed_vpa",
			Help: "Current number of managed VPAs by namespace and profile.",
		},
		[]string{"namespace", "profile"},
	)

	// ReconcileErrors counts reconciliation errors by controller/kind/reason.
	ReconcileErrors = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "autovpa_reconcile_errors_total",
			Help: "Total number of reconciliation errors, labeled by controller, kind, and reason.",
		},
		[]string{"controller", "kind", "reason"},
	)
)

func init() {
	metrics.Registry.MustRegister(
		VPACreated,
		VPAUpdated,
		VPASkipped,
		VPADeletedObsolete,
		VPADeletedOptOut,
		VPADeletedWorkloadGone,
		VPADeletedOwnerGone,
		VPADeletedOrphaned,
		VPAManaged,
		ReconcileErrors,
	)
}
