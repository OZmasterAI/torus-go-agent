package providers

import (
	"context"
	"fmt"
	"math/rand/v2"
	"sync"
	t "torus_go_agent/internal/types"
)

// RoutingEntry pairs a provider key with a weight for weighted routing.
type RoutingEntry struct {
	Key    string // "provider:model"
	Weight int    // relative weight (e.g. 80, 20)
}

// Router manages multiple providers and supports manual switching,
// weighted routing, and fallback chains.
type Router struct {
	mu            sync.RWMutex
	providers     map[string]t.Provider
	active        t.Provider
	weights       []RoutingEntry // if set, weighted mode is active
	totalWeight   int
	fallbackOrder []string // provider keys in fallback order
}

// NewRouter creates a provider router with an initial active provider.
func NewRouter(initial t.Provider) *Router {
	r := &Router{
		providers: make(map[string]t.Provider),
		active:    initial,
	}
	r.providers[initial.Name()+":"+initial.ModelID()] = initial
	return r
}

// AddProvider registers an additional provider.
func (r *Router) AddProvider(p t.Provider) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.providers[p.Name()+":"+p.ModelID()] = p
}

// Switch changes the active provider. Context (messages) is preserved by the caller.
func (r *Router) Switch(name, model string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	key := name + ":" + model
	if p, ok := r.providers[key]; ok {
		r.active = p
		return nil
	}
	return fmt.Errorf("provider %q not registered", key)
}

// SetWeights enables weighted routing. Entries reference registered provider keys.
// Pass nil or empty to disable weighted routing and return to manual mode.
func (r *Router) SetWeights(entries []RoutingEntry) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.weights = entries
	r.totalWeight = 0
	for _, e := range entries {
		r.totalWeight += e.Weight
	}
}

// SetFallbackOrder sets the fallback chain for when a provider errors.
// Keys are "provider:model" strings. Pass nil to disable fallback.
func (r *Router) SetFallbackOrder(keys []string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.fallbackOrder = keys
}

// Name returns the active provider's name (satisfies types.Provider).
func (r *Router) Name() string { return r.Active().Name() }

// ModelID returns the active provider's model ID (satisfies types.Provider).
func (r *Router) ModelID() string { return r.Active().ModelID() }

// Active returns the current provider.
func (r *Router) Active() t.Provider {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.active
}

// IsWeighted returns true if weighted routing is active.
func (r *Router) IsWeighted() bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.weights) > 0 && r.totalWeight > 0
}

// pick selects a provider by weighted random, or returns the active provider.
func (r *Router) pick() t.Provider {
	if len(r.weights) == 0 || r.totalWeight == 0 {
		return r.active
	}
	n := rand.IntN(r.totalWeight)
	for _, e := range r.weights {
		n -= e.Weight
		if n < 0 {
			if p, ok := r.providers[e.Key]; ok {
				return p
			}
			break
		}
	}
	return r.active
}

// fallback returns the ordered list of providers to try after a failure.
func (r *Router) fallback(exclude string) []t.Provider {
	var result []t.Provider
	for _, key := range r.fallbackOrder {
		if key == exclude {
			continue
		}
		if p, ok := r.providers[key]; ok {
			result = append(result, p)
		}
	}
	return result
}

// Complete delegates to the active or weighted-selected provider, with fallback on error.
func (r *Router) Complete(ctx context.Context, systemPrompt string, messages []t.Message, tools []t.Tool, maxTokens int) (*t.AssistantMessage, error) {
	r.mu.RLock()
	primary := r.pick()
	r.mu.RUnlock()

	resp, err := primary.Complete(ctx, systemPrompt, messages, tools, maxTokens)
	if err == nil {
		return resp, nil
	}

	// Try fallback chain
	r.mu.RLock()
	fallbacks := r.fallback(primary.Name() + ":" + primary.ModelID())
	r.mu.RUnlock()

	for _, fb := range fallbacks {
		resp, fbErr := fb.Complete(ctx, systemPrompt, messages, tools, maxTokens)
		if fbErr == nil {
			return resp, nil
		}
	}
	return nil, err // return original error if all fallbacks fail
}

// StreamComplete delegates streaming to the active or weighted-selected provider, with fallback on error.
func (r *Router) StreamComplete(ctx context.Context, systemPrompt string, messages []t.Message, tools []t.Tool, maxTokens int) (<-chan t.StreamEvent, error) {
	r.mu.RLock()
	primary := r.pick()
	r.mu.RUnlock()

	ch, err := primary.StreamComplete(ctx, systemPrompt, messages, tools, maxTokens)
	if err == nil {
		return ch, nil
	}

	// Try fallback chain
	r.mu.RLock()
	fallbacks := r.fallback(primary.Name() + ":" + primary.ModelID())
	r.mu.RUnlock()

	for _, fb := range fallbacks {
		ch, fbErr := fb.StreamComplete(ctx, systemPrompt, messages, tools, maxTokens)
		if fbErr == nil {
			return ch, nil
		}
	}
	return nil, err
}
