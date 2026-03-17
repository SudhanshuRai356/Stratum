package tui

import (
	"context"
	"fmt"
	"math"
	"os"
	"strings"
	"time"

	"github.com/Kushal-39/Stratum/internal/api"
	"github.com/Kushal-39/Stratum/internal/app"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type model struct {
	state         api.TorrentState
	logs          []logEntry
	controls      chan<- app.ControlCommand
	activeTab     int
	width         int
	height        int
	downloadTrend []float64
	uploadTrend   []float64
	peerTrend     []int
}

type logEntry struct {
	when     time.Time
	event    app.EventType
	severity app.EventSeverity
	text     string
}

var (
	titleStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("230")).Background(lipgloss.Color("24")).Padding(0, 1)
	panelStyle = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("240")).Padding(0, 1)
	helpStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("246"))
	warnStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("214"))
	errStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("196")).Bold(true)
	goodStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("82")).Bold(true)
	infoStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("39"))
)

func newModel(controls chan<- app.ControlCommand) model {
	return model{
		controls:      controls,
		activeTab:     0,
		downloadTrend: make([]float64, 0, 40),
		uploadTrend:   make([]float64, 0, 40),
		peerTrend:     make([]int, 0, 40),
	}
}

func (m model) Init() tea.Cmd {
	return nil
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "q", "ctrl+c":
			if m.controls != nil {
				select {
				case m.controls <- app.ControlCommand{Type: app.CmdStop}:
				default:
				}
			}
			return m, tea.Quit
		case "p":
			if m.controls != nil {
				select {
				case m.controls <- app.ControlCommand{Type: app.CmdPause}:
				default:
				}
			}
		case "r":
			if m.controls != nil {
				select {
				case m.controls <- app.ControlCommand{Type: app.CmdResume}:
				default:
				}
			}
		case "a":
			if m.controls != nil {
				select {
				case m.controls <- app.ControlCommand{Type: app.CmdReannounce}:
				default:
				}
			}
		case "left", "h":
			m.activeTab = (m.activeTab + 3) % 4
		case "right", "l", "tab":
			m.activeTab = (m.activeTab + 1) % 4
		case "1":
			m.activeTab = 0
		case "2":
			m.activeTab = 1
		case "3":
			m.activeTab = 2
		case "4":
			m.activeTab = 3
		}
	case app.RuntimeEvent:
		if msg.Type == app.EventState {
			m.state = msg.State
		}
		if msg.DownloadBPS > 0 || m.state.DownloadSpeed > 0 {
			m.downloadTrend = appendTrend(m.downloadTrend, coalesce(msg.DownloadBPS, m.state.DownloadSpeed), 40)
		}
		if msg.UploadBPS > 0 {
			m.uploadTrend = appendTrend(m.uploadTrend, msg.UploadBPS, 40)
		}
		if msg.ActivePeers > 0 || m.state.ConnectedPeers > 0 {
			peers := msg.ActivePeers
			if peers == 0 {
				peers = m.state.ConnectedPeers
			}
			m.peerTrend = appendIntTrend(m.peerTrend, peers, 40)
		}
		if msg.Message != "" {
			sev := msg.Severity
			if sev == "" {
				sev = app.SeverityInfo
			}
			m.logs = append(m.logs, logEntry{when: msg.Timestamp, event: msg.Type, severity: sev, text: msg.Message})
			if len(m.logs) > 16 {
				m.logs = m.logs[len(m.logs)-16:]
			}
		}
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
	}
	return m, nil
}

func (m model) View() string {
	title := titleStyle.Render("STRATUM TELEMETRY")
	tabs := renderTabs(m.activeTab)

	var body string
	switch m.activeTab {
	case 0:
		body = m.renderOverview()
	case 1:
		body = m.renderPeers()
	case 2:
		body = m.renderPieces()
	default:
		body = m.renderNetwork()
	}

	help := helpStyle.Render("Controls: p pause | r resume | a reannounce | 1-4 tabs | tab/left/right switch | q quit")
	return strings.Join([]string{title, tabs, body, help}, "\n")
}

