package middleware

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/logoes0/peeriodic.git/utils"
)

func TestRecoverer(t *testing.T) {
	panicHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		panic("something went critically wrong!")
	})

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/test-panic", nil)

	handler := Recoverer(panicHandler)
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("expected status 500, got %d", rec.Code)
	}

	var resp utils.Response
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode JSON error response: %v", err)
	}

	if resp.Success {
		t.Errorf("expected success to be false, got true")
	}
	if resp.Error == "" {
		t.Errorf("expected error message in response")
	}
}
