# Implementation Plan: TUI-B-NEW

## Design Decision
Clean rewrite of the Bubble Tea TUI in `internal/ui-b/` with composable sub-models, file splitting, and architectural improvements. The existing TUI at `internal/ui/` remains untouched.

## Success Criteria
- All current features replicated (chat, streaming, overlays, sidebar, tool cards, commands)
- Can be selected as an alternative channel via `--tui-b` flag or config
- All new code has test coverage
- `go build ./...` and `go test ./...` pass
- Existing TUI unmodified

## File Structure

```
internal/ui-b/
├── model.go          — Model struct, sub-models, Init(), NewModel()
├── update.go         — Update() with clean routing to sub-models
├── view.go           — View() + layout composition
├── messages.go       — all tea.Msg types
├── styles.go         — Theme struct + all lipgloss styles
├── input.go          — inputModel: textarea, cursor, @autocomplete, wrapping
├── chat.go           — chatModel: viewport, messages, glamour rendering
├── streaming.go      — streamModel: single event channel, agent runner
├── overlays.go       — overlayModel: generic reusable overlay for all types
├── sidebar.go        — sidebarModel: stats, files, flags, click handling
├── status.go         — statusModel: progress bar, spinner, completion, status bar
├── toolcards.go      — ToolCardRenderer interface + per-tool renderers
├── commands.go       — slash command dispatch + handlers
└── tui.go            — StartTUI() entry point + channel registration
```

Also: `internal/channels/tui-b/tui.go` — channel shim (like existing tui channel)

## Tasks

---

### Phase 1: Foundation

---

### Task 1: Create messages.go — all tea.Msg types

**Test first:**
```go
// internal/ui-b/messages_test.go
func TestStreamEventTypes(t *testing.T) {
    // Verify all event types are distinct
    var msgs []tea.Msg
    msgs = append(msgs, StreamEventMsg{})
    msgs = append(msgs, AgentDoneMsg{})
    msgs = append(msgs, AgentErrorMsg{})
    msgs = append(msgs, TickMsg{})
    msgs = append(msgs, WorkflowDoneMsg{})
    if len(msgs) != 5 { t.Fatal("expected 5 msg types") }
}
```

**Implementation:** `internal/ui-b/messages.go`
```go
package uib

// StreamEventMsg is a unified event from the agent stream.
// Replaces the 3 separate channels (deltaCh, toolCh, statusCh).
type StreamEventMsg struct {
    Type       StreamEventType
    Delta      string            // for TextDelta
    Tool       ToolEvent         // for ToolEnd
    StatusHook string            // for StatusUpdate
}

type StreamEventType int
const (
    StreamTextDelta StreamEventType = iota
    StreamToolStart
    StreamToolEnd
    StreamStatusUpdate
)

type ToolEvent struct {
    Name     string
    Args     map[string]any
    Result   string
    IsError  bool
    FilePath string
    Duration time.Duration
}

type AgentDoneMsg struct {
    Text      string
    TokensIn  int
    TokensOut int
    Cost      float64
    Elapsed   time.Duration
}

type AgentErrorMsg struct{ Err error }

type TickMsg time.Time

type WorkflowDoneMsg struct {
    Text string
    Err  error
}

// DisplayMsg is a rendered chat message.
type DisplayMsg struct {
    Role     string // "user", "assistant", "error", "tool"
    Text     string
    IsError  bool
    Rendered string     // cached glamour output
    Tool     *ToolEvent // set when Role == "tool"
    Ts       time.Time
}
```

**Verify:** `go build ./internal/ui-b/`
**Depends on:** nothing

---

### Task 2: Create styles.go — Theme struct with all styles

**Test first:**
```go
// internal/ui-b/styles_test.go
func TestDefaultTheme(t *testing.T) {
    theme := DefaultTheme()
    if theme.User.GetForeground() == (lipgloss.NoColor{}) {
        t.Fatal("User style has no color")
    }
    if theme.Prompt.GetForeground() == (lipgloss.NoColor{}) {
        t.Fatal("Prompt style has no color")
    }
}
```

