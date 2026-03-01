# DevOps Terminal Dashboard - Next Tasks

This document outlines the specific tasks that need to be completed to address the current implementation gaps. Tasks are prioritized based on their impact and dependency order.

## Immediate Tasks (Next 1-2 weeks)

### Core Features

1. **Complete Plugin System Integration**
   - ✅ Create plugin interface and plugin manager
   - ✅ Add plugin loading and unloading capability
   - ✅ Implement plugin UI with details view and metrics display
   - Implement plugin directory scanning and automatic loading
   - Add hot reload capability for plugins
   - Create documentation for plugin development

2. **Improve Notification Center**
   - ✅ Create data model for notifications
   - ✅ Implement notification storage in SQLite
   - ✅ Implement notification UI with filtering capabilities
   - Add notification badges on tabs when new notifications arrive
   - Implement notification rules and severity-based styling
   - Add notification sound alerts (configurable)

3. **Cloud Provider Integration Plugins**
   - Implement AWS plugin (EC2, S3, CloudWatch)
   - Implement Azure plugin (VMs, Storage, Monitor)
   - Implement GCP plugin (Compute, Storage, Monitoring)
   - Create unified cloud resources view

### UI Component Completion

4. **Fix Tab Navigation**
   - ✅ Modify the `updateLayout()` function to properly handle tab switching
   - ✅ Fix rendering issues when changing between tabs
   - ✅ Ensure proper tab height calculation

5. **Complete HTTP Endpoints Tab**
   - ✅ Implement the `updateHTTPTabData()` function to display real data
   - ✅ Add status indicators with color coding (green for up, red for down)
   - ✅ Fix table layout and ensure it scales properly with window resize
   - ✅ Add response time history visualization

6. **Complete Git Repository Tab** 
   - ✅ Implement the `updateGitTabData()` function to display real data
   - ✅ Display repository information, branch details, and commit history
   - ✅ Add color coding for modified files and pending commits
   - ✅ Ensure table scales properly with window resize

7. **Help System**
   - ✅ Create a help panel that shows keyboard shortcuts
   - ✅ Implement toggle functionality (with '?' key)
   - ✅ Document all available commands and navigation options

### Storage Implementation

8. **SQLite Integration**
   - ✅ Design database schema for metrics history
   - ✅ Create `internal/storage` package
   - ✅ Implement database connection handling
   - ✅ Add metrics storage functionality
   - ✅ Implement query methods for historical data
   - ✅ Fix storage configuration to respect retention period

## Technical Debt to Address

1. **Update Documentation**
   - Update PRD to reflect the change from BubbleTea to TermUI
   - Document the current architecture in README
   - Add developer documentation for extending the dashboard
   - Add plugin development guide

2. **Tests**
   - Add unit tests for UI components
   - Add unit tests for storage components
   - Improve test coverage for collectors
   - Add integration tests for the full application
   - Add tests for plugin system

3. **Code Refactoring**
   - Extract duplicate UI code into reusable functions
   - Improve error handling across the application
   - Standardize logging approach

## Advanced Features to Implement

1. **Dashboard Presets**
   - Create predefined dashboard layouts for different use cases
   - Implement layout saving and loading
   - Add user preferences storage

2. **Kubernetes Integration**
   - Create Kubernetes plugin for cluster monitoring
   - Display pod status, resource usage
   - Show deployment and service status
   - Add container logs viewing capability

3. **CI/CD Pipeline Integration**
   - Create plugins for different CI/CD systems (Jenkins, GitHub Actions, GitLab CI)
   - Display pipeline status and history
   - Show build logs
   - Add trigger capability for pipelines

4. **Advanced Visualization**
   - Add more chart types (bar charts, pie charts)
   - Implement heatmaps for correlating metrics
   - Add custom dashboard widgets

## Implementation Strategy for Plugins

1. **Plugin Directory Structure**
   ```
   plugins/
     aws/
       aws.so    # Compiled plugin
       README.md # Documentation
     azure/
       azure.so
       README.md
     kubernetes/
       kubernetes.so
       README.md
   ```

2. **Plugin Loading Process**
   - Scan plugins directory on startup
   - Load enabled plugins from configuration
   - Initialize plugins with their configuration
   - Register plugin metrics and notifications

3. **Plugin Development Guide**
   ```go
   // Example plugin implementation
   package main

   import (
       "github.com/Teomazivila/maz-term/pkg/models"
       "github.com/Teomazivila/maz-term/pkg/plugins"
   )

   type MyPlugin struct {
       // plugin implementation
   }

   // NewPlugin is the exported plugin constructor
   func NewPlugin() plugins.Plugin {
       return &MyPlugin{
           // initialize plugin
       }
   }

   // Implement all required interface methods
   // ...
   ```

## Testing Strategy

1. **Manual Testing Checklist**
   - Verify all tabs display correctly
   - Test tab navigation with keyboard shortcuts
   - Test window resizing behavior
   - Test all data collectors update in real-time
   - Test plugin loading and unloading
   - Test notification creation and management

2. **Automated Tests**
   - Create mock collectors for testing UI components
   - Create mock plugins for testing plugin manager
   - Add benchmarks for performance-critical sections
   - Add integration tests for the full application

## Resources Required

- Go plugin system documentation (for plugin development)
- Cloud provider SDK documentation (AWS, Azure, GCP)
- Kubernetes Go client documentation
- CI/CD system API documentation 