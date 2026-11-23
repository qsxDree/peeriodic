package utils

import (
	"net/http/httptest"
	"strings"
	"testing"
)

func TestDecodeJSON(t *testing.T) {
	cases := []struct {
		name, body, contentType string
		status                  int
	}{
		{"object", `{"content":"hello"}`, "application/json; charset=utf-8", 200},
		{"empty content", `{"content":""}`, "application/json", 200},
		{"trailing whitespace", "{} \n", "application/json", 200},
		{"empty body", "", "application/json", 400},
		{"null", "null", "application/json", 400},
		{"array", "[]", "application/json", 400},
		{"unknown field", `{"unknown":1}`, "application/json", 400},
		{"second object", "{} {}", "application/json", 400},
		{"trailing garbage", "{} garbage", "application/json", 400},
		{"false media type", "{}", "application/jsonp", 415},
		{"oversized", `{"content":"` + strings.Repeat("a", 1024*1024) + `"}`, "application/json", 413},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest("POST", "/", strings.NewReader(tc.body))
			req.Header.Set("Content-Type", tc.contentType)
			rec := httptest.NewRecorder()
			var dst struct {
				Content string `json:"content"`
			}
			err := DecodeJSON(rec, req, &dst)
			if rec.Code != tc.status || (err != nil) != (tc.status != 200) {
				t.Fatalf("status=%d err=%v body=%s", rec.Code, err, rec.Body)
			}
		})
	}
}
