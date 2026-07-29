package ui

import (
	"fmt"
	"image"
	"strings"
	"time"

	"github.com/Teomazivila/maz-term/pkg/models"
	ui "github.com/gizak/termui/v3"
	"github.com/gizak/termui/v3/widgets"
)

// updateCloudTabData refreshes the Cloud tab.
func (a *App) updateCloudTabData() {
	tab := a.getTabByName("Cloud")
	if tab == nil {
		return
	}

	if len(tab.Widgets) == 0 {
		summary := newPanel("AWS")
		instances := newTable("Instances",
			[]string{"Instance", "Type", "Region", "State", "CPU%", "Uptime"},
			[]int{26, 14, 14, 12, 8, 0})
		storage := newTable("Storage and databases",
			[]string{"Resource", "Kind", "Region", "State"},
			[]int{34, 16, 14, 0})

		spark := widgets.NewSparkline()
		spark.LineColor = ui.ColorGreen
		history := widgets.NewSparklineGroup(spark)
		history.Title = "Running instances"
		history.BorderStyle.Fg = ui.ColorCyan

		tab.Widgets = []ui.Drawable{summary, instances, storage, history}
		tab.Panels = []*widgets.Paragraph{summary}
		tab.Tables = []*widgets.Table{instances, storage}
		tab.Sparklines = []*widgets.SparklineGroup{history}
	}

	if a.CloudCollector == nil {
		a.setProviderUnconfigured(tab, "AWS",
			"Enable it with:\n\n  cloud:\n    enabled: [\"aws\"]\n    aws:\n      region: eu-west-1\n\n"+
				"Credentials come from the AWS credential chain\n(AWS_* variables, a shared profile, or instance metadata).")
		return
	}

	metrics := a.CloudCollector.GetLatestMetrics()

	if len(tab.Panels) > 0 {
		var b strings.Builder
		fmt.Fprintf(&b, "regions   %s\n", strings.Join(metrics.Regions, ", "))
		fmt.Fprintf(&b, "instances %d (%s)\n", len(metrics.InstanceMetrics),
			countByStatus(metrics.InstanceMetrics))
		fmt.Fprintf(&b, "buckets   %d\n", len(metrics.StorageMetrics))
		fmt.Fprintf(&b, "databases %d\n", len(metrics.DatabaseMetrics))
		fmt.Fprintf(&b, "updated   %s\n", FormatTime(metrics.LastUpdated))

		// Failures are reported verbatim so a partial view is visibly partial.
		if len(metrics.Errors) > 0 {
			b.WriteString("\nproblems\n")
			for _, problem := range limitStrings(metrics.Errors, 6) {
				fmt.Fprintf(&b, "  %s\n", problem)
			}
		} else if metrics.LastUpdated.IsZero() {
			b.WriteString("\ncollecting...\n")
		}

		tab.Panels[0].Text = b.String()
	}

	if len(tab.Tables) > 0 {
		table := tab.Tables[0]
		rows := [][]string{{"Instance", "Type", "Region", "State", "CPU%", "Uptime"}}
		styles := map[int]ui.Style{0: headerStyle}

		for i, instance := range metrics.InstanceMetrics {
			cpu := "-"
			if instance.CPUUtilization > 0 {
				cpu = fmt.Sprintf("%.1f", instance.CPUUtilization)
			}
			uptime := "-"
			if instance.UptimeHours > 0 {
				uptime = FormatDuration(time.Duration(instance.UptimeHours) * time.Hour)
			}

			rows = append(rows, []string{
				TruncateString(instance.Name, 24),
				instance.Type,
				instance.Region,
				string(instance.Status),
				cpu,
				uptime,
			})
			styles[i+1] = resourceStatusStyle(instance.Status)
		}

		if len(metrics.InstanceMetrics) == 0 {
			rows = append(rows, []string{emptyLabel(metrics.Errors, "no instances"), "-", "-", "-", "-", "-"})
			styles[1] = subtleStyle
		}

		table.Rows = rows
		table.RowStyles = styles
	}

	if len(tab.Tables) > 1 {
		table := tab.Tables[1]
		rows := [][]string{{"Resource", "Kind", "Region", "State"}}
		styles := map[int]ui.Style{0: headerStyle}
		row := 1

		for _, bucket := range metrics.StorageMetrics {
			rows = append(rows, []string{
				TruncateString(bucket.Name, 32), bucket.Type, bucket.Region, "-",
			})
			styles[row] = normalStyle
			row++
		}
		for _, database := range metrics.DatabaseMetrics {
			rows = append(rows, []string{
				TruncateString(database.Name, 32),
				TruncateString(database.Engine, 14),
				database.Region,
				string(database.Status),
			})
			styles[row] = resourceStatusStyle(database.Status)
			row++
		}

		if row == 1 {
			rows = append(rows, []string{emptyLabel(metrics.Errors, "no storage or databases"), "-", "-", "-"})
			styles[1] = subtleStyle
		}

		table.Rows = rows
		table.RowStyles = styles
	}

	a.applyInfraSparkline(tab, func() ([]models.TimeSeriesPoint, error) {
		return a.Storage.GetCloudInstanceCountHistory(a.HistoryRange, sparklinePoints)
	})
}

