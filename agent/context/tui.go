package context

import (
	"fmt"
	"strings"
	"sync"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

var (
	tuiProgram   *tea.Program
	tuiProgramMu sync.RWMutex
)

// SetTUIProgram sets the global TUI program (called from start.go after HTTP READY)
func SetTUIProgram(p *tea.Program) {
	tuiProgramMu.Lock()
	tuiProgram = p
	tuiProgramMu.Unlock()
}

// GetTUIProgram returns the global TUI program (nil if not in TUI mode)
func GetTUIProgram() *tea.Program {
	tuiProgramMu.RLock()
	defer tuiProgramMu.RUnlock()
	return tuiProgram
}

// SendTUI sends a message to the TUI program if available
func SendTUI(msg tea.Msg) {
	if p := GetTUIProgram(); p != nil {
		p.Send(msg)
	}
}

// TUILogWriter implements io.Writer to bridge gou DevWriter -> TUI AppLogMsg
type TUILogWriter struct {
	Program *tea.Program
}

func (w *TUILogWriter) Write(p []byte) (n int, err error) {
	content := strings.TrimRight(string(p), "\n")
	if content == "" {
		return len(p), nil
	}
	w.Program.Send(AppLogMsg{Content: content})
	return len(p), nil
}

// ─── Styles ───────────────────────────────────────────────────────────────────

var (
	boxRunning = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("33")).
			PaddingLeft(1).PaddingRight(1)

	boxDone = lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("240")).
		PaddingLeft(1).PaddingRight(1)

	boxFailed = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("31")).
			PaddingLeft(1).PaddingRight(1)

	boxAppLog = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("240")).
			PaddingLeft(1).PaddingRight(1)

	sRunning = lipgloss.NewStyle().Foreground(lipgloss.Color("33"))
	sDone    = lipgloss.NewStyle().Foreground(lipgloss.Color("34"))
	sFailed  = lipgloss.NewStyle().Foreground(lipgloss.Color("31"))
	sDim     = lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
	sBold    = lipgloss.NewStyle().Bold(true)
	sYellow  = lipgloss.NewStyle().Foreground(lipgloss.Color("33"))
	sRed     = lipgloss.NewStyle().Foreground(lipgloss.Color("31"))
	sBlue    = lipgloss.NewStyle().Foreground(lipgloss.Color("34"))
	sMagenta = lipgloss.NewStyle().Foreground(lipgloss.Color("35"))
	sTree    = lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
)

// ─── Data ─────────────────────────────────────────────────────────────────────

// RequestPanel represents a single top-level agent request
type RequestPanel struct {
	RequestID   string
	ShortID     string
	AssistantID string
	StartTime   time.Time
	EndTime     time.Time // set when done/failed, freezes elapsed display
	Status      PanelStatus
	Nodes       []TreeNode
	ParentID    string
	Collapsed   bool
	viewRow     int // Y offset of the header line (for mouse click)
}

// TreeNode represents a step within a request panel
type TreeNode struct {
	Kind      NodeKind
	Label     string
	Status    NodeStatus
	Detail    string
	Children  []*TreeNode
	StartTime time.Time
	EndTime   time.Time
	Collapsed bool
}

// AgentTUIModel is the bubbletea Model for agent request visualization
type AgentTUIModel struct {
	panels       []*RequestPanel
	panelIndex   map[string]int // requestID -> index in panels (first registration wins)
	appLogs      []AppLogEntry
	appLogExpand bool
	appLogRow    int // Y offset of app log header
	cursor       int
	width        int
	height       int
	scrollOffset int
	autoFollow   bool // auto-scroll to bottom when new content arrives
	mouseOn      bool
	quitting     bool
}

// NewAgentTUIModel creates a new TUI model
func NewAgentTUIModel() AgentTUIModel {
	return AgentTUIModel{
		panels:     []*RequestPanel{},
		panelIndex: map[string]int{},
		appLogs:    []AppLogEntry{},
		width:      80,
		height:     24,
		autoFollow: true,
	}
}

