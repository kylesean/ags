package main

import (
	"context"
	"log"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/kylesean/agsw/internal/pool"
)

// : 默认轮询间隔 3m + 抖动；429 单号即查。
func TestDefaultQuotaInterval(t *testing.T) {
	if DefaultQuotaInterval != 3*time.Minute {
		t.Fatalf("DefaultQuotaInterval = %s, want 3m", DefaultQuotaInterval)
	}
}

func TestJitteredInterval(t *testing.T) {
	base := 3 * time.Minute
	lo, hi := 135*time.Second, 225*time.Second
	for i := 0; i < 200; i++ {
		got := jitteredInterval(base)
		if got < lo || got > hi {
			t.Fatalf("jitteredInterval(%s) = %s, want [%s,%s]", base, got, lo, hi)
		}
	}
	if jitteredInterval(0) != 0 {
		t.Fatalf("jitteredInterval(0) 应返回 0")
	}
}

func TestTriggerSingleAccount(t *testing.T) {
	var mu sync.Mutex
	seen := map[string]int{}
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		seen[r.Header.Get("Authorization")]++
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"groups":[{"displayName":"Gemini Models","buckets":[
			{"bucketId":"gemini-5h","remainingFraction":0.9,"resetTime":"2099-01-01T00:00:00Z"},
			{"bucketId":"gemini-weekly","remainingFraction":0.9,"resetTime":"2099-01-01T00:00:00Z"}]}]}`))
	}))
	defer up.Close()

	now := time.Now()
	sel := pool.NewSelector([]*pool.Account{
		{Name: "A", Email: "a@x", AccessToken: "TOK-A", Expiry: now.Add(time.Hour)},
		{Name: "B", Email: "b@x", AccessToken: "TOK-B", Expiry: now.Add(time.Hour)},
	}, nil)
	qw := newQuotaWatcher(sel, up.URL, "agsw-test", 0, log.New(discardWriter{}, "", 0))
	qw.checkOne(context.Background(), "A")
	mu.Lock()
	defer mu.Unlock()
	if len(seen) != 1 || seen["Bearer TOK-A"] != 1 {
		t.Fatalf("单号即查应只查 A，got %v", seen)
	}
}
