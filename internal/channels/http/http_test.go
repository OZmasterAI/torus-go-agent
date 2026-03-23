package http

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestHandleHealth(t *testing.T) {
	req := httptest.NewRequest("GET", "/api/health", nil)
	w := httptest.NewRecorder()
	handleHealth(w, req)

	if w.Code != 200 {
		t.Errorf("status: got %d, want 200", w.Code)
	}
	if !strings.Contains(w.Body.String(), `"ok"`) {
		t.Errorf("body: got %q, want ok", w.Body.String())
	}
}

func TestAuthMiddleware_NoKey(t *testing.T) {
	called := false
	handler := authMiddleware("", func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(200)
	})

	req := httptest.NewRequest("GET", "/", nil)
	w := httptest.NewRecorder()
	handler(w, req)

	if !called {
		t.Error("handler should be called when no API key is configured")
	}
}

func TestAuthMiddleware_ValidKey(t *testing.T) {
	called := false
	handler := authMiddleware("secret123", func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(200)
	})

	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("Authorization", "Bearer secret123")
	w := httptest.NewRecorder()
	handler(w, req)

	if !called {
		t.Error("handler should be called with valid key")
	}
}

func TestAuthMiddleware_InvalidKey(t *testing.T) {
	called := false
	handler := authMiddleware("secret123", func(w http.ResponseWriter, r *http.Request) {
		called = true
	})

	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("Authorization", "Bearer wrongkey")
	w := httptest.NewRecorder()
	handler(w, req)

	if called {
		t.Error("handler should not be called with invalid key")
	}
	if w.Code != http.StatusUnauthorized {
		t.Errorf("status: got %d, want 401", w.Code)
	}
}

func TestAuthMiddleware_MissingHeader(t *testing.T) {
	handler := authMiddleware("secret123", func(w http.ResponseWriter, r *http.Request) {})

	req := httptest.NewRequest("GET", "/", nil)
	w := httptest.NewRecorder()
	handler(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("status: got %d, want 401", w.Code)
	}
}

func TestHandleChat_MethodNotAllowed(t *testing.T) {
	handler := handleChat(nil)

	req := httptest.NewRequest("GET", "/api/chat", nil)
	w := httptest.NewRecorder()
	handler(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("status: got %d, want 405", w.Code)
	}
}

func TestHandleChat_InvalidJSON(t *testing.T) {
	handler := handleChat(nil)

	req := httptest.NewRequest("POST", "/api/chat", strings.NewReader("not json"))
	w := httptest.NewRecorder()
	handler(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status: got %d, want 400", w.Code)
	}
}

func TestHandleChat_EmptyMessage(t *testing.T) {
	handler := handleChat(nil)

	req := httptest.NewRequest("POST", "/api/chat", strings.NewReader(`{"message":""}`))
	w := httptest.NewRecorder()
	handler(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status: got %d, want 400", w.Code)
	}
}

func TestHttpChannel_Name(t *testing.T) {
	ch := &httpChannel{}
	if ch.Name() != "http" {
		t.Errorf("Name: got %q, want %q", ch.Name(), "http")
	}
}
