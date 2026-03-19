// Package http provides an HTTP/SSE channel for the agent.
// Any frontend (web UI, opencode TUI, curl) can connect via:
//   POST /api/chat    — send a message, receive streaming SSE response
//   GET  /api/status  — agent info (model, provider, branch)
//   POST /api/new     — create a new conversation branch
//   POST /api/clear   — clear context on current branch
package http

import (
	"context"
	"embed"
	"encoding/json"
	"fmt"
	"io/fs"
	"log"
	"net/http"
	"os"
	"sync"
	"time"

	"go_sdk_agent/internal/channels"
	"go_sdk_agent/internal/config"
	"go_sdk_agent/internal/core"
	"go_sdk_agent/internal/features"
)

//go:embed web/*
var webFS embed.FS

func init() { channels.Register(&httpChannel{}) }

type httpChannel struct{}

func (h *httpChannel) Name() string { return "http" }

func (h *httpChannel) Start(agent *core.Agent, cfg config.Config, _ *features.SkillRegistry) error {
	port := os.Getenv("TORUS_HTTP_PORT")
	if port == "" {
		port = "8080"
	}

	apiKey := os.Getenv("TORUSGO_API_KEY")
	srv := &server{agent: agent, cfg: cfg, apiKey: apiKey}

	mux := http.NewServeMux()
	mux.HandleFunc("/api/chat", srv.auth(srv.handleChat))
	mux.HandleFunc("/api/status", srv.auth(srv.handleStatus))
	mux.HandleFunc("/api/new", srv.auth(srv.handleNew))
	mux.HandleFunc("/api/clear", srv.auth(srv.handleClear))

	// Serve embedded web UI at root
	webRoot, _ := fs.Sub(webFS, "web")
	mux.Handle("/", http.FileServer(http.FS(webRoot)))

	addr := ":" + port
	if apiKey != "" {
		log.Printf("[http] listening on %s (auth enabled)", addr)
	} else {
		log.Printf("[http] listening on %s (no auth — set TORUSGO_API_KEY to secure)", addr)
	}
	log.Printf("[http] web UI → http://localhost%s", addr)
	return http.ListenAndServe(addr, corsHandler(mux))
}

type server struct {
	agent  *core.Agent
	cfg    config.Config
	apiKey string     // optional — if set, requires Authorization: Bearer <key>
	mu     sync.Mutex // serialize agent.Run calls
}

// corsHandler adds CORS headers and handles OPTIONS preflight.
func corsHandler(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// auth wraps a handler with bearer token validation (skipped if TORUSGO_API_KEY is unset).
func (s *server) auth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if s.apiKey != "" {
			token := r.Header.Get("Authorization")
			if token != "Bearer "+s.apiKey {
				http.Error(w, "unauthorized", http.StatusUnauthorized)
				return
			}
		}
		next(w, r)
	}
}

type chatRequest struct {
	Message string `json:"message"`
}

type statusResponse struct {
	Provider string `json:"provider"`
	Model    string `json:"model"`
	Branch   string `json:"branch"`
	Head     string `json:"head"`
	Messages int    `json:"messages"`
}

// handleChat accepts a JSON message and streams the response as SSE.
func (s *server) handleChat(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "POST only", http.StatusMethodNotAllowed)
		return
	}

	var req chatRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid JSON: "+err.Error(), http.StatusBadRequest)
		return
	}
	if req.Message == "" {
		http.Error(w, "message required", http.StatusBadRequest)
		return
	}

	// Non-streaming mode: ?stream=false returns plain JSON
	if r.URL.Query().Get("stream") == "false" {
		s.mu.Lock()
		response, err := s.agent.Run(r.Context(), req.Message)
		s.mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
			return
		}
		json.NewEncoder(w).Encode(map[string]string{"response": response})
		return
	}

	// Set SSE headers
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming not supported", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	// Serialize access to agent (single-threaded agent loop)
	s.mu.Lock()
	defer s.mu.Unlock()

	// Set up streaming delta callback
	s.agent.OnStreamDelta = func(delta string) {
		data, _ := json.Marshal(map[string]string{"type": "delta", "text": delta})
		fmt.Fprintf(w, "data: %s\n\n", data)
		flusher.Flush()
	}

	// Send start event
	fmt.Fprintf(w, "data: %s\n\n", `{"type":"start"}`)
	flusher.Flush()

	// Run agent
	response, err := s.agent.Run(r.Context(), req.Message)

	// Clear callback
	s.agent.OnStreamDelta = nil

	if err != nil {
		data, _ := json.Marshal(map[string]string{"type": "error", "error": err.Error()})
		fmt.Fprintf(w, "data: %s\n\n", data)
		flusher.Flush()
		return
	}

	// Send complete event with full response
	data, _ := json.Marshal(map[string]string{"type": "done", "text": response})
	fmt.Fprintf(w, "data: %s\n\n", data)
	flusher.Flush()
}

// handleStatus returns agent info.
func (s *server) handleStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "GET only", http.StatusMethodNotAllowed)
		return
	}

	brID, _, headNode, msgCount := s.agent.DAG().CurrentBranchInfo()
	resp := statusResponse{
		Provider: s.cfg.Agent.Provider,
		Model:    s.cfg.Agent.Model,
		Branch:   brID,
		Head:     headNode,
		Messages: msgCount,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

// handleNew creates a new conversation branch.
func (s *server) handleNew(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "POST only", http.StatusMethodNotAllowed)
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	oldBranch := s.agent.DAG().CurrentBranchID()
	s.agent.Hooks().Fire(context.Background(), core.HookBeforeNewBranch, &core.HookData{
		AgentID: "main",
		Meta:    map[string]any{"old_branch": oldBranch},
	})

	newBranch, _ := s.agent.DAG().NewBranch(fmt.Sprintf("session-%d", time.Now().Unix()))

	s.agent.Hooks().Fire(context.Background(), core.HookAfterNewBranch, &core.HookData{
		AgentID: "main",
		Meta:    map[string]any{"old_branch": oldBranch, "new_branch": newBranch},
	})

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"branch": newBranch})
}

// handleClear resets the head on the current branch.
func (s *server) handleClear(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "POST only", http.StatusMethodNotAllowed)
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	branchID := s.agent.DAG().CurrentBranchID()
	s.agent.Hooks().Fire(context.Background(), core.HookPreClear, &core.HookData{
		AgentID: "main",
		Meta:    map[string]any{"branch": branchID},
	})

	s.agent.DAG().ResetHead()

	s.agent.Hooks().Fire(context.Background(), core.HookPostClear, &core.HookData{
		AgentID: "main",
		Meta:    map[string]any{"branch": branchID},
	})

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "cleared"})
}
