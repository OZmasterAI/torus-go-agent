package providers

import (
	"context"
	"math"
	"strconv"
	"strings"
	"sync"
	"time"

	t "torus_go_agent/internal/types"
)

// RewardRouter wraps a Router and asynchronously scores responses using
// a reward model to learn per-model quality weights over time.
// Enable with NewRewardRouter; disable by using the Router directly.
type RewardRouter struct {
	router      *Router
	rewardModel *OpenRouterProvider // the reward model provider (nvidia NIM endpoint)
	scores      map[string]*modelStats
	mu          sync.RWMutex
	totalScored int
	updateEvery int // recalculate weights every N scored responses (default 10)
	ctx         context.Context
	cancel      context.CancelFunc
}

type modelStats struct {
	totalScore float64
	count      int
}

// NewRewardRouter wraps an existing Router with async reward-model scoring.
// The reward model runs in the background after each response -- no latency impact.
// After every updateEvery scored responses, router weights are adjusted so
// higher-scoring models receive more traffic.
func NewRewardRouter(router *Router, apiKey string) *RewardRouter {
	reward := NewNvidiaProvider(apiKey, "nvidia/llama-3.1-nemotron-70b-reward")
	scores := make(map[string]*modelStats)
	ctx, cancel := context.WithCancel(context.Background())
	return &RewardRouter{
		router:      router,
		rewardModel: reward,
		scores:      scores,
		updateEvery: 10,
		ctx:         ctx,
		cancel:      cancel,
	}
}

// Name returns the underlying router's name (satisfies types.Provider).
func (rr *RewardRouter) Name() string { return rr.router.Name() }

// ModelID returns the underlying router's model ID (satisfies types.Provider).
func (rr *RewardRouter) ModelID() string { return rr.router.ModelID() }

// Close cancels the internal context, which cascade-cancels all in-flight
// scoring goroutines. Safe to call multiple times.
func (rr *RewardRouter) Close() { rr.cancel() }

// Complete delegates to the underlying router, then asynchronously scores the
// response using the reward model. The response is returned immediately.
func (rr *RewardRouter) Complete(ctx context.Context, systemPrompt string, messages []t.Message, tools []t.Tool, maxTokens int) (*t.AssistantMessage, error) {
	resp, err := rr.router.Complete(ctx, systemPrompt, messages, tools, maxTokens)
	if err != nil {
		return nil, err
	}

	// Extract the user prompt for scoring (last user message)
	userPrompt := lastUserMessage(messages)
	// Extract the assistant response text
	assistantText := extractText(resp)
	// Get which model actually served this
	modelKey := resp.Model

	// Async score -- fire and forget
	if userPrompt != "" && assistantText != "" {
		go rr.scoreAndUpdate(userPrompt, assistantText, modelKey)
	}

	return resp, nil
}

// StreamComplete delegates streaming to the underlying router, wrapping the
// channel to intercept the final message for async reward scoring.
func (rr *RewardRouter) StreamComplete(ctx context.Context, systemPrompt string, messages []t.Message, tools []t.Tool, maxTokens int) (<-chan t.StreamEvent, error) {
	ch, err := rr.router.StreamComplete(ctx, systemPrompt, messages, tools, maxTokens)
	if err != nil {
		return nil, err
	}

	// Wrap the channel to intercept the final message for scoring
	userPrompt := lastUserMessage(messages)
	wrapped := make(chan t.StreamEvent, 32)
	go func() {
		defer close(wrapped)
		for ev := range ch {
			wrapped <- ev
			// When we see the final assembled message, score it async
			if ev.Type == t.EventMessageStop && ev.Response != nil && userPrompt != "" {
				assistantText := extractText(ev.Response)
				modelKey := ev.Response.Model
				if assistantText != "" {
					go rr.scoreAndUpdate(userPrompt, assistantText, modelKey)
				}
			}
		}
	}()

	return wrapped, nil
}

// scoreAndUpdate calls the reward model to score a response, then updates
// the per-model statistics. After every updateEvery scores, router weights
// are recalculated so higher-scoring models receive more traffic.
func (rr *RewardRouter) scoreAndUpdate(userPrompt, assistantText, modelKey string) {
	if rr.rewardModel == nil {
		return
	}
	// Build the scoring request -- just user + assistant messages
	ctx, cancel := context.WithTimeout(rr.ctx, 10*time.Second)
	defer cancel()

	msgs := []t.Message{
		{Role: t.RoleUser, Content: []t.ContentBlock{{Type: "text", Text: userPrompt}}},
		{Role: t.RoleAssistant, Content: []t.ContentBlock{{Type: "text", Text: assistantText}}},
	}

	// Use the reward model's Complete (not streaming)
	resp, err := rr.rewardModel.Complete(ctx, "", msgs, nil, 16)
	if err != nil {
		return // silently skip on error -- reward scoring is best-effort
	}

	// Parse the score from the response content
	scoreText := extractText(resp)
	score, err := strconv.ParseFloat(strings.TrimSpace(scoreText), 64)
	if err != nil {
		return
	}

	// Update stats
	rr.mu.Lock()
	stats, ok := rr.scores[modelKey]
	if !ok {
		stats = &modelStats{}
		rr.scores[modelKey] = stats
	}
	stats.totalScore += score
	stats.count++
	rr.totalScored++
	shouldUpdate := rr.totalScored > 0 && rr.totalScored%rr.updateEvery == 0
	rr.mu.Unlock()

	// Periodically recalculate router weights
	if shouldUpdate {
		rr.recalculateWeights()
	}
}

// recalculateWeights computes new router weights from accumulated reward scores.
// Models with higher average scores receive proportionally more traffic.
// Scores are shifted to be positive (subtract minimum, add 1) and scaled
// to integer weights for the router.
func (rr *RewardRouter) recalculateWeights() {
	rr.mu.RLock()
	defer rr.mu.RUnlock()

	if len(rr.scores) < 2 {
		return // need at least 2 models scored to compare
	}

	// Calculate average score per model
	type modelAvg struct {
		key string
		avg float64
	}
	var avgs []modelAvg
	minAvg := math.MaxFloat64
	for key, stats := range rr.scores {
		if stats.count == 0 {
			continue
		}
		avg := stats.totalScore / float64(stats.count)
		avgs = append(avgs, modelAvg{key: key, avg: avg})
		if avg < minAvg {
			minAvg = avg
		}
	}

	if len(avgs) < 2 {
		return
	}

	// Shift scores to be positive (subtract min, add 1 to avoid zero weights)
	// Then convert to integer weights (multiply by 100 for granularity).
	// Router keys use "nvidia:<modelID>" format (see NewNvidiaFreeRouter).
	var entries []RoutingEntry
	for _, ma := range avgs {
		shifted := ma.avg - minAvg + 1.0 // guarantee positive
		weight := int(shifted * 100)
		if weight < 1 {
			weight = 1
		}
		entries = append(entries, RoutingEntry{Key: "nvidia:" + ma.key, Weight: weight})
	}

	rr.router.SetWeights(entries)
}

// lastUserMessage extracts the text of the last user message.
func lastUserMessage(messages []t.Message) string {
	for i := len(messages) - 1; i >= 0; i-- {
		if messages[i].Role == t.RoleUser {
			for _, b := range messages[i].Content {
				if b.Type == "text" && b.Text != "" {
					return b.Text
				}
			}
		}
	}
	return ""
}

// extractText pulls the first text block from an AssistantMessage.
func extractText(msg *t.AssistantMessage) string {
	for _, b := range msg.Content {
		if b.Type == "text" && b.Text != "" {
			return b.Text
		}
	}
	return ""
}
