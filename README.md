# DevOps Terminal Dashboard

![GitHub release (latest by date)](https://img.shields.io/github/v/release/yourusername/devops-terminal-dashboard?style=flat-square)
![Go version](https://img.shields.io/badge/Go-1.24%2B-blue?style=flat-square)
![License](https://img.shields.io/badge/License-MIT-green?style=flat-square)

A powerful, customizable terminal-based dashboard for DevOps professionals. Monitor systems, services, and infrastructure in a single unified interface without leaving your terminal.

![Dashboard Preview](docs/images/dashboard_preview.png)

## Features

- **Terminal-based UI**: Fast, keyboard-driven interface built with [Bubble Tea](https://github.com/charmbracelet/bubbletea)
- **Real-time Monitoring**: Track system metrics, service health, and deployment status
- **Multiple Integrations**:
  - Local system metrics (CPU, memory, disk, network)
  - Cloud providers (AWS, GCP, Azure)
  - Kubernetes clusters
  - CI/CD pipelines (GitHub Actions, Jenkins, GitLab CI)
  - Git repositories
  - HTTP endpoints and APIs
- **Customizable Layout**: Configure your perfect dashboard with drag-and-drop panels
- **Low Resource Footprint**: Minimal CPU and memory usage
- **Cross-Platform**: Works on Linux, macOS, and Windows

## Installation

### From Binary

```bash
# Linux/macOS
curl -sfL https://github.com/yourusername/devops-terminal-dashboard/releases/latest/download/install.sh | sh

# Windows (PowerShell)
irm https://github.com/yourusername/devops-terminal-dashboard/releases/latest/download/install.ps1 | iex
```

### From Source

```bash
# Clone repository
git clone https://github.com/yourusername/devops-terminal-dashboard.git
cd devops-terminal-dashboard

# Build
make build

# Install
make install
```

## Quick Start

1. Create a basic configuration file:

```bash
devops-dashboard init
```

2. Start the dashboard:

```bash
devops-dashboard
```

3. Press `?` to view keyboard shortcuts and help.

## Configuration

The dashboard is configured via YAML files located in `~/.config/devops-dashboard/config.yaml` by default.

### Basic Configuration

```yaml
# ~/.config/devops-dashboard/config.yaml
general:
  refresh: 5s
  theme: default
  history_retention: 7d

layout:
  - name: "System Overview"
    rows:
      - size: 1
        panels: ["cpu", "memory", "disk", "network"]
      - size: 2
        panels: ["processes"]
  
  - name: "Services"
    rows:
      - size: 1
        panels: ["service-status"]
      - size: 2
        panels: ["http-endpoints"]

metrics:
  local:
    enabled: true
    
  endpoints:
    - name: "API Gateway"
      url: "https://api.example.com/health"
      method: "GET"
      interval: 30s
      alert:
        status_code: 200
        response_time: 500ms
  
  git:
    repositories:
      - path: "~/projects/main-service"
        remote: "origin"
        branch: "main"
```

## Keyboard Shortcuts

| Key | Action |
|-----|--------|
| `q` | Quit application |
| `?` | Show help |
| `1-9` | Switch tabs |
| `r` | Refresh data |
| `c` | Clear alerts |
| `f` | Filter view |
| `s` | Save current layout |
| `/` | Search |
| `Ctrl+e` | Export data |

## Plugins

The dashboard supports plugins for custom metrics and visualizations. To create a plugin:

```go
package myplugin

import (
    "github.com/yourusername/devops-terminal-dashboard/plugin"
)

type MyPlugin struct {}

func (p *MyPlugin) Initialize(ctx context.Context) error {
    // Plugin initialization code
    return nil
}

func (p *MyPlugin) Collect() (plugin.Metrics, error) {
    // Metric collection logic
    return metrics, nil
}

func (p *MyPlugin) RenderPanel(screen *bubbleteascreeen.Screen) {
    // UI rendering code
}

// Export the plugin
var Plugin = &MyPlugin{}
```

## Contributing

Contributions are welcome! Please check our [Contributing Guidelines](CONTRIBUTING.md) before submitting PRs.

1. Fork the repository
2. Create your feature branch (`git checkout -b feature/amazing-feature`)
3. Make your changes
4. Run tests (`make test`)
5. Commit your changes (`git commit -m 'Add amazing feature'`)
6. Push to the branch (`git push origin feature/amazing-feature`)
7. Open a Pull Request

## Roadmap

- [x] Core dashboard framework
- [x] System metrics
- [x] HTTP endpoint monitoring
- [x] Git integration
- [ ] Cloud provider integration
- [ ] Kubernetes monitoring
- [ ] CI/CD pipeline integration
- [ ] Notification center
- [ ] Data export/reporting

## License

This project is licensed under the MIT License - see the [LICENSE](LICENSE) file for details.