# Remediation Report

> ## Status as of 2026-07-29
>
> Branch `fix/remediation-critical-and-high`, commits `357631c..9c3b127` (14).
>
> | Severity | Found | Fixed | Outstanding |
> |---|---|---|---|
> | 🔴 Critical | 10 | 10 | 0 |
> | 🟠 High | 20 | 20 | 0 |
> | 🟡 Medium | 26 | 26 | 0 |
> | 🟢 Low | 8 | 8 | 0 |
>
> **All findings addressed.** C-009 closed last: the simulated collectors are
> replaced by real ones against the AWS, Kubernetes and GitHub Actions APIs, each
> read-only, off by default, and reporting the reason when a provider cannot be
> reached. See docs/adr/0001-infrastructure-integrations.md.
>
> Six further defects were found during remediation that this report did not
> record, each now fixed and covered by a test:
>
> 1. System metrics were never recorded on macOS. gopsutil v3 needs cgo for CPU
>    times on darwin and the code treated the resulting error as fatal for the
>    whole sample. Tests missed it because `go test` enables cgo while the shipped
>    binary does not. Fixed by moving to gopsutil v4 and making secondary readings
>    best-effort.
> 2. Process enumeration ran on the collection path and outlasted several refresh
>    intervals, so `Collect` never returned and nothing was stored at all.
> 3. `GetFilteredNotifications` built its SQL `IN` clause by interpolating values,
>    a genuine injection vector from plugin-writable notification rows. The
>    original report described only the redundant Go-side filtering.
> 4. `StoreHTTPMetrics` stamped rows with an unvalidated `LastChecked`, so a zero
>    value became a year-1 timestamp invisible to every time-window query.
> 5. `SaveConfig` serialised Go field names rather than the configured keys, so no
>    saved configuration could be read back.
> 6. Decoding into a pre-populated defaults struct merged slices, so a layout or
>    endpoint list in the file was appended to the defaults instead of replacing
>    them.
>
> Verification: `go build ./...` and the plugin-tagged build pass; tests pass
> under `-race` (collector 88.1%, plugins 91.8%, storage 74.8%, config 65.7%,
> ui 41.8%); `go vet` and `gofmt` clean; all six release targets cross-compile
> cgo-free; and the binary was run against a real pty, recording real CPU, memory,
> disk, HTTP and Git samples with zero fabricated rows and no warnings.
>
> ### Remaining work (beyond the review's scope)
>
> These are unbuilt features rather than defects; see progress.md "Known gaps".
>
> 1. **UI coverage** is 39.0%. Frame assembly, layout, key handling, the range
>    selector and the provider tabs are covered; the per-tab data-population
>    functions are not.
> 2. **Alert thresholds** from PRD §2.2 are parsed but not evaluated.
> 3. **Command palette** (PRD §2.1) and the metric comparison view are not built.
> 4. **Per-resource infrastructure history.** Only aggregates are persisted for
>    the three providers; per-instance or per-pod series would need a schema
>    change (ADR-0001).

---

**Repository**: maz-term (DevOps Terminal Dashboard)
**Review Date**: 2026-07-29
**Reviewer**: adversarial review session (fresh context, no authorship bias)
**Commit reviewed**: `24a99b5` (main)
**Specification files reviewed**: `PRD.md`, `execution-plan.md`, `implementation-plan.md`, `progress.md`, `next-tasks.md`, `README.md`, `README-termui.md`, `config.yaml`
**Scope**: 10,427 LOC Go across `cmd/`, `internal/storage/`, `pkg/{collector,config,models,plugins,ui}`, `tests/`

---

## Executive Summary

The application **does not work at runtime**. Three independent defects each break it on their own:

1. `layoutTab()` — the function that positions and renders every dashboard widget — **is never called from anywhere**. The main loop calls `updateData()` and `updateLayout()`, neither of which invokes `ui.Render` on any dashboard widget. The program initialises termui, clears the screen, and shows **a blank terminal** until you quit.
2. `App.SetStorageProvider()` runs **before** `App.initCollectors()` creates the collectors, so the system/HTTP/git collectors are handed a `nil` storage provider. **No collected metric is ever written to the database.**
3. `main.go` builds a `storage.Config` without `DataPath`, so SQLite opens with an **empty filename** — an anonymous temporary database that is destroyed on close. Even if writes worked, history would not survive a restart.

The History tab appears to work only because `SeedDemoDataIfEmpty()` injects 100 fabricated data points per metric family into the database on first run. Combined with mock cloud/Kubernetes/CI-CD collectors that report **"Connected"** while backed by hardcoded fake EC2 instances and pods, the tool's failure mode is the worst possible one for monitoring software: **it displays invented infrastructure data as if it were real**.

`go build ./...` also fails outright (`pkg/plugins/sample`), test coverage is **0.0% on every source package** against a mandated 80%, there is **no CI pipeline** (`.github/` does not exist), and `golangci-lint` has never run because the Makefile's lint guard is silently broken.

Counts: **10 Critical, 20 High, 26 Medium, 8 Low.**

**Fix first:** C-001 (render path). Nothing else is observable until the UI draws.

---

## Findings by Severity

### 🔴 CRITICAL

#### C-001: Dashboard never renders — `layoutTab` is dead code
- **Category**: Correctness / Spec Gap
- **Location**: `pkg/ui/tabs.go:10` (defined, never called); `pkg/ui/app_core.go:131-139`; `pkg/ui/ui_update.go:768-792`
- **Description**: `grep` for `layoutTab` finds only its own definition. The `Run()` loop calls `updateData()` (builds widget state, renders nothing) and `updateLayout()` (calls `SetRect`/`Lock`/`Unlock` only — no `ui.Render`). The only reachable `ui.Render` calls are `plugins.go:26,88,223` (StatusBar, and only if a `plugins/` dir exists — it does not) and `handlers.go:333` / `notifications.go:291` (annotation form). All 344 lines of `tabs.go` — `layoutSystemTab`, `layoutHTTPTab`, `layoutGitTab`, `layoutHistoryTab`, `layoutNotificationsTab`, `layoutPluginsTab`, `layoutDefaultTab`, `layoutCloudTab`, `layoutKubernetesTab`, `layoutCICDTab` — are unreachable.
- **Expected**: Each tick renders the active tab's widgets, status bar, and tab bar.
- **Actual**: Blank terminal. Every UI feature marked ✅ in `progress.md` is invisible.
- **Fix**: In `updateLayout()`, after sizing: compute the content rect, call `a.layoutTab(a.Tabs[a.ActiveTabIndex], rect)`, then `ui.Render(a.StatusBar, a.TabBar)`; render `a.HelpPanel` when `ShowHelp`. Remove the double-render inside `layoutTab` (see M-021).
- **Effort**: S

