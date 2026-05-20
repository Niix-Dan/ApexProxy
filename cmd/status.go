package cmd

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/niix-dan/apexproxy/internal/metrics"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/spf13/cobra"
)

type tickMsg time.Time
type responseMsg metrics.ProxyStats
type errorMsg error

var statusCmd = &cobra.Command{
	Use:   "status",
	Short: "Launches the interactive TUI to display real-time metrics",
	Run: func(cmd *cobra.Command, args []string) {
		p := tea.NewProgram(initialModel())
		if _, err := p.Run(); err != nil {
			fmt.Printf("Error running TUI: %v", err)
		}
	},
}

func init() {
	rootCmd.AddCommand(statusCmd)
}

type model struct {
	stats  metrics.ProxyStats
	err    error
	paused bool
}

func initialModel() model {
	return model{
		stats: metrics.ProxyStats{
			Routes:     []metrics.RouteStat{},
			RecentLogs: []metrics.LogEntry{},
		},
	}
}

func (m model) Init() tea.Cmd {
	return tea.Batch(fetchMetrics(), tick())
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "q", "ctrl+c":
			return m, tea.Quit
		case "r":
			return m, resetMetrics()
		case "p":
			m.paused = !m.paused
			return m, nil
		}
	case responseMsg:
		if !m.paused {
			m.stats = metrics.ProxyStats(msg)
		}
		m.err = nil
	case errorMsg:
		m.err = msg
	case tickMsg:
		if !m.paused {
			return m, tea.Batch(fetchMetrics(), tick())
		}
		return m, tick()
	}
	return m, nil
}

func (m model) View() string {
	var sb strings.Builder

	sb.WriteString("┌──────────────────────────────────────────────────────────────────────────────┐\n")
	statusStr := "[RUNNING]"
	if m.paused {
		statusStr = "[PAUSED] "
	}
	sb.WriteString(fmt.Sprintf("│  APEX PROXY STATUS  %s                       Uptime: %-16s │\n", statusStr, m.stats.Uptime))
	sb.WriteString("└──────────────────────────────────────────────────────────────────────────────┘\n")

	if m.err != nil {
		sb.WriteString(fmt.Sprintf("\n   Error connecting to proxy instance: %v\n\n", m.err))
		sb.WriteString(" ──────────────────────────────────────────────────────────────────────────────\n")
		sb.WriteString(" [q] Quit  \n")
		return sb.String()
	}

	sb.WriteString(" Global Metrics:\n")
	sb.WriteString(fmt.Sprintf("   Requests/sec:  %-12.2f │  Active Connections: %d\n", m.stats.ReqsPerSec, m.stats.ActiveConnections))
	sb.WriteString(fmt.Sprintf("   Avg Latency:   %-12s │  Memory Usage:       %s\n\n", m.stats.AvgLatency, m.stats.MemUsage))

	sb.WriteString(" Bandwidth:\n")
	sb.WriteString(fmt.Sprintf("   In: %-12s  Out: %s\n\n", m.stats.BandwidthIn, m.stats.BandwidthOut))

	total := m.stats.Codes2xx + m.stats.Codes3xx + m.stats.Codes4xx + m.stats.Codes5xx
	p2, p3, p4, p5 := 0, 0, 0, 0
	if total > 0 {
		p2 = int((m.stats.Codes2xx * 100) / total)
		p3 = int((m.stats.Codes3xx * 100) / total)
		p4 = int((m.stats.Codes4xx * 100) / total)
		p5 = int((m.stats.Codes5xx * 100) / total)
	}

	bar := func(pct int) string {
		filled := int(float64(pct) / 3.33)
		if filled > 30 {
			filled = 30
		}
		return strings.Repeat("█", filled)
	}

	sb.WriteString(" HTTP Response Codes:\n")
	sb.WriteString(fmt.Sprintf("   2xx: [%-30s] %3d%%  (%d req)\n", bar(p2), p2, m.stats.Codes2xx))
	sb.WriteString(fmt.Sprintf("   3xx: [%-30s] %3d%%  (%d req)\n", bar(p3), p3, m.stats.Codes3xx))
	sb.WriteString(fmt.Sprintf("   4xx: [%-30s] %3d%%  (%d req)\n", bar(p4), p4, m.stats.Codes4xx))
	sb.WriteString(fmt.Sprintf("   5xx: [%-30s] %3d%%  (%d req)\n\n", bar(p5), p5, m.stats.Codes5xx))

	sb.WriteString(" Active Routes:\n")
	sb.WriteString(fmt.Sprintf("   %-10s %-13s %-24s %-9s %-8s %s\n", "PATH", "STRATEGY", "TARGET", "HEALTH", "REQ", "ERRS(5xx)"))
	for _, route := range m.stats.Routes {
		sb.WriteString(fmt.Sprintf("   %-10s %-13s %-24s [%-4s]   %-8d %d\n",
			route.Path, route.Strategy, route.Target, route.Health, route.Reqs, route.Errors))
	}
	sb.WriteString("\n")

	sb.WriteString(" Recent Logs:\n")
	for _, logEntry := range m.stats.RecentLogs {
		sb.WriteString(fmt.Sprintf("   %s [%d] %s %s - %s - IP: %s\n",
			logEntry.Timestamp, logEntry.Status, logEntry.Method, logEntry.Path, logEntry.Latency, logEntry.IP))
	}
	if len(m.stats.RecentLogs) == 0 {
		sb.WriteString("   No requests recorded yet.\n")
	}

	sb.WriteString(" \n ──────────────────────────────────────────────────────────────────────────────\n")
	sb.WriteString(" [q] Quit  |  [r] Reset Metrics  |  [p] Pause Monitoring\n")

	return sb.String()
}

func newSignedRequest(method, url string) (*http.Request, error) {
	req, err := http.NewRequest(method, url, nil)
	if err != nil {
		return nil, err
	}
	metrics.SignRequest(req)
	return req, nil
}

func fetchMetrics() tea.Cmd {
	return func() tea.Msg {
		req, err := newSignedRequest(http.MethodGet, "http://127.0.0.1:9090/metrics")
		if err != nil {
			return errorMsg(err)
		}
		client := http.Client{Timeout: 500 * time.Millisecond}
		resp, err := client.Do(req)
		if err != nil {
			return errorMsg(err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			return errorMsg(fmt.Errorf("metrics endpoint returned %s", resp.Status))
		}
		var s metrics.ProxyStats
		if err := json.NewDecoder(resp.Body).Decode(&s); err != nil {
			return errorMsg(err)
		}
		return responseMsg(s)
	}
}

func resetMetrics() tea.Cmd {
	return func() tea.Msg {
		req, err := newSignedRequest(http.MethodGet, "http://127.0.0.1:9090/reset")
		if err != nil {
			return errorMsg(err)
		}
		client := http.Client{Timeout: 500 * time.Millisecond}
		client.Do(req) //nolint:errcheck
		return nil
	}
}

func tick() tea.Cmd {
	return tea.Tick(time.Second, func(t time.Time) tea.Msg {
		return tickMsg(t)
	})
}
