package ui

import (
	"fmt"
	"log/slog"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/Teomazivila/maz-term/pkg/models"
	ui "github.com/gizak/termui/v3"
	"github.com/gizak/termui/v3/widgets"
)

// historyPoints is how many samples each history plot requests.
const historyPoints = 200

// sparklinePoints caps the in-memory sparkline series.
const sparklinePoints = 120

// Row styles shared by the metric tables. termui tables colour whole rows via
// RowStyles; they do not parse inline markup, so status colouring is applied
// here rather than embedded in the cell text.
var (
	headerStyle = ui.NewStyle(ui.ColorWhite, ui.ColorClear, ui.ModifierBold)
	normalStyle = ui.NewStyle(ui.ColorWhite)
	goodStyle   = ui.NewStyle(ui.ColorGreen)
	warnStyle   = ui.NewStyle(ui.ColorYellow)
	badStyle    = ui.NewStyle(ui.ColorRed)
	subtleStyle = ui.NewStyle(ui.ColorClear)
)

// updateData refreshes the widget state the visible tab needs.
//
// Only the active tab is refreshed. Rebuilding every tab each frame meant
// formatting thousands of table rows and issuing a dozen SQLite queries every
// second for views that were not on screen; on a cluster with 487 pods and 500
// services that alone made the interface feel sluggish.
func (a *App) updateData() {
	switch a.activeTabName() {
	case "System":
		a.updateSystemTabData()
	case "HTTP":
		a.updateHTTPTabData()
	case "Git":
		a.updateGitTabData()
	case "History":
		a.updateHistoryTabData()
	case "Cloud":
		a.updateCloudTabData()
	case "Kubernetes":
		a.updateKubernetesTabData()
	case "CI/CD":
		a.updateCICDTabData()
	case "Notifications":
		a.updateNotificationsTabData()
	case "Plugins":
		a.updatePluginsTabData()
	}

	// The unread badge is shown on the tab bar from every tab, so it is refreshed
	// regardless, but on its own slower cadence.
	if a.notificationsRefresh.ready() {
		a.updateTabBadges()
	}
}

// newTable builds a bordered table.
func newTable(title string, columns ...Column) *DataTable {
	table := NewDataTable(title)
	table.Columns = columns
	table.RowStyle = normalStyle
	return table
}

// textRow builds an unstyled row.
func textRow(cells ...string) Row { return Row{Cells: cells} }

// styledRow builds a row with an explicit style.
func styledRow(style ui.Style, cells ...string) Row {
	return Row{Cells: cells, Style: &style}
}

// noticeRow renders a single subdued line in place of data.
func noticeRow(columns int, text string) []Row {
	cells := make([]string, columns)
	cells[0] = text
	return []Row{styledRow(subtleStyle, cells...)}
}

// newPanel builds a bordered wrapping paragraph.
func newPanel(title string) *widgets.Paragraph {
	panel := widgets.NewParagraph()
	panel.Title = title
	panel.WrapText = true
	panel.BorderStyle.Fg = ui.ColorCyan
	return panel
}

