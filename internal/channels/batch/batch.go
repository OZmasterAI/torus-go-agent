// Package batch provides a non-interactive channel for automated evaluation.
// It reads a prompt from a file, runs the agent, and writes structured trace output.
package batch

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"time"

	"torus_go_agent/internal/channels"
	"torus_go_agent/internal/config"
	"torus_go_agent/internal/core"
	"torus_go_agent/internal/features"
)

func init() { channels.Register(&batchChannel{}) }

// Config is set by cmd/main.go before Start is called.
var Config = struct {
	PromptFile string
	OutputDir  string
	WorkDir    string // working directory for agent tool execution (default: current dir)
	MultiTurn  bool   // parse prompt file as JSON array of user messages
}{}

type batchChannel struct{}

func (b *batchChannel) Name() string { return "batch" }

// TraceEvent records a single event from the agent loop.
type TraceEvent struct {
	Time       string         `json:"time"`
	Type       string         `json:"type"`
	Turn       int            `json:"turn,omitempty"`
	Text       string         `json:"text,omitempty"`
	ToolName   string         `json:"tool_name,omitempty"`
	ToolArgs   map[string]any `json:"tool_args,omitempty"`
	ToolResult string         `json:"tool_result,omitempty"`
	ToolError  bool           `json:"tool_error,omitempty"`
	Error      string         `json:"error,omitempty"`
	Usage      *UsageRecord   `json:"usage,omitempty"`
}

// UsageRecord is a JSON-friendly copy of types.Usage.
type UsageRecord struct {
	InputTokens      int     `json:"input_tokens"`
	OutputTokens     int     `json:"output_tokens"`
	CacheReadTokens  int     `json:"cache_read_tokens,omitempty"`
	CacheWriteTokens int     `json:"cache_write_tokens,omitempty"`
	TotalTokens      int     `json:"total_tokens"`
	Cost             float64 `json:"cost,omitempty"`
}

// Result is the top-level output written to result.json.
type Result struct {
	Prompt       string       `json:"prompt"`
	Response     string       `json:"response"`
	Error        string       `json:"error,omitempty"`
	Turns        int          `json:"turns"`
	TotalInput   int          `json:"total_input_tokens"`
	TotalOutput  int          `json:"total_output_tokens"`
	TotalCost    float64      `json:"total_cost"`
	DurationMs   int64        `json:"duration_ms"`
	ToolCalls    int          `json:"tool_calls"`
	Trace        []TraceEvent `json:"trace"`
}