**Implementation:** `internal/ui-b/styles.go`
```go
package uib

type Theme struct {
    // Glow gradient (progress bar)
    GlowBright, GlowMed, GlowDim, GlowFaint, GlowFaint2, GlowTrack lipgloss.Style

    // Messages
    User, AssistantPrefix, Timestamp, Error, Cursor, Prompt, Dim lipgloss.Style

    // Header
    HeaderBar, HeaderDim, Separator, InputBorder lipgloss.Style

    // Status
    Status, ScrollHint lipgloss.Style

    // Tool cards
    ToolHeader, ToolDim, ToolSep, DiffAdd, DiffDel lipgloss.Style

    // Sidebar
    SidebarBorder, SidebarTitle, SidebarFile, SidebarCount lipgloss.Style

    // Autocomplete / Palette
    ACNormal, ACSelected lipgloss.Style

    // Overlay
    OverlayTitle, OverlayHint, Keybind lipgloss.Style

    // Completion
    Completion, Check lipgloss.Style
}

func DefaultTheme() Theme { /* all 30+ styles defined here */ }
```

**Verify:** `go test ./internal/ui-b/ -run TestDefaultTheme`
**Depends on:** nothing

---

### Task 3: Create toolcards.go — ToolCardRenderer interface

**Test first:**
```go
// internal/ui-b/toolcards_test.go
func TestToolCardRegistry(t *testing.T) {
    reg := NewToolCardRegistry(DefaultTheme())
    card := reg.Render(&ToolEvent{Name: "bash", Args: map[string]any{"command": "ls"}}, 80)
    if !strings.Contains(card, "bash") { t.Fatal("bash card should contain tool name") }
    if !strings.Contains(card, "ls") { t.Fatal("bash card should contain command") }
}
```

**Implementation:** `internal/ui-b/toolcards.go`
```go
package uib

type ToolCardRenderer interface {
    Render(ev *ToolEvent, maxWidth int, theme Theme) string
}

type ToolCardRegistry struct {
    renderers map[string]ToolCardRenderer
    fallback  ToolCardRenderer
    theme     Theme
}

func NewToolCardRegistry(theme Theme) *ToolCardRegistry { /* register edit, write, bash, read, glob, grep */ }
func (r *ToolCardRegistry) Register(name string, renderer ToolCardRenderer) { /* add custom */ }
func (r *ToolCardRegistry) Render(ev *ToolEvent, maxWidth int) string { /* dispatch */ }

// Built-in renderers
type editCardRenderer struct{}   // shows diff
type writeCardRenderer struct{}  // shows file + line count
type bashCardRenderer struct{}   // shows command + truncated output
type readCardRenderer struct{}   // shows file path
type searchCardRenderer struct{} // shows pattern + match count (glob/grep)
type defaultCardRenderer struct{} // truncated result
```

**Verify:** `go test ./internal/ui-b/ -run TestToolCard`
**Depends on:** Task 2

---

### Task 4: Create status.go — statusModel (progress bar, spinner, completion)

**Test first:**
```go
// internal/ui-b/status_test.go
func TestStatusModelProgressBar(t *testing.T) {
    s := newStatusModel(DefaultTheme())
    s.processing = true
    s.startTime = time.Now().Add(-3 * time.Second)
    bar := s.renderProgressBar(80)
    if !strings.Contains(bar, "\u2501") { t.Fatal("should contain bar chars") }
}

func TestStatusModelCompletion(t *testing.T) {
    s := newStatusModel(DefaultTheme())
    s.lastElapsed = 1200 * time.Millisecond
    view := s.renderCompletion()
    if !strings.Contains(view, "Toroidal cycle complete") { t.Fatal("should show completion") }
}
```

**Implementation:** `internal/ui-b/status.go`
```go
package uib

type statusModel struct {
    theme        Theme
    processing   bool
    barPos, barDir int
    startTime    time.Time
    lastElapsed  time.Duration
    statusPhrase string
    statusLine   string
    // usage
    totalTokensIn, totalTokensOut int
    totalCost float64
}

func newStatusModel(theme Theme) statusModel { /* init */ }
func (s *statusModel) Update(msg tea.Msg) tea.Cmd { /* handle TickMsg, advance bar */ }
func (s statusModel) renderProgressBar(width int) string { /* bouncing bar + spinner + phrase */ }
func (s statusModel) renderCompletion() string { /* ✔ Toroidal cycle complete | duration: Xs */ }
func (s statusModel) renderStatusBar(width int) string { /* bottom status line */ }
```

**Verify:** `go test ./internal/ui-b/ -run TestStatusModel`
**Depends on:** Task 2

