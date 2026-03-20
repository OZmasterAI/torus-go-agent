package rawtui

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"golang.org/x/term"

	"go_sdk_agent/internal/commands"
	"go_sdk_agent/internal/core"
	"go_sdk_agent/internal/features"
)

// chatMsgKind identifies a message type in the display buffer.
type chatMsgKind int

const (
	msgUser chatMsgKind = iota
	msgAssistant
	msgToolCall
	msgToolResult
	msgInfo
	msgSuccess
	msgError
)

type chatMessage struct {
	kind      chatMsgKind
	content   string
	isError   bool
	duration  time.Duration
	leadBlank bool
}

const charsPerToken = 4

var funnyTexts = []string{
	"pondering the cosmos...",
	"consulting the oracle...",
	"herding electrons...",
	"untangling spaghetti...",
	"asking the rubber duck...",
	"dividing by zero...",
	"reticulating splines...",
	"compiling thoughts...",
	"traversing the astral plane...",
	"shaking the magic 8-ball...",
	"feeding the hamsters...",
	"polishing pixels...",
	"summoning the muse...",
}

// App is the raw terminal TUI application.
type App struct {
	// Terminal
	fd       int
	oldState *term.State
	width    int

	// Rendering
	prevRowCount  int
	sepRow        int
	inputStartRow int
	scrollShift   int

	// Input
	inputState
	stdinDup *os.File
	stdinCh  chan byte

	// Event channels
	resultCh chan any
	quit     bool

	// Chat state
	messages      []chatMessage
	agent         *core.Agent
	model         string
	contextWindow int
	skills        *features.SkillRegistry
	agentRunning  bool
	streamingText string
	needsTextSep  bool

	// Command completion
	cmdComplete    bool // true when showing command completions
	cmdMatches     []string
	cmdIdx         int

	// Pending tool call for box rendering
	pendingToolCall string
	toolStartTime   time.Time
	toolTimer       *time.Ticker

	// Agent status animation
	agentStartTime time.Time
	agentTicker    *time.Ticker
	agentElapsed   time.Duration
	agentTextIndex int

	// Token tracking
	sessionInputTokens  int
	sessionOutputTokens int

	// Ctrl+C double-tap
	ctrlCTime time.Time
	ctrlCHint bool
}

type ctrlCExpiredMsg struct{}
type resizeMsg struct{}
type toolTimerTickMsg struct{}
type agentTickMsg struct{}

// NewApp creates a new raw TUI app wired to the given agent.
func NewApp(agent *core.Agent, model string, contextWindow int, skills *features.SkillRegistry) *App {
	cwd, _ := os.Getwd()
	hist := NewHistory(cwd, 0)
	_ = hist.Load()
	return &App{
		agent:         agent,
		model:         model,
		contextWindow: contextWindow,
		skills:        skills,
		resultCh:      make(chan any, 16),
		inputState:    inputState{history: hist},
	}
}

// Run enters raw mode, runs the event loop, and restores the terminal on exit.
func (a *App) Run() error {
	fd := int(os.Stdin.Fd())
	oldState, err := term.MakeRaw(fd)
	if err != nil {
		return fmt.Errorf("entering raw mode: %w", err)
	}
	a.fd = fd
	a.oldState = oldState

	startTime := time.Now()

	defer func() {
		if r := recover(); r != nil {
			term.Restore(fd, oldState)
			panic(r)
		}
	}()

	// Enter alt-screen, enable bracketed paste
	fmt.Print("\033[?1049h")
	fmt.Print("\033[?2004h")
	defer func() {
		fmt.Print("\033[?25h")
		fmt.Print("\033[?2004l")
		fmt.Print("\033[?1049l")
		end := time.Now()
		fmt.Printf("[tui2 %s -> %s]\r\n",
			startTime.Format("Jan 02 15:04"),
			end.Format("Jan 02 15:04"))
		term.Restore(fd, oldState)
	}()

	a.width = getTerminalWidth()

	// SIGWINCH handler
	sigWinch := make(chan os.Signal, 1)
	signal.Notify(sigWinch, syscall.SIGWINCH)
	go func() {
		for range sigWinch {
			a.width = getTerminalWidth()
			a.resultCh <- resizeMsg{}
		}
	}()

	// Show startup info
	a.messages = append(a.messages, chatMessage{kind: msgInfo, content: fmt.Sprintf("Model: %s — type /help for commands", a.model)})

	a.render()

	// Start stdin reader
	a.stdinDup, a.stdinCh = startStdinReader()
	readByte := func() (byte, bool) {
		b, ok := <-a.stdinCh
		return b, ok
	}

	// Main event loop
	for {
		if a.agentRunning {
			select {
			case ch, ok := <-a.stdinCh:
				if !ok {
					goto done
				}
				a.drainResults()
				if a.handleByte(ch, a.stdinCh, readByte) {
					goto done
				}
			case result := <-a.resultCh:
				a.handleResult(result)
			}
		} else {
			select {
			case ch, ok := <-a.stdinCh:
				if !ok {
					goto done
				}
				a.drainResults()
				if a.handleByte(ch, a.stdinCh, readByte) {
					goto done
				}
			case result := <-a.resultCh:
				a.handleResult(result)
			}
		}
	}
done:
	a.cleanup()
	return nil
}

