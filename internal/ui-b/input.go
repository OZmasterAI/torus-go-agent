package uib

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

const acMaxResults = 8

// inputModel handles text input with cursor movement, wrapping, and
// @-autocomplete for file paths.
type inputModel struct {
	theme     Theme
	value     string
	cursorPos int
	width     int

	// Autocomplete state.
	acMode   bool
	acQuery  string
	acList   []string
	acIdx    int
	acFiles  []string
	acLoaded bool
}

func newInputModel(theme Theme, width int) inputModel {
	return inputModel{theme: theme, width: width}
}

// Update handles key events. Returns submitted=true when Enter is pressed
// with non-empty input (and not in autocomplete mode).
func (m *inputModel) Update(msg tea.Msg) (submitted bool, cmd tea.Cmd) {
	keyMsg, ok := msg.(tea.KeyMsg)
	if !ok {
		return false, nil
	}

	// Autocomplete key handling.
	if m.acMode {
		switch keyMsg.Type {
		case tea.KeyTab:
			if len(m.acList) > 0 {
				m.acIdx = (m.acIdx + 1) % len(m.acList)
			}
			return false, nil
		case tea.KeyShiftTab:
			if len(m.acList) > 0 {
				m.acIdx = (m.acIdx - 1 + len(m.acList)) % len(m.acList)
			}
			return false, nil
		case tea.KeyEnter:
			if len(m.acList) > 0 {
				atPos := strings.LastIndex(m.value, "@")
				if atPos >= 0 {
					replacement := m.acList[m.acIdx] + " "
					m.value = m.value[:atPos] + replacement
					m.cursorPos = len([]rune(m.value))
				}
			}
			m.acMode = false
			return false, nil
		case tea.KeyEscape:
			m.acMode = false
			return false, nil
		case tea.KeyBackspace:
			runes := []rune(m.value)
			pos := m.cursorPos
			if pos > len(runes) {
				pos = len(runes)
			}
			if pos > 0 {
				m.value = string(runes[:pos-1]) + string(runes[pos:])
				m.cursorPos = pos - 1
				m.updateAutocomplete()
			}
			return false, nil
		default:
			if keyMsg.Type == tea.KeyRunes {
				char := string(keyMsg.Runes)
				m.insertAtCursor(char)
				if char == " " || char == "\t" {
					m.acMode = false
				} else {
					m.updateAutocomplete()
				}
			} else if keyMsg.Type == tea.KeySpace {
				m.insertAtCursor(" ")
				m.acMode = false
			}
			return false, nil
		}
	}

	// Normal key handling.
	switch keyMsg.Type {
	case tea.KeyEnter:
		input := strings.TrimSpace(m.value)
		if input == "" {
			return false, nil
		}
		return true, nil

	case tea.KeyBackspace:
		runes := []rune(m.value)
		pos := m.cursorPos
		if pos > len(runes) {
			pos = len(runes)
		}
		if pos > 0 {
			deleted := runes[pos-1]
			m.value = string(runes[:pos-1]) + string(runes[pos:])
			m.cursorPos = pos - 1
			if deleted == '@' {
				m.acMode = false
			}
		}
		return false, nil

	case tea.KeyDelete:
		runes := []rune(m.value)
		pos := m.cursorPos
		if pos < len(runes) {
			m.value = string(runes[:pos]) + string(runes[pos+1:])
		}
		return false, nil

	case tea.KeyLeft:
		if m.cursorPos > 0 {
			m.cursorPos--
		}
		return false, nil

	case tea.KeyRight:
		runes := []rune(m.value)
		if m.cursorPos < len(runes) {
			m.cursorPos++
		}
		return false, nil

	case tea.KeyCtrlW:
		m.deleteWordBackward()
		return false, nil

	case tea.KeyCtrlU:
		m.killToLineStart()
		return false, nil

	case tea.KeyCtrlA:
		m.moveToLineStart()
		return false, nil

	case tea.KeyCtrlE:
		m.moveToLineEnd()
		return false, nil

	case tea.KeyCtrlLeft:
		m.moveToPrevWord()
		return false, nil

	case tea.KeyCtrlRight:
		m.moveToNextWord()
		return false, nil

	default:
		if keyMsg.Type == tea.KeyRunes {
			if keyMsg.Paste {
				m.insertAtCursor(string(keyMsg.Runes))
				return false, nil
			}
			char := string(keyMsg.Runes)
			if strings.HasPrefix(char, "[") || strings.HasPrefix(char, "<") {
				return false, nil
			}
			m.insertAtCursor(char)
			if char == "@" {
				m.acMode = true
				m.acQuery = ""
				m.acIdx = 0
				m.ensureFileList()
				m.acList = m.acFiles
			}
		} else if keyMsg.Type == tea.KeySpace {
			m.insertAtCursor(" ")
		}
		return false, nil
	}
}

