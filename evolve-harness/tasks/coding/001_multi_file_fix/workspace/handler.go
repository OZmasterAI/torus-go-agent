package main

import (
	"encoding/json"
	"net/http"
	"strconv"
)

// Handler holds route handlers for the user API.
type Handler struct {
	repo *UserRepo
}

// NewHandler creates a handler with the given repository.
func NewHandler(repo *UserRepo) *Handler {
	return &Handler{repo: repo}
}

// HandleGetUser returns a single user by ID.
// BUG 1: calls repo.GetUser but repo.go defines FindUser
// BUG 2: accesses user.Username but models.go defines Name
func (h *Handler) HandleGetUser(w http.ResponseWriter, r *http.Request) {
	idStr := r.URL.Query().Get("id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, UserResponse{
			Success: false,
			Error:   "invalid user id",
		})
		return
	}

	user, err := h.repo.GetUser(id)
	if err != nil {
		writeJSON(w, http.StatusNotFound, UserResponse{
			Success: false,
			Error:   err.Error(),
		})
		return
	}

	writeJSON(w, http.StatusOK, UserResponse{
		Success: true,
		Data: &User{
			ID:        user.ID,
			Name:      user.Username,
			Email:     user.Email,
			Active:    user.Active,
			CreatedAt: user.CreatedAt,
		},
	})
}

// HandleListUsers returns all users.
func (h *Handler) HandleListUsers(w http.ResponseWriter, r *http.Request) {
	users := h.repo.ListUsers()
	writeJSON(w, http.StatusOK, ListResponse{
		Success: true,
		Data:    users,
		Count:   len(users),
	})
}

// HandleCreateUser creates a new user from the request body.
func (h *Handler) HandleCreateUser(w http.ResponseWriter, r *http.Request) {
	var req UserCreateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, UserResponse{
			Success: false,
			Error:   "invalid request body",
		})
		return
	}
	if err := req.Validate(); err != nil {
		writeJSON(w, http.StatusBadRequest, UserResponse{
			Success: false,
			Error:   err.Error(),
		})
		return
	}
	user, err := h.repo.CreateUser(req.Name, req.Email)
	if err != nil {
		writeJSON(w, http.StatusConflict, UserResponse{
			Success: false,
			Error:   err.Error(),
		})
		return
	}
	writeJSON(w, http.StatusCreated, UserResponse{
		Success: true,
		Data:    user,
	})
}

// HandleDeleteUser deletes a user by ID.
func (h *Handler) HandleDeleteUser(w http.ResponseWriter, r *http.Request) {
	idStr := r.URL.Query().Get("id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, UserResponse{
			Success: false,
			Error:   "invalid user id",
		})
		return
	}
	if err := h.repo.DeleteUser(id); err != nil {
		writeJSON(w, http.StatusNotFound, UserResponse{
			Success: false,
			Error:   err.Error(),
		})
		return
	}
	writeJSON(w, http.StatusOK, UserResponse{Success: true})
}

// writeJSON encodes v as JSON and writes it to w.
func writeJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}