func (m AgentTUIModel) Init() tea.Cmd {
	return tickCmd()
}

func tickCmd() tea.Cmd {
	return tea.Tick(500*time.Millisecond, func(t time.Time) tea.Msg {
		return TickMsg(t)
	})
}

// ─── Update ───────────────────────────────────────────────────────────────────

func (m AgentTUIModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil

	case tea.KeyMsg:
		return m.handleKey(msg)

	case tea.MouseMsg:
		return m.handleMouse(msg)

	case AgentEventMsg:
		return m.handleAgentEvent(msg), nil

	case AppLogMsg:
		m.appLogs = append(m.appLogs, AppLogEntry{
			Content: msg.Content,
			Time:    time.Now(),
		})
		return m, nil

	case TickMsg:
		return m, tickCmd()
	}
	return m, nil
}

func (m AgentTUIModel) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	topPanels := m.topLevelPanels()
	total := len(topPanels) + 1 // +1 for app log
	viewH := m.viewHeight()

	switch msg.String() {
	case "q", "ctrl+c":
		m.quitting = true
		return m, tea.Quit

	// Scrolling
	case "j", "down":
		m.scrollOffset++
		m.autoFollow = false
	case "k", "up":
		if m.scrollOffset > 0 {
			m.scrollOffset--
		}
		m.autoFollow = false
	case "pgdown", "ctrl+d":
		m.scrollOffset += viewH / 2
		m.autoFollow = false
	case "pgup", "ctrl+u":
		m.scrollOffset -= viewH / 2
		if m.scrollOffset < 0 {
			m.scrollOffset = 0
		}
		m.autoFollow = false
	case "G", "end":
		m.autoFollow = true
	case "g", "home":
		m.scrollOffset = 0
		m.autoFollow = false

	// Cursor navigation for panel selection (wraps around)
	case "tab":
		m.cursor = (m.cursor + 1) % total
		m.scrollToCursor(topPanels)
	case "shift+tab":
		m.cursor = (m.cursor - 1 + total) % total
		m.scrollToCursor(topPanels)

	case "enter", " ":
		if m.cursor < len(topPanels) {
			topPanels[m.cursor].Collapsed = !topPanels[m.cursor].Collapsed
		} else {
			m.appLogExpand = !m.appLogExpand
		}
	case "c":
		m.appLogExpand = !m.appLogExpand
	case "a":
		for _, p := range m.panels {
			p.Collapsed = false
		}
		m.appLogExpand = true
	case "A":
		for _, p := range m.panels {
			p.Collapsed = true
		}
		m.appLogExpand = false
	case "m":
		m.mouseOn = !m.mouseOn
		if m.mouseOn {
			return m, tea.EnableMouseCellMotion
		}
		return m, tea.DisableMouse
	}
	return m, nil
}

func (m AgentTUIModel) viewHeight() int {
	h := m.height - 2 // reserve for status bar
	if h < 4 {
		h = 4
	}
	return h
}

func (m *AgentTUIModel) scrollToCursor(topPanels []*RequestPanel) {
	targetRow := 0
	if m.cursor < len(topPanels) {
		targetRow = topPanels[m.cursor].viewRow
	} else {
		targetRow = m.appLogRow
	}
	viewH := m.viewHeight()
	if targetRow < m.scrollOffset {
		m.scrollOffset = targetRow
	} else if targetRow >= m.scrollOffset+viewH {
		m.scrollOffset = targetRow - viewH + 3
	}
}

func (m AgentTUIModel) handleMouse(msg tea.MouseMsg) (tea.Model, tea.Cmd) {
	switch {
	case msg.Button == tea.MouseButtonWheelUp:
		m.scrollOffset -= 3
		if m.scrollOffset < 0 {
			m.scrollOffset = 0
		}
		m.autoFollow = false
		return m, nil
	case msg.Button == tea.MouseButtonWheelDown:
		m.scrollOffset += 3
		m.autoFollow = false
		return m, nil
	}

	if msg.Button != tea.MouseButtonLeft || msg.Action != tea.MouseActionRelease {
		return m, nil
	}
	y := msg.Y + m.scrollOffset

	// Check app log header
	if y == m.appLogRow {
		m.appLogExpand = !m.appLogExpand
		return m, nil
	}

	// Check panel headers
	for _, p := range m.panels {
		if p.ParentID != "" {
			continue
		}
		if y == p.viewRow {
			p.Collapsed = !p.Collapsed
			return m, nil
		}
	}
	return m, nil
}