// updateSystemTabData refreshes the System tab.
func (a *App) updateSystemTabData() {
	tab := a.getTabByName("System")
	if tab == nil {
		return
	}

	if len(tab.Widgets) == 0 {
		cpuGauge := widgets.NewGauge()
		cpuGauge.Title = "CPU"
		cpuGauge.BarColor = ui.ColorRed
		cpuGauge.BorderStyle.Fg = ui.ColorCyan

		memGauge := widgets.NewGauge()
		memGauge.Title = "Memory"
		memGauge.BarColor = ui.ColorBlue
		memGauge.BorderStyle.Fg = ui.ColorCyan

		spark := widgets.NewSparkline()
		spark.LineColor = ui.ColorRed
		cpuHistory := widgets.NewSparklineGroup(spark)
		cpuHistory.Title = "CPU history"
		cpuHistory.BorderStyle.Fg = ui.ColorCyan

		diskChart := widgets.NewBarChart()
		diskChart.Title = "Disk usage %"
		diskChart.BarWidth = 8
		diskChart.BarGap = 1
		diskChart.BarColors = []ui.Color{ui.ColorGreen}
		diskChart.LabelStyles = []ui.Style{normalStyle}
		diskChart.NumStyles = []ui.Style{ui.NewStyle(ui.ColorBlack)}
		diskChart.NumFormatter = func(f float64) string { return fmt.Sprintf("%.0f", f) }
		diskChart.BorderStyle.Fg = ui.ColorCyan

		processes := newTable("Top processes",
			Column{Title: "PID", Width: 7, Align: AlignRight},
			Column{Title: "CPU%", Width: 6, Align: AlignRight},
			Column{Title: "MEM%", Width: 6, Align: AlignRight},
			Column{Title: "RSS", Width: 10, Align: AlignRight},
			Column{Title: "COMMAND", Weight: 1})

		tab.Widgets = []ui.Drawable{cpuGauge, memGauge, cpuHistory, diskChart, processes}
		tab.Gauges = []*widgets.Gauge{cpuGauge, memGauge}
		tab.Sparklines = []*widgets.SparklineGroup{cpuHistory}
		tab.BarCharts = []*widgets.BarChart{diskChart}
		tab.Tables = []*DataTable{processes}
	}

	if a.SystemCollector == nil {
		return
	}
	metrics := a.SystemCollector.GetLatestMetrics()

	if len(tab.Gauges) > 0 {
		gauge := tab.Gauges[0]
		gauge.Percent = clampPercent(metrics.CPU.UsagePercent)
		gauge.Label = fmt.Sprintf("%.1f%%  load %.2f %.2f %.2f",
			metrics.CPU.UsagePercent,
			metrics.CPU.LoadAverage.Load1,
			metrics.CPU.LoadAverage.Load5,
			metrics.CPU.LoadAverage.Load15)
	}

	if len(tab.Gauges) > 1 {
		gauge := tab.Gauges[1]
		gauge.Percent = clampPercent(metrics.Memory.UsagePercent)
		gauge.Label = fmt.Sprintf("%.1f%%  %s / %s",
			metrics.Memory.UsagePercent,
			FormatBytes(metrics.Memory.Used),
			FormatBytes(metrics.Memory.Total))
	}

	if len(tab.Sparklines) > 0 && len(tab.Sparklines[0].Sparklines) > 0 {
		line := tab.Sparklines[0].Sparklines[0]
		line.Data = appendCapped(line.Data, metrics.CPU.UsagePercent, sparklinePoints)
		line.Title = fmt.Sprintf("CPU %.1f%%", metrics.CPU.UsagePercent)
	}

	if len(tab.BarCharts) > 0 {
		chart := tab.BarCharts[0]
		data := make([]float64, 0, len(metrics.Disk.Filesystems))
		labels := make([]string, 0, len(metrics.Disk.Filesystems))
		colors := make([]ui.Color, 0, len(metrics.Disk.Filesystems))

		for _, fs := range metrics.Disk.Filesystems {
			// System and pseudo volumes crowd out the real ones and their long
			// paths collide into an unreadable strip of labels.
			if isPseudoFilesystem(fs.MountPoint) {
				continue
			}
			data = append(data, fs.UsagePercent)
			labels = append(labels, shortMountPoint(fs.MountPoint))
			colors = append(colors, usageColor(fs.UsagePercent))
		}

		chart.Data = data
		chart.Labels = labels
		if len(colors) > 0 {
			chart.BarColors = colors
		}
	}

	if len(tab.Tables) > 0 {
		table := tab.Tables[0]
		rows := make([]Row, 0, len(metrics.Processes))

		for _, proc := range metrics.Processes {
			rows = append(rows, styledRow(usageStyle(proc.CPUPercent),
				strconv.Itoa(int(proc.PID)),
				fmt.Sprintf("%.1f", proc.CPUPercent),
				fmt.Sprintf("%.1f", proc.MemoryPercent),
				FormatBytes(proc.MemoryBytes),
				proc.Command,
			))
		}

		if len(rows) == 0 {
			rows = noticeRow(5, "collecting…")
		}
		table.SetRows(rows)
	}
}

