package rawtui

import (
	"fmt"
	"math"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/rivo/uniseg"
	"golang.org/x/term"
)

// ansiEscRe matches ANSI escape sequences (CSI and OSC).
var ansiEscRe = regexp.MustCompile("\x1b\\[[0-9;]*[A-Za-z]|\x1b\\].*?\x1b\\\\")

// visibleWidth returns the visible (printable) width of s, stripping ANSI escapes
// and accounting for multi-byte / wide Unicode characters via uniseg.
func visibleWidth(s string) int {
	return uniseg.StringWidth(ansiEscRe.ReplaceAllString(s, ""))
}

// padCodeBlockRow strips a trailing SGR reset sequence, pads the visible content
// to width with spaces, and re-appends the reset so the background fills the row.
func padCodeBlockRow(row string, width int) string {
	const reset = "\033[0m"
	trimmed := strings.TrimSuffix(row, reset)
	vw := visibleWidth(trimmed)
	if vw < width {
		trimmed += strings.Repeat(" ", width-vw)
	}
	return trimmed + reset
}

// writeRows positions the cursor at terminal row `from` (1-based) and writes
// each row, clearing to end-of-line before printing.
func writeRows(buf *strings.Builder, rows []string, from int) {
	for i, row := range rows {
		fmt.Fprintf(buf, "\033[%d;1H\033[0m\033[2K%s", from+i, row)
	}
}

// lerpColor linearly interpolates between two RGB colours by factor t in [0,1].
func lerpColor(r1, g1, b1, r2, g2, b2 int, t float64) (int, int, int) {
	lerp := func(a, b int, t float64) int {
		return int(math.Round(float64(a) + t*float64(b-a)))
	}
	return lerp(r1, r2, t), lerp(g1, g2, t), lerp(b1, b2, t)
}

// progressBar renders a 3-cell wide gradient progress bar.
// The fill uses block-element partial characters and transitions from green to red.
func progressBar(n, max int) string {
	const totalCells = 3
	const dimBg = "\033[48;5;240m"

	partials := []string{"█", "▉", "▊", "▋", "▌", "▍", "▎", "▏"}

	if max <= 0 {
		return dimBg + strings.Repeat(" ", totalCells) + "\033[0m"
	}

	ratio := float64(n) / float64(max)
	if ratio > 1 {
		ratio = 1
	}
	if ratio < 0 {
		ratio = 0
	}

	// Total "eighths" filled across all cells.
	eighths := int(math.Round(ratio * float64(totalCells) * 8))

	var sb strings.Builder

	r, g, b := lerpColor(78, 201, 100, 230, 70, 70, ratio)
	fgColor := fmt.Sprintf("\033[38;2;%d;%d;%dm", r, g, b)

	for i := 0; i < totalCells; i++ {
		cellEighths := eighths - i*8
		if cellEighths >= 8 {
			// Full block
			sb.WriteString(dimBg + fgColor + "█")
		} else if cellEighths > 0 {
			// Partial block — partials[0]="█" (8/8), partials[7]="▏" (1/8)
			idx := 8 - cellEighths
			if idx >= len(partials) {
				idx = len(partials) - 1
			}
			sb.WriteString(dimBg + fgColor + partials[idx])
		} else {
			// Empty cell
			sb.WriteString(dimBg + " ")
		}
	}
	sb.WriteString("\033[0m")
	return sb.String()
}

// truncateWithEllipsis truncates s to maxLen visible characters, appending "…"
// if the string was shortened. It does not handle embedded ANSI sequences.
func truncateWithEllipsis(s string, maxLen int) string {
	if maxLen <= 0 {
		return ""
	}
	if uniseg.StringWidth(s) <= maxLen {
		return s
	}
	// Walk grapheme clusters until we would exceed maxLen-1 visible cols.
	var out strings.Builder
	cols := 0
	state := -1
	remaining := s
	for len(remaining) > 0 {
		var cluster string
		cluster, remaining, _, state = uniseg.FirstGraphemeClusterInString(remaining, state)
		w := uniseg.StringWidth(cluster)
		if cols+w > maxLen-1 {
			break
		}
		out.WriteString(cluster)
		cols += w
	}
	out.WriteString("…")
	return out.String()
}

// isCSIFinal reports whether b is the final byte of a CSI (Control Sequence
// Introducer) escape sequence, i.e. in the range 0x40-0x7E.
func isCSIFinal(c byte) bool {
	return c >= 0x40 && c <= 0x7E
}