---

### Phase 2: Core

---

### Task 5: Create input.go — inputModel (textarea, autocomplete, wrapping)

**Test first:**
```go
// internal/ui-b/input_test.go
func TestInputModelPlaceholder(t *testing.T) {
    m := newInputModel(DefaultTheme(), 80)
    view := m.View()
    if !strings.Contains(view, "Type a message...") { t.Fatal("empty input should show placeholder") }
}

func TestInputModelAutocomplete(t *testing.T) {
    m := newInputModel(DefaultTheme(), 80)
    m.SetValue("@ma")
    m.triggerAutocomplete([]string{"main.go", "Makefile"})
    if len(m.acList) != 2 { t.Fatalf("expected 2 matches, got %d", len(m.acList)) }
}
```

**Implementation:** `internal/ui-b/input.go`
```go
package uib

type inputModel struct {
    theme     Theme
    value     string
    cursorPos int
    width     int
    // autocomplete
    acMode  bool
    acQuery string
    acList  []string
    acIdx   int
    acFiles []string
    acLoaded bool
}

func newInputModel(theme Theme, width int) inputModel { /* init */ }
func (m *inputModel) Update(msg tea.Msg) (bool, tea.Cmd) { /* returns submitted=true on Enter */ }
func (m inputModel) View() string { /* prompt + wrapped input + cursor + placeholder */ }
func (m inputModel) Value() string { return m.value }
func (m *inputModel) SetValue(s string) { /* set + move cursor to end */ }
func (m *inputModel) Clear() { /* reset */ }
func (m *inputModel) Resize(width int) { /* update width */ }
func (m *inputModel) insertAtCursor(text string) { /* insert + resize */ }
func (m *inputModel) triggerAutocomplete(files []string) { /* filter files by acQuery */ }
func (m inputModel) renderAutocomplete() string { /* dropdown list */ }
```

**Verify:** `go test ./internal/ui-b/ -run TestInputModel`
**Depends on:** Task 2

---

### Task 6: Create chat.go — chatModel (viewport, messages, glamour)

**Test first:**
```go
// internal/ui-b/chat_test.go
func TestChatModelAddMessage(t *testing.T) {
    c := newChatModel(DefaultTheme(), 80, 20)
    c.AddMessage("user", "hello")
    if len(c.messages) != 1 { t.Fatal("expected 1 message") }
    if c.messages[0].Role != "user" { t.Fatal("expected user role") }
}
```

**Implementation:** `internal/ui-b/chat.go`
```go
package uib

type chatModel struct {
    theme       Theme
    messages    []DisplayMsg
    viewport    viewport.Model
    glamRenderer *glamour.TermRenderer
    toolCards   *ToolCardRegistry
    ready       bool
}

func newChatModel(theme Theme, width, height int) chatModel { /* init viewport + glamour */ }
func (c *chatModel) Update(msg tea.Msg) tea.Cmd { /* handle WindowSizeMsg for viewport */ }
func (c chatModel) View() string { return c.viewport.View() }
func (c *chatModel) AddMessage(role, text string) { /* append + rebuild */ }
func (c *chatModel) AddToolCard(ev *ToolEvent) { /* append tool msg + placeholder */ }
func (c *chatModel) AppendDelta(delta string) { /* streaming text append */ }
func (c *chatModel) Rebuild() { /* re-render all messages into viewport */ }
func (c *chatModel) Resize(width, height int) { /* update viewport + glamour */ }
```

**Verify:** `go test ./internal/ui-b/ -run TestChatModel`
**Depends on:** Tasks 2, 3

---

### Task 7: Create streaming.go — single event channel

**Test first:**
```go
// internal/ui-b/streaming_test.go
func TestStreamEventChannel(t *testing.T) {
    ch := make(chan StreamEventMsg, 16)
    ch <- StreamEventMsg{Type: StreamTextDelta, Delta: "hello"}
    ev := <-ch
    if ev.Delta != "hello" { t.Fatal("expected hello") }
}
```

