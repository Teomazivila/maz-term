package ui

import (
	"fmt"
	"image"
	"strconv"
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
			Column{Title: "INSTANCE", Weight: 3},
			Column{Title: "TYPE", Width: 13},
			Column{Title: "REGION", Width: 14},
			Column{Title: "STATE", Width: 10},
			Column{Title: "CPU%", Width: 6, Align: AlignRight},
			Column{Title: "UPTIME", Width: 8, Align: AlignRight})
		storage := newTable("Storage and databases",
			Column{Title: "RESOURCE", Weight: 3},
			Column{Title: "KIND", Width: 16},
			Column{Title: "REGION", Width: 14},
			Column{Title: "STATE", Weight: 1})

		spark := widgets.NewSparkline()
		spark.LineColor = ui.ColorGreen
		history := widgets.NewSparklineGroup(spark)
		history.Title = "Running instances"
		history.BorderStyle.Fg = ui.ColorCyan

		tab.Widgets = []ui.Drawable{summary, instances, storage, history}
		tab.Panels = []*widgets.Paragraph{summary}
		tab.Tables = []*DataTable{instances, storage}
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

		// Which local credentials resolved, so the operator can confirm at a
		// glance that the dashboard is pointed at the account they expect.
		identity := a.CloudCollector.Identity()
		if identity.Profile != "" {
			fmt.Fprintf(&b, "profile   %s\n", identity.Profile)
		}
		if identity.Source != "" {
			fmt.Fprintf(&b, "creds     %s\n", TruncateString(identity.Source, 48))
		}

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
		rows := make([]Row, 0, len(metrics.InstanceMetrics))

		for _, instance := range metrics.InstanceMetrics {
			cpu := "-"
			if instance.CPUUtilization > 0 {
				cpu = fmt.Sprintf("%.1f", instance.CPUUtilization)
			}
			uptime := "-"
			if instance.UptimeHours > 0 {
				uptime = FormatDuration(time.Duration(instance.UptimeHours) * time.Hour)
			}

			rows = append(rows, styledRow(resourceStatusStyle(instance.Status),
				instance.Name, instance.Type, instance.Region,
				string(instance.Status), cpu, uptime))
		}

		if len(rows) == 0 {
			rows = noticeRow(6, emptyLabel(metrics.Errors, "no instances"))
		}
		table.SetRows(rows)
	}

	if len(tab.Tables) > 1 {
		table := tab.Tables[1]
		rows := make([]Row, 0, len(metrics.StorageMetrics)+len(metrics.DatabaseMetrics))

		for _, bucket := range metrics.StorageMetrics {
			rows = append(rows, textRow(bucket.Name, bucket.Type, bucket.Region, "-"))
		}
		for _, database := range metrics.DatabaseMetrics {
			rows = append(rows, styledRow(resourceStatusStyle(database.Status),
				database.Name, database.Engine, database.Region, string(database.Status)))
		}

		if len(rows) == 0 {
			rows = noticeRow(4, emptyLabel(metrics.Errors, "no storage or databases"))
		}
		table.SetRows(rows)
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
			Column{Title: "NAMESPACE", Weight: 2},
			Column{Title: "POD", Weight: 4},
			Column{Title: "READY", Width: 6, Align: AlignRight},
			Column{Title: "STATUS", Width: 18},
			Column{Title: "RESTARTS", Width: 9, Align: AlignRight},
			Column{Title: "AGE", Width: 6, Align: AlignRight},
			Column{Title: "NODE", Weight: 3})
		nodes := newTable("Nodes",
			Column{Title: "NODE", Weight: 3},
			Column{Title: "STATUS", Width: 22},
			Column{Title: "CPU%", Width: 6, Align: AlignRight},
			Column{Title: "MEM%", Width: 6, Align: AlignRight},
			Column{Title: "PODS", Width: 6, Align: AlignRight},
			Column{Title: "VERSION", Weight: 1})

		spark := widgets.NewSparkline()
		spark.LineColor = ui.ColorBlue
		history := widgets.NewSparklineGroup(spark)
		history.Title = "Running pods"
		history.BorderStyle.Fg = ui.ColorCyan

		tab.Widgets = []ui.Drawable{summary, pods, nodes, history}
		tab.Panels = []*widgets.Paragraph{summary}
		tab.Tables = []*DataTable{pods, nodes}
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

		identity := a.KubernetesCollector.Identity()
		if identity.User != "" {
			fmt.Fprintf(&b, "user        %s\n", TruncateString(identity.User, 40))
		}
		if identity.Server != "" {
			fmt.Fprintf(&b, "server      %s\n", TruncateString(identity.Server, 40))
		}
		if identity.Source != "" {
			fmt.Fprintf(&b, "kubeconfig  %s\n", TruncateString(identity.Source, 40))
		}
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
		rows := make([]Row, 0, len(metrics.Pods))

		for _, pod := range metrics.Pods {
			ready, total := 0, len(pod.Containers)
			for _, container := range pod.Containers {
				if container.Ready {
					ready++
				}
			}
			readyCell := "-"
			if total > 0 {
				readyCell = fmt.Sprintf("%d/%d", ready, total)
			}

			age := "-"
			if !pod.StartTime.IsZero() {
				age = FormatDuration(time.Since(pod.StartTime))
			}

			rows = append(rows, styledRow(podStatusStyle(pod),
				pod.Namespace, pod.Name, readyCell, pod.Status,
				strconv.Itoa(pod.RestartCount), age, pod.Node))
		}

		if len(rows) == 0 {
			rows = noticeRow(7, emptyLabel(metrics.Errors, "no pods"))
		}
		table.SetRows(rows)
		table.Title = fmt.Sprintf("Pods (%d)", len(metrics.Pods))
	}

	if len(tab.Tables) > 1 {
		table := tab.Tables[1]
		rows := make([]Row, 0, len(metrics.Nodes))

		for _, node := range metrics.Nodes {
			cpu, mem := "-", "-"
			if node.CPUUsage > 0 {
				cpu = fmt.Sprintf("%.0f", node.CPUUsage)
			}
			if node.MemoryUsage > 0 {
				mem = fmt.Sprintf("%.0f", node.MemoryUsage)
			}

			style := badStyle
			if strings.HasPrefix(node.Status, "Ready") {
				style = goodStyle
			}

			rows = append(rows, styledRow(style,
				node.Name, node.Status, cpu, mem,
				strconv.Itoa(node.AllocatablePods), node.KubeletVersion))
		}

		if len(rows) == 0 {
			rows = noticeRow(6, emptyLabel(metrics.Errors, "no nodes"))
		}
		table.SetRows(rows)
		table.Title = fmt.Sprintf("Nodes (%d)", len(metrics.Nodes))
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
			Column{Title: "REPOSITORY", Weight: 2},
			Column{Title: "WORKFLOW", Weight: 2},
			Column{Title: "LAST RUN", Width: 10},
			Column{Title: "SUCCESS", Width: 8, Align: AlignRight},
			Column{Title: "MEAN", Width: 8, Align: AlignRight},
			Column{Title: "WHEN", Weight: 1})
		runs := newTable("Recent runs",
			Column{Title: "WORKFLOW", Weight: 2},
			Column{Title: "STATUS", Width: 10},
			Column{Title: "BRANCH", Weight: 2},
			Column{Title: "TRIGGER", Width: 14},
			Column{Title: "DURATION", Width: 9, Align: AlignRight},
			Column{Title: "WHEN", Weight: 1})

		spark := widgets.NewSparkline()
		spark.LineColor = ui.ColorGreen
		history := widgets.NewSparklineGroup(spark)
		history.Title = "Success rate %"
		history.BorderStyle.Fg = ui.ColorCyan

		tab.Widgets = []ui.Drawable{summary, workflows, runs, history}
		tab.Panels = []*widgets.Paragraph{summary}
		tab.Tables = []*DataTable{workflows, runs}
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
		rows := make([]Row, 0, len(metrics.Workflows))

		for _, workflow := range metrics.Workflows {
			mean := "-"
			if workflow.AverageDuration > 0 {
				mean = workflow.AverageDuration.Round(time.Second).String()
			}
			rate := "-"
			if workflow.RunCount > 0 {
				rate = fmt.Sprintf("%.0f%%", workflow.SuccessRate)
			}

			rows = append(rows, styledRow(cicdStatusStyle(workflow.LastRunStatus),
				workflow.Repository, workflow.Name,
				string(orUnknown(workflow.LastRunStatus)), rate, mean,
				FormatTime(workflow.LastRunTime)))
		}

		if len(rows) == 0 {
			rows = noticeRow(6, emptyLabel(metrics.Errors, "no workflows"))
		}
		table.SetRows(rows)
	}

	if len(tab.Tables) > 1 {
		table := tab.Tables[1]
		rows := make([]Row, 0, 64)

		for _, workflow := range metrics.Workflows {
			for _, run := range workflow.RecentRuns {
				duration := "-"
				if run.Duration > 0 {
					duration = run.Duration.Round(time.Second).String()
				}
				rows = append(rows, styledRow(cicdStatusStyle(run.Status),
					workflow.Name, string(run.Status), run.Branch,
					run.Trigger, duration, FormatTime(run.StartTime)))
			}
		}

		if len(rows) == 0 {
			rows = noticeRow(6, emptyLabel(metrics.Errors, "no runs"))
		}
		table.SetRows(rows)
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
		table.SetRows(noticeRow(max(len(table.Columns), 1), "not configured"))
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
