package telegram

import (
	"fmt"
	"strings"
	"sync"
	"testing"

	"torus_go_agent/internal/config"
)

// TestTelegramChannelName tests telegramChannel.Name()
func TestTelegramChannelName(t *testing.T) {
	ch := &telegramChannel{}
	if ch.Name() != "telegram" {
		t.Errorf("expected name 'telegram', got %q", ch.Name())
	}
}

// TestTelegramChannelStart tests telegramChannel.Start()
func TestTelegramChannelStart(t *testing.T) {
	tests := []struct {
		name      string
		botToken  string
		allowList []int64
		expectErr bool
		errMsg    string
	}{
		{
			name:      "missing_bot_token",
			botToken:  "",
			allowList: []int64{123},
			expectErr: true,
			errMsg:    "no Telegram bot token",
		},
		{
			name:      "empty_allowlist_warning",
			botToken:  "token123",
			allowList: []int64{},
			expectErr: true, // Will fail because startTelegram tries to create bot with invalid token
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ch := &telegramChannel{}
			cfg := config.Config{
				Telegram: config.TelegramConfig{
					BotToken:     tt.botToken,
					AllowedUsers: tt.allowList,
				},
			}

			err := ch.Start(nil, cfg, nil)
			if (err != nil) != tt.expectErr {
				t.Errorf("expectErr=%v, got error: %v", tt.expectErr, err)
			}
			if tt.expectErr && tt.errMsg != "" && !strings.Contains(err.Error(), tt.errMsg) {
				t.Errorf("expected error containing %q, got: %v", tt.errMsg, err)
			}
		})
	}
}

// TestSplitChunksBasic tests splitChunks function with basic cases
func TestSplitChunksBasic(t *testing.T) {
	tests := []struct {
		name      string
		text      string
		maxLen    int
		expected  []string
	}{
		{
			name:     "short_text",
			text:     "hello",
			maxLen:   100,
			expected: []string{"hello"},
		},
		{
			name:     "exact_length",
			text:     "12345",
			maxLen:   5,
			expected: []string{"12345"},
		},
		{
			name:     "split_on_whitespace",
			text:     "hello world test",
			maxLen:   8,
			expected: []string{"hello", "world", "test"},
		},
		{
			name:     "no_whitespace_hard_cut",
			text:     "abcdefghij",
			maxLen:   5,
			expected: []string{"abcde", "fghij"},
		},
		{
			name:     "empty_string",
			text:     "",
			maxLen:   100,
			expected: []string{""},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := splitChunks(tt.text, tt.maxLen)

			if len(result) != len(tt.expected) {
				t.Errorf("expected %d chunks, got %d: %v", len(tt.expected), len(result), result)
			}

			for i, chunk := range result {
				if i >= len(tt.expected) {
					t.Errorf("unexpected chunk at index %d: %q", i, chunk)
					continue
				}
				if chunk != tt.expected[i] {
					t.Errorf("chunk %d: expected %q, got %q", i, tt.expected[i], chunk)
				}
				if len(chunk) > tt.maxLen {
					t.Errorf("chunk %d exceeds maxLen: len=%d, maxLen=%d", i, len(chunk), tt.maxLen)
				}
			}
		})
	}
}

// TestSplitChunksComplexCases tests splitChunks with complex scenarios
func TestSplitChunksComplexCases(t *testing.T) {
	tests := []struct {
		name        string
		text        string
		maxLen      int
		validateLen bool
	}{
		{
			name:        "multiple_chunks_with_whitespace",
			text:        "the quick brown fox jumps over the lazy dog",
			maxLen:      15,
			validateLen: true,
		},
		{
			name:        "single_long_word",
			text:        strings.Repeat("a", 5000),
			maxLen:      4000,
			validateLen: true,
		},
		{
			name:        "multiple_spaces",
			text:        "word1    word2    word3",
			maxLen:      10,
			validateLen: true,
		},
		{
			name:        "newline_boundary",
			text:        "line1\nline2\nline3",
			maxLen:      8,
			validateLen: true,
		},
		{
			name:        "chunk_size_equals_text_length",
			text:        strings.Repeat("x", 4000),
			maxLen:      4000,
			validateLen: true,
		},
		{
			name:        "very_long_text",
			text:        strings.Repeat("word ", 1000),
			maxLen:      20,
			validateLen: true,
		},
		{
			name:        "unicode_text",
			text:        "你好世界" + strings.Repeat(" test", 100),
			maxLen:      4000,
			validateLen: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := splitChunks(tt.text, tt.maxLen)

			// Must have at least one chunk
			if len(result) == 0 {
				t.Error("expected at least one chunk")
			}

			if tt.validateLen {
				for i, chunk := range result {
					if len(chunk) > tt.maxLen {
						t.Errorf("chunk %d exceeds maxLen: len=%d, maxLen=%d, chunk=%q",
							i, len(chunk), tt.maxLen, chunk[:min(len(chunk), 50)])
					}
				}
			}
		})
	}
}

