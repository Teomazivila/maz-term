# DevOps Terminal Dashboard - Next Tasks

This document outlines the specific tasks that need to be completed to address the current implementation gaps. Tasks are prioritized based on their impact and dependency order.

## Immediate Tasks (Next 1-2 weeks)

### UI Component Completion

1. **Fix Tab Navigation**
   - Modify the `updateLayout()` function in `pkg/ui/app.go` to properly handle tab switching
   - Fix rendering issues when changing between tabs
   - Ensure proper tab height calculation

2. **Complete HTTP Endpoints Tab**
   - Implement the `updateHTTPTabData()` function to display real data from HTTP collector
   - Add status indicators with color coding (green for up, red for down)
   - Fix table layout and ensure it scales properly with window resize
   - Add response time history visualization

3. **Complete Git Repository Tab** 
   - Implement the `updateGitTabData()` function to display real data from Git collector
   - Display repository information, branch details, and commit history
   - Add color coding for modified files and pending commits
   - Ensure table scales properly with window resize

4. **Add Process Table to System Tab**
   - Implement process data collection in system metrics collector
   - Add sorting capability (by CPU, memory usage)
   - Add process filtering functionality

### Core Improvements

5. **Add Help Screen**
   - Create a help panel that shows keyboard shortcuts
   - Implement toggle functionality (likely with '?' key)
   - Document all available commands and navigation options

6. **Implement Keyboard Shortcuts**
   - Complete the `handleEvent()` function with all required shortcuts
   - Add refresh data command (r key)
   - Add filter functionality (f key)
   - Add export functionality (Ctrl+e)

7. **Fix Resize Handling**
   - Improve window resize handling for all components
   - Ensure UI components scale appropriately
   - Fix potential rendering issues during resize

### Storage Implementation

8. **Design SQLite Schema**
   - Design database schema for metrics history
   - Define retention policies
   - Plan query patterns for visualization

9. **Implement SQLite Integration**
   - Create `internal/storage` package
   - Implement database connection handling
   - Add metrics storage functionality
   - Implement query methods for historical data

## Technical Debt to Address

1. **Update Documentation**
   - Update PRD to reflect the change from BubbleTea to TermUI
   - Document the current architecture in README
   - Add developer documentation for extending the dashboard

2. **Tests**
   - Add unit tests for UI components
   - Improve test coverage for collectors
   - Add integration tests for the full application

3. **Code Refactoring**
   - Extract duplicate UI code into reusable functions
   - Improve error handling across the application
   - Standardize logging approach

## Implementation Notes

### HTTP Tab Update Strategy

```go
// In pkg/ui/app.go
func (a *App) updateHTTPTabData() {
    // Get the latest metrics from collector
    metrics := a.httpCollector.GetLatestMetrics()
    
    // Get the HTTP table from the active tab
    if len(a.tabs[1].tables) > 0 {
        table := a.tabs[1].tables[0]
        
        // Reset the table to just the header row
        table.Rows = [][]string{
            {"Endpoint", "URL", "Status", "Response Time", "Last Checked"},
        }
        
        // Add each endpoint to the table
        for name, metric := range metrics {
            status := "DOWN"
            statusColor := ui.ColorRed
            if metric.IsUp {
                status = "UP"
                statusColor = ui.ColorGreen
            }
            
            // Format the response time
            responseTime := fmt.Sprintf("%.0fms", float64(metric.ResponseTime.Milliseconds()))
            
            // Format the last checked time
            lastChecked := metric.LastChecked.Format("15:04:05")
            
            // Add the row
            table.Rows = append(table.Rows, []string{
                name, metric.URL, status, responseTime, lastChecked,
            })
            
            // Set color based on status
            rowIdx := len(table.Rows) - 1
            if metric.IsUp {
                table.RowStyles[rowIdx] = ui.NewStyle(statusColor)
            } else {
                table.RowStyles[rowIdx] = ui.NewStyle(statusColor)
            }
        }
    }
}
```

### Git Tab Update Strategy

```go
// In pkg/ui/app.go
func (a *App) updateGitTabData() {
    // Get the latest metrics from collector
    metrics := a.gitCollector.GetLatestMetrics()
    
    // Update status table
    if len(a.tabs[2].tables) > 0 {
        statusTable := a.tabs[2].tables[0]
        
        // Update with real data
        statusTable.Rows = [][]string{
            {"Property", "Value"},
            {"Repository", metrics.Name},
            {"Branch", metrics.Branch},
            {"Commit Count", strconv.Itoa(metrics.CommitCount)},
            {"Last Commit", metrics.LastCommit.Format("2006-01-02 15:04:05")},
            {"Modified Files", strconv.Itoa(metrics.ModifiedFiles)},
            {"Pending Commits", strconv.Itoa(metrics.PendingCommits)},
        }
        
        // Highlight modified files row if there are modifications
        if metrics.ModifiedFiles > 0 {
            statusTable.RowStyles[5] = ui.NewStyle(ui.ColorYellow)
        } else {
            statusTable.RowStyles[5] = ui.NewStyle(ui.ColorWhite)
        }
    }
    
    // Update commit history table
    if len(a.tabs[2].tables) > 1 {
        commitTable := a.tabs[2].tables[1]
        
        // Reset to header row
        commitTable.Rows = [][]string{
            {"Hash", "Author", "Date", "Message"},
        }
        
        // Add commit history
        for _, commit := range metrics.CommitHistory {
            shortHash := commit.Hash
            if len(shortHash) > 8 {
                shortHash = shortHash[:8]
            }
            
            commitTable.Rows = append(commitTable.Rows, []string{
                shortHash,
                commit.Author,
                commit.Timestamp.Format("2006-01-02"),
                commit.Message,
            })
        }
    }
}
```

## Testing Strategy

1. **Manual Testing Checklist**
   - Verify all tabs display correctly
   - Test tab navigation with keyboard shortcuts
   - Test window resizing behavior
   - Test all data collectors update in real-time

2. **Automated Tests**
   - Create mock collectors for testing UI components
   - Add benchmarks for performance-critical sections
   - Add integration tests for the full application

## Resources Required

- SQLite driver for Go (github.com/mattn/go-sqlite3)
- Additional documentation on TermUI for advanced layouts
- Access to test systems with different terminal sizes and capabilities 