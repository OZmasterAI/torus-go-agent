package features

import (
	"strings"
	"testing"
	"unicode/utf8"
)

// TestRoutingEdge_UnicodeAndMultibyte tests handling of Unicode and multibyte characters
func TestRoutingEdge_UnicodeAndMultibyte(t *testing.T) {
	tests := []struct {
		name     string
		text     string
		expected bool
	}{
		{
			name:     "emoji only",
			text:     "😀😁😂",
			expected: true,
		},
		{
			name:     "emoji with text (under limit)",
			text:     "Hello 👋 world",
			expected: true,
		},
		{
			name:     "mixed unicode scripts",
			text:     "Hello مرحبا 你好",
			expected: true,
		},
		{
			name:     "cyrillic text",
			text:     "Привет мир",
			expected: true,
		},
		{
			name:     "chinese characters",
			text:     "你好世界",
			expected: true,
		},
		{
			name:     "emoji keyword containing (emoji + implement)",
			text:     "Can you 🎯 implement this?",
			expected: false,
		},
		{
			name:     "unicode at character boundary (159 multibyte chars)",
			text:     strings.Repeat("中", 53) + "a",
			expected: false, // byte length exceeds threshold even though rune count is low
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := IsSimpleMessage(tt.text)
			if result != tt.expected {
				t.Errorf("IsSimpleMessage(%q) = %v, want %v", tt.text, result, tt.expected)
			}
		})
	}
}

// TestRoutingEdge_BoundaryConditions tests exact boundary values
func TestRoutingEdge_BoundaryConditions(t *testing.T) {
	tests := []struct {
		name     string
		text     string
		expected bool
	}{
		{
			name:     "exactly 159 bytes (at limit)",
			text:     strings.Repeat("x", 159),
			expected: true,
		},
		{
			name:     "exactly 160 bytes (at boundary)",
			text:     strings.Repeat("x", 160),
			expected: false,
		},
		{
			name:     "exactly 161 bytes (over boundary)",
			text:     strings.Repeat("x", 161),
			expected: false,
		},
		{
			name:     "exactly 27 words (at limit)",
			text:     strings.Repeat("w ", 27),
			expected: true,
		},
		{
			name:     "exactly 28 words (at boundary)",
			text:     strings.Repeat("w ", 28),
			expected: false,
		},
		{
			name:     "exactly 29 words (over boundary)",
			text:     strings.Repeat("w ", 29),
			expected: false,
		},
		{
			name:     "word count with varied whitespace (27 words)",
			text:     "a  b   c    d     e      f       g        h         i          j           k l m n o p q r s t u v w x",
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := IsSimpleMessage(tt.text)
			if result != tt.expected {
				t.Errorf("IsSimpleMessage(%q) = %v, want %v", tt.text, result, tt.expected)
			}
		})
	}
}

// TestRoutingEdge_KeywordPartialMatches tests keyword detection with partial matches
func TestRoutingEdge_KeywordPartialMatches(t *testing.T) {
	tests := []struct {
		name     string
		text     string
		expected bool
	}{
		{
			name:     "keyword substring (implementation)",
			text:     "The implementation is done",
			expected: false,
		},
		{
			name:     "keyword substring (debugging)",
			text:     "Debugging is fun",
			expected: false,
		},
		{
			name:     "keyword substring (refactoring)",
			text:     "Refactoring code is necessary",
			expected: false,
		},
		{
			name:     "keyword substring (analysis)",
			text:     "Statistical analysis of data",
			expected: true, // "analysis" is not a tracked keyword
		},
		{
			name:     "keyword substring (comparison)",
			text:     "This comparison shows differences",
			expected: true, // "comparison" is not a tracked keyword
		},
		{
			name:     "keyword substring (architect)",
			text:     "The architect designed it",
			expected: false,
		},
		{
			name:     "keyword substring (designer)",
			text:     "The designer created mockups",
			expected: false,
		},
		{
			name:     "keyword substring (migration)",
			text:     "The migration is complete",
			expected: true, // "migration" is not a tracked keyword
		},
		{
			name:     "keyword substring (optimization)",
			text:     "Optimization improved speed",
			expected: true, // "optimization" is not a tracked keyword
		},
		{
			name:     "keyword substring (security)",
			text:     "Security is important",
			expected: false,
		},
		{
			name:     "word containing keyword (reimplements)",
			text:     "This reimplements the old code",
			expected: false,
		},
		{
			name:     "word not containing keyword (implant)",
			text:     "The dental implant works well",
			expected: true,
		},
		{
			name:     "word not containing keyword (debugging) but contains debug",
			text:     "debugging is helpful",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := IsSimpleMessage(tt.text)
			if result != tt.expected {
				t.Errorf("IsSimpleMessage(%q) = %v, want %v", tt.text, result, tt.expected)
			}
		})
	}
}