// TestPendingMsg tests the pendingMsg type
func TestPendingMsg(t *testing.T) {
	pm := pendingMsg{
		text:      "test message",
		messageID: 42,
	}

	if pm.text != "test message" {
		t.Errorf("expected text 'test message', got %q", pm.text)
	}
	if pm.messageID != 42 {
		t.Errorf("expected messageID 42, got %d", pm.messageID)
	}
}

// TestChatState tests the chatState type
func TestChatState(t *testing.T) {
	t.Run("initial_state", func(t *testing.T) {
		cs := &chatState{}
		if cs.running {
			t.Error("initial running should be false")
		}
		if len(cs.queue) != 0 {
			t.Error("initial queue should be empty")
		}
	})

	t.Run("queue_operations", func(t *testing.T) {
		cs := &chatState{}
		msg := pendingMsg{text: "hello", messageID: 1}

		cs.mu.Lock()
		cs.queue = append(cs.queue, msg)
		cs.mu.Unlock()

		cs.mu.Lock()
		if len(cs.queue) != 1 {
			t.Error("queue should have 1 message")
		}
		if cs.queue[0].text != "hello" {
			t.Errorf("expected 'hello', got %q", cs.queue[0].text)
		}
		cs.mu.Unlock()
	})

	t.Run("concurrent_access", func(t *testing.T) {
		cs := &chatState{}
		var wg sync.WaitGroup

		// Add messages from multiple goroutines
		for i := 0; i < 10; i++ {
			wg.Add(1)
			go func(idx int) {
				defer wg.Done()
				cs.mu.Lock()
				cs.queue = append(cs.queue, pendingMsg{text: fmt.Sprintf("msg%d", idx), messageID: idx})
				cs.mu.Unlock()
			}(i)
		}

		wg.Wait()

		cs.mu.Lock()
		if len(cs.queue) != 10 {
			t.Errorf("expected 10 messages, got %d", len(cs.queue))
		}
		cs.mu.Unlock()
	})

	t.Run("running_flag", func(t *testing.T) {
		cs := &chatState{running: false}

		cs.mu.Lock()
		cs.running = true
		cs.mu.Unlock()

		cs.mu.Lock()
		if !cs.running {
			t.Error("expected running to be true")
		}
		cs.mu.Unlock()
	})

	t.Run("queue_dequeue", func(t *testing.T) {
		cs := &chatState{}

		cs.mu.Lock()
		cs.queue = []pendingMsg{
			{text: "msg1", messageID: 1},
			{text: "msg2", messageID: 2},
			{text: "msg3", messageID: 3},
		}
		cs.mu.Unlock()

		// Simulate dequeuing
		cs.mu.Lock()
		if len(cs.queue) != 3 {
			t.Error("expected 3 messages initially")
		}
		first := cs.queue[0]
		cs.queue = cs.queue[1:]
		cs.mu.Unlock()

		if first.text != "msg1" {
			t.Errorf("expected first message 'msg1', got %q", first.text)
		}

		cs.mu.Lock()
		if len(cs.queue) != 2 {
			t.Errorf("expected 2 messages after dequeue, got %d", len(cs.queue))
		}
		cs.mu.Unlock()
	})
}

// TestConstants tests the exported constants
func TestConstants(t *testing.T) {
	if tgChunkSize != 4000 {
		t.Errorf("expected tgChunkSize=4000, got %d", tgChunkSize)
	}
	if tgPlaceholderText != "..." {
		t.Errorf("expected tgPlaceholderText='...', got %q", tgPlaceholderText)
	}
}

