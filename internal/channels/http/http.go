package http

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"

	"torus_go_agent/internal/channels"
	"torus_go_agent/internal/config"
	"torus_go_agent/internal/core"
	"torus_go_agent/internal/features"
)

func init() { channels.Register(&httpChannel{}) }

type httpChannel struct{}

func (h *httpChannel) Name() string { return "http" }

func (h *httpChannel) Start(agent *core.Agent, cfg config.Config, _ *features.SkillRegistry) error {
	port := os.Getenv("TORUS_HTTP_PORT")
	if port == "" {
		port = "8080"
	}
	apiKey := os.Getenv("TORUSGO_API_KEY")

	mux := http.NewServeMux()
	mux.HandleFunc("/api/health", handleHealth)
	mux.HandleFunc("/api/chat", authMiddleware(apiKey, handleChat(agent)))

	log.Printf("[http] listening on :%s", port)
	return http.ListenAndServe(":"+port, mux)
}

// authMiddleware checks the Authorization header against the API key.
// If no key is configured, all requests are allowed.
func authMiddleware(apiKey string, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if apiKey != "" {
			got := r.Header.Get("Authorization")
			if got != "Bearer "+apiKey {
				http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
				return
			}
		}
		next(w, r)
	}
}

func handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Write([]byte(`{"status":"ok"}`))
}

type chatRequest struct {
	Message string `json:"message"`
	Stream  *bool  `json:"stream,omitempty"` // default true
}

type chatResponse struct {
	Text  string `json:"text,omitempty"`
	Error string `json:"error,omitempty"`
}

func handleChat(agent *core.Agent) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
			return
		}

		var req chatRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, `{"error":"invalid JSON"}`, http.StatusBadRequest)
			return
		}
		if req.Message == "" {
			http.Error(w, `{"error":"message required"}`, http.StatusBadRequest)
			return
		}

		stream := req.Stream == nil || *req.Stream

		if stream {
			handleStreamChat(w, r, agent, req.Message)
		} else {
			handleBlockingChat(w, r, agent, req.Message)
		}
	}
}

func handleStreamChat(w http.ResponseWriter, r *http.Request, agent *core.Agent, message string) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, `{"error":"streaming not supported"}`, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	ctx := r.Context()
	for ev := range agent.RunStream(ctx, message) {
		data := map[string]any{"type": string(ev.Type)}
		switch ev.Type {
		case core.EventAgentTextDelta:
			data["text"] = ev.Text
		case core.EventAgentThinkingDelta:
			data["text"] = ev.Text
		case core.EventAgentToolStart:
			data["tool"] = ev.ToolName
			data["args"] = ev.ToolArgs
		case core.EventAgentToolEnd:
			data["tool"] = ev.ToolName
			if ev.ToolResult != nil {
				data["result"] = ev.ToolResult.Content
			}
		case core.EventAgentTurnStart:
			data["turn"] = ev.Turn
		case core.EventAgentTurnEnd:
			data["turn"] = ev.Turn
			if ev.Usage != nil {
				data["usage"] = ev.Usage
			}
		case core.EventAgentDone:
			data["text"] = ev.Text
		case core.EventAgentError:
			data["error"] = ev.Error.Error()
		}

		jsonData, err := json.Marshal(data)
		if err != nil {
			continue
		}
		fmt.Fprintf(w, "data: %s\n\n", jsonData)
		flusher.Flush()
	}
}

func handleBlockingChat(w http.ResponseWriter, r *http.Request, agent *core.Agent, message string) {
	text, err := agent.Run(context.Background(), message)
	w.Header().Set("Content-Type", "application/json")
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(chatResponse{Error: err.Error()})
		return
	}
	json.NewEncoder(w).Encode(chatResponse{Text: text})
}
