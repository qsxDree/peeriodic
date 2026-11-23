package middleware

import (
	"net/http"
)

// DefaultMaxBodyBytes is 1MB
const DefaultMaxBodyBytes int64 = 1024 * 1024

// MaxBodyBytes limits the maximum allowable request body size in bytes
func MaxBodyBytes(limit int64) func(http.HandlerFunc) http.HandlerFunc {
	if limit <= 0 {
		limit = DefaultMaxBodyBytes
	}
	return func(next http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			r.Body = http.MaxBytesReader(w, r.Body, limit)
			next(w, r)
		}
	}
}