**Implementation:** `internal/ui-b/streaming.go`
```go
package uib

// runAgentStream launches the agent and sends all events through a single channel.
func runAgentStream(agent *core.Agent, input string, eventCh chan<- StreamEventMsg) tea.Cmd {
    return func() tea.Msg {
        start := time.Now()
        var finalText string
        var finalErr error
        var totalIn, totalOut int
        var totalCost float64
        var toolStart time.Time
        for ev := range agent.RunStream(context.Background(), input) {
            switch ev.Type {
            case core.EventAgentTextDelta:
                eventCh <- StreamEventMsg{Type: StreamTextDelta, Delta: ev.Text}
            case core.EventAgentToolStart:
                toolStart = time.Now()
                eventCh <- StreamEventMsg{Type: StreamToolStart}
            case core.EventAgentToolEnd:
                dur := time.Duration(0)
                if !toolStart.IsZero() { dur = time.Since(toolStart) }
                fp, _ := ev.ToolArgs["file_path"].(string)
                eventCh <- StreamEventMsg{Type: StreamToolEnd, Tool: ToolEvent{
                    Name: ev.ToolName, Args: ev.ToolArgs, Result: ev.ToolResult.Content,
                    IsError: ev.ToolResult.IsError, FilePath: fp, Duration: dur,
                }}
                toolStart = time.Time{}
            case core.EventAgentTurnEnd:
                if ev.Usage != nil { totalIn += ev.Usage.InputTokens; totalOut += ev.Usage.OutputTokens; totalCost += ev.Usage.Cost }
            case core.EventAgentDone:
                finalText = ev.Text
            case core.EventStatusUpdate:
                eventCh <- StreamEventMsg{Type: StreamStatusUpdate, StatusHook: ev.StatusHook}
            case core.EventAgentError:
                finalErr = ev.Error
            }
        }
        close(eventCh)
        if finalErr != nil { return AgentErrorMsg{Err: finalErr} }
        return AgentDoneMsg{Text: finalText, TokensIn: totalIn, TokensOut: totalOut, Cost: totalCost, Elapsed: time.Since(start)}
    }
}

// waitForStreamEvent polls the single event channel.
func waitForStreamEvent(ch <-chan StreamEventMsg) tea.Cmd {
    if ch == nil { return nil }
    return func() tea.Msg {
        ev, ok := <-ch
        if !ok { return nil }
        return ev
    }
}
```

**Verify:** `go test ./internal/ui-b/ -run TestStream`
**Depends on:** Task 1

---

### Task 8: Create overlays.go — generic overlay component

**Test first:**
```go
// internal/ui-b/overlays_test.go
func TestOverlayFilter(t *testing.T) {
    o := newOverlayModel(DefaultTheme())
    items := []OverlayItem{{Name: "New conversation", Command: "/new"}, {Name: "Exit", Command: "/exit"}}
    o.Open("palette", items)
    o.SetQuery("ex")
    if len(o.Filtered()) != 1 { t.Fatalf("expected 1 match, got %d", len(o.Filtered())) }
    if o.Filtered()[0].Command != "/exit" { t.Fatal("expected /exit") }
}
```

**Implementation:** `internal/ui-b/overlays.go`
```go
package uib

type OverlayItem struct {
    Name    string
    Key     string // keybind hint
    Command string
}

type overlayModel struct {
    theme    Theme
    active   bool
    kind     string // "palette", "sessions", "help", "workflow"
    items    []OverlayItem
    filtered []OverlayItem
    query    string
    idx      int
    // workflow-specific state
    workflow workflowState
}

func newOverlayModel(theme Theme) overlayModel { /* init */ }
func (o *overlayModel) Open(kind string, items []OverlayItem) { /* activate */ }
func (o *overlayModel) Close() { o.active = false }
func (o overlayModel) Active() bool { return o.active }
func (o overlayModel) Kind() string { return o.kind }
func (o *overlayModel) SetQuery(q string) { /* filter items */ }
func (o overlayModel) Filtered() []OverlayItem { return o.filtered }
func (o overlayModel) Selected() OverlayItem { return o.filtered[o.idx] }
func (o *overlayModel) Update(msg tea.Msg) (string, tea.Cmd) { /* returns selected command or "" */ }
func (o overlayModel) View() string { /* render based on kind */ }
```

**Verify:** `go test ./internal/ui-b/ -run TestOverlay`
**Depends on:** Task 2

---

### Task 9: Create sidebar.go — sidebarModel