// updateHTTPTabData refreshes the HTTP tab.
func (a *App) updateHTTPTabData() {
	tab := a.getTabByName("HTTP")
	if tab == nil {
		return
	}

	if len(tab.Widgets) == 0 {
		endpoints := newTable("Endpoints",
			Column{Title: "ENDPOINT", Weight: 3},
			Column{Title: "STATUS", Width: 7, Align: AlignRight},
			Column{Title: "RESPONSE", Width: 10, Align: AlignRight},
			Column{Title: "AVAIL", Width: 8, Align: AlignRight},
			Column{Title: "CHECKS", Width: 7, Align: AlignRight},
			Column{Title: "LAST CHECK", Weight: 1})

		respLine := widgets.NewSparkline()
		respLine.LineColor = ui.ColorGreen
		respHistory := widgets.NewSparklineGroup(respLine)
		respHistory.Title = "Response time"
		respHistory.BorderStyle.Fg = ui.ColorCyan

		availLine := widgets.NewSparkline()
		availLine.LineColor = ui.ColorBlue
		availHistory := widgets.NewSparklineGroup(availLine)
		availHistory.Title = "Availability"
		availHistory.BorderStyle.Fg = ui.ColorCyan

		details := newPanel("Details")

		tab.Widgets = []ui.Drawable{endpoints, respHistory, availHistory, details}
		tab.Tables = []*DataTable{endpoints}
		tab.Sparklines = []*widgets.SparklineGroup{respHistory, availHistory}
		tab.Panels = []*widgets.Paragraph{details}
	}

	if a.HTTPCollector == nil {
		return
	}
	metrics := a.HTTPCollector.GetLatestMetrics()

	// Map iteration order is random, so endpoints are sorted for a stable table.
	names := make([]string, 0, len(metrics))
	for name := range metrics {
		names = append(names, name)
	}
	sort.Strings(names)

	if len(tab.Tables) > 0 {
		table := tab.Tables[0]
		rows := make([]Row, 0, len(names))

		for _, name := range names {
			metric := metrics[name]

			status := "-"
			switch {
			case metric.Error != "":
				status = "ERR"
			case metric.StatusCode > 0:
				status = strconv.Itoa(metric.StatusCode)
			}

			lastCheck := "-"
			if !metric.LastChecked.IsZero() {
				lastCheck = FormatTime(metric.LastChecked)
			}

			// Availability is a real percentage over the collector's rolling
			// window, not a rendering of the boolean IsUp field.
			availability, checks := "-", "-"
			if metric.ChecksInWindow > 0 {
				availability = fmt.Sprintf("%.0f%%", metric.Availability)
				checks = strconv.Itoa(metric.ChecksInWindow)
			}

			style := badStyle
			if metric.IsUp {
				style = goodStyle
			}

			rows = append(rows, styledRow(style,
				displayName(name, metric.URL),
				status,
				formatResponseTime(metric),
				availability,
				checks,
				lastCheck,
			))
		}

		if len(rows) == 0 {
			rows = noticeRow(6, "no endpoints configured")
		}
		table.SetRows(rows)
	}

	// The sparklines and the detail panel follow the first endpoint by name.
	if len(names) == 0 {
		if len(tab.Panels) > 0 {
			tab.Panels[0].Text = "No endpoints configured.\n\n" +
				"Add entries under endpoints: in your configuration file."
		}
		return
	}

	primary := metrics[names[0]]

	if len(tab.Sparklines) > 0 && len(tab.Sparklines[0].Sparklines) > 0 {
		group := tab.Sparklines[0]
		group.Title = "Response time - " + TruncateString(names[0], 24)
		line := group.Sparklines[0]
		line.Data = appendCapped(line.Data, float64(primary.ResponseTime.Milliseconds()), sparklinePoints)
	}

	if len(tab.Sparklines) > 1 && len(tab.Sparklines[1].Sparklines) > 0 {
		group := tab.Sparklines[1]
		group.Title = "Availability - " + TruncateString(names[0], 24)
		line := group.Sparklines[0]
		line.Data = appendCapped(line.Data, primary.Availability, sparklinePoints)
	}

	if len(tab.Panels) > 0 {
		var b strings.Builder
		for _, name := range names {
			metric := metrics[name]
			fmt.Fprintf(&b, "%s\n  %s\n", name, metric.URL)
			if metric.Error != "" {
				fmt.Fprintf(&b, "  unreachable: %s\n", metric.Error)
			} else {
				fmt.Fprintf(&b, "  status %d in %s, availability %.1f%% over %d checks\n",
					metric.StatusCode, metric.ResponseTime.Round(time.Millisecond),
					metric.Availability, metric.ChecksInWindow)
			}
		}
		tab.Panels[0].Text = b.String()
	}
}

