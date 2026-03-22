package ui

import (
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/bubbles/viewport"
)

// TestFmtTok tests the token count formatter.
func TestFmtTok(t *testing.T) {
	tests := []struct {
		name string
		n    int
		want string
	}{
		// Small numbers (< 1k)
		{"zero", 0, "0"},
		{"single digit", 5, "5"},
		{"hundreds", 999, "999"},

		// Thousands (1k - 999k)
		{"exactly 1000", 1000, "1.0k"},
		{"1500", 1500, "1.5k"},
		{"10000", 10000, "10.0k"},
		{"50000", 50000, "50.0k"},
		{"999000", 999000, "999.0k"},

		// Millions (>= 1M)
		{"exactly 1M", 1000000, "1.0M"},
		{"1.5M", 1500000, "1.5M"},
		{"10M", 10000000, "10.0M"},
		{"100M", 100000000, "100.0M"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := fmtTok(tt.n)
			if got != tt.want {
				t.Errorf("fmtTok(%d) = %q, want %q", tt.n, got, tt.want)
			}
		})
	}
}

// TestTruncStr tests string truncation.
func TestTruncStr(t *testing.T) {
	tests := []struct {
		name   string
		s      string
		maxLen int
		want   string
	}{
		// No truncation needed
		{"empty string", "", 10, ""},
		{"short string", "hello", 10, "hello"},
		{"exact length", "hello", 5, "hello"},

		// Truncation with ellipsis (maxLen >= 4)
		{"truncate simple", "hello world", 8, "hello w…"},
		{"truncate to 5", "hello world", 5, "hell…"},
		{"truncate to 4", "longstring", 4, "lon…"},

		// Edge cases (maxLen < 4)
		{"maxLen 3", "hello", 3, "hel"},
		{"maxLen 2", "hello", 2, "he"},
		{"maxLen 1", "hello", 1, "h"},
		{"maxLen 0", "hello", 0, "hello"},
		{"maxLen negative", "hello", -5, "hello"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := truncStr(tt.s, tt.maxLen)
			if got != tt.want {
				t.Errorf("truncStr(%q, %d) = %q, want %q", tt.s, tt.maxLen, got, tt.want)
			}
		})
	}
}

// TestTruncPath tests path truncation with smart handling of directory and base names.
func TestTruncPath(t *testing.T) {
	tests := []struct {
		name   string
		path   string
		maxLen int
		want   string
	}{
		// No truncation needed
		{"short path", "/tmp/file.txt", 20, "/tmp/file.txt"},
		{"exact length", "/tmp/file.txt", 13, "/tmp/file.txt"},

		// Truncation cases - the function uses the last remain chars of the dir path
		// maxLen=30: base="tui.go"(6), remain=30-6-4=20, so last 20 chars of parent dir
		{"long path", "/home/user/projects/go_sdk_agent/internal/ui/tui.go", 30, "…/dk_agent/internal/ui/tui.go"},
		// maxLen=20: base="file.txt"(8), remain=20-8-4=8, so last 8 chars of parent dir
		{"very long path", "/a/b/c/d/e/f/g/h/i/j/k/file.txt", 20, "…//h/i/j/k/file.txt"},

		// Base name very long (base >= maxLen-4, so truncate just the base)
		{"long base only", "/short/very_long_filename_that_exceeds_max.txt", 20, "very_long_filename_…"},

		// Tiny maxLen (base name longer than maxLen-4, returns truncated base)
		// maxLen=5: base="file.txt"(8), truncStr("file.txt",5) = s[:4]+"…" = "file…"
		{"maxLen 5", "/home/user/file.txt", 5, "file…"},
		// maxLen=4: base="file.txt"(8), since maxLen < 4 is false (4 is not < 4), truncStr returns s[:3]+"…" = "fil…"
		{"maxLen 4", "/home/user/file.txt", 4, "fil…"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := truncPath(tt.path, tt.maxLen)
			if got != tt.want {
				t.Errorf("truncPath(%q, %d) = %q, want %q", tt.path, tt.maxLen, got, tt.want)
			}
		})
	}
}

