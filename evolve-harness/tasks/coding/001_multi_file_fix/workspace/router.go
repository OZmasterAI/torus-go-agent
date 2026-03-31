package main

import "net/http"

// SetupRoutes registers all API routes on the given mux.
// BUG 3: calls h.HandleHealth which does not exist on Handler
func SetupRoutes(mux *http.ServeMux, h *Handler) {
	mux.HandleFunc("/api/user", h.HandleGetUser)
	mux.HandleFunc("/api/users", h.HandleListUsers)
	mux.HandleFunc("/api/users/create", h.HandleCreateUser)
	mux.HandleFunc("/api/user/delete", h.HandleDeleteUser)
	mux.HandleFunc("/api/health", h.HandleHealth)
}
