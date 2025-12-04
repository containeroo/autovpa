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
	"fmt"
	"net"
	"sort"
	"strings"

	"github.com/containeroo/tinyflags"
)

const (
	profileAnnotation      string = "autovpa.containeroo.ch/profile"
	managedLabel           string = "autovpa.containeroo.ch/managed"
	ArgoTrackingAnnotation string = "argocd.argoproj.io/tracking-id"
	DefaultNameTemplate    string = "{{ .WorkloadName }}-vpa"
)

// Options holds all configuration options for the application.
type Options struct {
	WatchNamespaces        []string // Namespaces to watch
	MetricsAddr            string   // Address for the metrics server
	LeaderElection         bool     // Enable leader election
	ProbeAddr              string   // Address for health and readiness probes
	SecureMetrics          bool     // Serve metrics over HTTPS
	EnableHTTP2            bool     // Enable HTTP/2 for servers
	EnableMetrics          bool     // Enable or disable metrics
	LogEncoder             string   // Log format: "json" or "console"
	LogStacktraceLevel     string   // Stacktrace log level
	LogDev                 bool     // Enable development logging mode
	ProfileAnnotation      string   // Annotation key workloads must set to request a profile.
	ManagedLabel           string   // Annotation key to mark VPAs as managed by the operator.
	ArgoManaged            bool     // Propagate the Argo tracking annotation to managed VPAs.
	ArgoTrackingAnnotation string   // Annotation key to propagate when ArgoManaged is enabled.
	DefaultNameTemplate    string   // Template used to render managed VPA names; can be overridden per profile.
	ConfigPath             string   // Path to the Config containing VPA profiles.
	CRDCheck               bool     // Enable the check for the VPA CRD.

	fs *tinyflags.FlagSet // parsed flagset (for changed-state queries)
}

// ParseArgs parses CLI flags into Options and handles --help/--version output.
func ParseArgs(args []string, version string) (Options, error) {
	options := Options{}

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
	tf.StringVar(&options.ConfigPath, "config", "config.yaml", "Path to configuration file").
		Short("c").
		Value()
	tf.Bool("disable-crd-check", false, "Disable the check for the VPA CRD").
		Finalize(func(v bool) bool {
			options.CRDCheck = !v
			return v
		}).
		Value()
	tf.StringVar(&options.ProfileAnnotation, "profile-annotation", profileAnnotation, "Annotation key workloads must set to request a profile").
		Placeholder("ANNOTATION").
		Value()
	tf.StringVar(&options.ManagedLabel, "managed-label", managedLabel, "Label key to mark VPAs as managed by the operator").
		Placeholder("LABEL").
		Value()
	tf.BoolVar(&options.ArgoManaged, "argo-managed", false, fmt.Sprintf("Add the annotation %q to the managed VPAs", ArgoTrackingAnnotation)).
		Strict().
		HideAllowed().
		Value()
	tf.StringVar(&options.DefaultNameTemplate, "vpa-name-template", DefaultNameTemplate, "Template used to render managed VPA names; override per profile with nameTemplate *\n").
		Placeholder("TEMPLATE-STRING").
		Value()

	// Controller
	tf.StringSliceVar(&options.WatchNamespaces, "watch-namespace", nil, "Namespaces to watch (can be repeated or comma-separated)").
		Placeholder("NAMESPACE").
		Value()

	// Metrics
	tf.BoolVar(&options.EnableMetrics, "metrics-enabled", true, "Enable or disable the metrics endpoint").
		Strict().
		HideAllowed().
		Value()
	metricsBindAddress := tf.TCPAddr("metrics-bind-address", &net.TCPAddr{IP: nil, Port: 8443}, "Metrics server address").
		Placeholder("ADDR:PORT").
		Value()
	tf.BoolVar(&options.SecureMetrics, "metrics-secure", true, "Serve metrics over HTTPS").
		Strict().
		HideAllowed().
		Value()

	// Server
	healthProbeaddress := tf.TCPAddr("health-probe-bind-address", &net.TCPAddr{IP: nil, Port: 8081}, "Health and readiness probe address").
		Placeholder("ADDR:PORT").
		Value()
	tf.BoolVar(&options.EnableHTTP2, "enable-http2", false, "Enable HTTP/2 for servers").
		Strict().
		HideAllowed().
		Value()
	tf.BoolVar(&options.LeaderElection, "leader-elect", true, "Enable leader election").
		Strict().
		HideAllowed().
		Value()

	// Logging
	tf.StringVar(&options.LogEncoder, "log-encoder", "json", "Log format (json, console)").
		Choices("json", "console").
		HideAllowed().
		Value()
	tf.BoolVar(&options.LogDev, "log-devel", false, "Enable development mode logging").Value()
	tf.StringVar(&options.LogStacktraceLevel, "log-stacktrace-level", "panic", "Stacktrace log level").
		Choices("info", "error", "panic").
		HideAllowed().
		Value()

	if err := tf.Parse(args); err != nil {
		return Options{}, err
	}

	options.MetricsAddr = (*metricsBindAddress).String()
	options.ProbeAddr = (*healthProbeaddress).String()
	options.ArgoTrackingAnnotation = ArgoTrackingAnnotation
	options.fs = tf // store the parsed flagset for changed-state queries

	return options, nil
}