// ─── Styling ───

func styledUserMsg(content string) string {
	lines := strings.Split(content, "\n")
	lines[0] = "\033[1m▸ " + lines[0] + "\033[0m"
	for i := 1; i < len(lines); i++ {
		lines[i] = "\033[1m" + lines[i] + "\033[0m"
	}
	return strings.Join(lines, "\n")
}

func styledToolCall(s string) string { return "\033[2;3m" + s + "\033[0m" }
func styledToolResult(s string, isErr bool) string {
	if isErr {
		return "\033[31;3m" + s + "\033[0m"
	}
	return "\033[2m" + s + "\033[0m"
}
func styledError(s string) string   { return "\033[31;3m" + s + "\033[0m" }
func styledSuccess(s string) string { return "\033[32;3m" + s + "\033[0m" }
func styledInfo(s string) string    { return "\033[34;3m" + s + "\033[0m" }

func renderMessage(msg chatMessage) string {
	var parts []string
	if msg.leadBlank {
		parts = append(parts, "")
	}
	content := strings.ReplaceAll(msg.content, "\r", "")
	var rendered string
	switch msg.kind {
	case msgUser:
		rendered = styledUserMsg(content)
	case msgAssistant:
		rendered = content
	case msgToolCall:
		rendered = styledToolCall(content)
	case msgToolResult:
		rendered = styledToolResult(content, msg.isError)
	case msgInfo:
		rendered = styledInfo(content)
	case msgSuccess:
		rendered = styledSuccess(content)
	case msgError:
		rendered = styledError(content)
	}
	parts = append(parts, rendered)
	return strings.Join(parts, "\n")
}

// ─── Tool display helpers ───

func toolCallSummary(name string, args map[string]any) string {
	switch name {
	case "Bash", "bash":
		if cmd, ok := args["command"].(string); ok && cmd != "" {
			if len(cmd) > 120 {
				cmd = cmd[:120] + "..."
			}
			return fmt.Sprintf("~ $ %s", cmd)
		}
	}
	return fmt.Sprintf("~ %s", name)
}

func collapseToolResult(result string) string {
	lines := strings.Split(result, "\n")
	if len(lines) <= 4 {
		return result
	}
	if len(lines) == 5 {
		return strings.Join(lines[:2], "\n") + "\n" + strings.Join(lines[2:], "\n")
	}
	head := strings.Join(lines[:2], "\n")
	tail := strings.Join(lines[len(lines)-2:], "\n")
	return fmt.Sprintf("%s\n...\n%s", head, tail)
}

func renderToolBox(title, content string, maxWidth int, isError bool, durationStr string) string {
	content = strings.ReplaceAll(content, "\t", " ")
	titleVW := visibleWidth(title)
	innerWidth := titleVW + 2
	if content != "" {
		for _, line := range strings.Split(content, "\n") {
			if lw := visibleWidth(line); lw > innerWidth {
				innerWidth = lw
			}
		}
	}
	if durationStr != "" {
		if minW := len(durationStr) + 2; minW > innerWidth {
			innerWidth = minW
		}
	}
	if maxWidth > 0 && innerWidth > maxWidth-2 {
		innerWidth = maxWidth - 2
	}
	if maxTitleVW := innerWidth - 2; titleVW > maxTitleVW && maxTitleVW >= 0 {
		title = truncateWithEllipsis(title, maxTitleVW)
		titleVW = visibleWidth(title)
	}

	var borderStyle, titleStyle, contentStyle, reset string
	if isError {
		borderStyle = "\033[31m"
		titleStyle = "\033[31;3m"
		contentStyle = "\033[31m"
	} else {
		borderStyle = "\033[2m"
		titleStyle = "\033[2;3m"
		contentStyle = "\033[2m"
	}
	reset = "\033[0m"

	var b strings.Builder
	pad := innerWidth - titleVW - 2
	if pad < 0 {
		pad = 0
	}
	b.WriteString(borderStyle + "┌ " + reset + titleStyle + title + reset + borderStyle + " " + strings.Repeat("─", pad) + "┐" + reset)

	if content != "" {
		for _, line := range strings.Split(content, "\n") {
			b.WriteByte('\n')
			b.WriteString(contentStyle)
			if visibleWidth(line) > innerWidth {
				line = truncateVisual(line, innerWidth)
			}
			b.WriteString(line)
			b.WriteString(reset)
		}
	}

	b.WriteByte('\n')
	b.WriteString(borderStyle + "└")
	if durationStr != "" {
		durPad := innerWidth - len(durationStr) - 2
		if durPad < 0 {
			durPad = 0
		}
		b.WriteString(strings.Repeat("─", durPad) + " " + reset + titleStyle + durationStr + reset + borderStyle + " ┘")
	} else {
		b.WriteString(strings.Repeat("─", innerWidth) + "┘")
	}
	b.WriteString(reset)
	return b.String()
}