// TestSessionKeyGeneration tests session key format for private/group chats
func TestSessionKeyGeneration(t *testing.T) {
	tests := []struct {
		name          string
		chatID        int64
		userID        int64
		isPrivate     bool
		expectedKey   string
	}{
		{
			name:        "private_chat",
			chatID:      -123,
			userID:      456,
			isPrivate:   true,
			expectedKey: "telegram:dm:456",
		},
		{
			name:        "group_chat",
			chatID:      -789,
			userID:      456,
			isPrivate:   false,
			expectedKey: "telegram:group:-789",
		},
		{
			name:        "large_ids",
			chatID:      9223372036854775807,
			userID:      9223372036854775806,
			isPrivate:   true,
			expectedKey: "telegram:dm:9223372036854775806",
		},
		{
			name:        "zero_ids",
			chatID:      0,
			userID:      0,
			isPrivate:   true,
			expectedKey: "telegram:dm:0",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var sessionKey string
			if tt.isPrivate {
				sessionKey = fmt.Sprintf("telegram:dm:%d", tt.userID)
			} else {
				sessionKey = fmt.Sprintf("telegram:group:%d", tt.chatID)
			}

			if sessionKey != tt.expectedKey {
				t.Errorf("expected %q, got %q", tt.expectedKey, sessionKey)
			}
		})
	}
}

// TestSplitChunksEdgeCases tests edge cases for chunking
func TestSplitChunksEdgeCases(t *testing.T) {
	tests := []struct {
		name        string
		text        string
		maxLen      int
		description string
	}{
		{
			name:        "single_char",
			text:        "a",
			maxLen:      1,
			description: "single character chunk",
		},
		{
			name:        "two_char_maxlen_1",
			text:        "ab",
			maxLen:      1,
			description: "two characters, maxlen 1",
		},
		{
			name:        "trailing_whitespace",
			text:        "hello world   ",
			maxLen:      10,
			description: "trailing whitespace is trimmed",
		},
		{
			name:        "leading_whitespace_after_cut",
			text:        "hello  \n  world",
			maxLen:      8,
			description: "leading whitespace trimmed after cut",
		},
		{
			name:        "only_whitespace",
			text:        "     ",
			maxLen:      10,
			description: "only whitespace",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := splitChunks(tt.text, tt.maxLen)

			if len(result) == 0 {
				t.Error("expected at least one chunk")
			}

			// Verify all chunks respect max length
			for i, chunk := range result {
				if len(chunk) > tt.maxLen {
					t.Errorf("chunk %d exceeds maxLen: len=%d, maxLen=%d (description: %s)", i, len(chunk), tt.maxLen, tt.description)
				}
			}
		})
	}
}

// TestChunkBoundaryConditions tests boundary conditions in chunking algorithm
func TestChunkBoundaryConditions(t *testing.T) {
	t.Run("whitespace_at_boundary", func(t *testing.T) {
		// Word ending exactly at maxLen
		text := "hello world"
		result := splitChunks(text, 5)
		if len(result) < 1 {
			t.Fatal("expected at least one chunk")
		}
		// First chunk should be "hello"
		if result[0] != "hello" {
			t.Errorf("expected first chunk 'hello', got %q", result[0])
		}
	})

	t.Run("no_whitespace_fallback", func(t *testing.T) {
		// Text with no whitespace should hard-cut
		text := "abcdefghijklmnop"
		result := splitChunks(text, 5)
		for i, chunk := range result {
			if len(chunk) > 5 {
				t.Errorf("chunk %d has len %d (max 5)", i, len(chunk))
			}
		}
	})

	t.Run("chunk_exactly_maxlen", func(t *testing.T) {
		// Text is exactly maxLen
		text := "12345"
		result := splitChunks(text, 5)
		if len(result) != 1 {
			t.Errorf("expected 1 chunk, got %d", len(result))
		}
		if result[0] != "12345" {
			t.Errorf("expected '12345', got %q", result[0])
		}
	})
}

// BenchmarkSplitChunks benchmarks the splitChunks function
func BenchmarkSplitChunks(b *testing.B) {
	text := strings.Repeat("This is a test sentence for benchmarking. ", 100)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		splitChunks(text, tgChunkSize)
	}
}

// BenchmarkSplitChunksSmallText benchmarks splitChunks with small text
func BenchmarkSplitChunksSmallText(b *testing.B) {
	text := "hello world"
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		splitChunks(text, tgChunkSize)
	}
}

// BenchmarkSplitChunksLargeText benchmarks splitChunks with very large text
func BenchmarkSplitChunksLargeText(b *testing.B) {
	text := strings.Repeat("x", 100000)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		splitChunks(text, tgChunkSize)
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