**Test first:**
```go
// internal/ui-b/sidebar_test.go
func TestSidebarRender(t *testing.T) {
    s := newSidebarModel(DefaultTheme())
    s.modifiedFiles["main.go"] = 3
    view := s.View(30)
    if !strings.Contains(view, "main.go") { t.Fatal("should show modified file") }
}
```

**Implementation:** `internal/ui-b/sidebar.go`
```go
package uib

type sidebarModel struct {
    theme         Theme
    modifiedFiles map[string]int
    toolEvents    []ToolEvent
    agentCfg      config.AgentConfig
    show          bool
}

func newSidebarModel(theme Theme, cfg config.AgentConfig) sidebarModel { /* init */ }
func (s *sidebarModel) TrackTool(ev ToolEvent) { /* update modifiedFiles */ }
func (s *sidebarModel) HandleClick(line int, agent *core.Agent) { /* toggle flags by line */ }
func (s sidebarModel) View(height int) string { /* session stats, files, config, flags */ }
```

**Verify:** `go test ./internal/ui-b/ -run TestSidebar`
**Depends on:** Task 2

---

### Phase 3: Composition

---

### Task 10: Create model.go — top-level Model composing all sub-models

**Test first:**
```go
// internal/ui-b/model_test.go
func TestNewModel(t *testing.T) {
    // Verify model creates without panic
    m := NewModel(nil, "test-model", config.AgentConfig{}, nil, nil)
    if m.chat.ready { t.Fatal("chat should not be ready before WindowSizeMsg") }
}
```

**Implementation:** `internal/ui-b/model.go`
```go
package uib

type Model struct {
    // Sub-models
    chat    chatModel
    input   inputModel
    overlay overlayModel
    sidebar sidebarModel
    status  statusModel

    // Deps
    agent     *core.Agent
    modelName string
    agentCfg  config.AgentConfig
    skills    *features.SkillRegistry
    subMgr    *features.SubAgentManager
    mcpClient *features.MCPClient

    // Streaming
    eventCh chan StreamEventMsg

    // Terminal
    width, height int
    ready         bool
}

func NewModel(agent *core.Agent, modelName string, cfg config.AgentConfig, skills *features.SkillRegistry, extras *TUIExtras) Model {
    theme := DefaultTheme()
    return Model{
        chat:    newChatModel(theme, 80, 20),
        input:   newInputModel(theme, 80),
        overlay: newOverlayModel(theme),
        sidebar: newSidebarModel(theme, cfg),
        status:  newStatusModel(theme),
        agent:   agent,
        modelName: modelName,
        agentCfg: cfg,
        skills:  skills,
        // extract subMgr, mcpClient from extras
    }
}

func (m Model) Init() tea.Cmd { return nil }
```

**Verify:** `go test ./internal/ui-b/ -run TestNewModel`
**Depends on:** Tasks 4, 5, 6, 8, 9

---

### Task 11: Create update.go — Update() routing to sub-models

**Test first:**
```go
// internal/ui-b/update_test.go
func TestUpdateRoutesToOverlay(t *testing.T) {
    m := NewModel(nil, "test", config.AgentConfig{}, nil, nil)
    m.width, m.height, m.ready = 80, 24, true
    // Open overlay
    m.overlay.Open("help", nil)
    // Send Escape — should close overlay
    newM, _ := m.Update(tea.KeyMsg{Type: tea.KeyEscape})
    model := newM.(Model)
    if model.overlay.Active() { t.Fatal("overlay should be closed after Escape") }
}
```

**Implementation:** `internal/ui-b/update.go`
```go
package uib

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
    switch msg := msg.(type) {
    case tea.WindowSizeMsg:
        m.width, m.height = msg.Width, msg.Height
        m.sidebar.show = m.width >= sidebarMinW
        if !m.ready { m.ready = true }
        chatW, chatH := m.chatDimensions()
        m.chat.Resize(chatW, chatH)
        m.input.Resize(m.width)
        return m, nil

    case tea.KeyMsg:
        if m.overlay.Active() {
            cmd, selected := m.overlay.Update(msg)
            if selected != "" { return m.executeCommand(selected) }
            return m, cmd
        }
        return m.handleKey(msg)

    case StreamEventMsg:
        return m.handleStreamEvent(msg)
    case AgentDoneMsg:
        return m.handleAgentDone(msg)
    case AgentErrorMsg:
        return m.handleAgentError(msg)
    case WorkflowDoneMsg:
        return m.handleWorkflowDone(msg)
    case TickMsg:
        cmd := m.status.Update(msg)
        return m, cmd
    case tea.MouseMsg:
        return m.handleMouse(msg)
    }
    return m, nil
}

func (m *Model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) { /* dispatch keys */ }
func (m *Model) handleStreamEvent(msg StreamEventMsg) (tea.Model, tea.Cmd) { /* route to chat/status */ }
func (m *Model) handleAgentDone(msg AgentDoneMsg) (tea.Model, tea.Cmd) { /* finalize */ }
func (m *Model) handleAgentError(msg AgentErrorMsg) (tea.Model, tea.Cmd) { /* show error */ }
func (m *Model) handleMouse(msg tea.MouseMsg) (tea.Model, tea.Cmd) { /* scroll + sidebar */ }
```