// ─── Rendering ───

func (a *App) buildBlockRows() []string {
	var rows []string
	rows = append(rows, "", fmt.Sprintf("  \033[1mTorus Agent\033[0m  \033[2m(%s)\033[0m", a.model), "")

	inCodeBlock := false
	skipNext := false
	for i, msg := range a.messages {
		if skipNext {
			skipNext = false
			goto blankLine
		}
		if msg.kind == msgToolCall {
			title := strings.ReplaceAll(msg.content, "\r", "")
			nextIdx := i + 1
			if nextIdx < len(a.messages) && a.messages[nextIdx].kind == msgToolResult {
				result := a.messages[nextIdx]
				content := strings.ReplaceAll(result.content, "\r", "")
				box := renderToolBox(title, content, a.width, result.isError, formatDuration(result.duration))
				if msg.leadBlank {
					rows = append(rows, "")
				}
				for _, logLine := range strings.Split(box, "\n") {
					rows = append(rows, wrapString(logLine, 0, a.width)...)
				}
				skipNext = true
				goto blankLine
			}
			if msg.leadBlank {
				rows = append(rows, "")
			}
			var liveDur string
			if !a.toolStartTime.IsZero() {
				liveDur = formatDuration(time.Since(a.toolStartTime))
			}
			box := renderToolBox(title, "", a.width, false, liveDur)
			if liveDur == "" {
				boxLines := strings.Split(box, "\n")
				if len(boxLines) > 1 {
					boxLines = boxLines[:len(boxLines)-1]
				}
				for _, logLine := range boxLines {
					rows = append(rows, wrapString(logLine, 0, a.width)...)
				}
			} else {
				for _, logLine := range strings.Split(box, "\n") {
					rows = append(rows, wrapString(logLine, 0, a.width)...)
				}
			}
			goto blankLine
		}
		if msg.kind == msgToolResult {
			content := strings.ReplaceAll(msg.content, "\r", "")
			box := renderToolBox("~ result", content, a.width, msg.isError, formatDuration(msg.duration))
			if msg.leadBlank {
				rows = append(rows, "")
			}
			for _, logLine := range strings.Split(box, "\n") {
				rows = append(rows, wrapString(logLine, 0, a.width)...)
			}
			goto blankLine
		}
		{
			rendered := renderMessage(msg)
			for _, logLine := range strings.Split(rendered, "\n") {
				wasInCodeBlock := inCodeBlock
				if msg.kind == msgAssistant {
					var skip bool
					logLine, inCodeBlock, skip = ProcessMarkdownLine(logLine, inCodeBlock)
					if skip {
						continue
					}
				}
				wrapped := wrapString(logLine, 0, a.width)
				if wasInCodeBlock && msg.kind == msgAssistant {
					for j := range wrapped {
						wrapped[j] = padCodeBlockRow(wrapped[j], a.width)
					}
				}
				rows = append(rows, wrapped...)
			}
		}
	blankLine:
		peekIdx := i + 1
		if skipNext {
			peekIdx = i + 2
		}
		peekHasBlank := peekIdx < len(a.messages) && a.messages[peekIdx].leadBlank
		peekIsAssistant := peekIdx < len(a.messages) && a.messages[peekIdx].kind == msgAssistant
		if !peekHasBlank && !(msg.kind == msgAssistant && peekIsAssistant) {
			rows = append(rows, "")
		}
	}

	if a.streamingText != "" {
		for _, logLine := range strings.Split(a.streamingText, "\n") {
			wasInCodeBlock := inCodeBlock
			var skip bool
			logLine, inCodeBlock, skip = ProcessMarkdownLine(logLine, inCodeBlock)
			if !skip {
				wrapped := wrapString(logLine, 0, a.width)
				if wasInCodeBlock {
					for j := range wrapped {
						wrapped[j] = padCodeBlockRow(wrapped[j], a.width)
					}
				}
				rows = append(rows, wrapped...)
			}
		}
		rows = append(rows, "")
	}

	if a.agentRunning {
		elapsed := time.Since(a.agentStartTime)
		text := funnyTexts[a.agentTextIndex]
		color := pastelColor(elapsed)
		label := fmt.Sprintf("%s\033[3m%s %.2fs\033[0m", color, text, elapsed.Seconds())
		rows = append(rows, label, "")
	} else if a.agentElapsed > 0 {
		rows = append(rows, fmt.Sprintf("\033[2m%.2fs\033[0m", a.agentElapsed.Seconds()), "")
	}

	return collapseBlankRows(rows)
}