func (b *batchChannel) Start(agent *core.Agent, cfg config.Config, _ *features.SkillRegistry) error {
	promptFile := Config.PromptFile
	outputDir := Config.OutputDir

	if promptFile == "" {
		return fmt.Errorf("batch: --batch=<prompt-file> is required")
	}

	// Read prompt
	promptBytes, err := os.ReadFile(promptFile)
	if err != nil {
		return fmt.Errorf("batch: read prompt: %w", err)
	}
	prompt := string(promptBytes)
	if prompt == "" {
		return fmt.Errorf("batch: prompt file is empty: %s", promptFile)
	}

	// Default output dir to current directory.
	// Resolve to absolute path before any chdir so result.json lands in the right place.
	if outputDir == "" {
		outputDir = "."
	}
	outputDir, err = filepath.Abs(outputDir)
	if err != nil {
		return fmt.Errorf("batch: resolve output dir: %w", err)
	}
	if err := os.MkdirAll(outputDir, 0o755); err != nil {
		return fmt.Errorf("batch: create output dir: %w", err)
	}

	// Change working directory if specified (so agent tools operate on the right files)
	if Config.WorkDir != "" {
		absWorkDir, err := filepath.Abs(Config.WorkDir)
		if err != nil {
			return fmt.Errorf("batch: resolve workdir: %w", err)
		}
		if err := os.Chdir(absWorkDir); err != nil {
			return fmt.Errorf("batch: chdir to %s: %w", absWorkDir, err)
		}
		log.Printf("[batch] workdir: %s", absWorkDir)
	}

	log.Printf("[batch] prompt: %s (%d chars)", promptFile, len(prompt))
	log.Printf("[batch] output: %s", outputDir)

	// Multi-turn mode: parse prompt as JSON array and send each message sequentially
	if Config.MultiTurn {
		return b.runMultiTurn(agent, prompt, outputDir)
	}

	// Run agent and collect trace
	start := time.Now()
	var trace []TraceEvent
	var finalText, finalErr string
	var totalIn, totalOut, toolCalls, turns int
	var totalCost float64

	ctx := context.Background()
	for ev := range agent.RunStream(ctx, prompt) {
		now := time.Now().UTC().Format(time.RFC3339Nano)

		switch ev.Type {
		case core.EventAgentTurnStart:
			turns = ev.Turn
			trace = append(trace, TraceEvent{
				Time: now, Type: "turn_start", Turn: ev.Turn,
			})

		case core.EventAgentTurnEnd:
			var usage *UsageRecord
			if ev.Usage != nil {
				usage = &UsageRecord{
					InputTokens:      ev.Usage.InputTokens,
					OutputTokens:     ev.Usage.OutputTokens,
					CacheReadTokens:  ev.Usage.CacheReadTokens,
					CacheWriteTokens: ev.Usage.CacheWriteTokens,
					TotalTokens:      ev.Usage.TotalTokens,
					Cost:             ev.Usage.Cost,
				}
				totalIn += ev.Usage.InputTokens
				totalOut += ev.Usage.OutputTokens
				totalCost += ev.Usage.Cost
			}
			trace = append(trace, TraceEvent{
				Time: now, Type: "turn_end", Turn: ev.Turn, Usage: usage,
			})

		case core.EventAgentToolStart:
			trace = append(trace, TraceEvent{
				Time: now, Type: "tool_start",
				ToolName: ev.ToolName, ToolArgs: ev.ToolArgs,
			})

		case core.EventAgentToolEnd:
			toolCalls++
			te := TraceEvent{
				Time: now, Type: "tool_end", ToolName: ev.ToolName,
			}
			if ev.ToolResult != nil {
				te.ToolResult = ev.ToolResult.Content
				te.ToolError = ev.ToolResult.IsError
			}
			trace = append(trace, te)

		case core.EventAgentDone:
			finalText = ev.Text
			trace = append(trace, TraceEvent{
				Time: now, Type: "done", Text: ev.Text,
			})

		case core.EventAgentError:
			if ev.Error != nil {
				finalErr = ev.Error.Error()
			}
			trace = append(trace, TraceEvent{
				Time: now, Type: "error", Error: finalErr,
			})

		case core.EventAgentTextDelta:
			// Skip deltas in trace — they bloat output.
			// Final text is captured in EventAgentDone.
		}
	}

	duration := time.Since(start)

	result := Result{
		Prompt:      prompt,
		Response:    finalText,
		Error:       finalErr,
		Turns:       turns,
		TotalInput:  totalIn,
		TotalOutput: totalOut,
		TotalCost:   totalCost,
		DurationMs:  duration.Milliseconds(),
		ToolCalls:   toolCalls,
		Trace:       trace,
	}

	// Write result.json
	resultPath := filepath.Join(outputDir, "result.json")
	f, err := os.Create(resultPath)
	if err != nil {
		return fmt.Errorf("batch: create result: %w", err)
	}
	enc := json.NewEncoder(f)
	enc.SetIndent("", "  ")
	if err := enc.Encode(result); err != nil {
		f.Close()
		return fmt.Errorf("batch: write result: %w", err)
	}
	f.Close()

	log.Printf("[batch] done in %s — turns=%d tools=%d tokens=%d/%d cost=$%.4f",
		duration.Round(time.Millisecond), turns, toolCalls, totalIn, totalOut, totalCost)
	log.Printf("[batch] result: %s", resultPath)

	if finalErr != "" {
		os.Exit(1)
	}
	return nil
}

// TurnResult captures one message exchange in multi-turn mode.
type TurnResult struct {
	Message  string       `json:"message"`
	Response string       `json:"response"`
	Turns    int          `json:"turns"`
	Input    int          `json:"input_tokens"`
	Output   int          `json:"output_tokens"`
	Cost     float64      `json:"cost"`
	Tools    int          `json:"tool_calls"`
	Trace    []TraceEvent `json:"trace"`
}

// MultiTurnResult is written to result.json in multi-turn mode.
type MultiTurnResult struct {
	Messages    []TurnResult `json:"messages"`
	TotalInput  int          `json:"total_input_tokens"`
	TotalOutput int          `json:"total_output_tokens"`
	TotalCost   float64      `json:"total_cost"`
	TotalTurns  int          `json:"total_turns"`
	TotalTools  int          `json:"total_tool_calls"`
	DurationMs  int64        `json:"duration_ms"`
	LastResponse string      `json:"last_response"`
}

