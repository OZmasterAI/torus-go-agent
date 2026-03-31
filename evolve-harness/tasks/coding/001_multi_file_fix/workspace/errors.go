package main

import "errors"

var (
	ErrNotFound      = errors.New("user not found")
	ErrNameRequired  = errors.New("name is required")
	ErrEmailRequired = errors.New("email is required")
	ErrDuplicateUser = errors.New("user with that email already exists")
)