// View renders the input prompt with cursor and placeholder.
func (m inputModel) View() string {
	prompt := m.theme.Prompt.Render("\u276f ")
	promptWidth := 2
	if m.value == "" {
		return prompt + m.theme.Dim.Render("Type a message...") + m.theme.Cursor.Render("\u2502")
	}
	runes := []rune(m.value)
	pos := m.cursorPos
	if pos > len(runes) {
		pos = len(runes)
	}
	before := string(runes[:pos])
	cursor := m.theme.Cursor.Render("\u2502")
	after := string(runes[pos:])
	raw := before + cursor + after

	lineW := m.width
	if lineW <= 0 {
		return prompt + raw
	}
	firstW := lineW - promptWidth
	if firstW <= 0 {
		firstW = lineW
	}

	var lines []string
	remaining := []rune(raw)
	if len(remaining) <= firstW {
		lines = append(lines, string(remaining))
		remaining = nil
	} else {
		lines = append(lines, string(remaining[:firstW]))
		remaining = remaining[firstW:]
	}
	for len(remaining) > 0 {
		w := lineW - promptWidth
		if w <= 0 {
			w = lineW
		}
		if len(remaining) <= w {
			lines = append(lines, string(remaining))
			remaining = nil
		} else {
			lines = append(lines, string(remaining[:w]))
			remaining = remaining[w:]
		}
	}

	indent := strings.Repeat(" ", promptWidth)
	var sb strings.Builder
	for i, line := range lines {
		if i == 0 {
			sb.WriteString(prompt + line)
		} else {
			sb.WriteByte('\n')
			sb.WriteString(indent + line)
		}
	}
	return sb.String()
}

// Value returns the current input text.
func (m inputModel) Value() string { return m.value }

// SetValue replaces the input text and moves cursor to end.
func (m *inputModel) SetValue(s string) {
	m.value = s
	m.cursorPos = len([]rune(s))
}

// Clear resets input state.
func (m *inputModel) Clear() {
	m.value = ""
	m.cursorPos = 0
	m.acMode = false
}

// Resize updates the width for wrapping.
func (m *inputModel) Resize(width int) { m.width = width }

// insertAtCursor inserts text at the current cursor position.
func (m *inputModel) insertAtCursor(text string) {
	runes := []rune(m.value)
	pos := m.cursorPos
	if pos > len(runes) {
		pos = len(runes)
	}
	inserted := []rune(text)
	newRunes := make([]rune, 0, len(runes)+len(inserted))
	newRunes = append(newRunes, runes[:pos]...)
	newRunes = append(newRunes, inserted...)
	newRunes = append(newRunes, runes[pos:]...)
	m.value = string(newRunes)
	m.cursorPos = pos + len(inserted)
}

func (m *inputModel) deleteWordBackward() {
	runes := []rune(m.value)
	pos := m.cursorPos
	if pos > len(runes) {
		pos = len(runes)
	}
	if pos > 0 {
		newPos := pos
		for newPos > 0 && runes[newPos-1] == ' ' {
			newPos--
		}
		for newPos > 0 && runes[newPos-1] != ' ' && runes[newPos-1] != '\n' {
			newPos--
		}
		m.value = string(runes[:newPos]) + string(runes[pos:])
		m.cursorPos = newPos
	}
}