// TestRoutingEdge_CodeBlockVariants tests various code block patterns
func TestRoutingEdge_CodeBlockVariants(t *testing.T) {
	tests := []struct {
		name     string
		text     string
		expected bool
	}{
		{
			name:     "triple backticks with language identifier",
			text:     "```go\nfunc main() {}\n```",
			expected: false,
		},
		{
			name:     "triple backticks with spaces",
			text:     "``` ```",
			expected: false,
		},
		{
			name:     "triple backticks on separate lines",
			text:     "```\ncode here\n```",
			expected: false,
		},
		{
			name:     "multiple separate code blocks",
			text:     "```code1``` and ```code2```",
			expected: false,
		},
		{
			name:     "backtick variants (single, not triple)",
			text:     "Use `let x = 5` for this",
			expected: true,
		},
		{
			name:     "double backticks (not triple)",
			text:     "Use ``code`` notation",
			expected: true,
		},
		{
			name:     "four backticks (more than triple)",
			text:     "Use ````code```` notation",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := IsSimpleMessage(tt.text)
			if result != tt.expected {
				t.Errorf("IsSimpleMessage(%q) = %v, want %v", tt.text, result, tt.expected)
			}
		})
	}
}

// TestRoutingEdge_URLVariants tests various URL patterns
func TestRoutingEdge_URLVariants(t *testing.T) {
	tests := []struct {
		name     string
		text     string
		expected bool
	}{
		{
			name:     "http URL at start",
			text:     "http://example.com is here",
			expected: false,
		},
		{
			name:     "https URL at start",
			text:     "https://example.com is here",
			expected: false,
		},
		{
			name:     "URL with port number",
			text:     "Visit https://localhost:8080/api",
			expected: false,
		},
		{
			name:     "URL with query parameters",
			text:     "Check https://example.com?foo=bar&baz=qux",
			expected: false,
		},
		{
			name:     "URL with fragment",
			text:     "Go to https://example.com#section",
			expected: false,
		},
		{
			name:     "multiple URLs",
			text:     "https://example.com and http://test.com",
			expected: false,
		},
		{
			name:     "ftp URL (not http/https)",
			text:     "Download from ftp://files.example.com",
			expected: true,
		},
		{
			name:     "domain without protocol",
			text:     "Visit example.com today",
			expected: true,
		},
		{
			name:     "localhost without protocol",
			text:     "Running on localhost:3000",
			expected: true,
		},
		{
			name:     "IP address without protocol",
			text:     "Server at 192.168.1.1",
			expected: true,
		},
		{
			name:     "URL at end of sentence",
			text:     "For more info see https://docs.example.com.",
			expected: false,
		},
		{
			name:     "mixed case protocol",
			text:     "Go to HTTPS://example.com",
			expected: true, // regex is case-sensitive, so HTTPS won't match
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := IsSimpleMessage(tt.text)
			if result != tt.expected {
				t.Errorf("IsSimpleMessage(%q) = %v, want %v", tt.text, result, tt.expected)
			}
		})
	}
}

