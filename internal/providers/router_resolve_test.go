package providers

import (
	"context"
	"testing"
)

// TestResolveRouter verifies the agent's primary provider and the *Router that
// cfg routing/fallback should target are resolved without double-wrapping when
// the provider is already a *Router (nvidia/free) or a *RewardRouter. (audit #4)
func TestResolveRouter(t *testing.T) {
	t.Parallel()

	// Plain provider → wrapped once; both returns are that wrapper.
	plain := &mockProvider{name: "anthropic", modelID: "claude-x"}
	ap, rt := ResolveRouter(plain)
	if rt == nil {
		t.Fatal("ResolveRouter(plain): nil router")
	}
	if ap != rt {
		t.Error("ResolveRouter(plain): agentProv should be the wrapper router")
	}
	if _, ok := ap.(*Router); !ok {
		t.Error("ResolveRouter(plain): agentProv should be *Router")
	}

	// Already a *Router → reused, NOT double-wrapped.
	inner := NewRouter(&mockProvider{name: "nvidia", modelID: "m1"})
	ap2, rt2 := ResolveRouter(inner)
	if rt2 != inner || ap2 != inner {
		t.Error("ResolveRouter(*Router): inner router must be reused (no double-wrap)")
	}

	// *RewardRouter → agentProv stays the reward router (scoring preserved),
	// but cfg routing targets its inner *Router.
	rr := NewRewardRouter(inner, "key")
	ap3, rt3 := ResolveRouter(rr)
	if ap3 != rr {
		t.Error("ResolveRouter(*RewardRouter): agentProv must be the reward router (keeps scoring)")
	}
	if rt3 != inner {
		t.Error("ResolveRouter(*RewardRouter): router must be the inner *Router for cfg routing")
	}
}

// TestRouterCompleteWithProvider verifies the router reports the registration key
// of the provider that actually served the response, so callers can attribute
// scores to the exact registered provider rather than the API-echoed model. (audit #3)
func TestRouterCompleteWithProvider(t *testing.T) {
	t.Parallel()
	p := &mockProvider{name: "nvidia", modelID: "modelX"}
	r := NewRouter(p)
	resp, key, err := r.CompleteWithProvider(context.Background(), "", nil, nil, 16)
	if err != nil {
		t.Fatalf("CompleteWithProvider: %v", err)
	}
	if resp == nil {
		t.Fatal("CompleteWithProvider: nil response")
	}
	if key != "nvidia:modelX" {
		t.Errorf("served key = %q, want nvidia:modelX", key)
	}
}

// TestRewardRouterRecalcUsesRegistrationKeys verifies recalculated weights use the
// router's registration keys (so pick() can find them) instead of a re-prefixed
// echoed-model string that would be silently dropped. (audit #3)
func TestRewardRouterRecalcUsesRegistrationKeys(t *testing.T) {
	t.Parallel()
	pa := &mockProvider{name: "nvidia", modelID: "modelA"}
	pb := &mockProvider{name: "nvidia", modelID: "modelB"}
	r := NewRouter(pa)
	r.AddProvider(pb)
	rr := NewRewardRouter(r, "key")

	// Simulate accumulated scores keyed by REGISTRATION keys (what the fixed
	// Complete path now records).
	rr.scores["nvidia:modelA"] = &modelStats{totalScore: 9, count: 1}
	rr.scores["nvidia:modelB"] = &modelStats{totalScore: 1, count: 1}
	rr.recalculateWeights()

	if !r.IsWeighted() {
		t.Fatal("expected weighted routing after recalculate")
	}
	foundA, foundB := false, false
	for _, e := range r.weights {
		if _, ok := r.providers[e.Key]; !ok {
			t.Errorf("recalc produced unregistered key %q (would be silently dropped)", e.Key)
		}
		switch e.Key {
		case "nvidia:modelA":
			foundA = true
		case "nvidia:modelB":
			foundB = true
		}
	}
	if !foundA || !foundB {
		t.Errorf("expected both registration keys in weights, got %+v", r.weights)
	}
}