#### C-002: Collectors get a nil storage provider — no metric is ever persisted
- **Category**: Correctness / Data Integrity
- **Location**: `cmd/maz-term/main.go:154-160`; `pkg/ui/app_core.go:113`, `151-182`, `185-202`
- **Description**: `main.go` does `ui.NewApp(cfg)` → `app.SetStorageProvider(storageAdapter)`. But `SystemCollector`, `HTTPCollector`, and `GitCollector` are only constructed inside `initCollectors()`, which runs later, from inside `Run()`. So all three `if a.XCollector != nil` guards in `SetStorageProvider` are false and the call is a no-op.
- **Expected**: Every collector writes its samples to SQLite.
- **Actual**: `BaseCollector.storage` stays nil; `StoreData` returns early at `collector.go:134`. The database only ever contains seeded demo rows.
- **Fix**: Move collector construction into `NewApp` (or pass the provider into `initCollectors`), and re-apply the provider after collectors exist. Add a startup assertion that logs at WARN if a collector has no storage provider while storage is enabled.
- **Effort**: S

#### C-003: SQLite opens an anonymous temporary database — all history lost on exit
- **Category**: Data Integrity
- **Location**: `cmd/maz-term/main.go:132-137`; `internal/storage/db.go:57-70`
- **Description**: `main.go` constructs `&storage.Config{RetentionPeriod: …, Logger: …}` — `DataPath` is the zero value `""`. `New()` only falls back to `DefaultConfig()` when the whole config is nil, so `~/.config/maz-term/data.db` is never used. The DSN becomes `"?_journal_mode=WAL&…"`, i.e. an empty filename, which SQLite treats as a private temporary database deleted on close.
- **Expected**: Persistent DB at `~/.config/maz-term/data.db` (as `storage.DefaultConfig()` intends).
- **Actual**: Fresh empty DB every launch → reseeded with demo data every launch → "historical data" is always fake.
- **Fix**: Set `DataPath` in `main.go`, or in `New()` default any empty field individually rather than only handling `config == nil`. Add a `--data-path` flag. Log the resolved path at startup.
- **Effort**: S

#### C-004: Fabricated demo data written into the real metrics database
- **Category**: Data Integrity / Rule Violation
- **Location**: `internal/storage/db.go:351-485`; called unconditionally at `cmd/maz-term/main.go:146`
- **Description**: `SeedDemoDataIfEmpty()` inserts 100 synthetic system-metric rows, 300 disk rows, 300 HTTP rows (with `rand`-generated uptime), and 100 git rows into the same tables that hold real samples, with no marker column distinguishing them. The History tab, CSV export, and retention cleanup all treat them as genuine.
- **Expected**: An empty database shows an empty history.
- **Actual**: An operator reading the History tab or an exported CSV sees invented CPU/memory/disk/availability curves and cannot tell them from measurements. Violates `Development_Rules` §2.1 (no sample/dummy data) and §9.1.
- **Fix**: Delete the function and its call site. If a demo mode is wanted, gate it behind an explicit `--demo` flag, write to a separate DB file, and label it in the UI.
- **Effort**: S

#### C-005: Pressing `q` hangs the process
- **Category**: Correctness (lifecycle)
- **Location**: `cmd/maz-term/main.go:77-91`; `pkg/ui/handlers.go:62-63`; `pkg/ui/app_core.go:131`
- **Description**: `q` sets `a.Running = false`; the loop exits; `Run()` returns **nil**. `main.go` only calls `cancel()` when `app.Run()` returns a non-nil error. With nil, nothing cancels `ctx`, so the `select` on `sigChan`/`ctx.Done()` blocks forever.
- **Expected**: `q` exits cleanly.
- **Actual**: `defer ui.Close()` restores the terminal, the UI vanishes, and the process hangs until the user sends SIGINT.
- **Fix**: Call `cancel()` unconditionally after `app.Run()` returns, and log the error only when non-nil.
- **Effort**: S

#### C-006: `PluginManager.ScanDirectory` deadlocks on the first plugin found
- **Category**: Correctness (concurrency)
- **Location**: `pkg/plugins/plugin.go:302-304` → `331` → `428`
- **Description**: `ScanDirectory` acquires `pm.mutex.Lock()`, then calls `pm.LoadPlugin(path)`, which acquires `pm.mutex.Lock()` again. Go's `sync.RWMutex` is not reentrant → permanent deadlock as soon as the walk encounters any `.so` file.
- **Expected**: Scan loads each plugin found.
- **Actual**: Process hangs. (Currently latent only because no `plugins/` directory ships and `LoadEnabledPlugins` — which does not hold the lock — is the path actually called.)
- **Fix**: Extract an unexported `loadPluginLocked(path)` that assumes the lock is held, and have both public entry points call it; or release the lock before loading.
- **Effort**: S

#### C-007: Annotation form and notification filters can never accumulate input
- **Category**: Correctness / Spec Gap
- **Location**: `pkg/ui/handlers.go:263-339` and `342-420`
- **Description**: Both handlers declare `staticValues := struct{…}{}` as a **local** variable, commented "Static variables to hold the form values". It is re-zeroed on every keystroke. In the annotation form, `title` can never exceed one character and `currentField` never persists past 0, so `submitAnnotation` is unreachable — `<Enter>` just re-prints "Enter annotation description:" forever. In the filter handler, `<Space>` toggles are discarded, so `<Enter>` always applies empty filters. `handlers.go:418` is a literal placeholder: `// Your UI update logic goes here...`.
- **Expected**: `progress.md` claims "Created an annotation form UI for adding new events ✅" and `implementation-plan.md` claims notification filtering complete ✅.
- **Actual**: Both features are 100% non-functional.
- **Fix**: Promote the form state to `App` fields (`annotationDraft`, `filterDraft`) or a dedicated form struct. Implement the filter UI render block. Violates `Development_Rules` §2.1/§2.2.
- **Effort**: M