**Verify:** `go test ./internal/ui-b/ -run TestUpdate`
**Depends on:** Task 10

---

### Task 12: Create view.go — View() composing sub-model views

**Test first:**
```go
// internal/ui-b/view_test.go
func TestViewContainsHeader(t *testing.T) {
    m := NewModel(nil, "test-model", config.AgentConfig{}, nil, nil)
    m.width, m.height, m.ready = 80, 24, true
    view := m.View()
    if !strings.Contains(view, "Torus Agent") { t.Fatal("view should contain header") }
}
```

**Implementation:** `internal/ui-b/view.go`
```go
package uib

func (m Model) View() string {
    if !m.ready { return "Loading...\n" }
    var sb strings.Builder

    sb.WriteString(m.renderHeader())
    sb.WriteByte('\n')
    sb.WriteString(m.theme().Separator.Render(strings.Repeat("─", m.width)))
    sb.WriteByte('\n')

    chatView := m.chat.View()
    if m.sidebar.show {
        chatView = lipgloss.JoinHorizontal(lipgloss.Top, chatView, " ", m.sidebar.View(m.chatHeight()))
    }
    sb.WriteString(chatView)
    sb.WriteByte('\n')

    if m.overlay.Active() {
        sb.WriteString(m.overlay.View())
    } else {
        if m.input.acMode && len(m.input.acList) > 0 {
            sb.WriteString(m.input.renderAutocomplete())
            sb.WriteByte('\n')
        }
        sb.WriteString(m.status.renderProcessingOrCompletion(m.width))
        sb.WriteString(m.renderInputBorders())
    }

    sb.WriteString(m.status.renderStatusBar(m.width))
    return sb.String()
}

func (m Model) renderHeader() string { /* ◉ Torus Agent + model + branch */ }
func (m Model) renderInputBorders() string { /* border + input.View() + border */ }
```

**Verify:** `go test ./internal/ui-b/ -run TestView`
**Depends on:** Tasks 10, 11

---

### Task 13: Create commands.go — slash command dispatch

**Test first:**
```go
// internal/ui-b/commands_test.go
func TestCommandParsing(t *testing.T) {
    if !isCommand("/new") { t.Fatal("/new should be a command") }
    if isCommand("hello") { t.Fatal("hello should not be a command") }
}
```

**Implementation:** `internal/ui-b/commands.go`
```go
package uib

func isCommand(input string) bool { return strings.HasPrefix(input, "/") }

func (m *Model) executeCommand(cmd string) (tea.Model, tea.Cmd) {
    switch {
    case cmd == "/new": return m.cmdNew()
    case cmd == "/clear": return m.cmdClear()
    case cmd == "/compact": return m.cmdCompact()
    case cmd == "/exit": return m, tea.Quit
    case strings.HasPrefix(cmd, "/fork"): return m.cmdFork(strings.TrimPrefix(cmd, "/fork"))
    case strings.HasPrefix(cmd, "/switch"): return m.cmdSwitch(strings.TrimPrefix(cmd, "/switch"))
    case strings.HasPrefix(cmd, "/steering"): return m.cmdSteering(strings.TrimPrefix(cmd, "/steering"))
    case cmd == "/branches": return m.cmdBranches()
    case strings.HasPrefix(cmd, "/alias"): return m.cmdAlias(strings.TrimPrefix(cmd, "/alias"))
    case strings.HasPrefix(cmd, "/messages"): return m.cmdMessages(strings.TrimPrefix(cmd, "/messages"))
    case cmd == "/stats": return m.cmdStats()
    case cmd == "/agents": return m.cmdAgents()
    case cmd == "/mcp-tools": return m.cmdMCPTools()
    case cmd == "/skills": return m.cmdSkills()
    case cmd == "/sequential", cmd == "/parallel", cmd == "/loop":
        return m.cmdWorkflow(strings.TrimPrefix(cmd, "/"))
    default:
        // Check skills
        return m.cmdSkillDispatch(cmd)
    }
}
// Each cmd* method calls into internal/commands package (reuse existing logic)
```

