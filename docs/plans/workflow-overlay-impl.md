# Implementation Plan: Interactive Workflow Overlay

## Design Decision
Option C — single TUI overlay for `/sequential`, `/parallel`, `/loop` commands.
User adds agents step-by-step (task + type), then runs. Overlay shared across all 3 workflow types.

## Success Criteria
- `/sequential`, `/parallel`, `/loop` open the overlay from both input and command palette
- User can add multiple agents with task + type (builder/researcher/tester)
- For `/loop`: user can set max_iterations and stop_phrase
- Enter on "Run" executes the workflow via existing RunSequential/RunParallel/RunLoop
- Results display in chat as an assistant message
- Esc cancels at any point
- All existing tests still pass (`go test ./...`)

## Tasks

### Task 1: Add Provider() and SystemPrompt() getters to Agent

The TUI needs access to the provider and system prompt to call workflow functions.
Currently these are unexported fields on Agent.

**Test first:**
```go
// In internal/core/loop_test.go
func TestAgentGetters(t *testing.T) {
    mp := &mockProvider{name: "mock", modelID: "m1"}
    cfg := typ.AgentConfig{SystemPrompt: "test prompt"}
    agent := core.NewAgent(cfg, mp, nil, nil)
    if agent.Provider() == nil { t.Fatal("Provider() returned nil") }
    if agent.SystemPrompt() != "test prompt" { t.Fatal("SystemPrompt() wrong") }
}
```

**Implementation:**
- File: `internal/core/loop.go` after line 341 (existing getters)
- Add:
  ```go
  func (a *Agent) Provider() t.Provider   { return a.provider }
  func (a *Agent) SystemPrompt() string   { return a.config.SystemPrompt }
  ```

**Verify:** `go test ./internal/core/`
**Depends on:** nothing

---

### Task 2: Add overlayWorkflow mode and state fields

**Test first:** `go build ./...` (compiles with new fields)

**Implementation:**
- File: `internal/ui/tui.go`
- Add to overlayMode enum (~line 159):
  ```go
  overlayWorkflow  // workflow builder (/sequential, /parallel, /loop)
  ```
- Add workflow state struct before Model:
  ```go
  type workflowAgent struct {
      Task      string
      AgentType string // "builder", "researcher", "tester"
  }

  type workflowState struct {
      mode          string // "sequential", "parallel", "loop"
      agents        []workflowAgent
      editIdx       int    // which field is focused: 0=task, 1=type, 2=actions
      taskInput     string // current task being typed
      typeIdx       int    // 0=builder, 1=researcher, 2=tester
      // loop-only fields
      maxIterations int
      stopPhrase    string
      loopField     int    // 0=task, 1=type, 2=max_iter, 3=stop_phrase, 4=actions
  }
  ```
- Add to Model struct (~line 278):
  ```go
  workflow workflowState
  ```

**Verify:** `go build ./...`
**Depends on:** nothing

---

### Task 3: Wire slash commands to open the overlay

**Test first:** Verify builds, manual test that `/sequential` opens overlay

**Implementation:**

File: `internal/ui/tui.go`
- In input dispatch (~line 610), add before the skills check:
  ```go
  if input == "/sequential" || input == "/parallel" || input == "/loop" {
      return m.openWorkflowOverlay(strings.TrimPrefix(input, "/"))
  }
  ```
- In `executePaletteCommand` switch (~line 1154), add cases:
  ```go
  case "/sequential", "/parallel", "/loop":
      return m.openWorkflowOverlay(strings.TrimPrefix(cmd, "/"))
  ```
- Add palette entries in `defaultPaletteCommands`:
  ```go
  {name: "Run sequential workflow", command: "/sequential"},
  {name: "Run parallel workflow", command: "/parallel"},
  {name: "Run loop workflow", command: "/loop"},
  ```
- Add the opener function:
  ```go
  func (m *Model) openWorkflowOverlay(mode string) (tea.Model, tea.Cmd) {
      m.overlay = overlayWorkflow
      m.workflow = workflowState{
          mode:          mode,
          typeIdx:       0,
          maxIterations: 5,
      }
      m.input = ""
      m.cursorPos = 0
      return m, nil
  }
  ```