#### C-008: Annotation persistence is a silent no-op
- **Category**: Data Integrity
- **Location**: `internal/storage/db.go:488-499`; `GetEventAnnotations` at `296-348`
- **Description**: `AddEventAnnotation` and `DeleteEventAnnotation` have empty bodies that `return nil`. `GetEventAnnotations` returns three hardcoded fake events ("System Restart", "Deployed version 1.2.3", "High CPU Alert"). `initSchema` never creates an annotations table.
- **Expected**: `progress.md`: "Event annotations storage ✅". `next-tasks.md`: annotation model + storage complete.
- **Actual**: The UI reports success and discards the annotation. The chart always shows the same three invented events.
- **Fix**: Add an `annotations` table in `initSchema` (id TEXT PRIMARY KEY, timestamp INTEGER, title, description, type, severity, source, tags) with a timestamp index; implement real INSERT/DELETE/SELECT. Return an error from the stubs in the interim rather than nil.
- **Effort**: M

#### C-009: Cloud / Kubernetes / CI-CD tabs report "Connected" while backed by hardcoded fakes
- **Category**: Correctness / Rule Violation
- **Location**: `cmd/maz-term/main.go:169-215`; `pkg/collector/mock_collector.go:18-265`; `pkg/ui/ui_update.go:525-536`, `591-602`, `656-667`
- **Description**: `main.go` starts only `NewMockCloudCollector`, `NewMockKubernetesCollector`, `NewMockCICDCollector`. The mocks return fixed fabricated inventories (`i-0123456789abcdef0` / `web-server-1` / `t3.medium`, S3 buckets, pods, workflow runs). The UI then sets the panel text to `"Cloud Provider: Connected"` / `"Cluster: Connected"` / `"CI/CD Provider: Connected"` with the comment "the actual implementation would parse cloudMetrics … For now, we show a connected state". The "real" `aws_collector.go` (354 LOC), `kubernetes_collector.go` (578 LOC) and `github_actions_collector.go` (341 LOC) are *also* simulators ("we would use the AWS SDK…") and are **never constructed anywhere** — `NewAWSCollector`/`NewKubernetesCollector` are not called; `NewGitHubActionsCollector` only exists as a definition.
- **Expected**: PRD §2.1 real cloud/K8s/CI-CD monitoring; or an honest "not configured" state.
- **Actual**: The dashboard asserts connectivity to infrastructure it has never contacted. For an incident-response tool this is actively dangerous.
- **Fix**: Delete the mocks from the production path. Show "Not configured" unless a real client is wired. Either implement the AWS SDK / client-go / GitHub API calls, or remove the tabs and the 1,273 LOC of simulator code and mark the feature unbuilt in the docs.
- **Effort**: L

#### C-010: `go build ./...` fails
- **Category**: Build
- **Location**: `pkg/plugins/sample/sample.go:1`
- **Description**: `package main` with a `NewPlugin()` export but no `func main()` → `runtime.main_main·f: function main is undeclared in the main package`. `make build` masks this because it only builds `./cmd/maz-term`.
- **Fix**: Build the sample with `-buildmode=plugin` from a Makefile target and exclude it from the default build (e.g. a `//go:build plugin` tag), or add a stub `func main() {}`.
- **Effort**: S

---

### 🟠 HIGH

#### H-001: The time-range selector does nothing
- **Location**: `pkg/ui/ui_update.go:802-811`; `pkg/ui/handlers.go:116-127`
- `[`/`]` adjust `HistoryRangeIdx` then call `updateHistoryRange()`, which **never assigns `a.HistoryRange` from `historyRangeOptions[a.HistoryRangeIdx]`** — it only rewrites the status bar and contains an empty stub ("Would update data based on new range"). `HistoryRange` stays 24h forever. Contradicts `progress.md` "Time Range Selection ✅".
- **Fix**: `a.HistoryRange = historyRangeOptions[a.HistoryRangeIdx].value`, then `a.updateData()`. **Effort**: S

#### H-002: Three-way disagreement on time-range options and keys
- **Location**: `pkg/ui/ui_update.go:13-23` vs `707`/`762` vs `pkg/ui/handlers.go:80-85`; `pkg/ui/app.go:156` vs `pkg/ui/app_core.go:23`
- `historyRangeOptions` has 6 entries (1h, 6h, 12h, 24h, 3d, 7d). The panel text advertises 7 (1h, 6h, 24h, 3d, 7d, **30d, 90d** — the last two do not exist, 12h is missing) and tells the user to "Press 1-7", but `1`-`9` are bound to **tab switching**. `NewApp` sets `HistoryRangeIdx = 3` ("Default index for 24 hours"); `createUI` then overwrites it with `2` ("Index for 24h") — index 2 is 12h. **Fix**: single source of truth; derive panel text from the options slice; reconcile the index. **Effort**: S

#### H-003: `exportData()` is a stub while the real exporter sits unused
- **Location**: `pkg/ui/handlers.go:537-539`; `internal/storage/adapter.go:164-285`
- Pressing `e` shows "Export functionality not yet implemented". A complete CSV exporter exists in the adapter and is never called. `progress.md` claims "Data export functionality (CSV format) ✅" and "Added export keyboard shortcut ('e') ✅". **Fix**: call `a.Storage`'s exporter (widen `StorageInterface` with `ExportData`), set `ExportInProgress`, report the output path. **Effort**: S

#### H-004: Data race and "send on closed channel" panic in the git collector
- **Location**: `pkg/collector/git_status.go:60-67` vs `83-102`
- `Collect` iterates `c.subscription` **without holding `c.mutex`**, while `Subscribe` appends and `Unsubscribe` closes the channel under the lock. Concurrent slice access is a race; a close racing an in-flight send panics the process. **Fix**: snapshot subscribers under `RLock`; make the receiver own the close, or guard with a per-subscriber context as the HTTP collector attempts. **Effort**: S