**Verify:** `go test ./internal/ui-b/ -run TestCommand`
**Depends on:** Task 10

---

### Phase 4: Integration

---

### Task 14: Create tui.go — StartTUI entry point

**Test first:** `go build ./internal/ui-b/` compiles

**Implementation:** `internal/ui-b/tui.go`
```go
package uib

func StartTUI(agent *core.Agent, modelName string, cfg config.AgentConfig, skills *features.SkillRegistry, extras *TUIExtras) error {
    m := NewModel(agent, modelName, cfg, skills, extras)
    p := tea.NewProgram(m, tea.WithAltScreen(), tea.WithMouseCellMotion())
    _, err := p.Run()
    return err
}
```

**Verify:** `go build ./internal/ui-b/`
**Depends on:** Tasks 10, 11, 12

---

### Task 15: Create channel shim + wire to main

**Test first:** `go build ./...` compiles with new channel

**Implementation:**

`internal/channels/tui-b/tui.go`:
```go
package tuib

import (
    "torus_go_agent/internal/channels"
    "torus_go_agent/internal/config"
    "torus_go_agent/internal/core"
    "torus_go_agent/internal/features"
    uib "torus_go_agent/internal/ui-b"
)

func init() { channels.Register(&tuiBChannel{}) }

var Extras *uib.TUIExtras

type tuiBChannel struct{}
func (t *tuiBChannel) Name() string { return "tui-b" }
func (t *tuiBChannel) Start(agent *core.Agent, cfg config.Config, skills *features.SkillRegistry) error {
    return uib.StartTUI(agent, cfg.Agent.Model, cfg.Agent, skills, Extras)
}
```

`cmd/main.go`: add import `_ "torus_go_agent/internal/channels/tui-b"` and allow `--tui-b` flag to select channel.

**Verify:** `go build ./...`
**Depends on:** Task 14

---

### Task 16: Feature parity verification

**Test first:**
```go
// internal/ui-b/parity_test.go
func TestAllSlashCommandsExist(t *testing.T) {
    commands := []string{"/new", "/clear", "/compact", "/fork", "/switch",
        "/steering", "/branches", "/alias", "/messages", "/stats",
        "/agents", "/mcp-tools", "/skills", "/sequential", "/parallel", "/loop", "/exit"}
    for _, cmd := range commands {
        if !isCommand(cmd) { t.Fatalf("%s should be recognized", cmd) }
    }
}

func TestAllKeybindingsExist(t *testing.T) {
    // Verify Ctrl+K, Ctrl+B, Ctrl+W, Ctrl+U, Ctrl+A, Ctrl+E, @, ?, / are handled
    m := NewModel(nil, "test", config.AgentConfig{}, nil, nil)
    m.width, m.height, m.ready = 80, 24, true
    keys := []tea.KeyType{tea.KeyCtrlK, tea.KeyCtrlB, tea.KeyCtrlW, tea.KeyCtrlU, tea.KeyCtrlA, tea.KeyCtrlE}
    for _, k := range keys {
        _, _ = m.Update(tea.KeyMsg{Type: k})
        // Should not panic
    }
}
```

**Verify:** `go test ./internal/ui-b/ -v`
**Depends on:** Tasks 11, 13

---

## Verification (end-to-end)
1. `go build ./...` — all packages compile
2. `go vet ./...` — no issues
3. `go test ./...` — all pass (including existing TUI tests)
4. Manual: `go run cmd/main.go --channel tui-b` launches the new TUI
5. Manual: verify chat, streaming, overlays, sidebar, commands all work
6. Existing TUI unchanged: `go run cmd/main.go` still uses original

## Rollback
- Delete `internal/ui-b/` and `internal/channels/tui-b/`
- Remove import from `cmd/main.go`
- All changes are additive — no existing code modified except the channel flag in main
