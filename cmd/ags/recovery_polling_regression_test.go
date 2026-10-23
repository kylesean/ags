package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/kylesean/ags/internal/pool"
	"github.com/kylesean/ags/internal/quota"
)

// 全池冷却时的轮询节奏：5h 窗口见底这种情形，所有号会在同一刻前后恢复。
// 按常规 3 分钟轮询意味着最长要等 3 分钟才可能切回来；而多号池里
// 一个号刚恢复、其它号还锁着时，越早发现越不容易撞 429。
//
// 所以全池冷却时把下一轮间隔压到 recoveryPollFloor，但不是 0
// （0 会退化成热循环，把额度接口刷爆）。
func TestNextIntervalShrinksWhenAllCooling(t *testing.T) {
	now := time.Now()
	sel := pool.NewSelector([]*pool.Account{
		{Name: "A", AccessToken: "T", Expiry: now.Add(time.Hour), CooldownUntil: now.Add(time.Minute)},
		{Name: "B", AccessToken: "T", Expiry: now.Add(time.Hour), CooldownUntil: now.Add(time.Minute)},
	}, nil)

	got := nextQuotaInterval(sel, 3*time.Minute, now)
	if got >= 3*time.Minute {
		t.Errorf("全池冷却时仍按常规 %s 轮询，应缩短（got %s）", 3*time.Minute, got)
	}
	if got < recoveryPollFloor {
		t.Errorf("间隔 %s 短于下限 %s，会退化成热循环", got, recoveryPollFloor)
	}
}

// 有账号健康时不能缩短：那会把正常的 3 分钟节奏变成无意义的空转。
func TestNextIntervalStaysWhenAnyHealthy(t *testing.T) {
	now := time.Now()
	sel := pool.NewSelector([]*pool.Account{
		{Name: "A", AccessToken: "T", Expiry: now.Add(time.Hour), CooldownUntil: now.Add(time.Minute)},
		{Name: "B", AccessToken: "T", Expiry: now.Add(time.Hour)},
	}, nil)

	if got := nextQuotaInterval(sel, 3*time.Minute, now); got != 3*time.Minute {
		t.Errorf("有健康账号时不该改节奏，got %s", got)
	}
}

// 冷却很久（周窗口耗尽）时，压到 floor 没意义但也无害 ——
// 关键是别为「一周后恢复」去每分钟刷一次接口。
// 这里用「不超过上限」表达这个约束：间隔取 min(常规, 剩余时间+余量)。
func TestNextIntervalRespectsFarFutureRecovery(t *testing.T) {
	now := time.Now()
	sel := pool.NewSelector([]*pool.Account{
		{Name: "A", AccessToken: "T", Expiry: now.Add(time.Hour), CooldownUntil: now.Add(6 * 24 * time.Hour)},
	}, nil)

	got := nextQuotaInterval(sel, 3*time.Minute, now)
	if got != 3*time.Minute {
		t.Errorf("远未来恢复时仍按常规轮询即可，got %s", got)
	}
}

// 429 单号即查后，若该号额度已恢复，因**额度耗尽**打上的冷却要立刻清掉，
// 不能等 resetTime 自然到期 —— 那可能是几天后。
func TestProbeRecoversAccountAndClearsQuotaCooldown(t *testing.T) {
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"groups":[{"displayName":"Gemini Models","buckets":[
			{"bucketId":"gemini-5h","remainingFraction":1,"resetTime":"2099-01-01T00:00:00Z"},
			{"bucketId":"gemini-weekly","remainingFraction":1,"resetTime":"2099-01-01T00:00:00Z"}]}]}`))
	}))
	defer up.Close()

	now := time.Now()
	sel := pool.NewSelector([]*pool.Account{
		{Name: "A", AccessToken: "T", Expiry: now.Add(time.Hour)},
	}, nil)
	// 模拟「周额度耗尽 → 冷却到一周后」。
	sel.SetQuotaState("A", true)
	sel.SetCooldownMax("A", now.Add(6*24*time.Hour))

	qw := newQuotaWatcher(sel, up.URL, "ags-test", 0, log.New(discardWriter{}, "", 0))
	qw.last["A"] = true // 上一轮判定为耗尽
	qw.checkOne(context.Background(), "A")

	if got := sel.CooldownOf("A"); !got.IsZero() {
		t.Errorf("额度已恢复，额度冷却应被清掉，got %v（还要等到 %s）", got, pool.FormatCooldown(got))
	}
}

// 但 429 打上的短避让不能被额度轮询洗掉 —— 那是「别马上再撞一次」的保险。
func TestProbeKeeps429Cooldown(t *testing.T) {
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"groups":[{"displayName":"Gemini Models","buckets":[
			{"bucketId":"gemini-5h","remainingFraction":1,"resetTime":"2099-01-01T00:00:00Z"},
			{"bucketId":"gemini-weekly","remainingFraction":1,"resetTime":"2099-01-01T00:00:00Z"}]}]}`))
	}))
	defer up.Close()

	now := time.Now()
	until := now.Add(45 * time.Second)
	sel := pool.NewSelector([]*pool.Account{
		{Name: "A", AccessToken: "T", Expiry: now.Add(time.Hour)},
	}, nil)
	sel.SetCooldown("A", until) // 429 来源

	qw := newQuotaWatcher(sel, up.URL, "ags-test", 0, log.New(discardWriter{}, "", 0))
	qw.checkOne(context.Background(), "A")

	if got := sel.CooldownOf("A"); got.IsZero() {
		t.Error("429 避让被额度轮询洗掉了，会立刻再撞一次限流")
	}
}

// 全池冷却错误要被认出来（writePickError 依赖 errors.As）。
func TestAllCoolingErrorDetected(t *testing.T) {
	err := error(&pool.AllCoolingError{Until: time.Now().Add(time.Minute), Names: []string{"A"}})
	var target *pool.AllCoolingError
	if !errors.As(err, &target) {
		t.Fatal("errors.As 没认出 AllCoolingError")
	}
	if target.RetryAfter(time.Now()) <= 0 {
		t.Error("RetryAfter 应为正")
	}
}

var _ = quota.Fetch