#### H-005: Same panic class in the HTTP collector
- **Location**: `pkg/collector/http_health.go:96-124`, `127-140`, `171-180`, `216-225`
- `notifySubscribers` snapshots subscribers, releases `subMutex`, then sends in goroutines. `Unsubscribe` and `cleanupAllSubscribers` can `close(sub.ch)` concurrently → send on closed channel → panic. **Fix**: never close from the sender side; signal via `sub.cancel()` and let the receiver drain, or hold the lock across the send with buffered channels. **Effort**: M

#### H-006: Plugin watcher: unsynchronised `pm.watcher` access
- **Location**: `pkg/plugins/plugin.go:169` and `195` vs `142-145`
- `watchPlugins` reads `pm.watcher.Events`/`pm.watcher.Errors` with no lock while `StopWatcher` sets `pm.watcher = nil` → data race and nil dereference. `StopWatcher` also uses `time.Sleep(100ms)` **while holding the mutex** as a substitute for synchronisation. **Fix**: capture the channels once before the loop; use a `sync.WaitGroup` to join the goroutine. **Effort**: S

#### H-007: Plugin change callback mutates UI state from the watcher goroutine
- **Location**: `pkg/ui/plugins.go:32-53`
- The fsnotify callback writes `a.StatusBar.Text`, reads `a.Tabs[a.ActiveTabIndex]`, and calls `a.updatePluginsTabData()` + `a.updateLayout()` — concurrently with the UI goroutine doing the same. termui widgets are not goroutine-safe for concurrent mutation. **Fix**: post an event onto a channel consumed by the `Run` loop. **Effort**: M

#### H-008: Unverified native plugin loading from a CWD-relative directory
- **Location**: `pkg/plugins/plugin.go:402-434`; `pkg/ui/plugins.go:17-20`; `config.yaml:66-67`
- `plugin.Open` executes arbitrary native code with **no signature check, no checksum, no path allowlist**, from the relative path `plugins` resolved against the process CWD, plus hot-reload on any file write. Running `maz-term` inside an untrusted directory yields arbitrary code execution in a process that (per the PRD) is meant to hold cloud and Kubernetes credentials. There is also no `recover()` around plugin calls, so a panicking plugin kills the dashboard, and `Plugin.Collect()` takes no `context` and is called while holding `pm.mutex` (`plugin.go:498-510`) so one slow plugin blocks the manager.
- **Fix**: resolve the plugin dir to an absolute path under the user's config dir; require an explicit allowlist with expected SHA-256 per plugin; refuse world-writable directories; wrap all plugin invocations in `recover()` and a timeout; add `ctx` to the interface. **Effort**: L

#### H-009: Command injection / unsafe scheme in `openURL`
- **Location**: `pkg/ui/handlers.go:440-463`
- A notification's `ActionURL` (from the DB, writable by any plugin) is passed unvalidated to `exec.Command("cmd", "/c", "start", url)` on Windows — `cmd.exe` metacharacters (`&`, `|`, `"`) allow command injection — and to `open`/`xdg-open` elsewhere, which will happily act on `file://` and other schemes (`open` can launch applications). **Fix**: `url.Parse` and allow only `http`/`https` with a non-empty host; reject everything else; on Windows use `rundll32 url.dll,FileProtocolHandler` or `exec.Command("cmd", "/c", "start", "", url)` with validation. **Effort**: S

#### H-010: Every cross-compile target is broken by `CGO_ENABLED=0`
- **Location**: `Makefile:73-89`
- `build-linux`, `build-macos`, `build-windows` (and the three `build-termui-*` variants) set `CGO_ENABLED=0`, but `internal/storage/db.go` imports `mattn/go-sqlite3`, which **requires CGO**. The produced binaries compile and then fail at runtime with `sql: unknown driver "sqlite3"`. Go plugins also require CGO. This breaks PRD §8.2 (single-binary cross-platform distribution).
- **Fix**: switch to a pure-Go driver (`modernc.org/sqlite`) and keep `CGO_ENABLED=0`, or set `CGO_ENABLED=1` with per-platform toolchains. The pure-Go driver is the better fit for the distribution goal. **Effort**: M

#### H-011: Makefile targets point at a non-existent package
- **Location**: `Makefile:10`, `34-35`, `37`, `52-54`, `68-70`, `82-89`; `README.md:64-72`
- `TERMUI_MAIN_PATH=./cmd/maz-term-ui` does not exist. `make build-termui`, `run-termui`, `build-all`, `install-termui` and three cross-compile targets all fail. The README instructs users to run `make run-termui` as the primary start command. **Fix**: delete the dual-binary targets (the TermUI implementation *is* `cmd/maz-term` now) and fix the README. **Effort**: S

#### H-012: `golangci-lint` has never run — the guard is silently always-false
- **Location**: `Makefile:56-62`
- `@if [ -x "$(command -v $(GOLINT))" ]` — `$(command -v golangci-lint)` is expanded by **make**, not the shell. It is an undefined make variable, expands to empty, so the test is `[ -x "" ]` → always false → always prints "golangci-lint is not installed. Skipping lint." Consistent with the absence of any `.golangci.yml`. **Fix**: `$$(command -v $(GOLINT))`, add a `.golangci.yml` (enable `errcheck`, `govet`, `staticcheck`, `revive`, `bodyclose`, `contextcheck`, `gosec`), and fix the fallout. **Effort**: M

#### H-013: 0.0% test coverage and no CI
- **Location**: `tests/` (4 files, 379 LOC); `.github/` absent
- `go test -cover ./...` reports **0.0% for every source package**; only the external `tests` package runs, and it reports "[no statements]". The tests are `assert.NotNil` smoke checks — `TestAppCreation` asserts the app is non-nil, `TestConfiguration` asserts the hardcoded default endpoints exist, `TestUIUtilities` tests termui's own `TerminalDimensions`. `TestCreateDefaultConfig` passes only because the CWD (`tests/`) happens to contain no `config.yaml`; run from the repo root it would load the real config and fail its `len(Layout) == 4` assertion. Violates `Development_Rules` §6.1 (80% minimum) and §6.4 (deterministic, behaviour-verifying tests). `implementation-plan.md` §7 ("Set up CI pipeline for testing") is unstarted despite commit `759ee0e "fix: Fixed workflows for deployment"`.
- **Fix**: table-driven unit tests for `config` (duration hook, env override, `SaveConfig` paths), `storage` (schema, insert/query round-trip, retention cutoff, filtered notifications) against a temp-file DB, and `collector` (git parsing with a fixture repo, HTTP checks against `httptest`). Add a GitHub Actions workflow running `go build ./...`, `go vet`, `go test -race -cover`, `golangci-lint`, `govulncheck`. **Effort**: L