// ─── Agent Events ─────────────────────────────────────────────────────────────

func (m *AgentTUIModel) handleAgentEvent(msg AgentEventMsg) tea.Model {
	switch msg.Event {
	case EventRequestStart:
		if idx, ok := m.panelIndex[msg.RequestID]; ok {
			// Delegate sub-call: same requestID, different assistantID.
			// Add as a tree node inside the existing panel instead of creating a new one.
			p := m.panels[idx]
			p.Nodes = append(p.Nodes, TreeNode{
				Kind:      NodeA2A,
				Label:     msg.AssistantID,
				Status:    NodeRunning,
				StartTime: time.Now(),
			})
			return m
		}

		panel := &RequestPanel{
			RequestID:   msg.RequestID,
			ShortID:     shortID(msg.RequestID),
			AssistantID: msg.AssistantID,
			StartTime:   time.Now(),
			Status:      PanelRunning,
			ParentID:    msg.ParentID,
		}
		m.panelIndex[msg.RequestID] = len(m.panels)
		m.panels = append(m.panels, panel)

	case EventRequestEnd:
		if idx, ok := m.panelIndex[msg.RequestID]; ok {
			p := m.panels[idx]

			// Only mark panel done if the ending assistantID matches the panel's original assistantID
			// (delegate sub-calls End with a different assistantID, they update their tree node instead)
			if msg.AssistantID == p.AssistantID || msg.AssistantID == "" {
				if errVal, has := msg.Data["error"]; has && errVal != nil {
					p.Status = PanelFailed
				} else {
					p.Status = PanelSuccess
				}
				p.EndTime = time.Now()
				p.Collapsed = true

				// Finalize any still-running child nodes (e.g. hook interrupted mid-execution)
				finalStatus := NodeDone
				if p.Status == PanelFailed {
					finalStatus = NodeFailed
				}
				for i := range p.Nodes {
					if p.Nodes[i].Status == NodeRunning {
						p.Nodes[i].Status = finalStatus
						p.Nodes[i].EndTime = p.EndTime
					}
				}
			} else {
				// Delegate sub-call finished: mark its tree node as done
				for i := len(p.Nodes) - 1; i >= 0; i-- {
					if p.Nodes[i].Kind == NodeA2A && p.Nodes[i].Label == msg.AssistantID && p.Nodes[i].Status == NodeRunning {
						p.Nodes[i].Status = NodeDone
						p.Nodes[i].EndTime = time.Now()
						break
					}
				}
			}
		}

	case EventLLMCall:
		if idx, ok := m.panelIndex[msg.RequestID]; ok {
			m.panels[idx].Nodes = append(m.panels[idx].Nodes, TreeNode{
				Kind: NodeLLM, Label: "LLM", Status: NodeRunning, StartTime: time.Now(),
			})
		}

	case EventLLMDone:
		if idx, ok := m.panelIndex[msg.RequestID]; ok {
			p := m.panels[idx]
			for i := len(p.Nodes) - 1; i >= 0; i-- {
				if p.Nodes[i].Kind == NodeLLM && p.Nodes[i].Status == NodeRunning {
					p.Nodes[i].Status = NodeDone
					p.Nodes[i].EndTime = time.Now()
					if d, has := msg.Data["detail"]; has {
						p.Nodes[i].Detail = fmt.Sprintf("%v", d)
					}
					break
				}
			}
		}

	case EventToolCall:
		if idx, ok := m.panelIndex[msg.RequestID]; ok {
			name := dataStr(msg.Data, "name")
			m.panels[idx].Nodes = append(m.panels[idx].Nodes, TreeNode{
				Kind: NodeTool, Label: name, Status: NodeRunning, StartTime: time.Now(),
			})
		}

	case EventToolDone:
		if idx, ok := m.panelIndex[msg.RequestID]; ok {
			name := dataStr(msg.Data, "name")
			p := m.panels[idx]
			for i := len(p.Nodes) - 1; i >= 0; i-- {
				if p.Nodes[i].Kind == NodeTool && p.Nodes[i].Label == name && p.Nodes[i].Status == NodeRunning {
					p.Nodes[i].Status = NodeDone
					p.Nodes[i].EndTime = time.Now()
					break
				}
			}
		}

	case EventHook:
		if idx, ok := m.panelIndex[msg.RequestID]; ok {
			name := dataStr(msg.Data, "name")
			m.panels[idx].Nodes = append(m.panels[idx].Nodes, TreeNode{
				Kind: NodeHook, Label: name, Status: NodeRunning, StartTime: time.Now(),
			})
		}

	case EventHookDone:
		if idx, ok := m.panelIndex[msg.RequestID]; ok {
			p := m.panels[idx]
			for i := len(p.Nodes) - 1; i >= 0; i-- {
				if p.Nodes[i].Kind == NodeHook && p.Nodes[i].Status == NodeRunning {
					p.Nodes[i].Status = NodeDone
					p.Nodes[i].EndTime = time.Now()
					break
				}
			}
		}

	case EventA2AStart:
		if idx, ok := m.panelIndex[msg.RequestID]; ok {
			target := dataStr(msg.Data, "target")
			m.panels[idx].Nodes = append(m.panels[idx].Nodes, TreeNode{
				Kind: NodeA2A, Label: target, Status: NodeRunning, StartTime: time.Now(),
			})
		}

	case EventA2ADone:
		if idx, ok := m.panelIndex[msg.RequestID]; ok {
			target := dataStr(msg.Data, "target")
			p := m.panels[idx]
			for i := len(p.Nodes) - 1; i >= 0; i-- {
				if p.Nodes[i].Kind == NodeA2A && p.Nodes[i].Status == NodeRunning && (target == "" || p.Nodes[i].Label == target) {
					p.Nodes[i].Status = NodeDone
					p.Nodes[i].EndTime = time.Now()
					break
				}
			}
		}
	}
	return m
}

