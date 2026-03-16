package errors

import (
	"errors"
	"fmt"
)

// Err is a typed error from Yango API or transport.
type Err struct {
	StatusCode int    // HTTP status (0 if transport error)
	Code       string // optional API error code
	Message    string
	RawBody    []byte
	Op         string // operation e.g. "GET /catalog"
}

func (e *Err) Error() string {
	if e.Op != "" {
		return fmt.Sprintf("%s: %s (status=%d)", e.Op, e.Message, e.StatusCode)
	}
	return fmt.Sprintf("%s (status=%d)", e.Message, e.StatusCode)
}

// IsRetryable returns true for network errors, 5xx, or 429.
func (e *Err) IsRetryable() bool {
	if e.StatusCode == 0 {
		return true // transport error
	}
	if e.StatusCode >= 500 {
		return true
	}
	if e.StatusCode == 429 {
		return true
	}
	return false
}

// Unwrap allows errors.Is/As to work. Err does not wrap another error but implements the interface.
func (e *Err) Unwrap() error { return nil }

// IsNotFound returns true if err is a 404.
func IsNotFound(err error) bool {
	var e *Err
	if errors.As(err, &e) {
		return e.StatusCode == 404
	}
	return false
}

// IsUnauthorized returns true if err is 401.
func IsUnauthorized(err error) bool {
	var e *Err
	if errors.As(err, &e) {
		return e.StatusCode == 401
	}
	return false
}

// IsRetryableError returns true if the error is retryable (transport or 5xx/429).
func IsRetryableError(err error) bool {
	var e *Err
	if errors.As(err, &e) {
		return e.IsRetryable()
	}
	return false
}

// AsErr returns the *Err if err is or wraps Err.
func AsErr(err error) (*Err, bool) {
	var e *Err
	ok := errors.As(err, &e)
	return e, ok
}