// updateKubernetesTabData refreshes the Kubernetes tab.
func (a *App) updateKubernetesTabData() {
	tab := a.getTabByName("Kubernetes")
	if tab == nil {
		return
	}

	if len(tab.Widgets) == 0 {
		summary := newPanel("Cluster")
		pods := newTable("Pods",
			[]string{"Namespace", "Pod", "Status", "Restarts", "Node"},
			[]int{18, 34, 16, 10, 0})
		nodes := newTable("Nodes",
			[]string{"Node", "Status", "CPU%", "Mem%", "Version"},
			[]int{28, 22, 8, 8, 0})

		spark := widgets.NewSparkline()
		spark.LineColor = ui.ColorBlue
		history := widgets.NewSparklineGroup(spark)
		history.Title = "Running pods"
		history.BorderStyle.Fg = ui.ColorCyan

		tab.Widgets = []ui.Drawable{summary, pods, nodes, history}
		tab.Panels = []*widgets.Paragraph{summary}
		tab.Tables = []*widgets.Table{pods, nodes}
		tab.Sparklines = []*widgets.SparklineGroup{history}
	}

	if a.KubernetesCollector == nil {
		a.setProviderUnconfigured(tab, "Kubernetes",
			"Enable it with:\n\n  kubernetes:\n    enabled: true\n    namespaces: [\"default\"]\n\n"+
				"Credentials come from kubeconfig ($KUBECONFIG or\n~/.kube/config) or the in-cluster service account.")
		return
	}

	metrics := a.KubernetesCollector.GetLatestMetrics()
	summary := models.KubernetesSummaryFrom(metrics)

	if len(tab.Panels) > 0 {
		var b strings.Builder
		cluster := metrics.ClusterName
		if cluster == "" {
			cluster = "unknown"
		}
		fmt.Fprintf(&b, "cluster     %s\n", cluster)
		fmt.Fprintf(&b, "nodes       %d ready of %d\n", summary.NodesReady, summary.NodesTotal)
		fmt.Fprintf(&b, "pods        %d running, %d pending, %d failed\n",
			summary.PodsRunning, summary.PodsPending, summary.PodsFailed)
		fmt.Fprintf(&b, "deployments %d available of %d\n",
			summary.DeploymentsAvailable, summary.DeploymentsTotal)
		fmt.Fprintf(&b, "services    %d\n", len(metrics.Services))
		fmt.Fprintf(&b, "restarts    %d\n", summary.RestartsTotal)
		fmt.Fprintf(&b, "updated     %s\n", FormatTime(metrics.LastUpdated))

		if len(metrics.Errors) > 0 {
			b.WriteString("\nproblems\n")
			for _, problem := range limitStrings(metrics.Errors, 6) {
				fmt.Fprintf(&b, "  %s\n", problem)
			}
		}

		tab.Panels[0].Text = b.String()
	}

	if len(tab.Tables) > 0 {
		table := tab.Tables[0]
		rows := [][]string{{"Namespace", "Pod", "Status", "Restarts", "Node"}}
		styles := map[int]ui.Style{0: headerStyle}

		for i, pod := range limitPods(metrics.Pods, 200) {
			rows = append(rows, []string{
				TruncateString(pod.Namespace, 16),
				TruncateString(pod.Name, 32),
				TruncateString(pod.Status, 14),
				fmt.Sprintf("%d", pod.RestartCount),
				TruncateString(pod.Node, 30),
			})
			styles[i+1] = podStatusStyle(pod)
		}

		if len(metrics.Pods) == 0 {
			rows = append(rows, []string{emptyLabel(metrics.Errors, "no pods"), "-", "-", "-", "-"})
			styles[1] = subtleStyle
		}

		table.Rows = rows
		table.RowStyles = styles
	}

	if len(tab.Tables) > 1 {
		table := tab.Tables[1]
		rows := [][]string{{"Node", "Status", "CPU%", "Mem%", "Version"}}
		styles := map[int]ui.Style{0: headerStyle}

		for i, node := range metrics.Nodes {
			cpu, mem := "-", "-"
			if node.CPUUsage > 0 {
				cpu = fmt.Sprintf("%.0f", node.CPUUsage)
			}
			if node.MemoryUsage > 0 {
				mem = fmt.Sprintf("%.0f", node.MemoryUsage)
			}

			rows = append(rows, []string{
				TruncateString(node.Name, 26),
				TruncateString(node.Status, 20),
				cpu,
				mem,
				node.KubeletVersion,
			})

			if strings.HasPrefix(node.Status, "Ready") {
				styles[i+1] = goodStyle
			} else {
				styles[i+1] = badStyle
			}
		}

		if len(metrics.Nodes) == 0 {
			rows = append(rows, []string{emptyLabel(metrics.Errors, "no nodes"), "-", "-", "-", "-"})
			styles[1] = subtleStyle
		}

		table.Rows = rows
		table.RowStyles = styles
	}

	a.applyInfraSparkline(tab, func() ([]models.TimeSeriesPoint, error) {
		return a.Storage.GetKubernetesPodCountHistory(a.HistoryRange, sparklinePoints)
	})
}