func (a *App) buildInputRows() []string {
	sep := strings.Repeat("─", a.width)
	rows := []string{sep}

	vlines := getVisualLines(a.input, a.cursor, a.width)
	for i, vl := range vlines {
		line := string(a.input[vl.start : vl.start+vl.length])
		if i == 0 {
			line = promptPrefix + line
		}
		rows = append(rows, line)
	}
	rows = append(rows, sep)

	// Command completion dropdown
	if a.cmdComplete && len(a.cmdMatches) > 0 {
		for i, m := range a.cmdMatches {
			prefix := "  "
			if i == a.cmdIdx {
				prefix = "\033[7m "
				rows = append(rows, prefix+m+" \033[0m")
			} else {
				rows = append(rows, "\033[2m"+prefix+m+"\033[0m")
			}
		}
	}

	if a.ctrlCHint {
		rows = append(rows, "\033[1;38;5;4mPress Ctrl-C again to exit\033[0m")
	}

	// Status line
	contextTokens := a.sessionInputTokens + len(a.input)/charsPerToken
	cw := a.contextWindow
	if cw <= 0 {
		cw = 200000
	}
	bar := progressBar(contextTokens, cw)
	modelLabel := "\033[2m" + a.model + "\033[0m"
	modelWidth := len(a.model)
	barWidth := 3
	padding := a.width - modelWidth - barWidth - 1
	if padding < 0 {
		padding = 0
	}
	rows = append(rows, modelLabel+strings.Repeat(" ", padding)+bar+" ")

	return rows
}

func (a *App) positionCursor(buf *strings.Builder) {
	s := a.scrollShift
	buf.WriteString("\033[?25h")
	curLine, curCol := cursorVisualPos(a.input, a.cursor, a.width)
	buf.WriteString(fmt.Sprintf("\033[%d;%dH", a.inputStartRow+curLine-s, curCol+1))
}

func (a *App) render() {
	blockRows := a.buildBlockRows()
	a.sepRow = len(blockRows) + 1
	a.inputStartRow = a.sepRow + 1
	inputRows := a.buildInputRows()
	allRows := append(blockRows, inputRows...)
	totalRows := len(allRows)
	th := getTerminalHeight()
	newScrollShift := 0
	if totalRows > th {
		newScrollShift = totalRows - th
	}

	var buf strings.Builder
	if newScrollShift > 0 && a.scrollShift > 0 && newScrollShift >= a.scrollShift {
		if extra := newScrollShift - a.scrollShift; extra > 0 {
			buf.WriteString(fmt.Sprintf("\033[%d;1H", th))
			for i := 0; i < extra; i++ {
				buf.WriteString("\r\n")
			}
		}
		visibleRows := allRows[newScrollShift:]
		writeRows(&buf, visibleRows, 1)
	} else {
		if a.scrollShift > 0 {
			buf.WriteString("\033[3J")
		}
		writeRows(&buf, allRows, 1)
	}
	buf.WriteString("\033[0m\033[J")
	a.prevRowCount = totalRows
	a.scrollShift = newScrollShift
	a.positionCursor(&buf)
	os.Stdout.WriteString(buf.String())
}

func (a *App) renderFull() {
	a.scrollShift = 0
	os.Stdout.WriteString("\033[3J")
	a.render()
}

func (a *App) renderInput() {
	inputRows := a.buildInputRows()
	totalRows := a.sepRow - 1 + len(inputRows)
	th := getTerminalHeight()
	newScrollShift := 0
	if totalRows > th {
		newScrollShift = totalRows - th
	}
	if newScrollShift < a.scrollShift {
		a.render()
		return
	}
	screenSepRow := a.sepRow - a.scrollShift
	if screenSepRow < 1 {
		a.render()
		return
	}
	var buf strings.Builder
	writeRows(&buf, inputRows, screenSepRow)
	buf.WriteString("\033[0m\033[J")
	a.scrollShift = newScrollShift
	a.prevRowCount = totalRows
	a.positionCursor(&buf)
	os.Stdout.WriteString(buf.String())
}

