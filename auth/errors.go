package auth

import "errors"

var (
	// ErrEmptySecret is returned when JWT operations are attempted without a configured secret.
	ErrEmptySecret = errors.New("auth: JWT secret is not set")
	// ErrEmptyPassword is returned when hashing an empty password.
	ErrEmptyPassword = errors.New("auth: password must not be empty")
	// ErrInvalidToken is returned when a token fails validation.
	ErrInvalidToken = errors.New("auth: invalid token")
	// ErrUnexpectedSigningMethod is returned when the token uses a non-HMAC signing method.
	ErrUnexpectedSigningMethod = errors.New("auth: unexpected signing method")
)