// TestWrapText tests word wrapping with ANSI sequence preservation.
func TestWrapText(t *testing.T) {
	tests := []struct {
		name     string
		text     string
		maxWidth int
		want     string
	}{
		// Basic wrapping (no ANSI) - note: spaces at line breaks are included in output
		{
			name:     "simple wrap",
			text:     "The quick brown fox jumps",
			maxWidth: 10,
			want:     "The quick \nbrown fox \njumps",
		},
		{
			name:     "exact fit no wrap",
			text:     "Hello",
			maxWidth: 10,
			want:     "Hello",
		},
		{
			name:     "long word wraps by grapheme",
			text:     "abcdefghijklmnop",
			maxWidth: 5,
			want:     "abcde\nfghij\nklmno\np",
		},

		// Paragraph breaks
		{
			name:     "two paragraphs",
			text:     "First paragraph\n\nSecond paragraph",
			maxWidth: 20,
			want:     "First paragraph\n\nSecond paragraph",
		},

		// ANSI color codes (should be preserved) - reset codes don't get replayed at line wraps
		{
			name:     "with ANSI color",
			text:     "\x1b[31mRed text here\x1b[0m",
			maxWidth: 8,
			want:     "\x1b[31mRed text\n\x1b[31mhere\x1b[0m",
		},

		// Edge cases
		{
			name:     "maxWidth 0",
			text:     "hello",
			maxWidth: 0,
			want:     "hello",
		},
		{
			name:     "empty string",
			text:     "",
			maxWidth: 10,
			want:     "",
		},
		{
			name:     "only spaces",
			text:     "   ",
			maxWidth: 10,
			want:     "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := wrapText(tt.text, tt.maxWidth)
			if got != tt.want {
				t.Errorf("wrapText(%q, %d):\ngot:\n%q\nwant:\n%q", tt.text, tt.maxWidth, got, tt.want)
			}
		})
	}
}

// TestIndentBlock tests indenting text blocks.
func TestIndentBlock(t *testing.T) {
	tests := []struct {
		name   string
		text   string
		prefix string
		want   string
	}{
		// Single line
		{
			name:   "single line",
			text:   "hello world",
			prefix: "  ",
			want:   "  hello world",
		},

		// Multiple lines
		{
			name:   "multiple lines",
			text:   "line1\nline2\nline3",
			prefix: "> ",
			want:   "> line1\n> line2\n> line3",
		},

		// Empty lines in between
		{
			name:   "with empty lines",
			text:   "line1\n\nline3",
			prefix: "| ",
			want:   "| line1\n\n| line3",
		},

		// Already indented text
		{
			name:   "already indented",
			text:   "  already indented",
			prefix: "→ ",
			want:   "→ already indented",
		},

		// Empty string
		{
			name:   "empty text",
			text:   "",
			prefix: "  ",
			want:   "",
		},

		// Text with leading whitespace
		{
			name:   "leading whitespace",
			text:   "   spaced",
			prefix: "* ",
			want:   "* spaced",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := indentBlock(tt.text, tt.prefix)
			if got != tt.want {
				t.Errorf("indentBlock(%q, %q):\ngot:  %q\nwant: %q", tt.text, tt.prefix, got, tt.want)
			}
		})
	}
}

// TestFmtTimestamp tests timestamp formatting.
func TestFmtTimestamp(t *testing.T) {
	tests := []struct {
		name string
		t    time.Time
		want string
	}{
		// Zero time returns spaces
		{
			name: "zero time",
			t:    time.Time{},
			want: "        ",
		},

		// Various times
		{
			name: "midnight",
			t:    time.Date(2024, 3, 22, 0, 0, 0, 0, time.UTC),
			want: "00:00:00",
		},
		{
			name: "noon",
			t:    time.Date(2024, 3, 22, 12, 30, 45, 0, time.UTC),
			want: "12:30:45",
		},
		{
			name: "afternoon",
			t:    time.Date(2024, 3, 22, 15, 4, 5, 0, time.UTC),
			want: "15:04:05",
		},
		{
			name: "almost midnight",
			t:    time.Date(2024, 3, 22, 23, 59, 59, 0, time.UTC),
			want: "23:59:59",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := fmtTimestamp(tt.t)
			if got != tt.want {
				t.Errorf("fmtTimestamp(%v) = %q, want %q", tt.t, got, tt.want)
			}
		})
	}
}