// updateGitTabData refreshes the Git tab.
func (a *App) updateGitTabData() {
	tab := a.getTabByName("Git")
	if tab == nil {
		return
	}

	if len(tab.Widgets) == 0 {
		summary := newPanel("Repository")

		changes := newTable("Changed files",
			Column{Title: "ST", Width: 4},
			Column{Title: "FILE", Weight: 1})
		commits := newTable("Recent commits",
			Column{Title: "COMMIT", Width: 9},
			Column{Title: "AUTHOR", Width: 18},
			Column{Title: "WHEN", Width: 12},
			Column{Title: "MESSAGE", Weight: 1})

		branches := widgets.NewList()
		branches.Title = "Branches"
		branches.WrapText = false
		branches.BorderStyle.Fg = ui.ColorCyan
		branches.TextStyle = normalStyle

		tab.Widgets = []ui.Drawable{summary, changes, commits, branches}
		tab.Panels = []*widgets.Paragraph{summary}
		tab.Tables = []*DataTable{changes, commits}
		tab.Lists = []*widgets.List{branches}
	}

	if a.GitCollector == nil {
		return
	}
	metrics := a.GitCollector.GetLatestMetrics()

	if len(tab.Panels) > 0 {
		panel := tab.Panels[0]

		// A path that is not a repository says so, instead of showing zeroes
		// that are indistinguishable from a clean tree.
		if !metrics.IsRepository {
			panel.Text = fmt.Sprintf("Not a Git repository:\n  %s\n\n%s",
				metrics.Path, metrics.Error)
		} else {
			lastCommit := "no commits"
			if !metrics.LastCommit.IsZero() {
				lastCommit = FormatTime(metrics.LastCommit)
			}

			// Unpushed commits are only meaningful with an upstream, and only as
			// of the last fetch, so the absence of one is stated rather than
			// rendered as a zero.
			unpushed := fmt.Sprintf("%d", metrics.PendingCommits)
			if !metrics.HasUpstream {
				unpushed = "no upstream"
			}

			panel.Text = fmt.Sprintf(
				"%s\n  path      %s\n  branch    %s\n  remote    %s\n  commits   %d\n  modified  %d\n  untracked %d\n  unpushed  %s\n  last      %s",
				metrics.Name, metrics.Path, metrics.Branch,
				gitRemoteLabel(metrics.Remote, metrics.RemoteURL),
				metrics.CommitCount, metrics.ModifiedFiles, metrics.UntrackedFiles,
				unpushed, lastCommit)

			if metrics.Error != "" {
				panel.Text += "\n\npartial data: " + metrics.Error
			}
		}
	}

	// Changed files are listed individually rather than summarised as a count.
	if len(tab.Tables) > 0 {
		table := tab.Tables[0]
		rows := make([]Row, 0, len(metrics.ChangedFiles))

		for _, change := range metrics.ChangedFiles {
			rows = append(rows, styledRow(gitStatusStyle(change.Status),
				strings.TrimSpace(change.Status), change.Path))
		}

		if len(rows) == 0 {
			label := "working tree clean"
			if !metrics.IsRepository {
				label = "-"
			}
			rows = noticeRow(2, label)
		}
		table.SetRows(rows)
	}

	if len(tab.Tables) > 1 {
		table := tab.Tables[1]
		rows := make([]Row, 0, len(metrics.CommitHistory))

		for _, commit := range metrics.CommitHistory {
			hash := commit.Hash
			if len(hash) > 8 {
				hash = hash[:8]
			}
			when := "-"
			if !commit.Timestamp.IsZero() {
				when = FormatTime(commit.Timestamp)
			}
			rows = append(rows, textRow(hash, commit.Author, when, commit.Message))
		}

		if len(rows) == 0 {
			rows = noticeRow(4, "no commits")
		}
		table.SetRows(rows)
	}

	// Branches come from git for-each-ref. The previous implementation appended
	// a fixed "main / develop / feature-new-ui" list that had nothing to do with
	// the repository.
	if len(tab.Lists) > 0 {
		list := tab.Lists[0]
		rows := make([]string, 0, len(metrics.Branches))

		for _, branch := range metrics.Branches {
			if branch == metrics.Branch {
				// termui lists do parse inline styles, using this syntax.
				rows = append(rows, fmt.Sprintf("[* %s](fg:green,mod:bold)", branch))
			} else {
				rows = append(rows, "  "+branch)
			}
		}

		if len(rows) == 0 {
			rows = append(rows, "  no branches")
		}

		list.Rows = rows
	}
}