// ─── View ─────────────────────────────────────────────────────────────────────

func (m AgentTUIModel) View() string {
	if m.quitting {
		return ""
	}

	boxW := m.width - 2
	if boxW < 40 {
		boxW = 40
	}

	// Render full content
	var sb strings.Builder
	row := 0
	topIdx := 0

	for _, panel := range m.panels {
		if panel.ParentID != "" {
			continue
		}
		selected := (topIdx == m.cursor)
		rendered := m.renderPanelBox(panel, boxW, selected, &row)
		sb.WriteString(rendered)
		sb.WriteString("\n")
		row++
		topIdx++
	}

	// App Log
	m.appLogRow = row
	sb.WriteString(m.renderAppLogBox(boxW, topIdx == m.cursor, &row))

	fullContent := sb.String()
	lines := strings.Split(fullContent, "\n")
	totalLines := len(lines)
	viewH := m.viewHeight()

	// Auto-follow: snap to bottom
	if m.autoFollow {
		m.scrollOffset = totalLines - viewH
	}

	// Clamp scroll offset
	maxScroll := totalLines - viewH
	if maxScroll < 0 {
		maxScroll = 0
	}
	if m.scrollOffset > maxScroll {
		m.scrollOffset = maxScroll
	}
	if m.scrollOffset < 0 {
		m.scrollOffset = 0
	}

	// Slice visible lines
	end := m.scrollOffset + viewH
	if end > totalLines {
		end = totalLines
	}
	visible := lines[m.scrollOffset:end]

	// Build output
	var out strings.Builder
	out.WriteString(strings.Join(visible, "\n"))

	// Status bar with scroll indicator
	mouseLabel := "off"
	if m.mouseOn {
		mouseLabel = "on"
	}
	scrollInfo := ""
	if totalLines > viewH {
		pct := 100
		if maxScroll > 0 {
			pct = m.scrollOffset * 100 / maxScroll
		}
		scrollInfo = fmt.Sprintf(" [%d%%]", pct)
	}
	followLabel := ""
	if m.autoFollow {
		followLabel = " AUTO"
	}
	hint := sDim.Render(fmt.Sprintf("  j/k:scroll  tab:select  space:toggle  a/A:all  G:bottom  g:top  m:mouse(%s)%s%s  q:quit",
		mouseLabel, scrollInfo, followLabel))
	out.WriteString("\n" + hint)

	return out.String()
}

