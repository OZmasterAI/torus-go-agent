package rawtui

import (
	"os"
	"syscall"
	"unicode/utf8"
)

const (
	promptPrefix     = "▸ "
	promptPrefixCols = 2
)

// vline represents a visual line in the input textarea.
type vline struct {
	start    int // rune index of first char
	length   int // number of runes
	startCol int // visual column where text starts
}

func getVisualLines(input []rune, cursor int, width int) []vline {
	var lines []vline
	start := 0
	startCol := promptPrefixCols
	length := 0
	lastSpaceIdx := -1
	for i, r := range input {
		if r == '\n' {
			lines = append(lines, vline{start, length, startCol})
			start = i + 1
			startCol = 0
			length = 0
			lastSpaceIdx = -1
			continue
		}
		length++
		if r == ' ' {
			lastSpaceIdx = i
		}
		if startCol+length >= width {
			if lastSpaceIdx >= start {
				wrapLen := lastSpaceIdx - start + 1
				lines = append(lines, vline{start, wrapLen, startCol})
				start = lastSpaceIdx + 1
				length = length - wrapLen
				startCol = 0
				lastSpaceIdx = -1
			} else {
				lines = append(lines, vline{start, length, startCol})
				start = i + 1
				startCol = 0
				length = 0
				lastSpaceIdx = -1
			}
		}
	}
	lines = append(lines, vline{start, length, startCol})
	return lines
}

func cursorVisualPos(input []rune, cursor int, width int) (int, int) {
	vlines := getVisualLines(input, cursor, width)
	for i, vl := range vlines {
		end := vl.start + vl.length
		if cursor >= vl.start && cursor <= end {
			if cursor == end && i < len(vlines)-1 && (end >= len(input) || input[end] != '\n') {
				continue
			}
			return i, vl.startCol + (cursor - vl.start)
		}
	}
	last := len(vlines) - 1
	vl := vlines[last]
	return last, vl.startCol + vl.length
}

// inputState holds the mutable input buffer state.
type inputState struct {
	input   []rune
	cursor  int
	history *History
}

func (s *inputState) abandonHistoryNav() {
	if s.history != nil && s.history.IsNavigating() {
		s.history.Reset()
	}
}

func (s *inputState) insertAtCursor(r rune) {
	s.abandonHistoryNav()
	s.input = append(s.input, 0)
	copy(s.input[s.cursor+1:], s.input[s.cursor:])
	s.input[s.cursor] = r
	s.cursor++
}

func (s *inputState) insertText(text string) {
	for _, r := range text {
		s.insertAtCursor(r)
	}
}

func (s *inputState) deleteBeforeCursor() {
	s.abandonHistoryNav()
	if s.cursor <= 0 {
		return
	}
	s.cursor--
	copy(s.input[s.cursor:], s.input[s.cursor+1:])
	s.input = s.input[:len(s.input)-1]
}

func (s *inputState) deleteAtCursor() {
	s.abandonHistoryNav()
	if s.cursor >= len(s.input) {
		return
	}
	copy(s.input[s.cursor:], s.input[s.cursor+1:])
	s.input = s.input[:len(s.input)-1]
}

func (s *inputState) deleteWordBackward() {
	s.abandonHistoryNav()
	if s.cursor <= 0 {
		return
	}
	for s.cursor > 0 && s.input[s.cursor-1] == ' ' {
		s.deleteBeforeCursor()
	}
	for s.cursor > 0 && s.input[s.cursor-1] != ' ' && s.input[s.cursor-1] != '\n' {
		s.deleteBeforeCursor()
	}
}

func (s *inputState) killLine() {
	s.abandonHistoryNav()
	end := s.cursor
	for end < len(s.input) && s.input[end] != '\n' {
		end++
	}
	s.input = append(s.input[:s.cursor], s.input[end:]...)
}

func (s *inputState) killToStart() {
	s.abandonHistoryNav()
	start := s.cursor
	for start > 0 && s.input[start-1] != '\n' {
		start--
	}
	s.input = append(s.input[:start], s.input[s.cursor:]...)
	s.cursor = start
}

func (s *inputState) moveUp(width int) {
	lineIdx, col := cursorVisualPos(s.input, s.cursor, width)
	if lineIdx == 0 {
		return
	}
	vlines := getVisualLines(s.input, s.cursor, width)
	prev := vlines[lineIdx-1]
	targetCol := col
	if targetCol > prev.startCol+prev.length {
		targetCol = prev.startCol + prev.length
	}
	if targetCol < prev.startCol {
		targetCol = prev.startCol
	}
	s.cursor = prev.start + (targetCol - prev.startCol)
}