#### H-014: Half of `config.yaml` is silently ignored
- **Location**: `pkg/config/config.go:29-36` vs `config.yaml`
- `AWSConfig`, `GitHubConfig`, `KubernetesConfig` are declared but **are not fields of `Config`**. There is no `Cloud`, `Kubernetes`, or `CICD` field, so the entire `cloud:`, `kubernetes:` and `cicd:` blocks in the shipped `config.yaml` (lines 75-95) decode into nothing. `LayoutTab` has `Name` + `Panels`, but the shipped config nests panels under `rows: [{size, panels}]` — so every tab parses with an **empty** `Panels` slice. And `Config` carries both `Endpoints` (`endpoints:`) and `Metrics.Endpoints` (`metrics.endpoints:`), plus both `Git` (`git:`) and `Metrics.Git` (`metrics.git:`); `app_core.go:161`/`177` read `Config.Endpoints`/`Config.Git`, while the shipped config populates only `metrics.endpoints`/`metrics.git`. **Net effect: shipping `config.yaml` yields zero monitored endpoints, no git repo, empty panels, and no cloud/K8s/CI-CD config.** `WeaklyTypedInput: true` plus no unknown-key check hides all of it.
- **Fix**: one schema, one place. Delete the duplicate fields, model `rows`, wire the cloud/K8s/CI-CD structs into `Config`, set `ErrorUnused: true` on the decoder so stray keys fail loudly, and validate at startup (refresh > 0, valid URLs, existing repo paths). **Effort**: M

#### H-015: `~` in repository paths is never expanded
- **Location**: `pkg/collector/git_status.go:27-40`, `228`; `config.yaml:62`; `README.md:137`
- Go does not expand `~`. `cmd.Dir = "~/projects/main-service"` fails to chdir, so every git command errors, every error is swallowed (`if err == nil` guards only), and the Git tab silently shows zeros with a `0001-01-01` last-commit date. The shipped config and the README both use `~/...`, so the documented configuration can never work. **Fix**: expand `~` via `os.UserHomeDir()` at config load; return an error when the path is not a git repository instead of `return nil` (`git_status.go:139-141`). **Effort**: S

#### H-016: Environment-variable configuration does not work
- **Location**: `pkg/config/config.go:146-147`, `175`
- `v.AutomaticEnv()` values are **not included in `v.AllSettings()`**, and `AllSettings()` is the only thing decoded into the struct. `SetEnvPrefix` is also called *after* `AutomaticEnv`. PRD §3.3 ("Support for environment variables") and §8.3 ("Support for environment-based secrets") are therefore unimplemented. **Fix**: bind keys explicitly with `v.BindEnv` for each field, or unmarshal via `v.Unmarshal` with explicit `SetDefault` for every key. Add a test that sets `MAZTERM_GENERAL_REFRESH` and asserts it wins. **Effort**: M

#### H-017: Plaintext credential fields with no protection and a committed config file
- **Location**: `pkg/config/config.go:98-113`; `.gitignore`; `config.yaml` (tracked)
- `AWSConfig.AccessKeyID`, `AWSConfig.SecretAccessKey` and `GitHubConfig.Token` are plain YAML string fields. `config.yaml` is committed to the repository and `.gitignore` covers only `.env` — a user following the documented pattern commits their AWS keys. PRD §3.3 "Secure credential storage" and §8.3 "Credential encryption at rest" are unimplemented (`implementation-plan.md` §2 lists all three security items as unstarted).
- **Fix**: remove secret fields from the file schema entirely; read credentials only from the environment or the platform keychain / the provider's own credential chain (AWS profile, `KUBECONFIG`, `GITHUB_TOKEN`). Add `config.yaml` to `.gitignore`, ship `config.example.yaml`, and refuse to start if a secret-looking key is present in the file. **Effort**: M

#### H-018: Wrong colour-markup dialect — style tags render as literal garbage
- **Location**: `pkg/ui/ui_update.go:252-260`, `442`, `445`, `473`; `pkg/ui/notifications.go:119-127`, `208-216`
- The code emits `[green]200[-]` / `[yellow]M[-]` / `[red]CRITICAL[-]` — **tview/tcell** syntax. termui v3 uses `[text](fg:green)`, and `widgets.Table` does not parse inline styles at all (it uses `RowStyles`). Users would see the literal text `[green]200[-]`. Contradicts `next-tasks.md` "✅ Add status indicators with color coding" and "✅ Add color coding for modified files".
- **Fix**: use `RowStyles`/`Table.RowStyles` and `ui.NewStyle(...)` for tables; `[text](fg:red)` for `Paragraph`/`List`. **Effort**: S

#### H-019: `SaveConfig` panics on short paths
- **Location**: `pkg/config/config.go:271`
- `dir := filePath[:len(filePath)-len("/config.yaml")]` — for any path shorter than 12 characters (`cfg.yaml`) the index is negative → **slice bounds out of range panic**; for any path not ending in `/config.yaml` it silently truncates to a wrong directory. **Fix**: `filepath.Dir(filePath)`. **Effort**: S

#### H-020: Notification filtering silently drops matches
- **Location**: `internal/storage/adapter.go:376-423`
- Filtering is done in Go over the first `count*2` rows returned by the DB. With a rare severity and a busy notification table, matching rows beyond that window are never seen — the UI shows "no results" while matches exist. **Fix**: push the filter into SQL (`WHERE severity IN (…) AND source IN (…)`) with parameterised placeholders and `LIMIT count`. **Effort**: S

---

### 🟡 MEDIUM