**Verify:** `go build ./...`
**Depends on:** Task 2

---

### Task 4: Handle keys in overlayWorkflow

**Test first:** `go build ./...`

**Implementation:**

File: `internal/ui/tui.go` — add case in `handleOverlayKey` (~line 1043):

```go
case overlayWorkflow:
    return m.handleWorkflowKey(msg)
```

New method in `internal/ui/tui_commands.go`:

```go
var workflowTypes = []string{"builder", "researcher", "tester"}

func (m *Model) handleWorkflowKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
    switch msg.Type {
    case tea.KeyEscape:
        m.overlay = overlayNone
        return m, nil
    case tea.KeyTab:
        // Cycle focus: task → type → [loop fields] → actions
        m.workflow.editIdx = (m.workflow.editIdx + 1) % m.workflowFieldCount()
        return m, nil
    case tea.KeyShiftTab:
        m.workflow.editIdx = (m.workflow.editIdx - 1 + m.workflowFieldCount()) % m.workflowFieldCount()
        return m, nil
    case tea.KeyEnter:
        return m.handleWorkflowEnter()
    case tea.KeyBackspace:
        return m.handleWorkflowBackspace()
    case tea.KeyUp, tea.KeyDown:
        // For type selector: cycle through types
        if m.workflowOnTypeField() {
            if msg.Type == tea.KeyUp {
                m.workflow.typeIdx = (m.workflow.typeIdx - 1 + 3) % 3
            } else {
                m.workflow.typeIdx = (m.workflow.typeIdx + 1) % 3
            }
        }
        return m, nil
    default:
        // Type into the active text field
        if msg.Type == tea.KeyRunes || msg.Type == tea.KeySpace {
            ch := string(msg.Runes)
            if msg.Type == tea.KeySpace { ch = " " }
            m.workflowTypeChar(ch)
        }
        return m, nil
    }
}
```

`handleWorkflowEnter` logic:
- If on "Add agent" action: append current task+type to agents, reset taskInput
- If on "Run" action: close overlay, execute workflow (Task 5)
- If on "Remove last": pop last agent

Helper methods:
- `workflowFieldCount()`: returns field count based on mode (loop has extra fields)
- `workflowOnTypeField()`: true if editIdx is on the type selector
- `workflowTypeChar(ch)`: appends char to the active text field (taskInput, stopPhrase, or maxIterations)
- `handleWorkflowBackspace()`: deletes last char from active text field

**Verify:** `go build ./...`
**Depends on:** Task 3

---

### Task 5: Execute workflow on "Run"

**Test first:** `go test ./...` (existing workflow tests still pass)

**Implementation:**

File: `internal/ui/tui_commands.go`:

