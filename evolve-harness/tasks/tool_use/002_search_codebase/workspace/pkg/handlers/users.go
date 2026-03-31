package handlers

import (
	"encoding/json"
	"net/http"
	"strconv"

	"example.com/app/pkg/models"
)

type UserHandler struct {
	store *models.UserStore
}

func NewUserHandler(store *models.UserStore) *UserHandler {
	return &UserHandler{store: store}
}

func (h *UserHandler) List(w http.ResponseWriter, r *http.Request) {
	users := h.store.All()
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(users)
}

func (h *UserHandler) GetByID(w http.ResponseWriter, r *http.Request) {
	idStr := r.URL.Query().Get("id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		http.Error(w, "invalid id parameter", http.StatusBadRequest)
		return
	}

	user, ok := h.store.Find(id)
	if !ok {
		http.Error(w, "user not found", http.StatusNotFound)
		return
	}

	// TODO: Add pagination support for large user lists in the List endpoint.
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(user)
}