func (b *batchChannel) runMultiTurn(agent *core.Agent, rawPrompt string, outputDir string) error {
	// Parse as JSON array of strings
	var messages []string
	if err := json.Unmarshal([]byte(rawPrompt), &messages); err != nil {
		// Reset partial results from failed string parse
		messages = nil
		// Try parsing as array of {role, content} objects
		var objMessages []struct {
			Role    string `json:"role"`
			Content string `json:"content"`
		}
		if err2 := json.Unmarshal([]byte(rawPrompt), &objMessages); err2 != nil {
			return fmt.Errorf("batch: multi-turn: expected JSON array of strings or [{role,content}]: %w", err)
		}
		for _, m := range objMessages {
			messages = append(messages, m.Content)
		}
	}
	if len(messages) == 0 {
		return fmt.Errorf("batch: multi-turn: empty message array")
	}

	log.Printf("[batch] multi-turn: %d messages", len(messages))
	start := time.Now()
	ctx := context.Background()

	var turnResults []TurnResult
	var totalIn, totalOut, totalTools, totalTurns int
	var totalCost float64

	for i, msg := range messages {
		log.Printf("[batch] multi-turn message %d/%d (%d chars)", i+1, len(messages), len(msg))

		var trace []TraceEvent
		var finalText string
		var msgIn, msgOut, msgTools, msgTurns int
		var msgCost float64

		for ev := range agent.RunStream(ctx, msg) {
			now := time.Now().UTC().Format(time.RFC3339Nano)
			switch ev.Type {
			case core.EventAgentTurnStart:
				msgTurns = ev.Turn
				trace = append(trace, TraceEvent{Time: now, Type: "turn_start", Turn: ev.Turn})
			case core.EventAgentTurnEnd:
				var usage *UsageRecord
				if ev.Usage != nil {
					usage = &UsageRecord{
						InputTokens:      ev.Usage.InputTokens,
						OutputTokens:     ev.Usage.OutputTokens,
						CacheReadTokens:  ev.Usage.CacheReadTokens,
						CacheWriteTokens: ev.Usage.CacheWriteTokens,
						TotalTokens:      ev.Usage.TotalTokens,
						Cost:             ev.Usage.Cost,
					}
					msgIn += ev.Usage.InputTokens
					msgOut += ev.Usage.OutputTokens
					msgCost += ev.Usage.Cost
				}
				trace = append(trace, TraceEvent{Time: now, Type: "turn_end", Turn: ev.Turn, Usage: usage})
			case core.EventAgentToolStart:
				trace = append(trace, TraceEvent{
					Time: now, Type: "tool_start", ToolName: ev.ToolName, ToolArgs: ev.ToolArgs,
				})
			case core.EventAgentToolEnd:
				msgTools++
				te := TraceEvent{Time: now, Type: "tool_end", ToolName: ev.ToolName}
				if ev.ToolResult != nil {
					te.ToolResult = ev.ToolResult.Content
					te.ToolError = ev.ToolResult.IsError
				}
				trace = append(trace, te)
			case core.EventAgentDone:
				finalText = ev.Text
				trace = append(trace, TraceEvent{Time: now, Type: "done", Text: ev.Text})
			case core.EventAgentError:
				if ev.Error != nil {
					trace = append(trace, TraceEvent{Time: now, Type: "error", Error: ev.Error.Error()})
				}
			}
		}

		turnResults = append(turnResults, TurnResult{
			Message:  msg,
			Response: finalText,
			Turns:    msgTurns,
			Input:    msgIn,
			Output:   msgOut,
			Cost:     msgCost,
			Tools:    msgTools,
			Trace:    trace,
		})
		totalIn += msgIn
		totalOut += msgOut
		totalCost += msgCost
		totalTools += msgTools
		totalTurns += msgTurns
	}

	duration := time.Since(start)
	lastResp := ""
	if len(turnResults) > 0 {
		lastResp = turnResults[len(turnResults)-1].Response
	}

	result := MultiTurnResult{
		Messages:     turnResults,
		TotalInput:   totalIn,
		TotalOutput:  totalOut,
		TotalCost:    totalCost,
		TotalTurns:   totalTurns,
		TotalTools:   totalTools,
		DurationMs:   duration.Milliseconds(),
		LastResponse: lastResp,
	}

	resultPath := filepath.Join(outputDir, "result.json")
	f, err := os.Create(resultPath)
	if err != nil {
		return fmt.Errorf("batch: create result: %w", err)
	}
	enc := json.NewEncoder(f)
	enc.SetIndent("", "  ")
	if err := enc.Encode(result); err != nil {
		f.Close()
		return fmt.Errorf("batch: write result: %w", err)
	}
	f.Close()

	log.Printf("[batch] multi-turn done in %s — messages=%d turns=%d tools=%d tokens=%d/%d cost=$%.4f",
		duration.Round(time.Millisecond), len(messages), totalTurns, totalTools, totalIn, totalOut, totalCost)
	log.Printf("[batch] result: %s", resultPath)
	return nil
}