func (s *inputState) moveDown(width int) {
	lineIdx, _ := cursorVisualPos(s.input, s.cursor, width)
	vlines := getVisualLines(s.input, s.cursor, width)
	if lineIdx >= len(vlines)-1 {
		return
	}
	_, col := cursorVisualPos(s.input, s.cursor, width)
	next := vlines[lineIdx+1]
	targetCol := col
	if targetCol > next.startCol+next.length {
		targetCol = next.startCol + next.length
	}
	if targetCol < next.startCol {
		targetCol = next.startCol
	}
	s.cursor = next.start + (targetCol - next.startCol)
}

func (s *inputState) moveWordLeft() {
	if s.cursor <= 0 {
		return
	}
	s.cursor--
	for s.cursor > 0 && s.input[s.cursor] == ' ' {
		s.cursor--
	}
	for s.cursor > 0 && s.input[s.cursor-1] != ' ' && s.input[s.cursor-1] != '\n' {
		s.cursor--
	}
}

func (s *inputState) moveWordRight() {
	if s.cursor >= len(s.input) {
		return
	}
	s.cursor++
	for s.cursor < len(s.input) && s.input[s.cursor] != ' ' && s.input[s.cursor] != '\n' {
		s.cursor++
	}
	for s.cursor < len(s.input) && s.input[s.cursor] == ' ' {
		s.cursor++
	}
}

func (s *inputState) value() string {
	return string(s.input)
}

func (s *inputState) setValue(v string) {
	s.input = []rune(v)
	s.cursor = len(s.input)
}

func (s *inputState) reset() {
	s.input = s.input[:0]
	s.cursor = 0
}

// startStdinReader creates a dup'd stdin fd and starts a goroutine that reads
// bytes into ch. Returns the dup'd file and the channel.
func startStdinReader() (*os.File, chan byte) {
	dupFd, err := syscall.Dup(int(os.Stdin.Fd()))
	if err != nil {
		return nil, nil
	}
	stdinDup := os.NewFile(uintptr(dupFd), "stdin-dup")
	ch := make(chan byte, 64)
	go func() {
		buf := make([]byte, 1)
		for {
			_, err := stdinDup.Read(buf)
			if err != nil {
				close(ch)
				return
			}
			ch <- buf[0]
		}
	}()
	return stdinDup, ch
}

func stopStdinReader(stdinDup *os.File, ch chan byte) {
	if stdinDup != nil {
		stdinDup.Close()
	}
	for {
		select {
		case _, ok := <-ch:
			if !ok {
				return
			}
		default:
			return
		}
	}
}

func utf8ByteLen(first byte) int {
	switch {
	case first&0x80 == 0:
		return 1
	case first&0xE0 == 0xC0:
		return 2
	case first&0xF0 == 0xE0:
		return 3
	default:
		return 4
	}
}

// readBracketedPaste reads paste content until the end marker ESC [ 2 0 1 ~.
func readBracketedPaste(readByte func() (byte, bool)) string {
	var content []byte
	for {
		ch, ok := readByte()
		if !ok {
			break
		}
		if ch == '\033' {
			e0, ok := readByte()
			if !ok {
				break
			}
			if e0 == '[' {
				e1, ok := readByte()
				if !ok {
					break
				}
				e2, ok := readByte()
				if !ok {
					break
				}
				e3, ok := readByte()
				if !ok {
					break
				}
				e4, ok := readByte()
				if !ok {
					break
				}
				if e1 == '2' && e2 == '0' && e3 == '1' && e4 == '~' {
					break
				}
				content = append(content, '\033', e0, e1, e2, e3, e4)
			} else {
				content = append(content, '\033', e0)
			}
		} else {
			content = append(content, ch)
		}
	}
	return string(content)
}

// readFullRune reads a potentially multi-byte UTF-8 rune given the first byte.
func readFullRune(first byte, readByte func() (byte, bool)) (rune, bool) {
	if first < 0x80 {
		return rune(first), true
	}
	b := []byte{first}
	n := utf8ByteLen(first)
	for i := 1; i < n; i++ {
		next, ok := readByte()
		if !ok {
			return 0, false
		}
		b = append(b, next)
	}
	r, _ := utf8.DecodeRune(b)
	return r, true
}