// truncateVisual truncates s to at most maxCols visible columns, correctly
// skipping over CSI and OSC ANSI escape sequences, and appends "…" if the
// string was cut short.
func truncateVisual(s string, maxCols int) string {
	if maxCols <= 0 {
		return ""
	}
	cols := 0
	var out strings.Builder
	i := 0
	bs := []byte(s)
	n := len(bs)

	for i < n {
		b := bs[i]

		// Detect ESC
		if b == 0x1b && i+1 < n {
			next := bs[i+1]
			if next == '[' {
				// CSI sequence: ESC [ ... <final>
				j := i + 2
				for j < n && !isCSIFinal(bs[j]) {
					j++
				}
				if j < n {
					j++ // include final byte
				}
				out.Write(bs[i:j])
				i = j
				continue
			} else if next == ']' {
				// OSC sequence: ESC ] ... ST (ESC \)
				j := i + 2
				for j+1 < n {
					if bs[j] == 0x1b && bs[j+1] == '\\' {
						j += 2
						break
					}
					j++
				}
				out.Write(bs[i:j])
				i = j
				continue
			}
		}

		// Regular content — measure one grapheme cluster.
		cluster, _, w, _ := uniseg.FirstGraphemeClusterInString(s[i:], -1)
		if cols+w > maxCols-1 {
			out.WriteString("…")
			return out.String()
		}
		out.WriteString(cluster)
		cols += w
		i += len(cluster)
	}

	return out.String()
}

// collapseBlankRows removes consecutive duplicate blank rows, keeping at most
// one blank row in a run.
func collapseBlankRows(rows []string) []string {
	out := make([]string, 0, len(rows))
	prevBlank := false
	for _, r := range rows {
		blank := isBlankRow(r)
		if blank && prevBlank {
			continue
		}
		out = append(out, r)
		prevBlank = blank
	}
	return out
}

// isBlankRow returns true if s is empty or contains only whitespace after
// ANSI escape sequences are stripped.
func isBlankRow(s string) bool {
	stripped := ansiEscRe.ReplaceAllString(s, "")
	return strings.TrimSpace(stripped) == ""
}

// formatDuration formats a duration for compact display:
//   - < 500ms  -> "<500ms"
//   - < 1s     -> "Nms"
//   - < 1min   -> "N.Ns"
//   - else     -> "NmNNs"
func formatDuration(d time.Duration) string {
	if d < 500*time.Millisecond {
		return "<500ms"
	}
	if d < time.Second {
		ms := d.Milliseconds()
		return fmt.Sprintf("%dms", ms)
	}
	if d < time.Minute {
		secs := d.Seconds()
		return fmt.Sprintf("%.1fs", secs)
	}
	mins := int(d.Minutes())
	secs := int(d.Seconds()) % 60
	return fmt.Sprintf("%dm%02ds", mins, secs)
}

// formatTokenCount formats a token count for compact display:
//   - >= 1,000,000 -> "N.Nm"
//   - >= 100,000   -> "Nk"
//   - >= 1,000     -> "N.Nk"
//   - else         -> "N"
func formatTokenCount(tokens int) string {
	switch {
	case tokens >= 1_000_000:
		return fmt.Sprintf("%.1fm", float64(tokens)/1_000_000)
	case tokens >= 100_000:
		return fmt.Sprintf("%dk", tokens/1_000)
	case tokens >= 1_000:
		return fmt.Sprintf("%.1fk", float64(tokens)/1_000)
	default:
		return fmt.Sprintf("%d", tokens)
	}
}

// hslToRGB converts HSL values (h in [0,360), s in [0,1], l in [0,1]) to RGB
// components each in [0,255].
func hslToRGB(h, s, l float64) (int, int, int) {
	h = math.Mod(h, 360)
	if h < 0 {
		h += 360
	}

	c := (1 - math.Abs(2*l-1)) * s
	x := c * (1 - math.Abs(math.Mod(h/60, 2)-1))
	m := l - c/2

	var r, g, b float64
	switch {
	case h < 60:
		r, g, b = c, x, 0
	case h < 120:
		r, g, b = x, c, 0
	case h < 180:
		r, g, b = 0, c, x
	case h < 240:
		r, g, b = 0, x, c
	case h < 300:
		r, g, b = x, 0, c
	default:
		r, g, b = c, 0, x
	}

	toInt := func(v float64) int {
		return int(math.Round((v + m) * 255))
	}
	return toInt(r), toInt(g), toInt(b)
}

// pastelColor returns a 24-bit foreground ANSI escape code that cycles through
// pastel hues based on elapsed time, producing a gentle animated rainbow effect.
// Hue advances at 45 degrees/second, completing a full cycle every 8 seconds.
func pastelColor(elapsed time.Duration) string {
	seconds := elapsed.Seconds()
	hue := math.Mod(seconds*45, 360)
	r, g, b := hslToRGB(hue, 0.6, 0.75)
	return fmt.Sprintf("\033[38;2;%d;%d;%dm", r, g, b)
}

