# Maz-Term with TermUI

A terminal-based DevOps dashboard using the TermUI library for responsive, attractive terminal visualization.

## Features

- **System Metrics Monitoring**: Display CPU, memory, disk usage, and process information in real-time
- **HTTP Endpoint Health Checks**: Monitor the status and response times of HTTP endpoints
- **Git Repository Status**: Track branch, commit history, and repository state
- **Responsive Layout**: Automatically adjusts to terminal window size
- **Tab-based Navigation**: Switch between different monitoring views
- **Real-time Updates**: Data refreshes automatically without flickering or content loss

## Usage

```
go run cmd/maz-term-ui/main.go
```

### Navigation

- `Left/Right Arrow` or `h/l`: Switch between tabs
- `1/2/3`: Jump to specific tabs
- `q` or `Ctrl+c`: Quit the application

## Architecture

The application uses the TermUI library (github.com/gizak/termui/v3) for rendering, which provides:

- Stable and responsive terminal UI components
- Optimized rendering with minimal flickering
- Better window resize handling
- Built-in widgets like gauges, sparklines, and bar charts

The data collection is handled by specialized collectors:
- SystemMetricsCollector: For CPU, memory, and disk metrics
- HTTPHealthChecker: For monitoring web endpoints
- GitStatusCollector: For tracking git repository information

## Comparison with Bubbletea

While Bubbletea is an excellent library for interactive TUIs, TermUI offers several advantages for dashboard-style applications:

1. **Built-in Dashboard Components**: TermUI provides ready-to-use widgets like gauges, sparklines, and charts
2. **Grid-based Layout**: Easier to implement complex dashboards with nested layouts
3. **Better Stability**: Less prone to flickering during updates
4. **Optimized for Data Visualization**: Designed specifically for displaying metrics and statistics

## Future Improvements

- [ ] Add customizable themes
- [ ] Implement more detailed system metrics
- [ ] Add alert thresholds for metrics
- [ ] Support for remote system monitoring
- [ ] Add export and logging capabilities 