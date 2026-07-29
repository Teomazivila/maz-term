# ADR-0001: Real AWS, Kubernetes and GitHub Actions integrations

## Status

Accepted

## Date

2026-07-29

## Context

maz-term previously shipped collectors for AWS, Kubernetes and CI/CD that
returned hardcoded inventories (`i-0123456789abcdef0`, `web-server-1`) while the
UI displayed "Cloud Provider: Connected" and "Cluster: Connected". None of them
was constructed by the application. They were removed in `5a40042` because a
monitoring tool that invents infrastructure state is worse than one that omits
it.

This ADR covers replacing them with real implementations.

Forces in play:

- **Distribution.** maz-term is one static binary, cross-compiled for six
  platform/architecture pairs with `CGO_ENABLED=0` (ADR context: this is enforced
  by CI). Any dependency must be pure Go.
- **Credentials.** The product is specified to operate locally and to send
  nothing anywhere without explicit configuration (PRD §8.3). Credentials must
  never live in the configuration file; a file containing them is already
  rejected at startup.
- **Honesty.** The failure mode that motivated this work was fabricated data. An
  unreachable provider must be visibly unreachable.
- **Scope.** These are read-only dashboard views, not a control plane. No
  mutating API calls.
- **Local tool.** There is no server, no shared database, and one operator. The
  operational budget for schema and dependencies is small.

## Options considered

### Dependencies for AWS

**Option A: `aws-sdk-go-v2` (chosen).** Official, modular, pure Go. Only
`config`, `ec2`, `s3`, `rds` and `cloudwatch` are imported rather than a
monolithic SDK. Credential resolution via the default chain, which already
handles environment variables, shared profiles, SSO and instance metadata.

- Operational cost: low. Credential handling is the risky part and it is solved.

**Option B: hand-rolled REST with SigV4 signing.** No dependency.

- Rejected. SigV4 is intricate and a signing bug fails closed in confusing ways.
  This is commodity functionality; the build-vs-integrate rule says integrate.

### Dependencies for Kubernetes

**Option A: `k8s.io/client-go` (chosen).** Official, pure Go.

- Operational cost: medium. It is a heavy module tree.
- Justified because kubeconfig handling is not simply "read a YAML file": it
  covers client certificates, bearer tokens, OIDC, and `exec` credential plugins
  (which is how EKS, GKE and AKS authenticate in practice). Reimplementing that
  would be both large and a security-sensitive place to be wrong.

**Option B: raw REST against the API server.** Rejected for the reason above.

### Dependencies for GitHub Actions

**Option A: plain `net/http` plus `encoding/json` (chosen).** No new dependency.

- Two read-only endpoints are needed: list workflows and list workflow runs.
  Both are documented JSON over HTTPS with a bearer token.
- The repository already has an HTTP client idiom in `http_health.go`, and rate
  limits are surfaced explicitly from the `X-RateLimit-*` headers, which is
  useful to display rather than hide.
- Fully testable against `httptest`.

**Option B: `go-github`.** Rejected. It is a good library, but it is a
dependency earning its place across two endpoints, and the guidance against
importing a library for one function applies. Revisit if the surface grows
beyond a handful of endpoints or if pagination semantics get complicated.

### Persistence shape

**Option A: summary time series per provider (chosen).** Three narrow tables
holding counts and aggregates: instances by status and mean CPU; pods by phase,
nodes ready; workflow success rate and running runs. Detailed current state
(every instance, pod and run) is held in memory for the tabs and not persisted.

- The History tab plots time series; the provider tabs show current state. This
  is exactly the split the UI needs.
- Retention stays cheap and the schema stays legible.

**Option B: fully normalised tables for every resource.** Rejected. It would
multiply the schema and the retention cost to store per-instance CloudWatch
datapoints that nothing plots, in a local single-operator tool.

**Option C: no persistence at all.** Rejected. Then the History tab could not
show infrastructure trends, which is a stated requirement (PRD §2.2).

## Decision

Implement three collectors in `pkg/collector`, on the existing `BaseCollector`
lifecycle:

1. `AWSCollector` using `aws-sdk-go-v2` — EC2 instances, S3 buckets, RDS
   instances, with CloudWatch supplying utilisation.
2. `KubernetesCollector` using `client-go` — nodes, pods, deployments, services,
   with the metrics API used when available.
3. `GitHubActionsCollector` using `net/http` — workflows and recent runs.

Each one:

- Is constructed only when its configuration block enables it. Defaults are off.
- Resolves credentials outside the configuration file: the AWS default chain,
  kubeconfig (or in-cluster service account), and `MAZTERM_GITHUB_TOKEN` or
  `GITHUB_TOKEN`.
- Records failures on the metrics struct's existing `Errors` field, which the UI
  renders verbatim. A provider that cannot be reached shows the reason. No code
  path reports a connected or healthy state that was not observed.
- Makes read-only calls only.
- Persists a summary sample per cycle; detailed state stays in memory.

Extend `pkg/collector` rather than creating a new package: same deployment
lifecycle, same ownership, same data model, and they share the collector
lifecycle and storage interface.

## Consequences

### Positive

- The three tabs show real infrastructure, or say why they cannot.
- Configuration keys that were parsed and ignored now do something.
- Credentials stay out of files by construction.
- The dependency additions are all pure Go, so the cgo-free six-platform
  distribution is preserved.

### Negative

- `client-go` adds a large module tree and lengthens builds noticeably.
- Three more moving parts to keep working against evolving remote APIs.
- Summary-only persistence means per-resource history is unavailable; adding it
  later is a schema change.

### Risks

- **API cost and rate limits.** Mitigated by provider-specific refresh intervals
  defaulting to minutes rather than seconds, bounded result counts, and
  surfacing GitHub's remaining rate limit in the UI.
- **Slow or hanging remote calls stalling a collection cycle.** Mitigated by a
  per-collector timeout shorter than the refresh interval, the same shape as the
  fix applied to process enumeration.
- **`client-go` version skew against older clusters.** Mitigated by using only
  long-stable core and apps/v1 APIs, and treating the optional metrics API as
  best-effort.
- **Credential misuse.** Mitigated by read-only calls, and by the existing
  startup rejection of credential keys in configuration files.

## Affected components

`pkg/collector`, `pkg/config` (already prepared), `internal/storage` (three new
tables), `pkg/ui` (three tabs), `cmd/maz-term` (wiring).
