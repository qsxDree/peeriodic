package utils

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
)

// Response represents a standard API response (backwards compatible)
type Response struct {
	Success   bool        `json:"success"`
	Message   string      `json:"message,omitempty"`
	Data      interface{} `json:"data,omitempty"`
	Error     string      `json:"error,omitempty"`
	RequestID string      `json:"request_id,omitempty"`
}

// JSONResponse sends a JSON response with the given status code
func JSONResponse(w http.ResponseWriter, statusCode int, data interface{}) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(statusCode)

	if data != nil {
		json.NewEncoder(w).Encode(data)
	}
}

// SuccessResponse sends a successful JSON response
func SuccessResponse(w http.ResponseWriter, data interface{}) {
	response := Response{
		Success: true,
		Data:    data,
	}
	JSONResponse(w, http.StatusOK, response)
}

// ErrorResponse sends an error JSON response with optional request_id
func ErrorResponse(w http.ResponseWriter, statusCode int, message string) {
	response := Response{
		Success: false,
		Error:   message,
	}
	JSONResponse(w, statusCode, response)
}

// BadRequest sends a 400 Bad Request response
func BadRequest(w http.ResponseWriter, message string) {
	ErrorResponse(w, http.StatusBadRequest, message)
}

// NotFound sends a 404 Not Found response
func NotFound(w http.ResponseWriter, message string) {
	ErrorResponse(w, http.StatusNotFound, message)
}

// InternalServerError sends a 500 Internal Server Error response
func InternalServerError(w http.ResponseWriter, message string) {
	if message == "" {
		message = "Internal server error"
	}
	ErrorResponse(w, http.StatusInternalServerError, message)
}

// MethodNotAllowed sends a 405 Method Not Allowed response
func MethodNotAllowed(w http.ResponseWriter) {
	ErrorResponse(w, http.StatusMethodNotAllowed, "Method not allowed")
}

// Unauthorized sends a 401 Unauthorized response
func Unauthorized(w http.ResponseWriter, message string) {
	if message == "" {
		message = "Unauthorized"
	}
	ErrorResponse(w, http.StatusUnauthorized, message)
}

// Forbidden sends a 403 Forbidden response
func Forbidden(w http.ResponseWriter, message string) {
	if message == "" {
		message = "Forbidden"
	}
	ErrorResponse(w, http.StatusForbidden, message)
}

// TooManyRequests sends a 429 Too Many Requests response
func TooManyRequests(w http.ResponseWriter, retryAfterSeconds int) {
	if retryAfterSeconds > 0 {
		w.Header().Set("Retry-After", fmt.Sprintf("%d", retryAfterSeconds))
	}
	ErrorResponse(w, http.StatusTooManyRequests, "Too many requests. Please slow down and try again later.")
}

// DecodeJSON validates a single JSON object and writes a client error on failure.
func DecodeJSON(w http.ResponseWriter, r *http.Request, dst any) error {
	if ct := r.Header.Get("Content-Type"); ct != "" {
		mediaType, _, err := mime.ParseMediaType(ct)
		if err != nil || mediaType != "application/json" {
			err := errors.New("Content-Type must be application/json")
			ErrorResponse(w, http.StatusUnsupportedMediaType, err.Error())
			return err
		}
	}
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1024*1024))
	var raw json.RawMessage
	err := dec.Decode(&raw)
	if err == nil {
		var extra any
		if trailingErr := dec.Decode(&extra); trailingErr != io.EOF {
			if trailingErr != nil {
				err = trailingErr
			} else {
				err = errors.New("request body must contain one JSON object")
			}
		}
	}
	if err == nil && (len(raw) == 0 || raw[0] != '{') {
		err = errors.New("request body must be a JSON object")
	}
	if err == nil {
		payload := json.NewDecoder(bytes.NewReader(raw))
		payload.DisallowUnknownFields()
		err = payload.Decode(dst)
	}
	if err != nil {
		status := http.StatusBadRequest
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			status = http.StatusRequestEntityTooLarge
		}
		ErrorResponse(w, status, "Invalid request body: "+err.Error())
	}
	return err
}