func (m *inputModel) killToLineStart() {
	runes := []rune(m.value)
	pos := m.cursorPos
	if pos > len(runes) {
		pos = len(runes)
	}
	lineStart := pos
	for lineStart > 0 && runes[lineStart-1] != '\n' {
		lineStart--
	}
	m.value = string(runes[:lineStart]) + string(runes[pos:])
	m.cursorPos = lineStart
}

func (m *inputModel) moveToLineStart() {
	runes := []rune(m.value)
	pos := m.cursorPos
	if pos > len(runes) {
		pos = len(runes)
	}
	for pos > 0 && runes[pos-1] != '\n' {
		pos--
	}
	m.cursorPos = pos
}

func (m *inputModel) moveToLineEnd() {
	runes := []rune(m.value)
	pos := m.cursorPos
	if pos > len(runes) {
		pos = len(runes)
	}
	for pos < len(runes) && runes[pos] != '\n' {
		pos++
	}
	m.cursorPos = pos
}

func (m *inputModel) moveToPrevWord() {
	runes := []rune(m.value)
	pos := m.cursorPos
	if pos > len(runes) {
		pos = len(runes)
	}
	if pos > 0 {
		pos--
		for pos > 0 && runes[pos] == ' ' {
			pos--
		}
		for pos > 0 && runes[pos-1] != ' ' && runes[pos-1] != '\n' {
			pos--
		}
	}
	m.cursorPos = pos
}

func (m *inputModel) moveToNextWord() {
	runes := []rune(m.value)
	pos := m.cursorPos
	if pos > len(runes) {
		pos = len(runes)
	}
	if pos < len(runes) {
		pos++
		for pos < len(runes) && runes[pos] != ' ' && runes[pos] != '\n' {
			pos++
		}
		for pos < len(runes) && runes[pos] == ' ' {
			pos++
		}
	}
	m.cursorPos = pos
}

// triggerAutocomplete filters the file list against acQuery.
func (m *inputModel) triggerAutocomplete(files []string) {
	m.acFiles = files
	m.acLoaded = true
	atPos := strings.LastIndex(m.value, "@")
	if atPos >= 0 {
		m.acQuery = m.value[atPos+1:]
	}
	m.acList = filterFiles(files, m.acQuery)
	if m.acIdx >= len(m.acList) {
		m.acIdx = 0
	}
}

func (m *inputModel) ensureFileList() {
	if m.acLoaded {
		return
	}
	m.acFiles = loadFiles(".", 3)
	m.acLoaded = true
}

func (m *inputModel) updateAutocomplete() {
	atPos := strings.LastIndex(m.value, "@")
	if atPos < 0 {
		m.acMode = false
		return
	}
	m.acQuery = m.value[atPos+1:]
	m.ensureFileList()
	m.acList = filterFiles(m.acFiles, m.acQuery)
	if m.acIdx >= len(m.acList) {
		m.acIdx = 0
	}
	if len(m.acList) == 0 {
		m.acMode = false
	}
}

// renderAutocomplete renders the autocomplete dropdown.
func (m inputModel) renderAutocomplete() string {
	var sb strings.Builder
	show := m.acList
	if len(show) > acMaxResults {
		show = show[:acMaxResults]
	}
	for i, path := range show {
		style := m.theme.ACNormal
		if i == m.acIdx {
			style = m.theme.ACSelected
		}
		entry := " " + truncStr(path, m.width-8) + " "
		sb.WriteString("  " + style.Render(entry))
		if i < len(show)-1 {
			sb.WriteByte('\n')
		}
	}
	if len(m.acList) > acMaxResults {
		sb.WriteByte('\n')
		sb.WriteString("  " + m.theme.Dim.Render(strings.Repeat(" ", 2)+"+"+strings.Repeat(" ", 0)))
	}
	return sb.String()
}
