package main

import "time"

// User represents a registered user in the system.
type User struct {
	ID        int       `json:"id"`
	Name      string    `json:"name"`
	Email     string    `json:"email"`
	Active    bool      `json:"active"`
	CreatedAt time.Time `json:"created_at"`
}

// UserResponse is the API response wrapper for a single user.
type UserResponse struct {
	Success bool   `json:"success"`
	Data    *User  `json:"data,omitempty"`
	Error   string `json:"error,omitempty"`
}

// ListResponse is the API response wrapper for a list of users.
type ListResponse struct {
	Success bool    `json:"success"`
	Data    []*User `json:"data,omitempty"`
	Count   int     `json:"count"`
}

// UserCreateRequest is the payload for creating a new user.
type UserCreateRequest struct {
	Name  string `json:"name"`
	Email string `json:"email"`
}

// Validate checks that the create request has required fields.
func (r *UserCreateRequest) Validate() error {
	if r.Name == "" {
		return ErrNameRequired
	}
	if r.Email == "" {
		return ErrEmailRequired
	}
	return nil
}