// TestRoutingEdge_CombinedComplexity tests combinations of complexity factors
func TestRoutingEdge_CombinedComplexity(t *testing.T) {
	tests := []struct {
		name     string
		text     string
		expected bool
	}{
		{
			name:     "long + keyword",
			text:     strings.Repeat("a", 200) + " implement",
			expected: false,
		},
		{
			name:     "long + code block",
			text:     strings.Repeat("a", 200) + " ```code```",
			expected: false,
		},
		{
			name:     "many words + keyword",
			text:     strings.Repeat("word ", 50) + "implement",
			expected: false,
		},
		{
			name:     "many words + URL",
			text:     strings.Repeat("word ", 50) + " https://example.com",
			expected: false,
		},
		{
			name:     "code block + URL",
			text:     "```code``` at https://example.com",
			expected: false,
		},
		{
			name:     "code block + keyword",
			text:     "```code``` to implement",
			expected: false,
		},
		{
			name:     "URL + keyword",
			text:     "https://example.com to implement",
			expected: false,
		},
		{
			name:     "all three: URL + code + keyword",
			text:     "https://example.com ```code``` implement",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := IsSimpleMessage(tt.text)
			if result != tt.expected {
				t.Errorf("IsSimpleMessage(%q) = %v, want %v", tt.text, result, tt.expected)
			}
		})
	}
}

// TestRoutingEdge_WhitespaceHandling tests various whitespace scenarios
func TestRoutingEdge_WhitespaceHandling(t *testing.T) {
	tests := []struct {
		name     string
		text     string
		expected bool
	}{
		{
			name:     "leading whitespace only",
			text:     "     ",
			expected: true,
		},
		{
			name:     "trailing whitespace only",
			text:     "     ",
			expected: true,
		},
		{
			name:     "mixed whitespace (spaces, tabs, newlines)",
			text:     "  \t  \n  \r\n  ",
			expected: true,
		},
		{
			name:     "words with excessive internal whitespace",
			text:     "word1      word2      word3",
			expected: true,
		},
		{
			name:     "tabs count as word separators",
			text:     "word\tword\tword",
			expected: true,
		},
		{
			name:     "newlines count as word separators",
			text:     "word\nword\nword",
			expected: true,
		},
		{
			name:     "zero-width characters (if supported)",
			text:     "hello‌world", // contains zero-width non-joiner
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := IsSimpleMessage(tt.text)
			if result != tt.expected {
				t.Errorf("IsSimpleMessage(%q) = %v, want %v", tt.text, result, tt.expected)
			}
		})
	}
}

// TestRoutingEdge_KeywordCaseSensitivity tests case sensitivity of keyword matching
func TestRoutingEdge_KeywordCaseSensitivity(t *testing.T) {
	tests := []struct {
		name     string
		text     string
		expected bool
	}{
		{
			name:     "keyword all uppercase",
			text:     "Can you IMPLEMENT this?",
			expected: false,
		},
		{
			name:     "keyword all lowercase",
			text:     "Can you implement this?",
			expected: false,
		},
		{
			name:     "keyword mixed case (first letter caps)",
			text:     "Can you Implement this?",
			expected: false,
		},
		{
			name:     "keyword mixed case (random caps)",
			text:     "Can you ImPlEmEnT this?",
			expected: false,
		},
		{
			name:     "all keywords uppercase",
			text:     "REFACTOR ANALYZE DEBUG OPTIMIZE",
			expected: false,
		},
		{
			name:     "keyword-like word, different case",
			text:     "This is about DEPLOYMENT",
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := IsSimpleMessage(tt.text)
			if result != tt.expected {
				t.Errorf("IsSimpleMessage(%q) = %v, want %v", tt.text, result, tt.expected)
			}
		})
	}
}

