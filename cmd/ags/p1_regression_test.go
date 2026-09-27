package main

import (
	"context"
	"log"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/kylesean/ags/internal/pool"
)

// RED: 401 应打短冷却（30s）并触发额度检查，对齐 429。
func TestReportStatus401Cooldowns(t *testing.T) {
	now := time.Now()
	a := &pool.Account{Name: "A", Email: "a@x", AccessToken: "AT", Expiry: now.Add(time.Hour)}
	sel := pool.NewSelector([]*pool.Account{a}, nil)
	triggered := false
	p := &selectorPicker{sel: sel, log: log.New(discardWriter{}, "", 0), on429: func(string) { triggered = true }}
	p.ReportStatus("A", http.StatusUnauthorized)
	if got := sel.CooldownOf("A"); got.IsZero() || time.Until(got) < 20*time.Second {
		t.Fatalf("401 应打 ~30s 冷却，got %v", got)
	}
	if !triggered {
		t.Fatalf("401 应触发额度检查（复用 on429 触发器）")
	}
}

type discardWriter struct{}

func (discardWriter) Write(p []byte) (int, error) { return len(p), nil }

// RED: 健康轮询不应洗掉 429 的 1m 冷却（取 max）。
func TestQuotaCheckPreserves429Cooldown(t *testing.T) {
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"groups":[{"displayName":"Gemini Models","buckets":[
			{"bucketId":"gemini-5h","remainingFraction":0.9,"resetTime":"2099-01-01T00:00:00Z"},
			{"bucketId":"gemini-weekly","remainingFraction":0.9,"resetTime":"2099-01-01T00:00:00Z"}]}]}`))
	}))
	defer up.Close()

	now := time.Now()
	a := &pool.Account{Name: "A", Email: "a@x", AccessToken: "AT", Expiry: now.Add(time.Hour)}
	sel := pool.NewSelector([]*pool.Account{a}, nil)
	// 模拟 429 刚打的 1m 冷却
	sel.SetCooldown("A", time.Now().Add(time.Minute))
	qw := newQuotaWatcher(sel, up.URL, "ags-test", 0, log.New(discardWriter{}, "", 0))
	qw.check(context.Background(), false)
	if got := sel.CooldownOf("A"); got.IsZero() || time.Until(got) < 30*time.Second {
		t.Fatalf("健康轮询不应清掉 429 冷却，got %v", got)
	}
}