func (m model) renderOverview() string {
	status := infoStyle.Render(strings.ToUpper(coalesceText(m.state.State, "initializing")))
	if strings.EqualFold(m.state.State, "Seeding") {
		status = goodStyle.Render("SEEDING")
	}
	if strings.Contains(strings.ToLower(m.state.State), "error") {
		status = errStyle.Render(strings.ToUpper(m.state.State))
	}

	progressBar := renderProgressBar(m.state.Progress, 40)
	stats := []string{
		fmt.Sprintf("Torrent: %s", coalesceText(m.state.Name, "(loading metadata)")),
		fmt.Sprintf("State: %s", status),
		fmt.Sprintf("Progress: %s %.2f%%", progressBar, m.state.Progress),
		fmt.Sprintf("Download: %s/s", formatRate(m.state.DownloadSpeed)),
		fmt.Sprintf("Peers: %d", m.state.ConnectedPeers),
		fmt.Sprintf("DL Trend: %s", renderSparkline(m.downloadTrend, 28)),
		fmt.Sprintf("UL Trend: %s", renderSparkline(m.uploadTrend, 28)),
	}

	left := panelStyle.Width(72).Render(strings.Join(stats, "\n"))
	right := panelStyle.Width(72).Render(m.renderLogLines(12))
	return lipgloss.JoinHorizontal(lipgloss.Top, left, right)
}

func (m model) renderPeers() string {
	lines := []string{
		fmt.Sprintf("Connected peers: %d", m.state.ConnectedPeers),
		fmt.Sprintf("Peer load trend: %s", renderIntSparkline(m.peerTrend, 48)),
		"",
		"Recent peer-impact events:",
		m.renderFilteredLogs(12, app.EventPeerBanned),
	}
	return panelStyle.Width(146).Render(strings.Join(lines, "\n"))
}

func (m model) renderPieces() string {
	completed := int(math.Round(m.state.Progress))
	lines := []string{
		"Piece telemetry",
		fmt.Sprintf("Approx completion: %d%%", completed),
		fmt.Sprintf("Progress bar: %s", renderProgressBar(m.state.Progress, 60)),
		"",
		"Recent piece events:",
		m.renderFilteredLogs(12, app.EventPieceCompleted, app.EventPieceFailed),
	}
	return panelStyle.Width(146).Render(strings.Join(lines, "\n"))
}

func (m model) renderNetwork() string {
	lines := []string{
		"Network and control log",
		fmt.Sprintf("Current download rate: %s/s", formatRate(m.state.DownloadSpeed)),
		fmt.Sprintf("Current peers: %d", m.state.ConnectedPeers),
		"",
		m.renderLogLines(14),
	}
	return panelStyle.Width(146).Render(strings.Join(lines, "\n"))
}

func (m model) renderLogLines(limit int) string {
	if len(m.logs) == 0 {
		return "(no events yet)"
	}

	start := len(m.logs) - limit
	if start < 0 {
		start = 0
	}

	lines := make([]string, 0, limit)
	for _, entry := range m.logs[start:] {
		stamp := entry.when.Format("15:04:05")
		prefix := "INFO"
		switch entry.severity {
		case app.SeverityWarn:
			prefix = "WARN"
		case app.SeverityError:
			prefix = "ERR "
		}
		line := fmt.Sprintf("[%s] %s %s", stamp, prefix, entry.text)
		if entry.severity == app.SeverityWarn {
			line = warnStyle.Render(line)
		}
		if entry.severity == app.SeverityError {
			line = errStyle.Render(line)
		}
		lines = append(lines, line)
	}

	return strings.Join(lines, "\n")
}