// ─── Input handling ───

func (a *App) handleByte(ch byte, stdinCh chan byte, readByte func() (byte, bool)) bool {
	if ch == '\033' {
		a.handleEscapeSequence(stdinCh, readByte)
		return false
	}
	if ch == 4 { // Ctrl+D
		return true
	}
	if ch == 3 { // Ctrl+C
		if a.ctrlCHint && time.Since(a.ctrlCTime) < 2*time.Second {
			return true
		}
		if a.agentRunning {
			// Cancel doesn't exist on our agent yet, just ignore
		}
		a.input = nil
		a.cursor = 0
		a.ctrlCHint = true
		a.ctrlCTime = time.Now()
		a.renderInput()
		go func() {
			time.Sleep(2 * time.Second)
			a.resultCh <- ctrlCExpiredMsg{}
		}()
		return false
	}
	if a.ctrlCHint {
		a.ctrlCHint = false
		a.ctrlCTime = time.Time{}
	}
	if ch == 0x17 { // Ctrl+W
		a.deleteWordBackward()
		a.updateCmdComplete()
		a.renderInput()
		return false
	}
	if ch == 0x15 { // Ctrl+U
		a.killToStart()
		a.updateCmdComplete()
		a.renderInput()
		return false
	}
	if ch == 0x0b { // Ctrl+K
		a.killLine()
		a.renderInput()
		return false
	}
	if ch == 0x01 { // Ctrl+A
		a.cursor = 0
		a.renderInput()
		return false
	}
	if ch == 0x05 { // Ctrl+E
		a.cursor = len(a.input)
		a.renderInput()
		return false
	}
	if ch == '\t' {
		if a.cmdComplete && len(a.cmdMatches) > 0 {
			a.cmdIdx = (a.cmdIdx + 1) % len(a.cmdMatches)
			a.renderInput()
		} else {
			a.updateCmdComplete()
			if a.cmdComplete {
				a.renderInput()
			}
		}
		return false
	}
	if ch == '\n' { // Shift+Enter (LF)
		a.insertAtCursor('\n')
		a.renderInput()
		return false
	}
	if ch == '\r' { // Enter
		if a.cmdComplete && len(a.cmdMatches) > 0 {
			a.acceptCmdComplete()
			a.renderInput()
			return false
		}
		a.handleEnter()
		return false
	}
	if ch == 127 || ch == 0x08 { // Backspace
		if a.cursor > 0 {
			a.deleteBeforeCursor()
			a.updateCmdComplete()
			a.renderInput()
		}
		return false
	}
	// Regular character
	r, ok := readFullRune(ch, readByte)
	if !ok {
		return true
	}
	a.insertAtCursor(r)
	a.updateCmdComplete()
	a.renderInput()
	return false
}

func (a *App) handleEscapeSequence(stdinCh chan byte, readByte func() (byte, bool)) {
	var b byte
	var ok bool
	select {
	case b, ok = <-stdinCh:
		if !ok {
			return
		}
	case <-time.After(50 * time.Millisecond):
		// Bare Escape pressed
		if a.cmdComplete {
			a.dismissCmdComplete()
			a.renderInput()
			return
		}
		a.inputState.reset()
		a.renderInput()
		return
	}
	if b == '\r' { // Alt+Enter
		a.insertAtCursor('\n')
		a.renderInput()
		return
	}
	if b == 0x7F { // Alt+Backspace
		a.deleteWordBackward()
		a.renderInput()
		return
	}
	if b != '[' {
		return
	}
	b, ok = readByte()
	if !ok {
		return
	}
	if b == '2' {
		a.handleCSIDigit2(readByte)
		return
	}
	if b == '1' {
		a.handleModifiedCSI(readByte)
		return
	}
	if b >= '3' && b <= '6' {
		tilde, ok := readByte()
		if !ok {
			return
		}
		if tilde == '~' && b == '3' {
			a.deleteAtCursor()
			a.renderInput()
		}
		return
	}
	switch b {
	case 'A': // Up
		lineIdx, _ := cursorVisualPos(a.input, a.cursor, a.width)
		if lineIdx == 0 && a.history != nil {
			if val, changed := a.history.Up(a.value()); changed {
				a.setValue(val)
			}
		} else {
			a.moveUp(a.width)
		}
		a.renderInput()
	case 'B': // Down
		lineIdx, _ := cursorVisualPos(a.input, a.cursor, a.width)
		vlines := getVisualLines(a.input, a.cursor, a.width)
		if lineIdx >= len(vlines)-1 && a.history != nil {
			if val, changed := a.history.Down(a.value()); changed {
				a.setValue(val)
			}
		} else {
			a.moveDown(a.width)
		}
		a.renderInput()
	case 'C': // Right
		if a.cursor < len(a.input) {
			a.cursor++
			a.renderInput()
		}
	case 'D': // Left
		if a.cursor > 0 {
			a.cursor--
			a.renderInput()
		}
	case 'H': // Home
		a.cursor = 0
		a.renderInput()
	case 'F': // End
		a.cursor = len(a.input)
		a.renderInput()
	}
}