```go
func (m *Model) executeWorkflow() (tea.Model, tea.Cmd) {
    if m.subMgr == nil {
        m.messages = append(m.messages, displayMsg{role: "error", text: "Sub-agent manager not available", isError: true})
        m.overlay = overlayNone
        m.rebuildContent()
        return m, nil
    }

    prov := m.agent.Provider()
    soul := m.agent.SystemPrompt()
    dag := m.agent.DAG()

    // Build SubAgentConfigs
    configs := make([]features.SubAgentConfig, 0, len(m.workflow.agents))
    for _, wa := range m.workflow.agents {
        configs = append(configs, features.SubAgentConfig{
            Task:      wa.Task,
            AgentType: wa.AgentType,
            Tools:     features.DefaultToolsForType(wa.AgentType),
            MaxTurns:  20,
        })
    }

    m.overlay = overlayNone

    // Show "running" message
    m.messages = append(m.messages, displayMsg{
        role: "assistant",
        text: fmt.Sprintf("Running %s workflow with %d agent(s)...", m.workflow.mode, len(configs)),
    })
    m.rebuildContent()

    // Execute in goroutine, send result as tea.Msg
    go func() {
        var resultText string
        var err error
        switch m.workflow.mode {
        case "sequential":
            resultText, err = features.RunSequential(context.Background(), dag, prov, soul, configs, m.subMgr, m.agent)
        case "parallel":
            results, e := features.RunParallel(context.Background(), dag, prov, soul, configs, m.subMgr, m.agent)
            err = e
            if err == nil {
                var sb strings.Builder
                for i, r := range results {
                    fmt.Fprintf(&sb, "=== Agent %d ===\n", i+1)
                    if r.Error != nil {
                        fmt.Fprintf(&sb, "Error: %s\n", r.Error.Error())
                    } else {
                        fmt.Fprintf(&sb, "%s\n", r.Text)
                    }
                }
                resultText = sb.String()
            }
        case "loop":
            cfg := configs[0]
            stopPhrase := m.workflow.stopPhrase
            shouldStop := func(result string, iteration int) bool {
                return stopPhrase != "" && strings.Contains(result, stopPhrase)
            }
            resultText, err = features.RunLoop(context.Background(), dag, prov, soul, cfg, m.subMgr, m.agent, shouldStop, m.workflow.maxIterations)
        }
        // Send result back via channel (deltaCh or statusCh pattern)
        // For simplicity, write directly — TUI Update handles workflowDoneMsg
    }()

    return m, nil
}
```

Note: Need a `workflowDoneMsg` type + tea.Cmd pattern to send results back asynchronously.
This follows the same pattern as `runAgentStream` — use a channel or tea.Cmd callback.

**Verify:** `go build ./...`, `go test ./...`
**Depends on:** Tasks 1, 4

---

### Task 6: Render the overlay

**Test first:** `go build ./...`

**Implementation:**

File: `internal/ui/tui.go`
- Add case in `renderOverlay()` (~line 1607):
  ```go
  case overlayWorkflow:
      return m.renderWorkflow()
  ```

New method:
```go
func (m Model) renderWorkflow() string {
    var sb strings.Builder

    title := strings.ToUpper(m.workflow.mode)
    sb.WriteString(styleOverlayTitle.Render("Workflow: " + title))
    sb.WriteString("  " + styleOverlayHint.Render("Tab cycle · ↑↓ type · Enter action · Esc cancel"))
    sb.WriteByte('\n')

    // Show added agents
    for i, a := range m.workflow.agents {
        sb.WriteString(fmt.Sprintf("  Agent %d: %s [%s]\n", i+1, a.Task, a.AgentType))
    }

    // Current agent input fields
    sb.WriteByte('\n')
    taskStyle, typeStyle := styleDim, styleDim
    if m.workflow.editIdx == 0 { taskStyle = styleACSelected }
    if m.workflow.editIdx == 1 { typeStyle = styleACSelected }

    sb.WriteString(taskStyle.Render(fmt.Sprintf("  Task: %s", m.workflow.taskInput + "█")))
    sb.WriteByte('\n')

    for i, t := range workflowTypes {
        s := styleDim
        if i == m.workflow.typeIdx { s = styleACSelected }
        if m.workflow.editIdx == 1 { /* highlight selector */ }
        sb.WriteString("  " + s.Render(t))
    }
    sb.WriteByte('\n')

    // Loop-specific fields
    if m.workflow.mode == "loop" {
        // max iterations, stop phrase fields
    }

    // Action buttons
    sb.WriteByte('\n')
    actions := []string{"[Add agent]", "[Run]", "[Remove last]"}
    // Highlight based on editIdx
    // ...

    return sb.String()
}
```

**Verify:** `go build ./...`, visual inspection
**Depends on:** Tasks 2, 4

---

## Verification (end-to-end)
1. `go build ./...` — compiles
2. `go vet ./...` — clean
3. `go test ./...` — all pass
4. Manual: run binary, type `/sequential`, add 2 agents, hit Run
5. Manual: run binary, type `/loop`, set iterations + stop phrase, hit Run

## Rollback
- Revert overlay enum + state fields from tui.go
- Revert command dispatch additions
- Revert Agent getters (Provider/SystemPrompt)
- All changes are additive — no existing code modified except dispatch chains
