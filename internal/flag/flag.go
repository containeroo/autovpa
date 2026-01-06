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

package flag

import (
	"net"

	"github.com/containeroo/tinyflags"
)

const (
	profileAnnotation   string = "autovpa.containeroo.ch/profile"
	managedLabel        string = "autovpa.containeroo.ch/managed"
	DefaultNameTemplate string = "{{ .WorkloadName }}-{{ .Profile }}-vpa"
)

// Options holds all configuration options for the application.
type Options struct {
	WatchNamespaces     []string       // Namespaces to watch
	MetricsAddr         string         // Address for the metrics server
	LeaderElection      bool           // Enable leader election
	ProbeAddr           string         // Address for health and readiness probes
	SecureMetrics       bool           // Serve metrics over HTTPS
	EnableHTTP2         bool           // Enable HTTP/2 for servers
	EnableMetrics       bool           // Enable or disable metrics
	LogEncoder          string         // Log format: "json" or "console"
	LogStacktraceLevel  string         // Stacktrace log level
	LogDev              bool           // Enable development logging mode
	ProfileAnnotation   string         // Annotation key workloads must set to request a profile.
	ManagedLabel        string         // Label key to mark VPAs as managed by the operator.
	DefaultNameTemplate string         // Template used to render managed VPA names; can be overridden per profile.
	ConfigPath          string         // Path to the Config containing VPA profiles.
	CRDCheck            bool           // Enable the check for the VPA CRD.
	SkipManagerStart    bool           // Skip starting the manager (used by tests).
	OverriddenValues    map[string]any // CLI overrides
}

// ParseArgs parses CLI flags into Options and handles --help/--version output.
func ParseArgs(args []string, version string) (Options, error) {
	opts := Options{}

	tf := tinyflags.NewFlagSet("autovpa", tinyflags.ContinueOnError)
	tf.Version(version)
	tf.EnvPrefix("AUTO_VPA")
	tf.HideEnvs()
	tf.Note("*) These variables are available in the template string: " +
		"\".WorkloadName\", \".Namespace\", \".Kind\", \".Profile\".\n" +
		"Template functions: toLower, replace, trim, truncate, dnsLabel.\n\n" +
		"Each flag can also be set via environment variable using the AUTO_VPA_ prefix, " +
		"e.g.: --log-encoder=json → AUTO_VPA_LOG_ENCODER=json")

	// Application
	tf.StringVar(&opts.ConfigPath, "config", "config.yaml", "Path to configuration file").
		Short("c").
		Value()
	tf.Bool("disable-crd-check", false, "Disable the check for the VPA CRD").
		Finalize(func(v bool) bool {
			opts.CRDCheck = !v
			return v
		}).
		Value()
	tf.StringVar(&opts.ProfileAnnotation, "profile-annotation", profileAnnotation, "Annotation key workloads must set to request a profile").
		Placeholder("ANNOTATION").
		Value()
	tf.StringVar(&opts.ManagedLabel, "managed-label", managedLabel, "Label key to mark VPAs as managed by the operator").
		Placeholder("LABEL").
		Value()
	tf.StringVar(&opts.DefaultNameTemplate, "vpa-name-template", DefaultNameTemplate, "Template used to render managed VPA names; override per profile with nameTemplate *\n").
		Placeholder("TEMPLATE-STRING").
		Value()

	// Controller
	tf.StringSliceVar(&opts.WatchNamespaces, "watch-namespace", nil, "Namespaces to watch (can be repeated or comma-separated)").
		Placeholder("NAMESPACE").
		Value()

	// Metrics
	tf.BoolVar(&opts.EnableMetrics, "metrics-enabled", true, "Enable or disable the metrics endpoint").
		Strict().
		HideAllowed().
		Value()
	metricsBindAddress := tf.TCPAddr("metrics-bind-address", &net.TCPAddr{IP: nil, Port: 8443}, "Metrics server address").
		Placeholder("ADDR:PORT").
		Value()
	tf.BoolVar(&opts.SecureMetrics, "metrics-secure", true, "Serve metrics over HTTPS").
		Strict().
		HideAllowed().
		Value()

	// Server
	healthProbeaddress := tf.TCPAddr("health-probe-bind-address", &net.TCPAddr{IP: nil, Port: 8081}, "Health and readiness probe address").
		Placeholder("ADDR:PORT").
		Value()
	tf.BoolVar(&opts.EnableHTTP2, "enable-http2", false, "Enable HTTP/2 for servers").
		Strict().
		HideAllowed().
		Value()
	tf.BoolVar(&opts.LeaderElection, "leader-elect", true, "Enable leader election").
		Strict().
		HideAllowed().
		Value()
	tf.BoolVar(&opts.SkipManagerStart, "skip-manager-start", false, "Skip starting the manager (tests only)").
		HideAllowed().
		Value()

	// Logging
	tf.StringVar(&opts.LogEncoder, "log-encoder", "json", "Log format (json, console)").
		Choices("json", "console").
		HideAllowed().
		Value()
	tf.BoolVar(&opts.LogDev, "log-devel", false, "Enable development mode logging").Value()
	tf.StringVar(&opts.LogStacktraceLevel, "log-stacktrace-level", "panic", "Stacktrace log level").
		Choices("info", "error", "panic").
		HideAllowed().
		Value()

	if err := tf.Parse(args); err != nil {
		return Options{}, err
	}

	opts.MetricsAddr = (*metricsBindAddress).String()
	opts.ProbeAddr = (*healthProbeaddress).String()
	opts.OverriddenValues = tf.OverriddenValues()

	return opts, nil
}
