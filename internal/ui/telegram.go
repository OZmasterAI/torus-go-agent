// Package ui provides user-interface channels for the agent (Telegram, TUI, etc.).
package ui

import (
	"context"
	"fmt"
	"log"
	"strings"
	"sync"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"

	"go_sdk_agent/internal/config"
	"go_sdk_agent/internal/core"
)

const (
	chunkSize       = 4000
	placeholderText = "..."
)

// pendingMsg holds a queued user message waiting for the active run to finish.
type pendingMsg struct {
	text      string
	messageID int
}

// chatState tracks per-chat concurrency state.
type chatState struct {
	mu      sync.Mutex
	running bool
	queue   []pendingMsg
}

// StartTelegram creates a Telegram bot, begins long-polling, and dispatches
// incoming messages to agent.Run. It blocks until polling stops or a fatal
// error occurs.
func StartTelegram(agent *core.Agent, cfg config.TelegramConfig) error {
	if len(cfg.AllowedUsers) == 0 {
		log.Printf("[telegram] WARNING: allowedUsers is empty — all requests will be denied")
	}

	allowSet := make(map[int64]struct{}, len(cfg.AllowedUsers))
	for _, id := range cfg.AllowedUsers {
		allowSet[id] = struct{}{}
	}

	bot, err := tgbotapi.NewBotAPI(cfg.BotToken)
	if err != nil {
		return fmt.Errorf("create bot: %w", err)
	}
	log.Printf("[telegram] authorized as @%s", bot.Self.UserName)

	u := tgbotapi.NewUpdate(0)
	u.Timeout = 60
	updates := bot.GetUpdatesChan(u)

	// sync.Map maps chatID (int64) → *chatState
	var chats sync.Map

	for update := range updates {
		if update.Message == nil || !update.Message.IsCommand() && update.Message.Text == "" {
			continue
		}

		msg := update.Message
		userID := msg.From.ID
		chatID := msg.Chat.ID
		text := msg.Text

		// Allowlist check (default-deny)
		if _, ok := allowSet[userID]; !ok {
			log.Printf("[telegram] denied user %d (chat %d)", userID, chatID)
			reply := tgbotapi.NewMessage(chatID, "Access denied.")
			bot.Send(reply) //nolint:errcheck
			continue
		}

		// Build session key
		var sessionKey string
		if msg.Chat.IsPrivate() {
			sessionKey = fmt.Sprintf("telegram:dm:%d", userID)
		} else {
			sessionKey = fmt.Sprintf("telegram:group:%d", chatID)
		}

		// Load or create chat state
		val, _ := chats.LoadOrStore(chatID, &chatState{})
		cs := val.(*chatState)

		cs.mu.Lock()
		if cs.running {
			// Agent busy — queue the message
			cs.queue = append(cs.queue, pendingMsg{text: text, messageID: msg.MessageID})
			cs.mu.Unlock()
			queuedReply := tgbotapi.NewMessage(chatID, "(queued)")
			bot.Send(queuedReply) //nolint:errcheck
			continue
		}
		cs.running = true
		cs.mu.Unlock()

		// Launch handler goroutine
		go handleMessage(bot, agent, cs, chatID, userID, sessionKey, text, msg.MessageID)
	}

	return nil
}

// handleMessage runs agent.Run for one message, then drains the queue.
func handleMessage(
	bot *tgbotapi.BotAPI,
	agent *core.Agent,
	cs *chatState,
	chatID int64,
	_ int64, // userID — reserved for future per-user session isolation
	sessionKey string,
	text string,
	_ int, // incomingMsgID — reserved
) {
	defer func() {
		// After finishing, drain queue
		for {
			cs.mu.Lock()
			if len(cs.queue) == 0 {
				cs.running = false
				cs.mu.Unlock()
				return
			}
			next := cs.queue[0]
			cs.queue = cs.queue[1:]
			cs.mu.Unlock()

			runOne(bot, agent, chatID, sessionKey, next.text)
		}
	}()

	runOne(bot, agent, chatID, sessionKey, text)
}