func (a *App) handleCSIDigit2(readByte func() (byte, bool)) {
	b0, ok := readByte()
	if !ok {
		return
	}
	b1, ok := readByte()
	if !ok {
		return
	}
	b2, ok := readByte()
	if !ok {
		return
	}
	if b0 == '0' && b1 == '0' && b2 == '~' {
		content := readBracketedPaste(readByte)
		content = strings.ReplaceAll(content, "\r", "")
		a.insertText(content)
		a.renderInput()
		return
	}
}

func (a *App) handleModifiedCSI(readByte func() (byte, bool)) {
	semi, ok := readByte()
	if !ok || semi != ';' {
		return
	}
	modByte, ok := readByte()
	if !ok {
		return
	}
	letter, ok := readByte()
	if !ok {
		return
	}
	modNum := int(modByte-'0') - 1
	isAlt := modNum&2 != 0
	isCtrl := modNum&4 != 0
	switch letter {
	case 'C': // Right
		if isCtrl || isAlt {
			a.moveWordRight()
		} else if a.cursor < len(a.input) {
			a.cursor++
		}
		a.renderInput()
	case 'D': // Left
		if isCtrl || isAlt {
			a.moveWordLeft()
		} else if a.cursor > 0 {
			a.cursor--
		}
		a.renderInput()
	case 'H':
		a.cursor = 0
		a.renderInput()
	case 'F':
		a.cursor = len(a.input)
		a.renderInput()
	}
}

func (a *App) handleEnter() {
	if a.agentRunning {
		return
	}
	a.dismissCmdComplete()
	val := strings.TrimSpace(strings.ReplaceAll(a.value(), "\r", ""))
	if val == "" {
		return
	}
	a.agentElapsed = 0
	if a.history != nil {
		a.history.Add(val)
	}

	// Handle slash commands
	if strings.HasPrefix(val, "/") {
		if a.handleCommand(val) {
			return
		}
	}

	a.messages = append(a.messages, chatMessage{kind: msgUser, content: val, leadBlank: true})
	a.inputState.reset()
	a.startAgent(val)
	a.render()
}

// builtinCommands returns the list of built-in slash command names.
func (a *App) builtinCommands() []string {
	return []string{"/clear", "/exit", "/quit", "/help", "/new", "/skills"}
}

// allCommands returns built-in + skill command names.
func (a *App) allCommands() []string {
	cmds := a.builtinCommands()
	if a.skills != nil {
		for _, s := range a.skills.List() {
			cmds = append(cmds, "/"+s.Name)
		}
	}
	return cmds
}

// handleCommand processes a slash command. Returns true if the input was consumed.
func (a *App) handleCommand(val string) bool {
	// Extract command name (first token)
	fields := strings.Fields(val)
	cmd := fields[0]

	switch cmd {
	case "/clear":
		commands.Clear(a.agent.DAG(), a.agent.Hooks())
		a.messages = nil
		a.streamingText = ""
		a.pendingToolCall = ""
		a.sessionInputTokens = 0
		a.sessionOutputTokens = 0
		a.messages = append(a.messages, chatMessage{kind: msgSuccess, content: "Context cleared."})
		a.inputState.reset()
		a.render()
		return true

	case "/exit", "/quit":
		a.quit = true
		return true

	case "/help":
		a.showHelp()
		a.inputState.reset()
		a.render()
		return true

	case "/new":
		a.handleNewBranch()
		a.inputState.reset()
		a.render()
		return true

	case "/skills":
		a.showSkills()
		a.inputState.reset()
		a.render()
		return true

	default:
		// Check skill commands
		if a.skills != nil {
			if skillName, ok := a.skills.IsSkillCommand(val); ok {
				if skill, found := a.skills.Get(skillName); found {
					beforeSkill := &core.HookData{
						AgentID: "main",
						Meta:    map[string]any{"skill": skillName, "input": val},
					}
					a.agent.Hooks().Fire(context.Background(), core.HookBeforeSkill, beforeSkill)
					if !beforeSkill.Block {
						prompt := a.skills.FormatSkillPrompt(skill, val)
						a.agent.Hooks().Fire(context.Background(), core.HookAfterSkill, &core.HookData{
							AgentID: "main",
							Meta:    map[string]any{"skill": skillName, "input": val},
						})
						a.messages = append(a.messages, chatMessage{
							kind: msgUser, content: val, leadBlank: true,
						})
						a.inputState.reset()
						a.startAgent(prompt)
						a.render()
						return true
					}
				}
			}
		}
	}
	return false
}