func (m AgentTUIModel) renderPanelBox(panel *RequestPanel, boxW int, selected bool, row *int) string {
	// Record header row for mouse
	panel.viewRow = *row

	elapsed := m.panelElapsed(panel)
	icon, statusText, style := panelStatusDisplay(panel.Status, elapsed)

	// Title line
	collapser := "▾"
	if panel.Collapsed {
		collapser = "▸"
	}
	cursor := " "
	if selected {
		cursor = "›"
	}
	title := fmt.Sprintf("%s %s %s %s  %s",
		sDim.Render(cursor),
		sDim.Render(collapser),
		sBold.Render(panel.ShortID),
		panel.AssistantID,
		style.Render(icon+" "+statusText),
	)

	if panel.Collapsed {
		box := boxForStatus(panel.Status).Width(boxW)
		result := box.Render(title)
		*row += strings.Count(result, "\n") + 1
		return result
	}

	// Build body
	var body strings.Builder
	body.WriteString(title + "\n")

	for _, node := range panel.Nodes {
		body.WriteString(m.renderTreeNode(node, "  ", false, panel))
	}

	// Render fork children (different requestID, parentID matches)
	children := m.childPanels(panel.RequestID)
	for i, child := range children {
		isLast := (i == len(children)-1)
		body.WriteString(m.renderChildSummary(child, "  ", isLast))
	}

	box := boxForStatus(panel.Status).Width(boxW)
	result := box.Render(body.String())
	*row += strings.Count(result, "\n") + 1
	return result
}

func (m AgentTUIModel) renderTreeNode(node TreeNode, prefix string, isChild bool, panel *RequestPanel) string {
	panelEnded := panel != nil && panel.Status != PanelRunning
	displayNode := node
	if panelEnded && displayNode.Status == NodeRunning {
		displayNode.Status = NodeFailed
	}
	icon, statusText := nodeStatusDisplay(displayNode)
	elapsed := m.nodeElapsed(node, panelEnded, panel.EndTime)

	label := ""
	switch node.Kind {
	case NodeHook:
		label = sMagenta.Render("Hook: "+node.Label) + " " + statusText
	case NodeLLM:
		detail := ""
		if node.Detail != "" {
			detail = " " + sDim.Render("["+node.Detail+"]")
		}
		label = sBlue.Render("LLM") + " " + statusText + detail
	case NodeTool:
		label = sTree.Render("├ ") + sYellow.Render(node.Label) + " " + statusText
	case NodeA2A:
		label = sTree.Render("⤷ ") + sBold.Render(node.Label) + " " + statusText
	case NodePhase:
		label = node.Label + " " + statusText
	default:
		label = node.Label + " " + statusText
	}

	_ = icon
	line := prefix + label
	if elapsed != "" {
		line += " " + sDim.Render(elapsed)
	}
	return line + "\n"
}

func (m AgentTUIModel) renderChildSummary(panel *RequestPanel, prefix string, isLast bool) string {
	elapsed := m.panelElapsed(panel)
	icon, statusText, style := panelStatusDisplay(panel.Status, elapsed)

	branch := sTree.Render("├─ ")
	if isLast {
		branch = sTree.Render("└─ ")
	}
	return fmt.Sprintf("%s%s%s %s %s\n",
		prefix, branch,
		sBold.Render(panel.ShortID+" "+panel.AssistantID),
		style.Render(icon+" "+statusText),
		sDim.Render(elapsed),
	)
}