// TestRoutingEdge_SpecialCharactersAndSymbols tests messages with special characters
func TestRoutingEdge_SpecialCharactersAndSymbols(t *testing.T) {
	tests := []struct {
		name     string
		text     string
		expected bool
	}{
		{
			name:     "markdown bold syntax",
			text:     "This is **bold** text",
			expected: true,
		},
		{
			name:     "markdown italic syntax",
			text:     "This is *italic* text",
			expected: true,
		},
		{
			name:     "markdown strikethrough",
			text:     "This is ~~strikethrough~~ text",
			expected: true,
		},
		{
			name:     "HTML tags",
			text:     "This is <b>bold</b> text",
			expected: true,
		},
		{
			name:     "JSON-like content",
			text:     `{"key": "value"}`,
			expected: true,
		},
		{
			name:     "mathematical symbols",
			text:     "What is 2 + 2 × 3 ÷ 4?",
			expected: true,
		},
		{
			name:     "arrows and mathematical notation",
			text:     "a → b → c",
			expected: true,
		},
		{
			name:     "currency symbols",
			text:     "This costs $100 or €100",
			expected: true,
		},
		{
			name:     "at mentions and email addresses",
			text:     "Contact @user or user@example.com",
			expected: true, // email addresses don't match the http:// or https:// pattern
		},
		{
			name:     "hashtags",
			text:     "#golang #testing #programming",
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := IsSimpleMessage(tt.text)
			if result != tt.expected {
				t.Errorf("IsSimpleMessage(%q) = %v, want %v", tt.text, result, tt.expected)
			}
		})
	}
}

// TestRoutingEdge_NullAndControlCharacters tests null bytes and control characters
func TestRoutingEdge_NullAndControlCharacters(t *testing.T) {
	tests := []struct {
		name     string
		text     string
		expected bool
	}{
		{
			name:     "null byte in text",
			text:     "hello\x00world",
			expected: true,
		},
		{
			name:     "bell character",
			text:     "hello\x07world",
			expected: true,
		},
		{
			name:     "vertical tab",
			text:     "hello\x0bworld",
			expected: true,
		},
		{
			name:     "form feed",
			text:     "hello\x0cworld",
			expected: true,
		},
		{
			name:     "escape character",
			text:     "hello\x1bworld",
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := IsSimpleMessage(tt.text)
			if result != tt.expected {
				t.Errorf("IsSimpleMessage(%q) = %v, want %v", tt.text, result, tt.expected)
			}
		})
	}
}

// TestRoutingEdge_Byte_vs_RuneLength validates byte vs rune handling
func TestRoutingEdge_Byte_vs_RuneLength(t *testing.T) {
	// This test validates that the function uses byte length (len()) not rune length
	// A string with 159 ASCII characters is 159 bytes
	asciiString := strings.Repeat("x", 159)
	if !IsSimpleMessage(asciiString) {
		t.Errorf("159 ASCII chars should be simple, got false")
	}

	// A string with 80 emoji is 80 runes but ~240 bytes (each emoji ~3 bytes)
	emojiString := strings.Repeat("😀", 80)
	if IsSimpleMessage(emojiString) {
		t.Errorf("80 emoji (~240 bytes) should be complex, got true")
	}

	// Verify the byte length assumption
	if len(emojiString) < 160 {
		t.Errorf("Test assumption broken: emoji string should exceed 160 bytes, got %d bytes", len(emojiString))
	}
	if utf8.RuneCountInString(emojiString) >= 160 {
		t.Errorf("Test assumption broken: emoji string should have <160 runes, got %d runes", utf8.RuneCountInString(emojiString))
	}
}

// TestRoutingEdge_LargePaddedStrings tests strings padded to exact boundaries with various content
func TestRoutingEdge_LargePaddedStrings(t *testing.T) {
	tests := []struct {
		name     string
		textFunc func() string
		expected bool
	}{
		{
			name: "159 chars with keyword at end",
			textFunc: func() string {
				return strings.Repeat("x", 145) + " implement"
			},
			expected: false, // keyword makes it complex despite being under 160 bytes
		},
		{
			name: "159 chars with URL at end",
			textFunc: func() string {
				return strings.Repeat("x", 110) + " https://example.com"
			},
			expected: false, // URL makes it complex despite byte limit
		},
		{
			name: "27 words with keyword as last word",
			textFunc: func() string {
				base := strings.Repeat("word ", 26)
				return base + "implement"
			},
			expected: false, // keyword makes it complex despite word count
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			text := tt.textFunc()
			result := IsSimpleMessage(text)
			if result != tt.expected {
				t.Errorf("IsSimpleMessage(%q) = %v, want %v", text, result, tt.expected)
			}
		})
	}
}