func (a *App) handleNewBranch() {
	newBranch, _ := commands.New(a.agent.DAG(), a.agent.Hooks())
	a.messages = nil
	a.streamingText = ""
	a.pendingToolCall = ""
	a.sessionInputTokens = 0
	a.sessionOutputTokens = 0
	a.messages = append(a.messages, chatMessage{
		kind: msgSuccess, content: fmt.Sprintf("New conversation started (branch: %s).", newBranch),
	})
}

func (a *App) showHelp() {
	var sb strings.Builder
	sb.WriteString("Commands:\n")
	sb.WriteString("  /help     — Show this help\n")
	sb.WriteString("  /clear    — Clear context on current branch\n")
	sb.WriteString("  /new      — Start new conversation branch\n")
	sb.WriteString("  /skills   — List available skills\n")
	sb.WriteString("  /exit     — Exit\n")
	sb.WriteString("\n")
	sb.WriteString("Keys:\n")
	sb.WriteString("  Enter       — Send message\n")
	sb.WriteString("  Shift+Enter — New line\n")
	sb.WriteString("  Tab         — Complete /command\n")
	sb.WriteString("  Ctrl+C ×2   — Exit\n")
	sb.WriteString("  Ctrl+D      — Exit\n")
	sb.WriteString("  Ctrl+W      — Delete word\n")
	sb.WriteString("  Ctrl+U      — Clear to start of line\n")
	sb.WriteString("  Ctrl+A/E    — Home / End")
	a.messages = append(a.messages, chatMessage{kind: msgInfo, content: sb.String(), leadBlank: true})
}

func (a *App) showSkills() {
	if a.skills == nil {
		a.messages = append(a.messages, chatMessage{kind: msgInfo, content: "No skills loaded."})
		return
	}
	list := a.skills.List()
	if len(list) == 0 {
		a.messages = append(a.messages, chatMessage{kind: msgInfo, content: "No skills found."})
		return
	}
	var sb strings.Builder
	sb.WriteString("Available skills:\n")
	for _, s := range list {
		sb.WriteString(fmt.Sprintf("  /%s — %s\n", s.Name, s.Description))
	}
	a.messages = append(a.messages, chatMessage{kind: msgInfo, content: strings.TrimRight(sb.String(), "\n"), leadBlank: true})
}

// ─── Command completion ───

func (a *App) updateCmdComplete() {
	val := a.value()
	if !strings.HasPrefix(val, "/") || strings.Contains(val, " ") || strings.Contains(val, "\n") {
		a.dismissCmdComplete()
		return
	}
	prefix := strings.ToLower(val)
	var matches []string
	for _, cmd := range a.allCommands() {
		if strings.HasPrefix(strings.ToLower(cmd), prefix) && cmd != val {
			matches = append(matches, cmd)
		}
	}
	if len(matches) == 0 {
		a.dismissCmdComplete()
		return
	}
	a.cmdComplete = true
	a.cmdMatches = matches
	if a.cmdIdx >= len(matches) {
		a.cmdIdx = 0
	}
}

func (a *App) dismissCmdComplete() {
	a.cmdComplete = false
	a.cmdMatches = nil
	a.cmdIdx = 0
}

func (a *App) acceptCmdComplete() {
	if !a.cmdComplete || len(a.cmdMatches) == 0 {
		return
	}
	selected := a.cmdMatches[a.cmdIdx]
	a.setValue(selected)
	a.dismissCmdComplete()
}

// ─── Agent ───

