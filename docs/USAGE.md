# DevOps Terminal Dashboard - Usage Guide

## Command-Line Options

The DevOps Terminal Dashboard supports the following command-line options:

```
Usage: maz-term [options]

Options:
  -config string   Path to configuration file
  -version         Print version information and exit
```

## Examples

### Using Default Configuration

To start the dashboard with the default configuration:

```bash
maz-term
```

The default configuration will look for a configuration file in the following locations:
1. Current directory (`./config.yaml`)
2. User's home config directory (`~/.config/maz-term/config.yaml`)
3. System configuration directory (`/etc/maz-term/config.yaml`)

### Using a Specific Configuration File

To start the dashboard with a specific configuration file:

```bash
maz-term -config /path/to/custom/config.yaml
```

### Checking the Version

To check the current version of the dashboard:

```bash
maz-term -version
```

## Configuration File

The configuration file uses YAML format and supports the following sections:

```yaml
general:
  refresh: 5s                # Data refresh interval
  theme: default             # UI theme
  history_retention: 7d      # How long to keep historical data

layout:
  - name: "System Overview"  # Tab name
    panels: ["cpu", "memory", "disk", "network"]  # Panels to display

  - name: "Git"
    panels: ["git-status"]

metrics:
  local:
    enabled: true            # Enable local system metrics
    
  endpoints:                 # HTTP endpoints to monitor
    - name: "Example API"
      url: "https://example.com/api/health"
      method: "GET"
      interval: 30s
      alert:
        status_code: 200
        response_time: 500ms
  
  git:
    repositories:            # Git repositories to monitor
      - path: "~/projects/main-service"
        remote: "origin"
        branch: "main"
```

## Runtime Controls

Once the dashboard is running, you can use the following keyboard controls:

| Key | Action |
|-----|--------|
| `q` | Quit application |
| `←/→` | Navigate between tabs |
| `h/l` | Alternative tab navigation |
| `1-9` | Jump to specific tab |
| `r` | Force data refresh |

## Terminal Requirements

For the best experience, the dashboard requires:

- Terminal with color support
- Minimum terminal size of 80x24 characters
- UTF-8 character encoding support 