// ChangedFlags checks if any of the flags were changed.
func (o Options) ChangedFlags() []string {
	var out []string
	// add adds a flag to the list of changed flags.
	add := func(k, v string) { out = append(out, fmt.Sprintf("%s=%s", k, v)) }

	if o.WasSet("metrics-bind-address") {
		add("metrics-bind-address", o.MetricsAddr)
	}
	if o.WasSet("leader-elect") {
		add("leader-elect", fmt.Sprintf("%v", o.LeaderElection))
	}
	if o.WasSet("health-probe-bind-address") {
		add("health-probe-bind-address", o.ProbeAddr)
	}
	if o.WasSet("metrics-secure") {
		add("metrics-secure", fmt.Sprintf("%v", o.SecureMetrics))
	}
	if o.WasSet("enable-http2") {
		add("enable-http2", fmt.Sprintf("%v", o.EnableHTTP2))
	}
	if o.WasSet("metrics-enabled") {
		add("metrics-enabled", fmt.Sprintf("%v", o.EnableMetrics))
	}
	if o.WasSet("log-encoder") {
		add("log-encoder", o.LogEncoder)
	}
	if o.WasSet("log-stacktrace-level") {
		add("log-stacktrace-level", o.LogStacktraceLevel)
	}
	if o.WasSet("log-devel") {
		add("log-devel", fmt.Sprintf("%v", o.LogDev))
	}
	if o.WasSet("profile-annotation") {
		add("profile-annotation", o.ProfileAnnotation)
	}
	if o.WasSet("managed-label") {
		add("managed-label", o.ManagedLabel)
	}
	if o.WasSet("argo-managed") {
		add("argo-managed", fmt.Sprintf("%v", o.ArgoManaged))
	}
	if o.WasSet("vpa-name-template") {
		add("vpa-name-template", o.DefaultNameTemplate)
	}
	if o.WasSet("config") {
		add("config", o.ConfigPath)
	}
	if o.WasSet("disable-crd-check") {
		add("disable-crd-check", fmt.Sprintf("%v", !o.CRDCheck))
	}
	if o.WasSet("watch-namespace") {
		add("watch-namespace", strings.Join(o.WatchNamespaces, ","))
	}

	sort.Strings(out) // sort for deterministic output
	return out
}

// WasSet reports whether the given flag name was explicitly set by the user.
// Returns false for unknown flags or if not set.
func (o Options) WasSet(name string) bool {
	if o.fs == nil {
		return false
	}
	fl := o.fs.LookupFlag(name)
	return fl != nil && fl.Value.Changed()
}
