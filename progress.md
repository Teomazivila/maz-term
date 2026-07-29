# maz-term - Implementation Progress

This is the single authoritative status document. `PRD.md` holds the
specification; `execution-plan.md`, `implementation-plan.md` and `next-tasks.md`
are historical planning records and should not be read as current state — they
contradicted each other and this file.

Every ✅ below has been verified by a test or by running the binary. Nothing is
marked complete on the strength of the code merely existing.

**Last verified**: 2026-07-29, commit range `357631c..HEAD`.

## Verified working

### Core
- [x] TermUI application that renders. Verified end-to-end against a real pty.
- [x] Configuration loading with validation, environment overrides and rejection
      of unknown keys and of credentials in files.
- [x] CLI flags: `-config`, `-data-path`, `-log-file`, `-no-storage`, `-debug`,
      `-version`.
- [x] Tab navigation and keyboard handling; in-app help generated from the
      bindings that exist.
- [x] Clean shutdown on `q`, `Ctrl-C`, SIGINT and SIGTERM, with collectors and
      the database closed in order.
- [x] Logs written to a file, never to the terminal.

### Data collection
- [x] System metrics: CPU (overall and per core), memory, swap, disk per mount,
      network per interface. Verified recording real values.
- [x] Top processes by CPU, sampled on an independent cadence so the sweep
      cannot stall the refresh cycle.
- [x] HTTP endpoint checks with status, response time, transport errors and a
      rolling availability percentage.
- [x] Git status: branch, real branch list, commit count, unpushed commits,
      tracked modifications and untracked files counted separately, individual
      changed files, and commit history.
- [ ] Cloud provider integrations — **not implemented**
- [ ] Kubernetes metrics — **not implemented**
- [ ] CI/CD pipeline status — **not implemented**

### Storage
- [x] SQLite persistence via a pure-Go driver, so the binary needs no cgo.
- [x] Metrics history for system, disk, HTTP and Git.
- [x] Event annotations, actually persisted, with tags that round-trip.
- [x] Notifications with SQL-level filtering by source and severity.
- [x] Retention policy that deletes only rows past the cutoff.
- [x] CSV export that reports read failures instead of writing nothing.
- [x] No demo or seeded data. An empty database renders an empty history.

### UI
- [x] CPU and memory gauges, CPU sparkline, disk bar chart, process table.
- [x] HTTP endpoint table with real availability, response-time and availability
      sparklines, per-endpoint detail.
- [x] Git summary, changed-file list, commit table, branch list.
- [x] History plots with selectable ranges (1h → 30d) driven by one table that
      the legend and key handler both read.
- [x] Annotation overlay and a form whose input accumulates.
- [x] Notification list, detail view and an interactive filter picker.
- [x] Plugin list and detail view.
- [x] Colour coding via termui's own styling, not markup from another library.
- [x] Graceful behaviour in a terminal too small to lay out.

### Plugins
- [x] Manager with panic containment and per-plugin collection timeouts.
- [x] Digest-verified loading confined to the plugin directory, refusing
      group- or world-writable files.
- [x] Hot reload via an event queue drained by the render goroutine.
- [x] Loading is opt-in at build time; the default binary stays portable.

### Quality gates
- [x] `go build ./...` and the plugin-tagged build both succeed.
- [x] Tests pass under `-race`.
- [x] `golangci-lint` configured and wired into `make lint` and CI.
- [x] CI: build, vet, formatting, race tests with coverage, lint, govulncheck,
      a six-platform cgo-free cross-compile matrix, and an AI-attribution check.
- [x] Cross-compilation verified for linux, darwin and windows on amd64/arm64.

## Known gaps

| Gap | Notes |
|---|---|
| Cloud, Kubernetes, CI/CD collectors | Configuration keys are parsed and validated; nothing reads them. The previous simulated collectors were removed rather than left to imply they worked. |
| UI test coverage 38.6% | Layout, frame assembly, key handling and range selection are covered. The per-tab data-population functions are not. |
| `cmd/maz-term` coverage 0% | Wiring only; exercised end-to-end rather than by unit tests. |
| Alert thresholds | `expected_status` and `timeout` are enforced; `AlertConfig` thresholds from the PRD are not evaluated. |
| Command palette | Not implemented (PRD §2.1). |
| Metric comparison view | Not implemented. Earlier documents claimed it complete; only a mode flag existed. |
| Per-endpoint intervals | Parsed and validated, but every collector shares `general.refresh`. |

## Corrections to earlier claims

Earlier revisions of this file marked the following complete. They were not, and
are now either implemented or listed above as gaps:

- "Historical data visualization" — nothing was persisted and the database was
  not durable; the History tab showed 800 seeded synthetic rows.
- "Data export functionality (CSV)" — the key handler reported "not yet
  implemented" while an exporter sat unused in the storage adapter.
- "Time range selection" — the keys moved an index without changing the window.
- "Event annotations storage" — add and delete were empty functions returning
  nil, and reads returned three hardcoded events.
- "Process table" — displayed one invented row; the model had no process field.
- "Notification filtering" — selections were discarded on every keystroke.
- "Interactive zoom" / "metric comparison" — state flags with no effect on any
  query. Zoom now narrows the window; comparison remains unimplemented.
- Colour coding — emitted another library's markup, which rendered as literal
  text.
