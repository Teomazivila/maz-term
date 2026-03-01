# DevOps Terminal Dashboard - Installation Guide

This document provides detailed installation instructions for the DevOps Terminal Dashboard.

## Prerequisites

- Go 1.21 or higher
- Git
- Make (for building from source)

## Installation Methods

### 1. Using Pre-built Binaries

#### Linux/macOS

```bash
# Download the latest release
curl -sfL https://github.com/yourusername/maz-term/releases/latest/download/maz-term-$(uname -s)-$(uname -m) -o maz-term

# Make it executable
chmod +x maz-term

# Move to a directory in your PATH
sudo mv maz-term /usr/local/bin/
```

#### Windows

```powershell
# Download the latest release (PowerShell)
Invoke-WebRequest -Uri "https://github.com/yourusername/maz-term/releases/latest/download/maz-term-windows-amd64.exe" -OutFile "maz-term.exe"

# Move to a directory in your PATH
Move-Item -Path "maz-term.exe" -Destination "C:\Windows\System32\"
```

### 2. Building from Source

#### Clone the Repository

```bash
git clone https://github.com/yourusername/maz-term.git
cd maz-term
```

#### Build and Install

```bash
# Build the application
make build

# Install to your PATH (Linux/macOS)
make install

# For Windows, manually copy the binary
# copy maz-term.exe C:\Windows\System32\
```

### 3. Using Go Install

```bash
go install github.com/yourusername/maz-term@latest
```

## Configuration Setup

### Default Configuration

After installation, create a basic configuration file:

```bash
# Create configuration directory
mkdir -p ~/.config/maz-term

# Create a basic configuration file
cat > ~/.config/maz-term/config.yaml << EOF
general:
  refresh: 5s
  theme: default
  history_retention: 7d

layout:
  - name: "System Overview"
    panels: ["cpu", "memory", "disk", "network"]
  
  - name: "Services"
    panels: ["http-endpoints"]
  
  - name: "Git"
    panels: ["git-status"]

metrics:
  local:
    enabled: true
    
  endpoints:
    - name: "Example API"
      url: "https://example.com/api/health"
      method: "GET"
      interval: 30s
  
  git:
    repositories:
      - path: "~/projects/main-repo"
        remote: "origin"
        branch: "main"
EOF
```

## Verifying Installation

Run the dashboard to verify it's correctly installed:

```bash
maz-term
```

You should see the dashboard launch in your terminal.

## Troubleshooting

### Common Issues

1. **Missing Dependencies**

   If you encounter errors about missing Go dependencies, run:
   ```bash
   go mod tidy
   ```

2. **Permission Issues**

   If you can't execute the binary on Linux/macOS:
   ```bash
   chmod +x /path/to/maz-term
   ```

3. **Terminal Compatibility**

   If you see display issues, ensure your terminal supports:
   - UTF-8 encoding
   - 256 colors
   - Minimum size of 80x24 characters

4. **Configuration Errors**

   If the dashboard can't find your configuration:
   ```bash
   maz-term -config /full/path/to/your/config.yaml
   ```

## Uninstallation

```bash
# Remove the binary
sudo rm /usr/local/bin/maz-term

# Remove configuration files
rm -rf ~/.config/maz-term
``` 