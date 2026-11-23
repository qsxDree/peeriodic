package middleware

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestValidationMiddleware(t *testing.T) {
	testHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		buf := new(bytes.Buffer)
		_, err := buf.ReadFrom(r.Body)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		w.WriteHeader(http.StatusOK)
	})

	t.Run("MaxBodyBytes - Under limit passes", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/test", bytes.NewReader([]byte("small payload")))
		rec := httptest.NewRecorder()
		MaxBodyBytes(100)(testHandler).ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Errorf("expected 200 OK, got %d", rec.Code)
		}
	})

	t.Run("MaxBodyBytes - Over limit fails", func(t *testing.T) {
		largeData := make([]byte, 200)
		req := httptest.NewRequest(http.MethodPost, "/test", bytes.NewReader(largeData))
		rec := httptest.NewRecorder()
		MaxBodyBytes(50)(testHandler).ServeHTTP(rec, req)
		if rec.Code == http.StatusOK {
			t.Errorf("expected error code for payload exceeding limit, got 200")
		}
	})

}