// TestFmtDuration tests duration formatting for display.
func TestFmtDuration(t *testing.T) {
	tests := []struct {
		name string
		d    time.Duration
		want string
	}{
		// Very short durations
		{
			name: "10ms",
			d:    10 * time.Millisecond,
			want: "<0.5s",
		},
		{
			name: "400ms",
			d:    400 * time.Millisecond,
			want: "<0.5s",
		},

		// Millisecond range (0.5s - 1s)
		{
			name: "500ms",
			d:    500 * time.Millisecond,
			want: "500ms",
		},
		{
			name: "750ms",
			d:    750 * time.Millisecond,
			want: "750ms",
		},
		{
			name: "999ms",
			d:    999 * time.Millisecond,
			want: "999ms",
		},

		// Second range (1s - 60s)
		{
			name: "1 second",
			d:    1 * time.Second,
			want: "1.0s",
		},
		{
			name: "1.5 seconds",
			d:    1500 * time.Millisecond,
			want: "1.5s",
		},
		{
			name: "30 seconds",
			d:    30 * time.Second,
			want: "30.0s",
		},
		{
			name: "59 seconds",
			d:    59 * time.Second,
			want: "59.0s",
		},

		// Minute range (>= 1 minute)
		{
			name: "1 minute",
			d:    1 * time.Minute,
			want: "1m00s",
		},
		{
			name: "1m30s",
			d:    1*time.Minute + 30*time.Second,
			want: "1m30s",
		},
		{
			name: "5 minutes",
			d:    5 * time.Minute,
			want: "5m00s",
		},
		{
			name: "1h (60 minutes)",
			d:    60 * time.Minute,
			want: "60m00s",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := fmtDuration(tt.d)
			if got != tt.want {
				t.Errorf("fmtDuration(%v) = %q, want %q", tt.d, got, tt.want)
			}
		})
	}
}

// TestNewDisplayMsg tests creation of display messages with timestamp.
func TestNewDisplayMsg(t *testing.T) {
	tests := []struct {
		name     string
		role     string
		text     string
		checkTs  bool
	}{
		{
			name:    "user message",
			role:    "user",
			text:    "hello",
			checkTs: true,
		},
		{
			name:    "assistant message",
			role:    "assistant",
			text:    "Hi there!",
			checkTs: true,
		},
		{
			name:    "empty text",
			role:    "system",
			text:    "",
			checkTs: true,
		},
	}

	now := time.Now()
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			msg := newDisplayMsg(tt.role, tt.text)
			if msg.role != tt.role {
				t.Errorf("newDisplayMsg(%q, %q).role = %q, want %q", tt.role, tt.text, msg.role, tt.role)
			}
			if msg.text != tt.text {
				t.Errorf("newDisplayMsg(%q, %q).text = %q, want %q", tt.role, tt.text, msg.text, tt.text)
			}
			if tt.checkTs {
				// Timestamp should be close to now (within 1 second)
				if msg.ts.Before(now.Add(-time.Second)) || msg.ts.After(now.Add(time.Second)) {
					t.Errorf("newDisplayMsg timestamp %v not close to %v", msg.ts, now)
				}
			}
		})
	}
}

