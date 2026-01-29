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

	"github.com/containeroo/tinyflags"

	"github.com/containeroo/autovpa/internal/config"
	"github.com/containeroo/autovpa/internal/controller"
	"github.com/containeroo/autovpa/internal/flag"
	"github.com/containeroo/autovpa/internal/logging"
	internalmetrics "github.com/containeroo/autovpa/internal/metrics"
	"github.com/containeroo/autovpa/internal/utils"

	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	crmetrics "sigs.k8s.io/controller-runtime/pkg/metrics"
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
	flags, err := flag.ParseArgs(args, version)
	if err != nil {
		if tinyflags.IsHelpRequested(err) || tinyflags.IsVersionRequested(err) {
			_, _ = fmt.Fprint(w, err.Error())
			return nil
		}
		return err
	}

	// Setup logger immediately so startup errors are correctly logged.
	logger, lErr := logging.InitLogging(flags, w)
	setupLog := logger.WithName("setup")
	setupLog.Info("initializing autovpa", "version", version)
	if lErr != nil {
		logger.Error(lErr, "error setting up logger")
		return err
	}

	cfg, err := config.LoadFile(flags.ConfigPath)
	if err != nil {
		logger.Error(err, "failed to load profiles")
		return err
	}
	if err := cfg.Validate(flags.DefaultNameTemplate); err != nil {
		logger.Error(err, "failed to validate profiles")
		return err
	}

	if len(flags.OverriddenValues) > 0 {
		setupLog.Info("CLI Overrides", "overrides", flags.OverriddenValues)
	}

	for name, profile := range cfg.Profiles {
		setupLog.Info("loaded profile",
			"name", name,
			"nameTemplate", utils.DefaultIfZero(profile.NameTemplate, flags.DefaultNameTemplate),
			"spec", profile.Spec,
		)
	}

	profilesCfg := controller.ProfileConfig{
		Entries:      cfg.Profiles,
		Default:      cfg.DefaultProfile,
		NameTemplate: flags.DefaultNameTemplate,
	}

	metaCfg := controller.MetaConfig{
		ProfileKey:   flags.ProfileAnnotation,
		ManagedLabel: flags.ManagedLabel,
	}

	meta := map[string]string{
		"Managed": flags.ManagedLabel,
		"Profile": flags.ProfileAnnotation,
	}
	if err := utils.ValidateUniqueKeys(meta); err != nil {
		logger.Error(err, "annotation/label keys must be unique")
		return err
	}
	setupLog.Info("configured annotation/label keys", "values", utils.FormatKeys(meta))

	tlsOpts := []func(*tls.Config){}
	if !flags.EnableHTTP2 {
		setupLog.Info("disabling HTTP/2 for compatibility")
		tlsOpts = append(tlsOpts, func(c *tls.Config) {
			c.NextProtos = []string{"http/1.1"}
		})
	}

	webhookServer := webhook.NewServer(webhook.Options{
		TLSOpts: tlsOpts,
	})

	metricsReg := internalmetrics.NewRegistry(crmetrics.Registry)

	metricsServerOptions := metricsserver.Options{
		BindAddress: "0", // disabled by default
	}
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

	cacheOpts := utils.ToCacheOptions(flags.WatchNamespaces)

	restCfg, err := ctrl.GetConfig()
	if err != nil {
		logger.Error(err, "unable to get Kubernetes REST config")
		return err
	}

	if flags.CRDCheck {
		if err := utils.EnsureVPAResource(restCfg); err != nil {
			logger.Error(err, "failed to ensure VPA CRD")
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
		logger.Error(err, "unable to create manager")
		return err
	}

	if len(flags.WatchNamespaces) == 0 {
		setupLog.Info("namespace scope", "mode", "cluster-wide")
	} else {
		setupLog.Info("namespace scope", "mode", "namespaced", "namespaces", flags.WatchNamespaces)
	}

	if err := (&controller.DeploymentReconciler{
		BaseReconciler: controller.BaseReconciler{
			Logger:     &logger,
			KubeClient: mgr.GetClient(),
			Recorder:   mgr.GetEventRecorderFor("deployment-controller"),
			Profiles:   profilesCfg,
			Meta:       metaCfg,
			Metrics:    metricsReg,
		},
	}).SetupWithManager(mgr); err != nil {
		logger.Error(err, "unable to create Deployment controller")
		return err
	}

	if err := (&controller.StatefulSetReconciler{
		BaseReconciler: controller.BaseReconciler{
			Logger:     &logger,
			KubeClient: mgr.GetClient(),
			Recorder:   mgr.GetEventRecorderFor("statefulset-controller"),
			Profiles:   profilesCfg,
			Meta:       metaCfg,
			Metrics:    metricsReg,
		},
	}).SetupWithManager(mgr); err != nil {
		logger.Error(err, "unable to create StatefulSet controller")
		return err
	}

	if err := (&controller.DaemonSetReconciler{
		BaseReconciler: controller.BaseReconciler{
			Logger:     &logger,
			KubeClient: mgr.GetClient(),
			Recorder:   mgr.GetEventRecorderFor("daemonset-controller"),
			Profiles:   profilesCfg,
			Meta:       metaCfg,
			Metrics:    metricsReg,
		},
	}).SetupWithManager(mgr); err != nil {
		logger.Error(err, "unable to create DaemonSet controller")
		return err
	}

	if err := (&controller.VPAReconciler{
		Logger:     &logger,
		KubeClient: mgr.GetClient(),
		Recorder:   mgr.GetEventRecorderFor("vpa-controller"),
		Meta:       metaCfg,
		Metrics:    metricsReg,
	}).SetupWithManager(mgr); err != nil {
		logger.Error(err, "unable to create VPA controller")
		return err
	}

	if err := mgr.AddHealthzCheck("healthz", healthz.Ping); err != nil {
		logger.Error(err, "failed to set up health check")
		return err
	}
	if err := mgr.AddReadyzCheck("readyz", healthz.Ping); err != nil {
		logger.Error(err, "failed to set up ready check")
		return err
	}

	setupLog.Info("starting manager")
	if err := mgr.Start(ctx); err != nil {
		logger.Error(err, "manager encountered an error while running")
		return err
	}

	return nil
}
