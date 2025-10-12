package auth

import "errors"

var (
	ErrNotFound       = errors.New("auth: not found")
	ErrAlreadyExists  = errors.New("auth: already exists")
	ErrInvalidInput   = errors.New("auth: invalid input")
	ErrUnauthorized   = errors.New("auth: unauthorized")
	ErrNotImplemented = errors.New("auth: not implemented")
)
