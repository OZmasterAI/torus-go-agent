package http

import (
	"crypto/sha256"
	"crypto/subtle"
	"encoding/json"
	"fmt"
	"log"
	"net"
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
	// Bind to loopback by default so the agent (which can execute tools) is
	// not exposed to the network. Set TORUS_HTTP_BIND (e.g. "0.0.0.0:8080")
	// to deliberately bind a wider address.
	bind := "127.0.0.1:" + port
	if v := os.Getenv("TORUS_HTTP_BIND"); v != "" {
		bind = v
	}

	// Fail closed on auth: require TORUSGO_API_KEY. Running without a key is
	// only permitted when the bind address is loopback AND the operator has
	// explicitly opted out via TORUS_HTTP_ALLOW_NOAUTH=1.
	apiKey := os.Getenv("TORUSGO_API_KEY")
	if apiKey == "" {
		if os.Getenv("TORUS_HTTP_ALLOW_NOAUTH") != "1" || !isLoopbackBind(bind) {
			return fmt.Errorf("[http] refusing to start: TORUSGO_API_KEY is not set; set it, or bind loopback and set TORUS_HTTP_ALLOW_NOAUTH=1 to explicitly run without authentication")
		}
		log.Println("[http] WARNING: TORUS_HTTP_ALLOW_NOAUTH=1 — accepting unauthenticated requests on loopback")
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/api/health", handleHealth)
	mux.HandleFunc("/api/chat", authMiddleware(apiKey, handleChat(agent)))

	log.Printf("[http] listening on %s", bind)
	return http.ListenAndServe(bind, mux)
}

// isLoopbackBind reports whether the bind address resolves to a loopback
// host ("localhost" or a loopback IP such as 127.0.0.1 / ::1).
func isLoopbackBind(bind string) bool {
	host, _, err := net.SplitHostPort(bind)
	if err != nil {
		host = bind
	}
	if host == "localhost" {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

// authMiddleware checks the Authorization header against the API key using a
// constant-time comparison. An empty apiKey (only reachable via the explicit
// TORUS_HTTP_ALLOW_NOAUTH=1 loopback opt-out in Start) allows all requests.
func authMiddleware(apiKey string, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if apiKey != "" {
			got := sha256.Sum256([]byte(r.Header.Get("Authorization")))
			want := sha256.Sum256([]byte("Bearer " + apiKey))
			if subtle.ConstantTimeCompare(got[:], want[:]) != 1 {
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
		r.Body = http.MaxBytesReader(w, r.Body, 1<<20) // 1 MB
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
	text, err := agent.Run(r.Context(), message)
	w.Header().Set("Content-Type", "application/json")
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(chatResponse{Error: err.Error()})
		return
	}
	json.NewEncoder(w).Encode(chatResponse{Text: text})
}