// runOne sends a placeholder, runs the agent, then edits the placeholder with
// the final response (chunking if necessary).
func runOne(bot *tgbotapi.BotAPI, agent *core.Agent, chatID int64, sessionKey string, text string) {
	// Send placeholder
	placeholderMsg := tgbotapi.NewMessage(chatID, placeholderText)
	sent, err := bot.Send(placeholderMsg)
	if err != nil {
		log.Printf("[telegram] send placeholder (chat %d): %v", chatID, err)
		// Fall back to sending without placeholder
		sent.MessageID = 0
	}

	// Run the agent
	// sessionKey is available for future session-scoped DAG branching.
	_ = sessionKey
	response, err := agent.Run(context.Background(), text)
	if err != nil {
		log.Printf("[telegram] agent.Run error (chat %d): %v", chatID, err)
		errText := fmt.Sprintf("Error: %v", err)
		if editErr := editOrSend(bot, chatID, sent.MessageID, errText); editErr != nil {
			log.Printf("[telegram] editOrSend error (chat %d): %v", chatID, editErr)
		}
		return
	}

	if response == "" {
		response = "(no response)"
	}

	// If response fits in one chunk, edit the placeholder
	if len(response) <= chunkSize {
		if err := editOrSend(bot, chatID, sent.MessageID, response); err != nil {
			log.Printf("[telegram] editOrSend (chat %d): %v", chatID, err)
		}
		return
	}

	// Long response: delete placeholder and send chunks
	if sent.MessageID != 0 {
		del := tgbotapi.NewDeleteMessage(chatID, sent.MessageID)
		bot.Request(del) //nolint:errcheck
	}
	if err := sendChunked(bot, chatID, response); err != nil {
		log.Printf("[telegram] sendChunked (chat %d): %v", chatID, err)
	}
}

// sendChunked splits text at chunkSize boundaries (on whitespace when possible)
// and sends each chunk as a separate message.
func sendChunked(bot *tgbotapi.BotAPI, chatID int64, text string) error {
	chunks := splitChunks(text, chunkSize)
	for _, chunk := range chunks {
		msg := tgbotapi.NewMessage(chatID, chunk)
		if _, err := bot.Send(msg); err != nil {
			return fmt.Errorf("send chunk: %w", err)
		}
	}
	return nil
}

// editOrSend edits msgID with text. If msgID is 0 or the edit fails with
// "message is not modified", it falls back to sending a new message.
func editOrSend(bot *tgbotapi.BotAPI, chatID int64, msgID int, text string) error {
	if msgID != 0 {
		edit := tgbotapi.NewEditMessageText(chatID, msgID, text)
		if _, err := bot.Send(edit); err != nil {
			// Telegram returns an error when content hasn't changed; ignore it.
			if !strings.Contains(err.Error(), "message is not modified") {
				// Edit failed for another reason — fall through to send new message
				log.Printf("[telegram] edit message %d failed: %v — sending new message", msgID, err)
			} else {
				return nil
			}
		} else {
			return nil
		}
	}

	// Fallback: send as new message
	newMsg := tgbotapi.NewMessage(chatID, text)
	if _, err := bot.Send(newMsg); err != nil {
		return fmt.Errorf("send message: %w", err)
	}
	return nil
}

// splitChunks divides text into segments no longer than maxLen. It tries to
// break on the last whitespace before the limit to avoid splitting words.
func splitChunks(text string, maxLen int) []string {
	if len(text) <= maxLen {
		return []string{text}
	}

	var chunks []string
	for len(text) > 0 {
		if len(text) <= maxLen {
			chunks = append(chunks, text)
			break
		}

		cut := maxLen
		// Walk back to find a whitespace boundary
		for cut > maxLen/2 && cut < len(text) && text[cut] != ' ' && text[cut] != '\n' {
			cut--
		}
		// If no whitespace found in the back half, hard-cut at maxLen
		if cut <= maxLen/2 {
			cut = maxLen
		}

		chunks = append(chunks, text[:cut])
		text = strings.TrimLeft(text[cut:], " \n")
	}
	return chunks
}