// updateHistoryTabData refreshes the History tab from storage.
func (a *App) updateHistoryTabData() {
	tab := a.getTabByName("History")
	if tab == nil {
		return
	}

	if len(tab.Widgets) == 0 {
		cpuPlot := newPlot("CPU usage %", ui.ColorGreen)
		memPlot := newPlot("Memory usage %", ui.ColorBlue)
		httpPlot := newPlot("HTTP response time (ms)", ui.ColorYellow)
		legend := newPanel("Range")

		tab.Widgets = []ui.Drawable{cpuPlot, memPlot, httpPlot, legend}
		tab.Plots = []*widgets.Plot{cpuPlot, memPlot, httpPlot}
		tab.Panels = []*widgets.Paragraph{legend}
	}

	if len(tab.Panels) > 0 {
		tab.Panels[0].Text = a.historyLegend()
	}

	if a.Storage == nil {
		for _, plot := range tab.Plots {
			plot.Data = nil
			plot.Title = strings.TrimSuffix(plot.Title, " - no storage") + " - no storage"
		}
		return
	}

	// Each plot is a windowed SQLite query over up to 200 points. Re-running them
	// every frame is wasted work when the underlying samples arrive far less often.
	if !a.historyRefresh.ready() {
		return
	}

	a.refreshAnnotations()

	if len(tab.Plots) > 0 {
		points, err := a.Storage.GetCPUUsageHistory(a.HistoryRange, historyPoints)
		a.applySeries(tab.Plots[0], "CPU usage %", points, err)
	}
	if len(tab.Plots) > 1 {
		points, err := a.Storage.GetMemoryUsageHistory(a.HistoryRange, historyPoints)
		a.applySeries(tab.Plots[1], "Memory usage %", points, err)
	}
	if len(tab.Plots) > 2 {
		endpoints, err := a.Storage.GetAllEndpoints()
		switch {
		case err != nil:
			a.applySeries(tab.Plots[2], "HTTP response time (ms)", nil, err)
		case len(endpoints) == 0:
			a.applySeries(tab.Plots[2], "HTTP response time (ms)", nil, nil)
		default:
			points, err := a.Storage.GetHTTPResponseTimeHistory(endpoints[0], a.HistoryRange, historyPoints)
			a.applySeries(tab.Plots[2], "HTTP response time - "+endpoints[0], points, err)
		}
	}
}

// newPlot builds a line plot.
func newPlot(title string, color ui.Color) *widgets.Plot {
	plot := widgets.NewPlot()
	plot.Title = title
	plot.AxesColor = ui.ColorWhite
	plot.LineColors = []ui.Color{color}
	plot.BorderStyle.Fg = ui.ColorCyan
	plot.Marker = widgets.MarkerBraille
	return plot
}

// applySeries loads points into a plot, reporting read failures and thin data in
// the title instead of leaving a stale or misleading chart.
//
// termui needs at least two points to draw a line, so a single sample is
// reported as such rather than plotted.
func (a *App) applySeries(plot *widgets.Plot, title string, points []models.TimeSeriesPoint, err error) {
	switch {
	case err != nil:
		plot.Data = nil
		plot.Title = title + " - unavailable"
		slog.Default().Error("history query failed", "series", title, "error", err)
		return
	case len(points) < 2:
		plot.Data = nil
		plot.Title = fmt.Sprintf("%s - collecting (%d samples)", title, len(points))
		return
	}

	values := make([]float64, len(points))
	for i, point := range points {
		values[i] = point.Value
	}
	plot.Data = [][]float64{values}

	suffix := ""
	if marks := len(a.annotationsInRange()); marks > 0 {
		suffix = fmt.Sprintf(", %d events", marks)
	}
	plot.Title = fmt.Sprintf("%s - %s%s", title, historyRanges[a.clampedRangeIndex()].Label, suffix)
}