// TestEdgeCasesAndIntegration tests combinations and edge cases.
func TestEdgeCasesAndIntegration(t *testing.T) {
	t.Run("truncStr and truncPath with unicode", func(t *testing.T) {
		// Test with emoji/unicode
		s := "hello💫world"
		got := truncStr(s, 10)
		if !strings.Contains(got, "…") {
			t.Errorf("truncStr should add ellipsis, got %q", got)
		}
	})

	t.Run("fmtDuration edge boundary at 500ms", func(t *testing.T) {
		// Just below 500ms threshold
		d1 := 499 * time.Millisecond
		got1 := fmtDuration(d1)
		if got1 != "<0.5s" {
			t.Errorf("fmtDuration(%v) = %q, want \"<0.5s\"", d1, got1)
		}

		// Exactly at 500ms
		d2 := 500 * time.Millisecond
		got2 := fmtDuration(d2)
		if got2 != "500ms" {
			t.Errorf("fmtDuration(%v) = %q, want \"500ms\"", d2, got2)
		}
	})

	t.Run("fmtTok at power-of-10 boundaries", func(t *testing.T) {
		// Test 999 (stays as number)
		if got := fmtTok(999); got != "999" {
			t.Errorf("fmtTok(999) = %q, want \"999\"", got)
		}

		// Test 1000 (becomes k)
		if got := fmtTok(1000); got != "1.0k" {
			t.Errorf("fmtTok(1000) = %q, want \"1.0k\"", got)
		}

		// Test 999999 (still k)
		if got := fmtTok(999999); got != "1000.0k" {
			t.Errorf("fmtTok(999999) = %q, want \"1000.0k\"", got)
		}

		// Test 1000000 (becomes M)
		if got := fmtTok(1000000); got != "1.0M" {
			t.Errorf("fmtTok(1000000) = %q, want \"1.0M\"", got)
		}
	})

	t.Run("indentBlock preserves empty lines", func(t *testing.T) {
		text := "hello\n\n\nworld"
		got := indentBlock(text, ">>")
		lines := strings.Split(got, "\n")
		if len(lines) != 4 {
			t.Errorf("indentBlock should preserve line count, got %d lines", len(lines))
		}
		if lines[1] != "" || lines[2] != "" {
			t.Errorf("indentBlock should preserve empty lines, got %v", lines)
		}
	})
}

// TestResizeViewportWithWrappedInput tests viewport shrinks when input wraps.
func TestResizeViewportWithWrappedInput(t *testing.T) {
	m := Model{width: 20, height: 30, ready: true}
	m.viewport = viewport.Model{}

	// Short input — baseline
	m.input = "hi"
	m.resizeViewport()
	baseHeight := m.viewport.Height

	// Long input that wraps — viewport should be shorter
	m.input = strings.Repeat("a", 40) // wraps to ~3 lines at width 20
	m.resizeViewport()
	wrappedHeight := m.viewport.Height

	if wrappedHeight >= baseHeight {
		t.Errorf("viewport should shrink with wrapped input: base=%d, wrapped=%d", baseHeight, wrappedHeight)
	}
}

// TestRenderInputLineWrapping tests that long input visually wraps.
func TestRenderInputLineWrapping(t *testing.T) {
	m := Model{width: 20}
	promptWidth := 2 // "❯ "

	t.Run("short input no wrap", func(t *testing.T) {
		m.input = "hello"
		m.cursorPos = 5
		result := m.renderInputLine()
		if strings.Contains(result, "\n") {
			t.Errorf("short input should not wrap, got: %q", result)
		}
	})

	t.Run("long input wraps", func(t *testing.T) {
		// First line fits 18 chars (20 - 2 for prompt), then wraps
		m.input = strings.Repeat("a", 30)
		m.cursorPos = 30
		result := m.renderInputLine()
		lines := strings.Split(result, "\n")
		if len(lines) < 2 {
			t.Errorf("expected wrapped lines, got %d line(s): %q", len(lines), result)
		}
		// Second line should be indented by promptWidth
		if len(lines) > 1 && !strings.HasPrefix(lines[1], strings.Repeat(" ", promptWidth)) {
			t.Errorf("wrapped line should be indented, got: %q", lines[1])
		}
	})

	t.Run("empty input shows placeholder", func(t *testing.T) {
		m.input = ""
		m.cursorPos = 0
		result := m.renderInputLine()
		if !strings.Contains(result, "Type a message...") {
			t.Errorf("empty input should show placeholder, got: %q", result)
		}
	})
}