func (m model) renderFilteredLogs(limit int, eventTypes ...app.EventType) string {
	if len(m.logs) == 0 {
		return "(no matching events)"
	}
	if len(eventTypes) == 0 {
		return m.renderLogLines(limit)
	}

	typeSet := make(map[string]struct{}, len(eventTypes))
	for _, t := range eventTypes {
		typeSet[string(t)] = struct{}{}
	}

	filtered := make([]logEntry, 0, len(m.logs))
	for _, l := range m.logs {
		if _, ok := typeSet[string(l.event)]; ok {
			filtered = append(filtered, l)
		}
	}
	if len(filtered) == 0 {
		return "(no matching events in current buffer)"
	}

	start := len(filtered) - limit
	if start < 0 {
		start = 0
	}

	lines := make([]string, 0, limit)
	for _, entry := range filtered[start:] {
		stamp := entry.when.Format("15:04:05")
		lines = append(lines, fmt.Sprintf("[%s] %s", stamp, entry.text))
	}
	return strings.Join(lines, "\n")
}

func renderTabs(active int) string {
	names := []string{"Overview", "Peers", "Pieces", "Network"}
	items := make([]string, 0, len(names))
	for i, n := range names {
		if i == active {
			items = append(items, goodStyle.Render("["+n+"]"))
			continue
		}
		items = append(items, lipgloss.NewStyle().Foreground(lipgloss.Color("245")).Render(n))
	}
	return strings.Join(items, "  ")
}

func renderProgressBar(percent float64, width int) string {
	if width < 10 {
		width = 10
	}
	if percent < 0 {
		percent = 0
	}
	if percent > 100 {
		percent = 100
	}
	filled := int(math.Round((percent / 100.0) * float64(width)))
	if filled > width {
		filled = width
	}
	return "[" + strings.Repeat("#", filled) + strings.Repeat("-", width-filled) + "]"
}

func renderSparkline(values []float64, width int) string {
	if len(values) == 0 {
		return strings.Repeat(".", width)
	}
	if width <= 0 {
		width = 16
	}
	start := len(values) - width
	if start < 0 {
		start = 0
	}
	s := values[start:]
	max := 0.0
	for _, v := range s {
		if v > max {
			max = v
		}
	}
	if max <= 0 {
		return strings.Repeat(".", len(s))
	}
	out := make([]byte, 0, len(s))
	for _, v := range s {
		ratio := v / max
		switch {
		case ratio > 0.85:
			out = append(out, '8')
		case ratio > 0.65:
			out = append(out, '6')
		case ratio > 0.45:
			out = append(out, '4')
		case ratio > 0.25:
			out = append(out, '2')
		default:
			out = append(out, '.')
		}
	}
	return string(out)
}

func renderIntSparkline(values []int, width int) string {
	if len(values) == 0 {
		return strings.Repeat(".", width)
	}
	floatValues := make([]float64, 0, len(values))
	for _, v := range values {
		floatValues = append(floatValues, float64(v))
	}
	return renderSparkline(floatValues, width)
}

func appendTrend(values []float64, value float64, max int) []float64 {
	values = append(values, value)
	if len(values) > max {
		values = values[len(values)-max:]
	}
	return values
}

func appendIntTrend(values []int, value int, max int) []int {
	values = append(values, value)
	if len(values) > max {
		values = values[len(values)-max:]
	}
	return values
}

func formatRate(bps float64) string {
	if bps >= 1024*1024 {
		return fmt.Sprintf("%.2f MiB", bps/(1024*1024))
	}
	if bps >= 1024 {
		return fmt.Sprintf("%.1f KiB", bps/1024)
	}
	return fmt.Sprintf("%.0f B", bps)
}

func coalesce(a, b float64) float64 {
	if a > 0 {
		return a
	}
	return b
}

func coalesceText(v, fallback string) string {
	if strings.TrimSpace(v) == "" {
		return fallback
	}
	return v
}

func Run(ctx context.Context, bus *app.EventBus, controls chan<- app.ControlCommand) error {
	m := newModel(controls)
	p := tea.NewProgram(m, tea.WithAltScreen(), tea.WithOutput(os.Stderr))

	events, unsub := bus.Subscribe()
	defer unsub()

	go func() {
		for {
			select {
			case <-ctx.Done():
				p.Quit()
				return
			case evt, ok := <-events:
				if !ok {
					return
				}
				p.Send(evt)
			}
		}
	}()

	_, err := p.Run()
	return err
}
