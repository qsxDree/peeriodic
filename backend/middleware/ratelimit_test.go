package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestIPRateLimiter(t *testing.T) {
	// Create a limiter with capacity 2 and refill 1 req/min (slow refill for test)
	limiter := NewIPRateLimiter(1, 2)
	defer limiter.Close()

	testHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	wrapped := limiter.Middleware(testHandler)

	// 1st request from IP 1.2.3.4 should succeed
	req1 := httptest.NewRequest(http.MethodGet, "/test", nil)
	req1.RemoteAddr = "1.2.3.4:1234"
	rec1 := httptest.NewRecorder()
	wrapped.ServeHTTP(rec1, req1)
	if rec1.Code != http.StatusOK {
		t.Errorf("expected request 1 to succeed, got %d", rec1.Code)
	}

	// 2nd request from IP 1.2.3.4 should succeed (burst capacity is 2)
	req2 := httptest.NewRequest(http.MethodGet, "/test", nil)
	req2.RemoteAddr = "1.2.3.4:1234"
	rec2 := httptest.NewRecorder()
	wrapped.ServeHTTP(rec2, req2)
	if rec2.Code != http.StatusOK {
		t.Errorf("expected request 2 to succeed, got %d", rec2.Code)
	}

	// 3rd request from IP 1.2.3.4 should be rate limited (429)
	req3 := httptest.NewRequest(http.MethodGet, "/test", nil)
	req3.RemoteAddr = "1.2.3.4:1234"
	rec3 := httptest.NewRecorder()
	wrapped.ServeHTTP(rec3, req3)
	if rec3.Code != http.StatusTooManyRequests {
		t.Errorf("expected request 3 to be rate limited (429), got %d", rec3.Code)
	}
	if rec3.Header().Get("Retry-After") == "" {
		t.Errorf("expected Retry-After header on 429 response")
	}

	// Request from different IP 5.6.7.8 should succeed
	reqOther := httptest.NewRequest(http.MethodGet, "/test", nil)
	reqOther.RemoteAddr = "5.6.7.8:1234"
	recOther := httptest.NewRecorder()
	wrapped.ServeHTTP(recOther, reqOther)
	if recOther.Code != http.StatusOK {
		t.Errorf("expected request from other IP to succeed, got %d", recOther.Code)
	}
}

func TestExtractIP(t *testing.T) {
	for _, header := range []string{"X-Forwarded-For", "X-Real-IP"} {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.RemoteAddr = "192.0.2.1:5678"
		req.Header.Set(header, "203.0.113.195")
		if ip := ExtractIP(req); ip != "192.0.2.1" {
			t.Fatalf("trusted spoofed %s: %s", header, ip)
		}
	}

	// RemoteAddr fallback
	reqRA := httptest.NewRequest(http.MethodGet, "/", nil)
	reqRA.RemoteAddr = "192.0.2.1:5678"
	if ip := ExtractIP(reqRA); ip != "192.0.2.1" {
		t.Errorf("expected 192.0.2.1, got %s", ip)
	}
}
