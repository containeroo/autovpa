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

package app

import (
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"strings"

	"github.com/containeroo/tinyflags"

	"github.com/containeroo/autovpa/internal/config"
	"github.com/containeroo/autovpa/internal/controller"
	"github.com/containeroo/autovpa/internal/flag"
	"github.com/containeroo/autovpa/internal/logging"
	"github.com/containeroo/autovpa/internal/utils"

	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	"sigs.k8s.io/controller-runtime/pkg/metrics/filters"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
)

var scheme = runtime.NewScheme()

func init() {
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
}

// Run is the main function of the application.
func Run(ctx context.Context, version string, args []string, w io.Writer) error {
	// Parse and validate command-line arguments
	flags, err := flag.ParseArgs(args, version)
	if err != nil {
		if tinyflags.IsHelpRequested(err) || tinyflags.IsVersionRequested(err) {
			fmt.Fprint(w, err.Error()) // nolint:errcheck
			return nil
		}
		return fmt.Errorf("error parsing arguments: %w", err)
	}

	// Configure logging
	logger, err := logging.InitLogging(flags, w)
	if err != nil {
		return fmt.Errorf("error setting up logger: %w", err)
	}

	setupLog := logger.WithName("setup")
	setupLog.Info("initializing autovpa", "version", version)

	// Load profiles
	cfg, err := config.LoadFile(flags.ConfigPath)
	if err != nil {
		return fmt.Errorf("failed to load profiles: %w", err)
	}
	if err := cfg.Validate(flags.DefaultNameTemplate); err != nil {
		return fmt.Errorf("failed to validate profiles: %w", err)
	}
	if overrides := flags.ChangedFlags(); len(overrides) > 0 {
		setupLog.Info("flag overrides", "values", strings.Join(overrides, ", "))
	}

	// Log profiles
	for name, profile := range cfg.Profiles {
		setupLog.Info("loaded profile",
			"name", name,
			"nameTemplate", utils.DefaultIfZero(profile.NameTemplate, flags.DefaultNameTemplate),
			"spec", profile.Spec,
		)
	}

	// Profiles config
	profilesCfg := controller.ProfileConfig{
		Entries:      cfg.Profiles,
		Default:      cfg.DefaultProfile,
		NameTemplate: flags.DefaultNameTemplate,
	}

	// Metadata config
	metaCfg := controller.MetaConfig{
		ProfileKey:   flags.ProfileAnnotation,
		ManagedLabel: flags.ManagedLabel,
	}

	// Validate annotation/label uniqueness
	meta := map[string]string{
		"Managed": flags.ManagedLabel,
		"Profile": flags.ProfileAnnotation,
	}
	if err := utils.ValidateUniqueKeys(meta); err != nil {
		return fmt.Errorf("annotation/label keys must be unique: %w", err)
	}
	setupLog.Info("configured annotation/label keys", "values", utils.FormatKeys(meta))

	// Configure HTTP/2 settings
	tlsOpts := []func(*tls.Config){}
	if !flags.EnableHTTP2 {
		setupLog.Info("disabling HTTP/2 for compatibility")
		tlsOpts = append(tlsOpts, func(c *tls.Config) {
			c.NextProtos = []string{"http/1.1"}
		})
	}

	// Set up webhook server (no admission webhooks registered yet; add here if needed).
	webhookServer := webhook.NewServer(webhook.Options{
		TLSOpts: tlsOpts,
	})

	// Configure metrics server
	metricsServerOptions := metricsserver.Options{BindAddress: "0"} // disable listener by default
	if flags.EnableMetrics {
		metricsServerOptions = metricsserver.Options{
			BindAddress:   flags.MetricsAddr,
			SecureServing: flags.SecureMetrics,
			TLSOpts:       tlsOpts,
		}
		if flags.SecureMetrics {
			metricsServerOptions.FilterProvider = filters.WithAuthenticationAndAuthorization
		}
	}

	// Create Cache Options
	cacheOpts := utils.ToCacheOptions(flags.WatchNamespaces)

	// Create and initialize the manager
	restCfg, err := ctrl.GetConfig()
	if err != nil {
		return fmt.Errorf("unable to get Kubernetes REST config: %w", err)
	}
	if flags.CRDCheck {
		if err := utils.EnsureVPAResource(restCfg); err != nil {
			return err
		}
	}
	mgr, err := ctrl.NewManager(restCfg, ctrl.Options{
		Scheme:                 scheme,
		Metrics:                metricsServerOptions,
		Logger:                 logger,
		WebhookServer:          webhookServer,
		HealthProbeBindAddress: flags.ProbeAddr,
		LeaderElection:         flags.LeaderElection,
		LeaderElectionID:       "fc1fdccd.autovpa.containeroo.ch",
		Cache:                  cacheOpts,
	})
	if err != nil {
		return fmt.Errorf("unable to create manager: %w", err)
	}

	// Log watching namespaces
	if len(flags.WatchNamespaces) == 0 {
		setupLog.Info("namespace scope", "mode", "cluster-wide")
	} else {
		setupLog.Info("namespace scope", "mode", "namespaced", "namespaces", flags.WatchNamespaces)
	}

	// Setup Deployment controller
	if err := (&controller.DeploymentReconciler{
		BaseReconciler: controller.BaseReconciler{
			Logger:     &logger,
			KubeClient: mgr.GetClient(),
			Recorder:   mgr.GetEventRecorderFor("deployment-controller"),
			Profiles:   profilesCfg,
			Meta:       metaCfg,
		},
	}).SetupWithManager(mgr); err != nil {
		return fmt.Errorf("unable to create Deployment controller: %w", err)
	}

	// Setup StatefulSet controller
	if err := (&controller.StatefulSetReconciler{
		BaseReconciler: controller.BaseReconciler{
			Logger:     &logger,
			KubeClient: mgr.GetClient(),
			Recorder:   mgr.GetEventRecorderFor("statefulset-controller"),
			Profiles:   profilesCfg,
			Meta:       metaCfg,
		},
	}).SetupWithManager(mgr); err != nil {
		return fmt.Errorf("unable to create StatefulSet controller: %w", err)
	}

	// Setup DaemonSet controller
	if err := (&controller.DaemonSetReconciler{
		BaseReconciler: controller.BaseReconciler{
			Logger:     &logger,
			KubeClient: mgr.GetClient(),
			Recorder:   mgr.GetEventRecorderFor("daemonset-controller"),
			Profiles:   profilesCfg,
			Meta:       metaCfg,
		},
	}).SetupWithManager(mgr); err != nil {
		return fmt.Errorf("unable to create DaemonSet controller: %w", err)
	}

	// Setup VPA controller
	if err := (&controller.VPAReconciler{
		Logger:     &logger,
		KubeClient: mgr.GetClient(),
		Recorder:   mgr.GetEventRecorderFor("vpa-controller"),
		Meta:       metaCfg,
	}).SetupWithManager(mgr); err != nil {
		return fmt.Errorf("unable to create VPA controller: %w", err)
	}

	// Register health and readiness checks
	if err := mgr.AddHealthzCheck("healthz", healthz.Ping); err != nil {
		return fmt.Errorf("failed to set up health check: %w", err)
	}
	if err := mgr.AddReadyzCheck("readyz", healthz.Ping); err != nil {
		return fmt.Errorf("failed to set up ready check: %w", err)
	}

	// Start the manager
	setupLog.Info("starting manager")
	if err := mgr.Start(ctx); err != nil {
		return fmt.Errorf("manager encountered an error while running: %w", err)
	}

	return nil
}