- **M-001 License mismatch.** `LICENSE` is **Apache-2.0**; `README.md:5` badge and `README.md:213-215` both claim MIT. Legal ambiguity for any consumer. Pick one. (`README.md`, `LICENSE`)
- **M-002 README documents a removed framework.** README:13, 66-83 present Bubble Tea as "the default implementation" with `make run` for it, but commit `d364dee` removed all Bubble Tea dependencies and `go.mod` has none. `execution-plan.md:117-123` still lists "UI: BubbleTea/Lipgloss" and "Go 1.21+" (PRD says 1.24+; `go.mod` says 1.24.1). (`README.md`, `execution-plan.md`)
- **M-003 Placeholder URLs and a missing image in the README.** `yourusername/devops-terminal-dashboard` appears in the release badge, both install one-liners, and the clone command; `docs/images/dashboard_preview.png` does not exist. Violates `Development_Rules` §2.1 (no placeholder values). (`README.md:3-9, 34-47`)
- **M-004 Documented CLI command does not exist.** README:61 tells users to run `devops-dashboard init`; the binary is `maz-term` and has no `init` subcommand — only `-config`, `-version`, `-no-storage`, `-debug`. (`README.md`, `cmd/maz-term/main.go:31-35`)
- **M-005 Keybindings disagree three ways.** README:144-154 documents `c`=clear alerts, `f`=filter, `s`=save layout, `/`=search, `Ctrl+e`=export; the in-app help (`app.go:126-150`) says `h`=toggle help, `e`=export; the code binds `?`=help, `h`=**previous tab**, `c`=comparison mode, `e`=export-stub, and has no `s` or `/` at all. Pressing the documented help key `h` switches tabs. (`README.md`, `pkg/ui/app.go`, `pkg/ui/handlers.go`)
- **M-006 17 stdout/stderr writes corrupt the TUI.** `db.go:364,483,506,519,532` (the last three on a 5-second ticker), `plugin.go:199,221,228,284`, `plugins.go:41,66,74,78,85`, `handlers.go:459`. Any write to the terminal while termui owns the screen scrambles the display. Also mixes `fmt.Print*` and stdlib `log` with the project's `slog`. (`next-tasks.md` "Standardize logging approach" is still open.)
- **M-007 `StoreData` discards every storage error.** `pkg/collector/collector.go:139-152` — six `_ = storage.Store…()` calls. A DB that is closed, locked, or type-mismatched fails silently forever. Combined with M-008 this is how C-002 stayed invisible.
- **M-008 `interface{}`-based storage contract.** `StorageProvider` (`collector.go:10-17`) passes `interface{}` and re-asserts the concrete type in every adapter method (`adapter.go:27-96`). A pointer/value mismatch produces a runtime error that M-007 then swallows. Use concrete typed methods.
- **M-009 Hardcoded fake process row.** `ui_update.go:169-182` appends `{"1234","5.2%","128 MB","sample-process"}` with the comment "Add some sample data since TopProcesses field doesn't exist". `progress.md` claims "Process table ✅". `models.SystemMetrics` has no process list, and `system_metrics.go` never collects one.
- **M-010 Hardcoded fake branch list.** `ui_update.go:470-479` shows `main`, `develop`, `feature/new-ui` regardless of the repository's actual branches. No `git branch` call exists.
- **M-011 "Availability" column shows a boolean.** `ui_update.go:279` — `fmt.Sprintf("%v", endpoint.IsUp)` prints `true`/`false` under a header promising a percentage; marked `// Availability placeholder`.
- **M-012 "Changed Files" table lists no files.** `ui_update.go:436-448` emits one synthetic row ("N modified files") under a `Status | File` header. PRD §2.1 requires modified-file tracking; `git status --porcelain` output is counted then thrown away.
- **M-013 `ModifiedFiles` counts untracked files too.** `git_status.go:167-177` counts every porcelain line, including `??` entries, and labels the total "modified files".
- **M-014 `CombinedOutput` merges stderr into parsed values.** `git_status.go:231` — any git warning on stderr is concatenated into the string that is then `Sscanf`'d or line-counted. Use `cmd.Output()` and capture stderr separately.
- **M-015 Every git error is swallowed.** `git_status.go:144-190` uses `if err == nil` guards exclusively; `collectRepoData` always returns nil. Failures are indistinguishable from zero values.
- **M-016 `context.Background()` in `initCollectors`.** `app_core.go:152` — the three real collectors ignore main's cancellable context entirely, so shutdown never stops them; they keep writing after `Close()`. `go a.SystemCollector.Start(...)` also discards the returned error and wraps a function that already spawns its own goroutine.
- **M-017 Context stored in a struct.** `BaseCollector.ctx` (`collector.go:38`) and `Database.ctx` (`db.go:23`) — flagged as an anti-pattern by the Go skill; pass ctx per call.
- **M-018 No goroutine join on collector stop.** `BaseCollector.Stop` cancels but does not wait; each collector's ticker goroutine is unowned. `main.stopCollectors` "waits" on a WaitGroup that tracks only the `Stop()` calls themselves.
- **M-019 Unchecked type assertion on resize.** `handlers.go:72` — `e.Payload.(ui.Resize)` panics if the payload type differs. Use the comma-ok form.
- **M-020 Missing `return` after `handlePluginEvents`.** `handlers.go:56-58` — on the Plugins tab, events are handled by the plugin handler **and then** fall through to the global switch, so one keypress triggers two actions.
- **M-021 Widgets rendered twice per frame.** `tabs.go:41-44` renders every widget individually after each layout function has already rendered the grid containing them. This is the likely root cause of the "UI flickering" that commits `4e435e5`/`de0807e`/`dffe305` tried to fix. (Latent behind C-001.)
- **M-022 Unguarded widget indexing in every layout function.** `layoutTab` guards only `len(tab.Widgets) == 0`, then `layoutSystemTab` indexes `[0..4]`, `layoutHTTPTab`/`layoutGitTab` `[0..3]`, `layoutNotificationsTab` `[0..2]`, `layoutPluginsTab` `[0..1]`. Any tab with 1-3 widgets panics. `layoutHistoryTab` additionally branches on `len(tab.Plots)` while indexing `tab.Widgets` — two different slices — and `tab.Widgets[5%len(tab.Plots)]` renders widget 0 twice when there are exactly 5 plots. (Latent behind C-001.)
- **M-023 Layout dispatch keys on tab names the shipped config never produces.** `tabs.go:17-39` switches on `"System"`, `"HTTP"`, `"CI/CD"`…; `config.yaml` defines `"System Overview"`, `"Services"`, `"Git"`, `"History"`, `"Notifications"`, `"Plugins"`. With the shipped config, `updateSystemTabData`/`updateHTTPTabData` (which look up `getTabByName("System")`/`("HTTP")`) return early and **no system or HTTP widget is ever created**; the tabs fall through to `layoutDefaultTab`. The `Cloud`/`Kubernetes`/`CI-CD` tabs are never created by any code path, so their update functions are dead.
- **M-024 SQLite pool misconfigured for concurrent writers.** `db.go:49-51` sets `MaxOpenConns: 25` with no `_busy_timeout` in the DSN. Multiple collectors writing concurrently will hit `SQLITE_BUSY`; those errors are then swallowed by M-007. Set `MaxOpenConns(1)` for writes or add `_busy_timeout=5000`.
- **M-025 Default config phones home to the author's personal domains.** `config.go:306-311` — `https://celesta.io` and `https://teomazivila.com` are polled by default, contradicting PRD §8.3 "Local-only operation by default. No data sent to external services without explicit configuration." `config.yaml` additionally defaults `cloud.enabled: ["aws"]`, `kubernetes.enabled: true`, `cicd.enabled: ["github"]`.
- **M-026 `go mod tidy` never run.** `go.mod:26` marks `mattn/go-sqlite3` `// indirect` although `internal/storage/db.go:15` imports it directly.

