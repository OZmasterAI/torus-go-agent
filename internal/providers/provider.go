package providers

import (
	"context"
	t "go_sdk_agent/internal/types"
)

// Provider is the interface all LLM providers implement.
type Provider interface {
	Complete(ctx context.Context, systemPrompt string, messages []t.Message, tools []t.Tool, maxTokens int) (*t.AssistantMessage, error)
	StreamComplete(ctx context.Context, systemPrompt string, messages []t.Message, tools []t.Tool, maxTokens int) (<-chan t.StreamEvent, error)
	Name() string
	ModelID() string
}

// Router manages multiple providers and supports mid-session swaps.
type Router struct {
	providers map[string]Provider
	active    Provider
}

// NewRouter creates a provider router with an initial active provider.
func NewRouter(initial Provider) *Router {
	r := &Router{
		providers: make(map[string]Provider),
		active:    initial,
	}
	r.providers[initial.Name()+":"+initial.ModelID()] = initial
	return r
}

// AddProvider registers an additional provider.
func (r *Router) AddProvider(p Provider) {
	r.providers[p.Name()+":"+p.ModelID()] = p
}

// Switch changes the active provider. Context (messages) is preserved by the caller.
func (r *Router) Switch(name, model string) error {
	key := name + ":" + model
	if p, ok := r.providers[key]; ok {
		r.active = p
		return nil
	}
	return nil // could auto-create if credentials available
}

// Active returns the current provider.
func (r *Router) Active() Provider { return r.active }

// Complete delegates to the active provider.
func (r *Router) Complete(ctx context.Context, systemPrompt string, messages []t.Message, tools []t.Tool, maxTokens int) (*t.AssistantMessage, error) {
	return r.active.Complete(ctx, systemPrompt, messages, tools, maxTokens)
}

// StreamComplete delegates streaming to the active provider.
func (r *Router) StreamComplete(ctx context.Context, systemPrompt string, messages []t.Message, tools []t.Tool, maxTokens int) (<-chan t.StreamEvent, error) {
	return r.active.StreamComplete(ctx, systemPrompt, messages, tools, maxTokens)
}
