package middleware

import (
	"fmt"
	"net/http"
	"runtime/debug"

	"github.com/logoes0/peeriodic.git/logger"
	"github.com/logoes0/peeriodic.git/utils"
)

// Recoverer catches panics, logs stack traces via structured logger, and returns RFC 7807 500
func Recoverer(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if rvr := recover(); rvr != nil {
				if rvr == http.ErrAbortHandler {
					// We should not recover http.ErrAbortHandler
					panic(rvr)
				}

				stack := string(debug.Stack())
				reqID := logger.GetRequestID(r.Context())

				logger.WithContext(r.Context()).Error(
					"Unhandled panic recovered",
					"error", fmt.Sprintf("%v", rvr),
					"request_id", reqID,
					"stack", stack,
					"path", r.URL.Path,
					"method", r.Method,
				)

				utils.InternalServerError(w, "An unexpected server error occurred")
			}
		}()

		next(w, r)
	}
}
