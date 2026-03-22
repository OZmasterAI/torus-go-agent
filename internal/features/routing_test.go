package features

import (
	"strings"
	"testing"
)

func TestIsSimpleMessage(t *testing.T) {
	tests := []struct {
		name     string
		text     string
		expected bool
	}{
		// Simple messages (should return true)
		{
			name:     "empty string",
			text:     "",
			expected: true,
		},
		{
			name:     "single word",
			text:     "hello",
			expected: true,
		},
		{
			name:     "short phrase",
			text:     "What time is it?",
			expected: true,
		},
		{
			name:     "greeting",
			text:     "Hi there, how are you?",
			expected: true,
		},
		{
			name:     "simple question",
			text:     "Is it raining outside?",
			expected: true,
		},
		{
			name:     "short sentence with punctuation",
			text:     "This is a test message.",
			expected: true,
		},
		{
			name:     "at word boundary (27 words)",
			text:     strings.Repeat("word ", 27),
			expected: true,
		},
		{
			name:     "at character boundary (159 chars)",
			text:     strings.Repeat("a", 159),
			expected: true,
		},

		// Complex messages due to length (should return false)
		{
			name:     "exactly 160 characters",
			text:     strings.Repeat("a", 160),
			expected: false,
		},
		{
			name:     "over 160 characters",
			text:     strings.Repeat("a", 200),
			expected: false,
		},

		// Complex messages due to word count (should return false)
		{
			name:     "exactly 28 words",
			text:     strings.Repeat("word ", 28),
			expected: false,
		},
		{
			name:     "over 28 words",
			text:     strings.Repeat("word ", 50),
			expected: false,
		},

		// Complex messages due to code blocks (should return false)
		{
			name:     "single backticks (not triple)",
			text:     "here is `some code`",
			expected: true,
		},
		{
			name:     "triple backticks code block",
			text:     "Here is code: ```python\nprint('hello')\n```",
			expected: false,
		},
		{
			name:     "triple backticks at start",
			text:     "```let x = 5;```",
			expected: false,
		},
		{
			name:     "triple backticks only",
			text:     "```",
			expected: false,
		},
		{
			name:     "multiple triple backticks",
			text:     "Start``` middle ```end",
			expected: false,
		},

		// Complex messages due to URLs (should return false)
		{
			name:     "http URL",
			text:     "Check out http://example.com",
			expected: false,
		},
		{
			name:     "https URL",
			text:     "Visit https://github.com/user/repo",
			expected: false,
		},
		{
			name:     "URL in middle",
			text:     "The page is at https://example.com/page",
			expected: false,
		},
		{
			name:     "URL-like but no protocol",
			text:     "Go to example.com",
			expected: true,
		},

		// Complex messages due to keywords (should return false)
		{
			name:     "keyword: implement",
			text:     "Can you implement this feature?",
			expected: false,
		},
		{
			name:     "keyword: refactor",
			text:     "Please refactor the code",
			expected: false,
		},
		{
			name:     "keyword: debug",
			text:     "Help me debug this",
			expected: false,
		},
		{
			name:     "keyword: analyze",
			text:     "Can you analyze the results?",
			expected: false,
		},
		{
			name:     "keyword: compare",
			text:     "Compare these two approaches",
			expected: false,
		},
		{
			name:     "keyword: architecture",
			text:     "What about the architecture?",
			expected: false,
		},
		{
			name:     "keyword: design",
			text:     "Design a new API",
			expected: false,
		},
		{
			name:     "keyword: migrate",
			text:     "How to migrate data?",
			expected: false,
		},
		{
			name:     "keyword: optimize",
			text:     "Optimize the query",
			expected: false,
		},
		{
			name:     "keyword: security",
			text:     "Check security vulnerabilities",
			expected: false,
		},
		{
			name:     "keyword: case insensitive (uppercase)",
			text:     "IMPLEMENT this feature",
			expected: false,
		},
		{
			name:     "keyword: case insensitive (mixed case)",
			text:     "Please ImPlEmEnT the feature",
			expected: false,
		},
		{
			name:     "keyword: partial match in word",
			text:     "The implementation is ready",
			expected: false,
		},

		// Edge cases
		{
			name:     "whitespace only",
			text:     "   \t  \n  ",
			expected: true,
		},
		{
			name:     "punctuation only",
			text:     "!?!?!?",
			expected: true,
		},
		{
			name:     "numbers only",
			text:     "123456789",
			expected: true,
		},
		{
			name:     "special characters",
			text:     "@#$%^&*()",
			expected: true,
		},
		{
			name:     "mixed: simple words with numbers",
			text:     "What is 2 + 2?",
			expected: true,
		},
		{
			name:     "combined: URL and code block",
			text:     "See https://example.com for ```code```",
			expected: false,
		},
		{
			name:     "combined: keyword and code block",
			text:     "Here's how to optimize: ```",
			expected: false,
		},
		{
			name:     "combined: keyword and URL",
			text:     "Optimize https://example.com",
			expected: false,
		},
		{
			name:     "word count with extra whitespace",
			text:     "word " + strings.Repeat("  ", 20) + "word",
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

// Benchmark IsSimpleMessage with various message types
func BenchmarkIsSimpleMessageSimple(b *testing.B) {
	text := "Hi there, how are you?"
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		IsSimpleMessage(text)
	}
}

func BenchmarkIsSimpleMessageLong(b *testing.B) {
	text := strings.Repeat("word ", 50)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		IsSimpleMessage(text)
	}
}

func BenchmarkIsSimpleMessageWithCodeBlock(b *testing.B) {
	text := "Here is code: ```python\nprint('hello')\n```"
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		IsSimpleMessage(text)
	}
}

func BenchmarkIsSimpleMessageWithURL(b *testing.B) {
	text := "Check out https://example.com for more info"
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		IsSimpleMessage(text)
	}
}

func BenchmarkIsSimpleMessageWithKeyword(b *testing.B) {
	text := "Can you implement this feature?"
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		IsSimpleMessage(text)
	}
}
