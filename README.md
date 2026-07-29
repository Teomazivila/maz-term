# maz-term

![Go version](https://img.shields.io/badge/Go-1.26%2B-blue?style=flat-square)
![License](https://img.shields.io/badge/License-Apache%202.0-green?style=flat-square)

A terminal dashboard for local system, HTTP endpoint and Git repository metrics.
One static binary, no agent, no daemon, no account.

**Everything on screen is measured.** maz-term never renders placeholder or
sample data: if a value cannot be collected, it says so instead of showing a
plausible number.

## Status

Working today:

| Area | State |
|---|---|
| System metrics (CPU, memory, disk, network, processes) | ✅ collected via gopsutil |
| HTTP endpoint health checks with rolling availability | ✅ |
| Git repository status, branches, changed files, commit log | ✅ |
| History with selectable time ranges, from SQLite | ✅ |
| Event annotations on history | ✅ |
| Notification centre with filtering | ✅ |
| CSV export | ✅ |
| Plugins (opt-in build, digest-verified) | ✅ |
| Cloud (AWS), Kubernetes and CI/CD monitoring | ❌ not implemented |

Cloud, Kubernetes and CI/CD are **not built**. Earlier revisions shipped
collectors for them that returned hardcoded inventories while the UI displayed
"Connected"; those have been removed rather than left to mislead. The
configuration keys exist and are validated, but nothing reads them yet.

## Install

### From source

```bash
git clone https://github.com/Teomazivila/maz-term.git
cd maz-term
make build
./maz-term
```

`make install` places the binary on your `GOPATH/bin`.

### Cross-compiled binaries

```bash
make release     # linux, darwin, windows on amd64 and arm64, plus SHA256SUMS
```

The SQLite driver is pure Go, so every target builds with `CGO_ENABLED=0` and
needs no system libraries.

## Configure

maz-term runs with sensible defaults and **contacts nothing** until you tell it
to. To customise, copy the example:

```bash
cp config.example.yaml ~/.config/maz-term/config.yaml
```

Configuration is read from, in order: the path given to `-config`, then
`./config.yaml`, `~/.config/maz-term/config.yaml`, `/etc/maz-term/config.yaml`.

A minimal file:

```yaml
general:
  refresh: 5s
  history_retention: 7d

endpoints:
  - name: "API gateway"
    url: "https://api.example.com/health"
    expected_status: 200
    timeout: 5s

git:
  repositories:
    - path: "~/projects/my-service"
```

Any key can be overridden from the environment with a `MAZTERM_` prefix:

```bash
MAZTERM_GENERAL_REFRESH=30s ./maz-term
```

Unknown keys are an error, so a typo is reported at startup rather than silently
ignored. See `config.example.yaml` for every option.

### Credentials

Credentials are never read from the configuration file, and a file containing
`access_key_id`, `secret_access_key` or `token` is rejected at startup. Use the
environment or the platform's own credential chain.

## Flags

| Flag | Purpose |
|---|---|
| `-config PATH` | configuration file to use |
| `-data-path PATH` | metrics database (default `~/.config/maz-term/data.db`) |
| `-log-file PATH` | log file (default `~/.config/maz-term/maz-term.log`) |
| `-no-storage` | run without recording history |
| `-debug` | debug-level logging |
| `-version` | print the version and exit |

Logs always go to a file, never to the terminal: the dashboard owns the screen
for its whole lifetime and any stray write corrupts the frame.

## Keyboard

| Key | Action |
|---|---|
| `q`, `Ctrl-C` | quit |
| `?` | toggle help |
| `Tab`, `→`, `l`, `n` | next tab |
| `Shift-Tab`, `←`, `h`, `p` | previous tab |
| `1`–`9` | jump to tab by position |
| `r` | refresh now |
| `e` | export metrics to CSV |

On the History tab:

| Key | Action |
|---|---|
| `[`, `]` | previous / next time range |
| `a` | toggle event annotations |
| `A` | add an annotation |
| `z` | zoom mode (arrows adjust, `Enter` applies, `Esc` cancels) |

On the Notifications tab:

| Key | Action |
|---|---|
| `↑`, `↓` | move selection |
| `m` | mark selected as read |
| `D` | dismiss selected |
| `C` | clear all |
| `d` | toggle detail view |
| `f` | filter by source and severity |
| `o` | open the selected notification's link |

On the Plugins tab: `↑`/`↓` to select, `R` to reload from disk.

Press `?` in the application for the same list; it is generated from the
bindings the code actually implements.

## Plugins

A plugin is native code executed inside the maz-term process, so loading is
deliberately restrictive:

- Support is **not** in the default build. Go's `plugin` package requires cgo and
  has no Windows support, which is incompatible with one portable binary.
- Each plugin's SHA-256 must be recorded under `plugins.allow`. An enabled
  plugin with no digest is refused.
- The file must resolve inside the configured plugin directory and must not be
  writable by group or others.
- Panics are contained and `Collect` runs under a timeout, so one bad plugin
  cannot take the dashboard down or stall it.

```bash
make build-with-plugins    # host build with plugin loading (needs cgo)
make sample-plugin         # builds the reference plugin and prints its digest
```

Then record the digest:

```yaml
plugins:
  directory: "~/.config/maz-term/plugins"
  enabled: ["sample"]
  allow:
    sample: "sha256:<digest printed above>"
```

See [docs/plugin-guide.md](docs/plugin-guide.md) for the interface.

## Development

```bash
make check        # formatting, vet, lint, tests under -race
make test-race    # tests with the race detector
make cover        # coverage per package
make vuln         # govulncheck
```

`make check` is what CI runs. Contributions need it to pass; see
[CONTRIBUTING.md](CONTRIBUTING.md).

## Architecture

```
cmd/maz-term        entry point, flags, wiring, shutdown
pkg/collector       system, HTTP and Git collectors on a shared lifecycle
pkg/config          configuration loading, validation, environment overrides
pkg/models          shared metric and notification types
pkg/plugins         plugin manager, with the loader behind a build tag
pkg/ui              termui rendering, layout, event handling
internal/storage    SQLite persistence, retention, CSV export
```

Collectors publish to subscribers and persist through a typed storage interface.
The UI owns rendering on a single goroutine; nothing else touches widget state.

## License

Apache License 2.0 — see [LICENSE](LICENSE).