// wrapString word-wraps s to fit within w columns, starting at startCol for
// the first line. It is fully ANSI-aware: escape sequences are passed through
// without consuming visible columns, and active SGR sequences are re-emitted
// at the start of each continuation line so colours/styles are preserved.
//
// Returns a slice of lines (without trailing newlines).
func wrapString(s string, startCol int, w int) []string {
	type token struct {
		text   string // raw bytes (may be ANSI sequence or text word/space)
		isAnsi bool
		width  int // visible width (0 for ANSI tokens)
	}

	// tokenise splits input into ANSI sequences and plain-text word/space runs.
	tokenise := func(input string) []token {
		var tokens []token
		for len(input) > 0 {
			// Try to match an ANSI escape at the start.
			loc := ansiEscRe.FindStringIndex(input)
			if loc != nil && loc[0] == 0 {
				tokens = append(tokens, token{text: input[:loc[1]], isAnsi: true})
				input = input[loc[1]:]
				continue
			}

			// Find where the next ANSI sequence starts (or end of string).
			end := len(input)
			if loc != nil {
				end = loc[0]
			}
			plain := input[:end]
			input = input[end:]

			// Split plain text into words and space runs.
			i := 0
			for i < len(plain) {
				if plain[i] == ' ' {
					j := i
					for j < len(plain) && plain[j] == ' ' {
						j++
					}
					tokens = append(tokens, token{
						text:  plain[i:j],
						width: j - i,
					})
					i = j
				} else {
					j := i
					for j < len(plain) && plain[j] != ' ' {
						j++
					}
					word := plain[i:j]
					tokens = append(tokens, token{
						text:  word,
						width: uniseg.StringWidth(word),
					})
					i = j
				}
			}
		}
		return tokens
	}

	tokens := tokenise(s)

	var lines []string
	var lineBuf strings.Builder
	col := startCol

	// activeSeqs tracks live SGR sequences so we can replay them on new lines.
	var activeSeqs []string

	flushLine := func() {
		lines = append(lines, lineBuf.String())
		lineBuf.Reset()
		col = 0
		// Re-emit active SGR sequences at the start of the new line.
		for _, seq := range activeSeqs {
			lineBuf.WriteString(seq)
		}
	}

	isSGR := func(t token) bool {
		if !t.isAnsi {
			return false
		}
		return len(t.text) > 2 &&
			t.text[0] == 0x1b &&
			t.text[1] == '[' &&
			t.text[len(t.text)-1] == 'm'
	}

	isReset := func(t token) bool {
		return t.text == "\033[0m" || t.text == "\033[m"
	}

	for _, tok := range tokens {
		if tok.isAnsi {
			lineBuf.WriteString(tok.text)
			if isSGR(tok) {
				if isReset(tok) {
					activeSeqs = activeSeqs[:0]
				} else {
					activeSeqs = append(activeSeqs, tok.text)
				}
			}
			continue
		}

		// Space token — skip at start of line, otherwise emit or wrap.
		if len(tok.text) > 0 && tok.text[0] == ' ' {
			if col == 0 {
				continue
			}
			if col+tok.width > w {
				flushLine()
				continue
			}
			lineBuf.WriteString(tok.text)
			col += tok.width
			continue
		}

		// Word token.
		wordWidth := tok.width
		if wordWidth == 0 {
			lineBuf.WriteString(tok.text)
			continue
		}

		// Word fits on the current line.
		if col+wordWidth <= w {
			lineBuf.WriteString(tok.text)
			col += wordWidth
			continue
		}

		// Word does not fit — start a new line if not already at col 0.
		if col > 0 {
			flushLine()
		}

		// Word is wider than a full line: break character by character.
		if wordWidth > w {
			remaining := tok.text
			for len(remaining) > 0 {
				cluster, rest, cw, _ := uniseg.FirstGraphemeClusterInString(remaining, -1)
				if col+cw > w {
					flushLine()
				}
				lineBuf.WriteString(cluster)
				col += cw
				remaining = rest
			}
			continue
		}

		// Word fits on a fresh line.
		lineBuf.WriteString(tok.text)
		col += wordWidth
	}

	// Flush remaining content.
	if lineBuf.Len() > 0 {
		lines = append(lines, lineBuf.String())
	}

	return lines
}

// getTerminalWidth returns the current terminal width in columns.
// Falls back to 80 if the size cannot be determined.
func getTerminalWidth() int {
	w, _, err := term.GetSize(int(os.Stdin.Fd()))
	if err != nil || w <= 0 {
		return 80
	}
	return w
}

// getTerminalHeight returns the current terminal height in rows.
// Falls back to 24 if the size cannot be determined.
func getTerminalHeight() int {
	_, h, err := term.GetSize(int(os.Stdin.Fd()))
	if err != nil || h <= 0 {
		return 24
	}
	return h
}
