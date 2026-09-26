package proxy

import (
	"encoding/json"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// RED: 400 错误体必须是合法 JSON，即使 err 含引号。
func TestBadRequestBodyIsValidJSON(t *testing.T) {
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("body 非法时不该转发到上游")
	}))
	defer up.Close()
	s, err := New(up.URL, Static(&Account{Name: "a", AccessToken: "T"}), log.New(io.Discard, "", 0))
	if err != nil {
		t.Fatal(err)
	}
	s.StripFields("sessionId")
	// 错位引号使 json 错误串含 '"'，手拼 JSON 会非法。
	req := httptest.NewRequest("POST", "http://proxy/x", strings.NewReader(`{"a" "b"}`))
	rec := httptest.NewRecorder()
	s.ServeHTTP(rec, req)
	if rec.Code != 400 {
		t.Fatalf("code = %d, want 400", rec.Code)
	}
	var v map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &v); err != nil {
		t.Fatalf("400 体不是合法 JSON: %v, body=%q", err, rec.Body.String())
	}
}