// updateCICDTabData refreshes the CI/CD tab.
func (a *App) updateCICDTabData() {
	tab := a.getTabByName("CI/CD")
	if tab == nil {
		return
	}

	if len(tab.Widgets) == 0 {
		summary := newPanel("GitHub Actions")
		workflows := newTable("Workflows",
			[]string{"Repository", "Workflow", "Last run", "Success", "Mean", "When"},
			[]int{22, 26, 12, 10, 10, 0})
		runs := newTable("Recent runs",
			[]string{"Workflow", "Status", "Branch", "Trigger", "Duration", "When"},
			[]int{24, 12, 20, 12, 10, 0})

		spark := widgets.NewSparkline()
		spark.LineColor = ui.ColorGreen
		history := widgets.NewSparklineGroup(spark)
		history.Title = "Success rate %"
		history.BorderStyle.Fg = ui.ColorCyan

		tab.Widgets = []ui.Drawable{summary, workflows, runs, history}
		tab.Panels = []*widgets.Paragraph{summary}
		tab.Tables = []*widgets.Table{workflows, runs}
		tab.Sparklines = []*widgets.SparklineGroup{history}
	}

	if a.CICDCollector == nil {
		a.setProviderUnconfigured(tab, "GitHub Actions",
			"Enable it with:\n\n  cicd:\n    enabled: [\"github\"]\n    github:\n      owner: your-org\n      repositories: [\"your-repo\"]\n\n"+
				"The API token is read from MAZTERM_GITHUB_TOKEN\nor GITHUB_TOKEN, never from this file.")
		return
	}

	metrics := a.CICDCollector.GetLatestMetrics()

	if len(tab.Panels) > 0 {
		var b strings.Builder
		fmt.Fprintf(&b, "workflows    %d (%d active)\n",
			metrics.Summary.TotalWorkflows, metrics.Summary.ActiveWorkflows)
		fmt.Fprintf(&b, "success rate %.1f%%\n", metrics.Summary.SuccessRate)
		fmt.Fprintf(&b, "failing      %d\n", metrics.Summary.FailedWorkflows)
		fmt.Fprintf(&b, "in progress  %d\n", metrics.Summary.RunningJobs)
		if metrics.Summary.AverageDuration > 0 {
			fmt.Fprintf(&b, "mean run     %s\n", metrics.Summary.AverageDuration.Round(time.Second))
		}
		fmt.Fprintf(&b, "updated      %s\n", FormatTime(metrics.LastUpdated))

		// The remaining API quota is worth showing rather than hiding, since
		// exhausting it is the usual reason data stops refreshing.
		if remaining, limit, reset := a.CICDCollector.RateLimit(); limit > 0 {
			fmt.Fprintf(&b, "api quota    %d of %d", remaining, limit)
			if !reset.IsZero() {
				fmt.Fprintf(&b, ", resets %s", reset.Format("15:04"))
			}
			b.WriteString("\n")
		}

		if len(metrics.Errors) > 0 {
			b.WriteString("\nproblems\n")
			for _, problem := range limitStrings(metrics.Errors, 6) {
				fmt.Fprintf(&b, "  %s\n", problem)
			}
		}

		tab.Panels[0].Text = b.String()
	}

	if len(tab.Tables) > 0 {
		table := tab.Tables[0]
		rows := [][]string{{"Repository", "Workflow", "Last run", "Success", "Mean", "When"}}
		styles := map[int]ui.Style{0: headerStyle}

		for i, workflow := range metrics.Workflows {
			mean := "-"
			if workflow.AverageDuration > 0 {
				mean = workflow.AverageDuration.Round(time.Second).String()
			}
			rate := "-"
			if workflow.RunCount > 0 {
				rate = fmt.Sprintf("%.0f%%", workflow.SuccessRate)
			}

			rows = append(rows, []string{
				TruncateString(workflow.Repository, 20),
				TruncateString(workflow.Name, 24),
				string(orUnknown(workflow.LastRunStatus)),
				rate,
				mean,
				FormatTime(workflow.LastRunTime),
			})
			styles[i+1] = cicdStatusStyle(workflow.LastRunStatus)
		}

		if len(metrics.Workflows) == 0 {
			rows = append(rows, []string{emptyLabel(metrics.Errors, "no workflows"), "-", "-", "-", "-", "-"})
			styles[1] = subtleStyle
		}

		table.Rows = rows
		table.RowStyles = styles
	}

	if len(tab.Tables) > 1 {
		table := tab.Tables[1]
		rows := [][]string{{"Workflow", "Status", "Branch", "Trigger", "Duration", "When"}}
		styles := map[int]ui.Style{0: headerStyle}
		row := 1

		for _, workflow := range metrics.Workflows {
			for _, run := range workflow.RecentRuns {
				if row > 60 {
					break
				}
				duration := "-"
				if run.Duration > 0 {
					duration = run.Duration.Round(time.Second).String()
				}
				rows = append(rows, []string{
					TruncateString(workflow.Name, 22),
					string(run.Status),
					TruncateString(run.Branch, 18),
					TruncateString(run.Trigger, 10),
					duration,
					FormatTime(run.StartTime),
				})
				styles[row] = cicdStatusStyle(run.Status)
				row++
			}
		}

		if row == 1 {
			rows = append(rows, []string{emptyLabel(metrics.Errors, "no runs"), "-", "-", "-", "-", "-"})
			styles[1] = subtleStyle
		}

		table.Rows = rows
		table.RowStyles = styles
	}

	a.applyInfraSparkline(tab, func() ([]models.TimeSeriesPoint, error) {
		return a.Storage.GetCICDSuccessRateHistory(a.HistoryRange, sparklinePoints)
	})
}

