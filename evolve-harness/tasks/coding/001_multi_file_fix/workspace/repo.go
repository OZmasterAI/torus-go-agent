package main

import (
	"sync"
	"time"
)

// UserRepo is an in-memory user repository.
type UserRepo struct {
	mu      sync.RWMutex
	users   map[int]*User
	nextID  int
}

// NewUserRepo creates a fresh repository with sample data.
func NewUserRepo() *UserRepo {
	repo := &UserRepo{
		users:  make(map[int]*User),
		nextID: 1,
	}
	// Seed with sample data.
	repo.CreateUser("Alice", "alice@example.com")
	repo.CreateUser("Bob", "bob@example.com")
	return repo
}

// FindUser looks up a user by ID.
func (r *UserRepo) FindUser(id int) (*User, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	u, ok := r.users[id]
	if !ok {
		return nil, ErrNotFound
	}
	return u, nil
}

// ListUsers returns all users in the repo.
func (r *UserRepo) ListUsers() []*User {
	r.mu.RLock()
	defer r.mu.RUnlock()
	result := make([]*User, 0, len(r.users))
	for _, u := range r.users {
		result = append(result, u)
	}
	return result
}

// CreateUser adds a new user and returns it.
func (r *UserRepo) CreateUser(name, email string) (*User, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	// Check for duplicate email.
	for _, u := range r.users {
		if u.Email == email {
			return nil, ErrDuplicateUser
		}
	}

	u := &User{
		ID:        r.nextID,
		Name:      name,
		Email:     email,
		Active:    true,
		CreatedAt: time.Now(),
	}
	r.users[r.nextID] = u
	r.nextID++
	return u, nil
}

// DeleteUser removes a user by ID.
func (r *UserRepo) DeleteUser(id int) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.users[id]; !ok {
		return ErrNotFound
	}
	delete(r.users, id)
	return nil
}