func (m AgentTUIModel) renderAppLogBox(boxW int, selected bool, row *int) string {
	cursor := " "
	if selected {
		cursor = "›"
	}
	collapser := "▸"
	if m.appLogExpand {
		collapser = "▾"
	}

	count := len(m.appLogs)
	title := fmt.Sprintf("%s %s %s (%d)",
		sDim.Render(cursor),
		sDim.Render(collapser),
		sBold.Render("App Output"),
		count,
	)

	if !m.appLogExpand || count == 0 {
		result := boxAppLog.Width(boxW).Render(title)
		*row += strings.Count(result, "\n") + 1
		return result
	}

	var body strings.Builder
	body.WriteString(title + "\n")

	start := 0
	if count > 50 {
		start = count - 50
	}
	for _, entry := range m.appLogs[start:] {
		body.WriteString("  " + entry.Content + "\n")
	}

	result := boxAppLog.Width(boxW).Render(body.String())
	*row += strings.Count(result, "\n") + 1
	return result
}

// ─── Helpers ──────────────────────────────────────────────────────────────────

func (m AgentTUIModel) topLevelPanels() []*RequestPanel {
	var result []*RequestPanel
	for _, p := range m.panels {
		if p.ParentID == "" {
			result = append(result, p)
		}
	}
	return result
}

func (m AgentTUIModel) childPanels(parentRequestID string) []*RequestPanel {
	var result []*RequestPanel
	for _, p := range m.panels {
		if p.ParentID == parentRequestID {
			result = append(result, p)
		}
	}
	return result
}

func (m AgentTUIModel) panelElapsed(p *RequestPanel) string {
	if p.Status != PanelRunning && !p.EndTime.IsZero() {
		return fmtDuration(p.EndTime.Sub(p.StartTime))
	}
	return fmtDuration(time.Since(p.StartTime))
}

func (m AgentTUIModel) nodeElapsed(n TreeNode, panelEnded bool, panelEndTime time.Time) string {
	if n.Status == NodeDone || n.Status == NodeFailed {
		if !n.EndTime.IsZero() {
			return fmtDuration(n.EndTime.Sub(n.StartTime))
		}
	}
	if n.Status == NodeRunning {
		if panelEnded && !panelEndTime.IsZero() {
			return fmtDuration(panelEndTime.Sub(n.StartTime))
		}
		return fmtDuration(time.Since(n.StartTime))
	}
	return ""
}

func panelStatusDisplay(status PanelStatus, elapsed string) (icon string, text string, style lipgloss.Style) {
	switch status {
	case PanelRunning:
		return "⟳", "running " + elapsed, sRunning
	case PanelSuccess:
		return "✓", "done " + elapsed, sDone
	case PanelFailed:
		return "✗", "failed " + elapsed, sFailed
	}
	return "", "", sDim
}

func nodeStatusDisplay(n TreeNode) (icon string, text string) {
	switch n.Status {
	case NodePending:
		return "…", sDim.Render("…")
	case NodeRunning:
		return "⟳", sRunning.Render("⟳")
	case NodeDone:
		return "✓", sDone.Render("✓")
	case NodeFailed:
		return "✗", sFailed.Render("✗")
	}
	return "", ""
}

func boxForStatus(status PanelStatus) lipgloss.Style {
	switch status {
	case PanelRunning:
		return boxRunning
	case PanelFailed:
		return boxFailed
	default:
		return boxDone
	}
}

func dataStr(data map[string]interface{}, key string) string {
	if v, ok := data[key]; ok {
		return fmt.Sprintf("%v", v)
	}
	return ""
}

func fmtDuration(d time.Duration) string {
	if d < time.Second {
		return fmt.Sprintf("%dms", d.Milliseconds())
	}
	return fmt.Sprintf("%.1fs", d.Seconds())
}