Also in this band: two divergent default-config builders (`createDefaultConfig` 4 tabs / no endpoints vs `DefaultConfig` 3 tabs / 4 endpoints, `config.go:221-254` and `285-327`); `ExportData(options interface{})` with map-based option extraction instead of the `ExportOptions` struct that already exists; `ExportData` using `os.Getenv("HOME")` (empty on Windows → writes to a relative path) instead of `os.UserHomeDir()`; `exportTimeSeriesData` ignoring `writer.Flush()`/`file.Close()` errors → silent truncation; `sanitizeFileName` calling `filepath.Clean`+`Base` nine times inside the replacement loop; unbounded goroutine fan-out (one per endpoint) in `HTTPHealthChecker.Collect`; `time.NewTicker(interval)` panicking if any interval is 0 with no validation; `tx.Rollback()` errors ignored in `cleanupOldData`; missing composite index on `http_metrics(endpoint_name, timestamp)` and `git_metrics(repo_name, timestamp)` though both are queried that way; all nine `update*TabData` functions running every second regardless of the active tab (PRD §7 targets <5% idle CPU); `os.Stat` misuse at `plugins.go:23` (`!os.IsNotExist(err)` treats a permission error as "directory exists"); Go's `plugin` package being fundamentally incompatible with PRD §8.2's static cross-platform binary (no Windows support, requires CGO and byte-identical dependency versions).

---

### 🟢 LOW

- **L-001** `updateTabNames` (`ui_update.go:795-799`) is dead code; the same assignment happens inline in `updateLayout`.
- **L-002** `fmt.Sscanf` used where `strconv.Atoi` is clearer and error-checked (`git_status.go:153`).
- **L-003** Zero-value times render as `0001-01-01 00:00:00` in the Git panel and HTTP details.
- **L-004** `notifySubscribers` allocates a `time.After` timer per subscriber per collection cycle (`http_health.go:115`).
- **L-005** Local grids reallocated every frame in all ten layout functions.
- **L-006** `Subscribe` builds IDs from `time.Now().UnixNano()` + `len(subscribers)` read outside the lock (`http_health.go:155`).
- **L-007** The five planning documents contradict each other on the plugin system (`progress.md:73` "Missing" vs `implementation-plan.md:9-18` "✅" vs `next-tasks.md:9-15` partial) and on the notification centre (`progress.md:36` missing vs `:43` storage complete).
- **L-008** `CODE_OF_CONDUCT.md` and `CONTRIBUTING.md` exist while the README's contribution flow references `make test` (which passes vacuously) — no `DCO`/sign-off or review gate is defined.

---

## Specification Compliance Matrix

| PRD Requirement | Status | Notes |
|---|---|---|
| §2.1 System monitoring (CPU/mem/disk/net) | ⚠️ Partial | Collection is genuinely real (`gopsutil`); display never renders (C-001); no remote systems |
| §2.1 Service status / HTTP health checks | ⚠️ Partial | Checker works; UI not rendered; "Availability" is a boolean (M-011) |
| §2.1 CI/CD pipeline status | ❌ Missing | Simulator only, never constructed; UI claims "Connected" (C-009) |
| §2.1 Infrastructure / cloud overview | ❌ Missing | Same (C-009) |
| §2.1 Kubernetes monitor | ❌ Missing | Same (C-009); no `client-go` dependency |
| §2.1 Notification center | ⚠️ Partial | Storage is real SQL; filtering non-functional (C-007); list never rendered |
| §2.1 Command palette | ❌ Missing | No implementation |
| §2.2 Historical performance tracking | ❌ Broken | Nothing persisted (C-002), DB not durable (C-003), data is fabricated (C-004) |
| §2.2 User-defined alert thresholds | ❌ Missing | `AlertConfig` parsed, never evaluated |
| §3.1 SQLite cache / history | ⚠️ Partial | Schema and queries real; never written to; no annotations table |
| §3.1 Plugin system | ⚠️ Partial | Loader exists but deadlocks (C-006), unverified code execution (H-008), no Windows support |
| §3.3 YAML configuration | ⚠️ Partial | Loads, but half the documented schema is ignored (H-014) |
| §3.3 Environment variable support | ❌ Missing | `AllSettings()` excludes env values (H-016) |
| §3.3 / §8.3 Secure credential storage | ❌ Missing | Plaintext fields in a committed file (H-017) |
| §4.1 Tab navigation / status bar | ⚠️ Partial | State machine exists; nothing drawn (C-001) |
| §4.2 Sparklines / gauges / bar charts | ⚠️ Partial | Widgets constructed, never rendered |
| §4.2 Colour-coded status indicators | ❌ Broken | Wrong markup dialect (H-018) |
| §4.3 Keyboard shortcuts for all actions | ⚠️ Partial | Bindings contradict both docs (M-005); `1-7` range keys collide with tab switching (H-002) |
| §4.3 Context-sensitive help | ⚠️ Partial | Static panel with inaccurate content; never rendered |
| §5.1 MVP: export/share | ❌ Missing | UI stub; real exporter unused (H-003) |
| §5.1 MVP: customizable refresh intervals | ⚠️ Partial | Parsed; collector intervals hardcoded (2s/5s/5s) and ignore config |
| §7 Load <2s / refresh <500ms / idle CPU <5% | ⚠️ Unverified | No benchmarks; full re-query + full table rebuild every second |
| §8.1 Go 1.24+, Make, testify | ✅ | `go.mod` 1.24.1 |
| §8.2 Single binary, cross-platform | ❌ Broken | CGO/sqlite conflict (H-010); plugins exclude Windows |
| §8.4 Docs / install guides | ⚠️ Partial | Present but substantially inaccurate (M-002/003/004/005) |