// historyLegend renders the range selector, derived from historyRanges so it can
// never disagree with what the keys actually do.
func (a *App) historyLegend() string {
	var b strings.Builder
	b.WriteString("[ and ] change range\n\n")

	for i, r := range historyRanges {
		marker := "  "
		if i == a.HistoryRangeIdx {
			marker = "> "
		}
		fmt.Fprintf(&b, "%s%s\n", marker, r.Label)
	}

	if a.HistoryRangeIdx < 0 {
		fmt.Fprintf(&b, "\n> custom %s (zoom)\n", a.HistoryRange.Round(time.Minute))
	}

	fmt.Fprintf(&b, "\na  annotations %s\nA  add annotation\nz  zoom\n",
		onOff(a.ShowAnnotations))

	return b.String()
}

// refreshAnnotations reloads the cached annotations for the current range.
func (a *App) refreshAnnotations() {
	annotations, err := a.Storage.GetEventAnnotations(a.HistoryRange)
	if err != nil {
		slog.Default().Error("failed to load annotations", "error", err)
		return
	}
	a.Annotations = annotations
}

// setHistoryRange selects the range at index and reloads the History tab.
//
// This is what makes the [ and ] keys work: the previous implementation moved
// the index and rewrote the status bar without ever assigning HistoryRange, so
// the window never actually changed.
func (a *App) setHistoryRange(index int) {
	if index < 0 || index >= len(historyRanges) {
		return
	}

	a.HistoryRangeIdx = index
	a.HistoryRange = historyRanges[index].Value
	a.ZoomMode = false

	a.setStatus("history range %s", historyRanges[index].Label)
	a.updateHistoryTabData()
}

func clampPercent(v float64) int {
	switch {
	case v < 0:
		return 0
	case v > 100:
		return 100
	default:
		return int(v)
	}
}

func appendCapped(data []float64, value float64, limit int) []float64 {
	data = append(data, value)
	if len(data) > limit {
		data = data[len(data)-limit:]
	}
	return data
}

func usageColor(percent float64) ui.Color {
	switch {
	case percent >= 90:
		return ui.ColorRed
	case percent >= 75:
		return ui.ColorYellow
	default:
		return ui.ColorGreen
	}
}

func usageStyle(percent float64) ui.Style {
	switch {
	case percent >= 90:
		return badStyle
	case percent >= 50:
		return warnStyle
	default:
		return normalStyle
	}
}

// gitStatusStyle colours a row by its porcelain status code.
func gitStatusStyle(status string) ui.Style {
	switch {
	case status == "??":
		return subtleStyle
	case strings.ContainsAny(status, "U"):
		return badStyle
	case strings.HasPrefix(status, "A"), strings.HasPrefix(status, "R"):
		return goodStyle
	case strings.ContainsAny(status, "D"):
		return badStyle
	default:
		return warnStyle
	}
}

// isPseudoFilesystem reports whether a mount point is a system volume rather
// than storage an operator cares about.
func isPseudoFilesystem(path string) bool {
	for _, prefix := range []string{
		"/dev", "/System/Volumes", "/private/var/vm", "/proc", "/sys", "/run",
		"/snap", "/var/lib/docker", "/System/Library",
	} {
		if path == prefix || strings.HasPrefix(path, prefix+"/") {
			return true
		}
	}
	return false
}

// shortMountPoint trims a mount point to a readable bar-chart label, keeping the
// final path element rather than an arbitrary tail of characters.
func shortMountPoint(path string) string {
	if len(path) <= 10 {
		return path
	}

	if idx := strings.LastIndex(path, "/"); idx > 0 && idx < len(path)-1 {
		last := path[idx+1:]
		if len(last) <= 10 {
			return last
		}
		return last[:9] + "…"
	}
	return path[:9] + "…"
}

// displayName prefers the configured endpoint name, falling back to its URL.
func displayName(name, url string) string {
	if name != "" {
		return name
	}
	return url
}

func formatResponseTime(metric models.EndpointMetrics) string {
	if metric.ResponseTime <= 0 {
		return "-"
	}
	return metric.ResponseTime.Round(time.Millisecond).String()
}

// gitRemoteLabel renders the configured remote and its URL.
func gitRemoteLabel(remote, url string) string {
	switch {
	case remote == "" && url == "":
		return "none"
	case url == "":
		return remote + " (not configured)"
	default:
		return remote + " " + url
	}
}

func onOff(v bool) string {
	if v {
		return "on"
	}
	return "off"
}