// setProviderUnconfigured fills a provider tab with instructions instead of data.
//
// It never claims a connection: the collector is nil precisely because nothing
// was configured.
func (a *App) setProviderUnconfigured(tab *Tab, provider, instructions string) {
	if len(tab.Panels) > 0 {
		tab.Panels[0].Text = provider + " is not configured.\n\n" + instructions
	}
	for _, table := range tab.Tables {
		if len(table.Rows) > 0 {
			header := table.Rows[0]
			blank := make([]string, len(header))
			for i := range blank {
				blank[i] = "-"
			}
			blank[0] = "not configured"
			table.Rows = [][]string{header, blank}
			table.RowStyles = map[int]ui.Style{0: headerStyle, 1: subtleStyle}
		}
	}
}

// applyInfraSparkline loads a persisted summary series into a tab's sparkline.
func (a *App) applyInfraSparkline(tab *Tab, load func() ([]models.TimeSeriesPoint, error)) {
	if a.Storage == nil || len(tab.Sparklines) == 0 || len(tab.Sparklines[0].Sparklines) == 0 {
		return
	}

	points, err := load()
	if err != nil {
		return
	}

	values := make([]float64, 0, len(points))
	for _, point := range points {
		values = append(values, point.Value)
	}
	tab.Sparklines[0].Sparklines[0].Data = values
}

