package storage

import "errors"

var (
	ErrNotFound   = errors.New("user not found")
	ErrUserExists = errors.New("user already exists")
)