## Execution Plan Compliance

| Plan step | Claimed | Actual |
|---|---|---|
| Phase 1 Setup | COMPLETED | ✅ but `go build ./...` fails (C-010), lint never runs (H-012) |
| Phase 2 Core UI | PARTIAL | Render path missing entirely (C-001) |
| Phase 2 Colour theme system | unchecked | `theme.go`/`styles.go` exist; wrong markup dialect (H-018) |
| Phase 3 Local metrics collector | ✅ | ✅ genuinely real |
| Phase 3 HTTP checker | ✅ | ✅ collection real; panic race (H-005) |
| Phase 3 Git status | ✅ | ⚠️ `~` unexpanded (H-015), errors swallowed (M-015), fake branches (M-010) |
| Phase 3 "Implement UI for HTTP / Git" | unchecked | Correctly unchecked — matches C-001/M-023 |
| Phase 4 Unit + integration tests | unchecked | Correctly unchecked — 0% coverage (H-013) |
| Phase 5 SQLite integration | PLANNED, but `next-tasks.md` marks all 6 sub-items ✅ | ⚠️ schema real, wiring broken (C-002/C-003) |
| Phase 6 Cloud / K8s / CI-CD / notifications | PLANNED | ❌ simulators (C-009); notifications partial |
| Phase 7 Plugin system | PLANNED, `implementation-plan.md` marks 3 of 4 ✅ | ⚠️ deadlock (C-006), unsafe loading (H-008) |
| `progress.md` "Recent Achievements" 1-5 | all ✅ | Zoom ⚠️ (state only), comparison ⚠️ (flag only), annotations ❌ (C-007/C-008), time ranges ❌ (H-001), export ❌ (H-003) |

## Test Coverage Gaps

| Area | Current | Recommended |
|---|---|---|
| `pkg/config` | 0.0% | Duration hook (`7d`,`1w`,`5s`, invalid); env override; `SaveConfig` short/odd paths; unknown-key rejection; `metrics.*` vs top-level precedence |
| `internal/storage` | 0.0% | Temp-file DB: schema init, insert→query round-trip per metric family, retention cutoff boundary, notification filter with >2×count rows, concurrent writer under `-race` |
| `pkg/collector` (git) | 0.0% | Fixture repo: branch, commit count, porcelain parsing, untracked vs modified, non-repo path, `~` path, upstream-missing |
| `pkg/collector` (http) | 0.0% | `httptest` server: 2xx/3xx/4xx/5xx, timeout, connection refused, `ExpectedStatus`, subscribe/unsubscribe under `-race` |
| `pkg/ui` | 0.0% | `updateLayout` renders (regression for C-001); layout functions with 1/2/3/5 widgets (M-022); annotation form accumulates input (C-007); range selector changes `HistoryRange` (H-001) |
| `pkg/plugins` | 0.0% | `ScanDirectory` with a `.so` present (regression for C-006); watcher start/stop under `-race`; panicking plugin is contained |

---

## Recommended Fix Order

**Gate 1 — make it run at all (all S, ~1 day)**
1. **C-001** render path — nothing else is observable without it.
2. **C-005** unconditional `cancel()` — so you can quit and iterate.
3. **C-010** sample-plugin build — so `go build ./...` and CI can pass.
4. **C-002** collector/storage wiring order.
5. **C-003** `DataPath`.
6. **C-004** delete demo seeding — do this *after* 2-5 so you can see whether real data now flows. This is the honesty gate.

**Gate 2 — stop lying to the operator (~1 day)**
7. **C-009** remove mock collectors from the production path; show "Not configured".
8. **C-008** real annotation storage, or surface an error.
9. **H-018** correct markup dialect.
10. **M-009 / M-010 / M-011 / M-012** remove every hardcoded fake row.

**Gate 3 — stop crashing (~1 day)**
11. **C-006** plugin deadlock; **C-007** form state.
12. **H-004 / H-005 / H-006 / H-007** the four channel/goroutine races.
13. **M-019 / M-020 / M-022** panic paths.

**Gate 4 — security (~1 day)**
14. **H-008** plugin trust boundary (or disable plugin loading until it is solved).
15. **H-009** URL validation.
16. **H-017** credentials out of the config file; `config.yaml` into `.gitignore`.
17. **M-025** local-only defaults.

**Gate 5 — make it shippable (~2-3 days)**
18. **H-012** lint config + fix fallout; **H-013** tests + CI.
19. **H-014 / H-016** one config schema, working env support, startup validation.
20. **H-010 / H-011** switch to `modernc.org/sqlite`, fix the Makefile.
21. **H-001 / H-002 / H-003** time ranges and export.
22. **M-006 / M-007 / M-008** structured logging (never to stdout while the TUI is up), no swallowed storage errors, typed storage contract.
23. **M-001 → M-005** reconcile every document with the code; delete or rewrite the five contradictory planning files into one.