func (a *App) startAgent(userMessage string) {
	a.agentRunning = true
	a.streamingText = ""
	a.needsTextSep = true
	a.agentStartTime = time.Now()
	a.agentElapsed = 0
	a.agentTextIndex = 0

	if a.agentTicker != nil {
		a.agentTicker.Stop()
	}
	a.agentTicker = time.NewTicker(50 * time.Millisecond)
	go func(ticker *time.Ticker, ch chan any) {
		for range ticker.C {
			ch <- agentTickMsg{}
		}
	}(a.agentTicker, a.resultCh)

	eventCh := a.agent.RunStream(context.Background(), userMessage)
	go func() {
		for ev := range eventCh {
			a.resultCh <- ev
		}
	}()
}

func (a *App) handleAgentEvent(event core.AgentEvent) {
	switch event.Type {
	case core.EventAgentTextDelta:
		a.streamingText += event.Text
		if idx := strings.LastIndex(a.streamingText, "\n"); idx >= 0 {
			a.messages = append(a.messages, chatMessage{
				kind:      msgAssistant,
				content:   a.streamingText[:idx],
				leadBlank: a.needsTextSep,
			})
			a.needsTextSep = false
			a.streamingText = a.streamingText[idx+1:]
		}
		a.render()

	case core.EventAgentToolStart:
		if a.streamingText != "" {
			a.messages = append(a.messages, chatMessage{
				kind:      msgAssistant,
				content:   a.streamingText,
				leadBlank: a.needsTextSep,
			})
			a.needsTextSep = false
			a.streamingText = ""
		}
		summary := toolCallSummary(event.ToolName, event.ToolArgs)
		a.messages = append(a.messages, chatMessage{kind: msgToolCall, content: summary, leadBlank: true})
		a.toolStartTime = time.Now()
		if a.toolTimer != nil {
			a.toolTimer.Stop()
		}
		a.toolTimer = time.NewTicker(100 * time.Millisecond)
		go func(ticker *time.Ticker, ch chan any) {
			for range ticker.C {
				ch <- toolTimerTickMsg{}
			}
		}(a.toolTimer, a.resultCh)
		a.render()

	case core.EventAgentToolEnd:
		if a.toolTimer != nil {
			a.toolTimer.Stop()
			a.toolTimer = nil
		}
		dur := time.Since(a.toolStartTime)
		a.toolStartTime = time.Time{}
		result := ""
		isErr := false
		if event.ToolResult != nil {
			result = collapseToolResult(event.ToolResult.Content)
			isErr = event.ToolResult.IsError
		}
		a.needsTextSep = true
		a.messages = append(a.messages, chatMessage{kind: msgToolResult, content: result, isError: isErr, duration: dur})
		a.render()

	case core.EventAgentDone:
		a.agentRunning = false
		if a.agentTicker != nil {
			a.agentTicker.Stop()
			a.agentTicker = nil
		}
		a.agentElapsed = time.Since(a.agentStartTime)
		if a.streamingText != "" {
			a.messages = append(a.messages, chatMessage{
				kind:      msgAssistant,
				content:   a.streamingText,
				leadBlank: a.needsTextSep,
			})
			a.streamingText = ""
		}
		a.render()

	case core.EventAgentError:
		a.agentRunning = false
		if a.agentTicker != nil {
			a.agentTicker.Stop()
			a.agentTicker = nil
		}
		a.agentElapsed = time.Since(a.agentStartTime)
		errMsg := "Agent error"
		if event.Error != nil {
			errMsg = event.Error.Error()
		}
		a.messages = append(a.messages, chatMessage{kind: msgError, content: errMsg})
		a.render()
	}
}

// ─── Async results ───

func (a *App) drainResults() {
	for {
		select {
		case result := <-a.resultCh:
			a.handleResult(result)
		default:
			return
		}
	}
}

func (a *App) handleResult(result any) {
	switch msg := result.(type) {
	case core.AgentEvent:
		a.handleAgentEvent(msg)
		return
	case toolTimerTickMsg:
		a.render()
		return
	case agentTickMsg:
		if a.agentRunning {
			elapsed := time.Since(a.agentStartTime)
			a.agentTextIndex = int(elapsed.Seconds()/3) % len(funnyTexts)
		}
		a.render()
		return
	case ctrlCExpiredMsg:
		if a.ctrlCHint {
			a.ctrlCHint = false
			a.ctrlCTime = time.Time{}
			a.renderInput()
		}
		return
	case resizeMsg:
		a.width = getTerminalWidth()
		a.renderFull()
		return
	}
}

func (a *App) cleanup() {
	if a.toolTimer != nil {
		a.toolTimer.Stop()
	}
	if a.agentTicker != nil {
		a.agentTicker.Stop()
	}
	stopStdinReader(a.stdinDup, a.stdinCh)
}

// Ensure json is used (for toolCallSummary args)
var _ = json.Marshal
var _ = math.Round
