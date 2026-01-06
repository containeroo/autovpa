# AutoVPA

[![Go Report Card](https://goreportcard.com/badge/github.com/containeroo/autovpa?style=flat-square)](https://goreportcard.com/report/github.com/containeroo/autovpa)
[![Go Doc](https://img.shields.io/badge/godoc-reference-blue.svg?style=flat-square)](https://godoc.org/github.com/containeroo/autovpa)
[![Release](https://img.shields.io/github/release/containeroo/autovpa.svg?style=flat-square)](https://github.com/containeroo/autovpa/releases/latest)
[![GitHub tag](https://img.shields.io/github/tag/containeroo/autovpa.svg?style=flat-square)](https://github.com/containeroo/autovpa/releases/latest)
[![license](https://img.shields.io/github/license/containeroo/autovpa.svg?style=flat-square)](LICENSE)

AutoVPA watches Deployments, StatefulSets, and DaemonSets and ensures a matching `VerticalPodAutoscaler` exists for each workload that opts in via an annotation. Profiles are defined once in a YAML file, and the operator renders the VPA spec with the selected profile.

## Prerequisites

- VPA CRDs installed in the cluster.
- Config file mounted at the configured `--config`.

## Quick Start

```bash
# Install (Helm)
helm upgrade --install autovpa ./deploy/kubernetes/chart/autovpa

# Or apply kustomize manifests
kubectl apply -k deploy/kubernetes
```

Annotate a workload to opt in and AutoVPA creates/updates the matching VPA using your default template (`{{ .WorkloadName }}-{{ .Profile }}-vpa` by default):

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: api
  namespace: demo
  annotations:
    autovpa.containeroo.ch/profile: default
spec:
  replicas: 2
  template:
    spec:
      containers:
        - name: api
          image: ghcr.io/example/api:latest
```

## Installation and Usage

- **Helm**: `helm upgrade --install autovpa ./deploy/kubernetes/chart/autovpa`
- **Kustomize/manifests**: apply `deploy/kubernetes/kustomization.yaml` (or the rendered manifests) after setting image/tag/args.
- **Profiles**: mount a YAML containing `defaultProfile` and `profiles` into the pod (default path `config.yaml`). Each profile may optionally set `nameTemplate` to override the default VPA name template.

### Namespaced Mode

By default, `AutoVPA` watches all namespaces. To restrict it to specific namespaces, pass the `--watch-namespace` flag. This flag can be repeated or comma-separated to specify multiple namespaces. When set, `AutoVPA` will only monitor workloads (and create/update their VPAs) within those namespaces.

If running in namespaced mode, ensure the associated `Role` and `RoleBinding` are configured accordingly. You can use `deploy/manifests/role.template` and `deploy/manifests/rolebinding.template` as starting points for custom RBAC definitions.

## Using AutoVPA

- Add the annotation `autovpa.containeroo.ch/profile: "<profile-name>"` to any Deployment, StatefulSet, or DaemonSet to enable VPA management.
  Use `default` to apply the operator's default profile.

- For each annotated workload, the operator automatically creates or updates a corresponding VPA:
  - **Name** is rendered from the configured template
    (default: `{{ .WorkloadName }}-{{ .Profile }}-vpa`, overridable globally or per profile).
  - **Labels** on the VPA include:
    - all labels from the workload,
    - the managed label (default: `autovpa.containeroo.ch/managed=true`),
    - the profile label (`autovpa.containeroo.ch/profile=<profile-name>`).

- **Profiles** are defined in a YAML file with:
  - a `defaultProfile` key, and
  - a `profiles` map containing per-profile settings (name template override, resource policy, etc.).
    Any `targetRef` fields included in profile specs are ignored - AutoVPA always sets these automatically for you.

### Profile file basics

- `defaultProfile` must name one of the entries in `profiles`.
- Profile specs are inline (no nested `spec:` key). `targetRef` is ignored and will be set automatically.
- `nameTemplate` is optional per profile; otherwise the global `--vpa-name-template` is used.
- `updatePolicy.updateMode` must be a string (`Off`, `Auto`, `Initial`, etc.); boolean `true`/`false` is tolerated and normalized to `Auto`/`Off`.

## Profile file example (config.yaml)

```yaml
---
defaultProfile: default
profiles:
  default:
    # Note: updateMode must be a string ("Off", "Auto", "Initial", etc.).
    updatePolicy:
      updateMode: Off
    resourcePolicy:
      containerPolicies:
        - containerName: "*"
          controlledResources: ["cpu", "memory"]
  safe:
    # optional per-profile VPA name override
    nameTemplate: "{{ .WorkloadName }}-vpa"
    # Profiles are defined inline; do not wrap fields under a separate "spec:" key.
    updatePolicy:
      updateMode: Auto
    resourcePolicy:
      containerPolicies:
        - containerName: "*"
          controlledResources: ["cpu", "memory"]
          minAllowed:
            cpu: 20m
            memory: 64Mi
  aggressive:
    updatePolicy:
      updateMode: Auto
    resourcePolicy:
      containerPolicies:
        - containerName: "*"
          controlledResources: ["cpu", "memory"]
          minAllowed:
            cpu: 100m
            memory: 128Mi
```

### Template hints

For the VPA template name, the following functions are available:

- `toLower`: simple casing helpers. Since the name of the workload is used as the VPA name, this is useful to ensure the name is DNS-1123 compliant. "ToUpp" is not a valid DNS-1123 subdomain.
  e.g.
  `{{ toLower "Hello" }}` → `hello`
- `replace`: replace all occurrences.
  e.g.
  `{{ replace "api-dev" "-" "." }}` → `api.dev`.
- `trim`: strip surrounding whitespace.
  e.g.
  `{{ trim " demo " }}` → `demo`
- `truncate`: keep the first N runes to cap length.
  e.g.
  `{{ truncate .WorkloadName 10 }}` → `myworkload` (first 10 runes)
- `dnsLabel`: normalize to a DNS-safe label (lowercase, non-alnum to `-`).
  e.g.
  `{{ dnsLabel "API_App" }}` → `api-app`

It will be rendered with the following variables:

- `.WorkloadName`: the name of the workload.
- `.Namespace`: the namespace of the workload.
- `.Kind`: the kind of the workload.
- `.Profile`: the profile name.

## Managed vs. Manual VPA Behavior

AutoVPA treats VPAs with the managed label (by default `autovpa.containeroo.ch/managed=true`) as its own and will try to keep them in sync with the workload.

### If someone removes the managed label from a VPA

- As long as the workload **still has** the profile annotation
  (`autovpa.containeroo.ch/profile: <profile-name>`), AutoVPA will:
  - Reconcile the workload again, and
  - Re-add the managed label (and profile label) on the VPA to match its desired state.

- If the workload **no longer has** the profile annotation and the managed label is removed from the VPA:
  - AutoVPA stops treating that VPA as managed and leaves it alone.
  - It effectively becomes a **manual** VPA that you own.

### If someone changes the profile label on a VPA

- AutoVPA always derives the desired profile from the **workload annotation**, not from the VPA.
- If you change or remove `autovpa.containeroo.ch/profile` on the VPA:
  - The next reconcile will reset the VPA's profile label (and spec) back to whatever the workload annotation says.
  - Any manual changes to the VPA spec that conflict with the profile will be overwritten.

In short:

- Edit the **workload** to change behavior permanently.
- Manual edits on **managed VPAs** are treated as temporary and will usually be corrected by the operator.

## Workload example

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: api
  namespace: demo
  annotations:
    autovpa.containeroo.ch/profile: aggressive # or "default" to use the default profile
spec:
  replicas: 2
  template:
    spec:
      containers:
        - name: api
          image: ghcr.io/example/api:latest
```

## Start Parameters

| Flag/Parameter                | Description                                                             | Default                                  | Env Var                              |
| :---------------------------- | :---------------------------------------------------------------------- | :--------------------------------------- | :----------------------------------- |
| `--config`                    | Path to the config file.                                                | `config.yaml`                            | `AUTO_VPA_CONFIG`                    |
| `--disable-crd-check`         | Disable the check for the VPA CRD.                                      | `false`                                  | `AUTO_VPA_DISABLE_CRD_CHECK`         |
| `--profile-annotation`        | Workload annotation key to select a profile.                            | `autovpa.containeroo.ch/profile`         | `AUTO_VPA_PROFILE_ANNOTATION`        |
| `--managed-label`             | Label applied to managed VPAs.                                          | `autovpa.containeroo.ch/managed`         | `AUTO_VPA_MANAGED_LABEL`             |
| `--vpa-name-template`         | Template for VPA names; per-profile `nameTemplate` can override. \*     | `{{ .WorkloadName }}-{{ .Profile }}-vpa` | `AUTO_VPA_VPA_NAME_TEMPLATE`         |
| `--watch-namespace`           | Namespaces to watch (repeatable/comma-separated). Watches all if unset. | (all)                                    | `AUTO_VPA_WATCH_NAMESPACE`           |
| `--metrics-enabled`           | Enable/disable metrics endpoint.                                        | `true`                                   | `AUTO_VPA_METRICS_ENABLED`           |
| `--metrics-bind-address`      | Metrics server address (e.g., `:8443`).                                 | `:8443`                                  | `AUTO_VPA_METRICS_BIND_ADDRESS`      |
| `--metrics-secure`            | Serve metrics over HTTPS.                                               | `true`                                   | `AUTO_VPA_METRICS_SECURE`            |
| `--enable-http2`              | Enable HTTP/2 for servers.                                              | `false`                                  | `AUTO_VPA_ENABLE_HTTP2`              |
| `--health-probe-bind-address` | Health/readiness probe address.                                         | `:8081`                                  | `AUTO_VPA_HEALTH_PROBE_BIND_ADDRESS` |
| `--leader-elect`              | Enable leader election.                                                 | `true`                                   | `AUTO_VPA_LEADER_ELECT`              |
| `--log-encoder`               | Log format (`json`, `console`).                                         | `json`                                   | `AUTO_VPA_LOG_ENCODER`               |
| `--log-stacktrace-level`      | Stacktrace log level (`info`, `error`, `panic`).                        | `panic`                                  | `AUTO_VPA_LOG_STACKTRACE_LEVEL`      |
| `--log-devel`                 | Enable development mode logging.                                        | `false`                                  | `AUTO_VPA_LOG_DEVEL`                 |

\*) Variables are available in the template string: `.WorkloadName`, `.Namespace`, `.Kind`, `.Profile`.
See [Func hints](#func-hints) for template helper details.

### Labels and annotations

- Managed label (default) `autovpa.containeroo.ch/managed=true` marks VPAs the operator owns; override with `--managed-label`.
- Profile annotation (default) `autovpa.containeroo.ch/profile=<profile>` opts workloads in; override with `--profile-annotation`.
- Keys must be unique; the operator will refuse to start if managed/profile keys collide.

### Metrics and HTTP/2

- Metrics are enabled by default on `:8443` with TLS. Toggle with `--metrics-enabled`, `--metrics-bind-address`, `--metrics-secure`.
- HTTP/2 is disabled by default for compatibility; enable with `--enable-http2` if your ingress/stack requires it.

## Prometheus Metrics

AutoVPA exposes counters for the VPAs it creates, updates, or skips while reconciling workloads.

### Available Metrics

1. **VPAs Created**
   - **Metric:** `autovpa_vpa_created_total`
   - **Labels:** `namespace`, `name`, `kind`, `profile`
2. **VPAs Updated**
   - **Metric:** `autovpa_vpa_updated_total`
   - **Labels:** `namespace`, `name`, `kind`, `profile`
3. **Workloads Skipped**
   - **Metric:** `autovpa_vpa_skipped_total`
   - **Labels:** `namespace`, `name`, `kind`, `reason`
4. **Managed VPAs Deleted (cleanup)**
   - **Metrics:** `autovpa_vpa_deleted_obsolete_total`, `autovpa_vpa_deleted_opt_out_total`, `autovpa_vpa_deleted_workload_gone_total`, `autovpa_vpa_deleted_owner_gone_total`, `autovpa_vpa_deleted_orphaned_total`
   - **Labels:** `namespace`, `kind` (or just `namespace` for orphaned)
5. **Managed VPA Inventory**
   - **Metric:** `autovpa_managed_vpa`
   - **Labels:** `namespace`, `profile`
6. **Reconcile Errors**
   - **Metric:** `autovpa_reconcile_errors_total`
   - **Labels:** `controller`, `kind`, `reason`

Alerts for missing metrics and skip spikes are provided in `deploy/kubernetes/manifests/prometheusrule.yaml` and the Helm chart.

## Running locally

```bash
GOCACHE=$(pwd)/.cache/go-build go run ./cmd/main.go \
  --config=deploy/kubernetes/manifests/config.yaml \
```

## Testing

- Unit tests: `GOCACHE=$(pwd)/.cache/go-build go test ./...` (or `make test` for fmt/vet/envtest + unit tests).
- E2E: `make e2e` (uses an existing cluster; see `make kind`/`make delete-kind` for local Kind helper). Scope with `make e2e-generic`, `make e2e-namespaced`, or `make e2e-vpa`.
- Lint: `make lint` or `make lint-fix`.

## Troubleshooting

- **VPA CRD missing**: startup fails unless `--disable-crd-check` is set. Install the VPA CRD or add the flag for environments where the CRD is not present yet.
- **Annotation missing / profile not found**: AutoVPA logs and emits events but does not requeue aggressively. Add the profile annotation or fix the profile name in your config.
- **Invalid name template**: the operator validates templates at startup; fix the template string or profile override before redeploying.

## License

This project is licensed under the Apache 2.0 License. See the [LICENSE](LICENSE) file for details.
