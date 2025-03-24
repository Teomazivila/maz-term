package ui

import (
	"context"
	"fmt"
	"time"

	"github.com/Teomazivila/maz-term/pkg/collector"
	"github.com/Teomazivila/maz-term/pkg/models"
	"github.com/charmbracelet/bubbles/table"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// GitPanel displays Git repository status
type GitPanel struct {
	collector    *collector.GitStatusCollector
	metrics      models.GitRepoMetrics
	width        int
	height       int
	ctx          context.Context
	cancelFunc   context.CancelFunc
	subscription chan models.GitRepoMetrics
	headerStyle  lipgloss.Style
	tableStyle   lipgloss.Style
	detailsTable table.Model
	repoPath     string
}

// NewGitPanel creates a new Git repository status panel
func NewGitPanel(repoPath string) *GitPanel {
	ctx, cancel := context.WithCancel(context.Background())
	c := collector.NewGitStatusCollector(repoPath)

	headerStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("#FFFDF5")).
		Background(lipgloss.Color("#F14E32")). // Git logo color
		Padding(0, 1)

	tableStyle := lipgloss.NewStyle().
		BorderStyle(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("240"))

	// Initialize details table
	columns := []table.Column{
		{Title: "Property", Width: 20},
		{Title: "Value", Width: 40},
	}

	rows := []table.Row{
		{"Repository", ""},
		{"Branch", ""},
		{"Commit Count", "0"},
		{"Last Commit", ""},
		{"Modified Files", "0"},
		{"Pending Commits", "0"},
	}

	detailsTable := table.New(
		table.WithColumns(columns),
		table.WithRows(rows),
		table.WithFocused(false),
		table.WithHeight(len(rows)),
	)

	detailsTable.SetStyles(table.Styles{
		Header:   lipgloss.NewStyle().Bold(true),
		Selected: lipgloss.NewStyle().Foreground(lipgloss.Color("229")).Bold(true),
		Cell:     lipgloss.NewStyle().Padding(0, 1),
	})

	panel := &GitPanel{
		collector:    c,
		ctx:          ctx,
		cancelFunc:   cancel,
		headerStyle:  headerStyle,
		tableStyle:   tableStyle,
		detailsTable: detailsTable,
		repoPath:     repoPath,
	}

	// Start the collector
	go c.Start(ctx, 5*time.Second)

	// Subscribe to metrics updates
	panel.subscription = c.Subscribe()

	// Start a goroutine to process metrics updates
	go panel.processMetricsUpdates()

	return panel
}

// Init initializes the panel
func (p *GitPanel) Init() tea.Cmd {
	// No initialization commands needed
	return nil
}

// Title returns the panel title
func (p *GitPanel) Title() string {
	return fmt.Sprintf("Git Repository: %s", p.metrics.Name)
}

// SetSize sets the panel size
func (p *GitPanel) SetSize(width, height int) {
	p.width = width
	p.height = height

	// Update table height
	p.detailsTable.SetHeight(height - 4) // Account for borders and header

	// Update column widths
	columns := p.detailsTable.Columns()
	if len(columns) >= 2 {
		propWidth := width / 3
		valueWidth := width - propWidth - 4 // Account for borders and padding

		columns[0].Width = propWidth
		columns[1].Width = valueWidth
		p.detailsTable.SetColumns(columns)
	}
}

// Update updates the panel
func (p *GitPanel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	p.detailsTable, cmd = p.detailsTable.Update(msg)
	return p, cmd
}

// View renders the panel
func (p *GitPanel) View() string {
	// Ensure we have minimum dimensions
	availWidth := MaxInt(p.width, 80)
	availHeight := MaxInt(p.height, 24)

	header := p.headerStyle.Render(p.Title())

	// Adjust table style to take up available space
	contentStyle := p.tableStyle.Copy().
		Width(availWidth - 4).  // Account for borders
		Height(availHeight - 6) // Account for header and update info

	content := contentStyle.Render(p.detailsTable.View())

	// Add last update time
	updateInfo := fmt.Sprintf("Last updated: %s", time.Now().Format("15:04:05"))
	if !p.metrics.LastCommit.IsZero() {
		updateInfo = fmt.Sprintf("Last updated: %s", p.metrics.LastCommit.Format("15:04:05"))
	}

	updateStyle := lipgloss.NewStyle().
		Italic(true).
		Foreground(lipgloss.Color("#AAAAAA")).
		Align(lipgloss.Right).
		Width(availWidth - 4)

	updateInfo = updateStyle.Render(updateInfo)

	return lipgloss.JoinVertical(lipgloss.Left, header, content, updateInfo)
}

// Close properly cleans up resources used by the panel
func (p *GitPanel) Close() {
	p.cancelFunc()
	p.collector.Unsubscribe(p.subscription)
	p.collector.Stop()
}

// processMetricsUpdates processes metrics updates from the subscription channel
func (p *GitPanel) processMetricsUpdates() {
	for metrics := range p.subscription {
		p.metrics = metrics
		p.updateTable()
	}
}

// updateTable updates the table with the latest metrics
func (p *GitPanel) updateTable() {
	rows := []table.Row{
		{"Repository", p.metrics.Name},
		{"Branch", p.metrics.Branch},
		{"Commit Count", fmt.Sprintf("%d", p.metrics.CommitCount)},
	}

	// Format last commit time
	lastCommitStr := "No commits yet"
	if !p.metrics.LastCommit.IsZero() {
		lastCommitStr = p.metrics.LastCommit.Format("2006-01-02 15:04:05")
	}
	rows = append(rows, table.Row{"Last Commit", lastCommitStr})

	// Add modified files count with color coding
	modifiedFilesStr := fmt.Sprintf("%d", p.metrics.ModifiedFiles)
	if p.metrics.ModifiedFiles > 0 {
		modifiedFilesStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#FFCC00"))
		modifiedFilesStr = modifiedFilesStyle.Render(modifiedFilesStr)
	}
	rows = append(rows, table.Row{"Modified Files", modifiedFilesStr})

	// Add pending commits count with color coding
	pendingCommitsStr := fmt.Sprintf("%d", p.metrics.PendingCommits)
	if p.metrics.PendingCommits > 0 {
		pendingCommitsStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#FFCC00"))
		pendingCommitsStr = pendingCommitsStyle.Render(pendingCommitsStr)
	}
	rows = append(rows, table.Row{"Pending Commits", pendingCommitsStr})

	p.detailsTable.SetRows(rows)
}