// layoutProviderTab arranges a provider tab: summary beside the primary table,
// with the secondary table and history below.
func (a *App) layoutProviderTab(tab *Tab, rect image.Rectangle) []ui.Drawable {
	rows := splitRows(rect, 0.46, 0.38, 0.16)
	top := splitCols(rows[0], 0.34, 0.66)

	rects := []image.Rectangle{top[0], top[1], rows[1], rows[2]}
	return compact(tab.Widgets, rects)
}

// countByStatus summarises instance states for the header line.
func countByStatus(instances []models.CloudInstanceMetrics) string {
	if len(instances) == 0 {
		return "none"
	}

	counts := make(map[models.ResourceStatus]int, 4)
	for _, instance := range instances {
		counts[instance.Status]++
	}

	parts := make([]string, 0, len(counts))
	for _, status := range []models.ResourceStatus{
		models.ResourceStatusRunning,
		models.ResourceStatusStopped,
		models.ResourceStatusPending,
		models.ResourceStatusError,
	} {
		if counts[status] > 0 {
			parts = append(parts, fmt.Sprintf("%d %s", counts[status], status))
		}
	}

	return strings.Join(parts, ", ")
}

// emptyLabel distinguishes "nothing found" from "could not look".
func emptyLabel(errs []string, whenFine string) string {
	if len(errs) > 0 {
		return "unavailable"
	}
	return whenFine
}

// limitStrings caps a list, noting how many were omitted.
func limitStrings(values []string, limit int) []string {
	if len(values) <= limit {
		return values
	}
	out := append([]string(nil), values[:limit]...)
	return append(out, fmt.Sprintf("... and %d more", len(values)-limit))
}

// limitPods caps the pod table.
func limitPods(pods []models.KubernetesPodMetrics, limit int) []models.KubernetesPodMetrics {
	if len(pods) <= limit {
		return pods
	}
	return pods[:limit]
}

func resourceStatusStyle(status models.ResourceStatus) ui.Style {
	switch status {
	case models.ResourceStatusRunning:
		return goodStyle
	case models.ResourceStatusPending:
		return warnStyle
	case models.ResourceStatusStopped:
		return subtleStyle
	default:
		return badStyle
	}
}

// podStatusStyle colours a pod row by the signal that matters: a pod whose phase
// is fine but whose containers are crash-looping is not healthy.
func podStatusStyle(pod models.KubernetesPodMetrics) ui.Style {
	switch pod.Phase {
	case "Running":
		if pod.Status != "Running" {
			return badStyle
		}
		for _, container := range pod.Containers {
			if !container.Ready {
				return warnStyle
			}
		}
		return goodStyle
	case "Succeeded":
		return subtleStyle
	case "Pending":
		return warnStyle
	default:
		return badStyle
	}
}

func cicdStatusStyle(status models.CICDStatus) ui.Style {
	switch status {
	case models.CICDStatusSuccess:
		return goodStyle
	case models.CICDStatusFailure:
		return badStyle
	case models.CICDStatusRunning, models.CICDStatusPending:
		return warnStyle
	default:
		return subtleStyle
	}
}

// orUnknown renders an unset status readably.
func orUnknown(status models.CICDStatus) models.CICDStatus {
	if status == "" {
		return "unknown"
	}
	return status
}
