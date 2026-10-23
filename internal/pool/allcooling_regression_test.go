package pool

import (
	"context"
	"errors"
	"testing"
	"time"
)

// 「全池冷却」和「池空 / token 全废」是两种完全不同的故障：
//   - 全池冷却 → 等一会儿就能恢复，网关应该短等重试或回 Retry-After
//   - 池空或 token 全废 → 等多久都没用，必须让用户去处理
//
// 旧实现把两者都塞进一个 errors.Join 里，上层只能解析错误字符串。
func TestPickReportsAllCooling(t *testing.T) {
	now := time.Now()
	reset := now.Add(20 * time.Second)
	sel := NewSelector([]*Account{
		{Name: "A", AccessToken: "T", Expiry: now.Add(time.Hour), CooldownUntil: reset},
		{Name: "B", AccessToken: "T", Expiry: now.Add(time.Hour), CooldownUntil: reset.Add(10 * time.Second)},
	}, nil)

	_, err := sel.Pick(context.Background())
	if err == nil {
		t.Fatal("全部冷却时应当报错")
	}

	var cooling *AllCoolingError
	if !errors.As(err, &cooling) {
		t.Fatalf("错误类型 = %T，想要 *AllCoolingError: %v", err, err)
	}
	// 最早解冻时刻必须是 A 的 20 秒后，不是 B 的 30 秒后：
	// 我们要的是「最快什么时候能恢复」，不是「最后一个什么时候恢复」。
	if !cooling.Until.Equal(reset) {
		t.Errorf("Until = %v，想要最早的 %v", cooling.Until, reset)
	}
	if len(cooling.Names) != 2 {
		t.Errorf("Names = %v，想要含两个账号", cooling.Names)
	}
}

// 只要有一个账号还有救，就不是 AllCoolingError。
// 否则上层会对一个「修一下就能用」的池子干等。
func TestPickNotAllCoolingWhenOneUsable(t *testing.T) {
	now := time.Now()
	sel := NewSelector([]*Account{
		{Name: "A", AccessToken: "T", Expiry: now.Add(time.Hour), CooldownUntil: now.Add(time.Minute)},
		{Name: "B", AccessToken: "T", Expiry: now.Add(time.Hour)},
	}, nil)

	a, err := sel.Pick(context.Background())
	if err != nil {
		t.Fatalf("B 可用却选不出来: %v", err)
	}
	if a.Name != "B" {
		t.Errorf("选到 %q，想要 B", a.Name)
	}
}

// 池空不能被当成 AllCoolingError —— 等下去永远不会好。
func TestPickEmptyPoolIsNotAllCooling(t *testing.T) {
	sel := NewSelector(nil, nil)
	_, err := sel.Pick(context.Background())
	if err == nil {
		t.Fatal("空池应当报错")
	}
	var cooling *AllCoolingError
	if errors.As(err, &cooling) {
		t.Errorf("空池被误判成全池冷却: %v", err)
	}
}

// token 全废（无 refresh_token 且已过期）也不能算全池冷却。
func TestPickDeadTokensIsNotAllCooling(t *testing.T) {
	now := time.Now()
	sel := NewSelector([]*Account{
		{Name: "A", Expiry: now.Add(-time.Hour)},
		{Name: "B", Expiry: now.Add(-time.Hour)},
	}, nil)

	_, err := sel.Pick(context.Background())
	if err == nil {
		t.Fatal("token 全废应当报错")
	}
	var cooling *AllCoolingError
	if errors.As(err, &cooling) {
		t.Errorf("token 全废被误判成全池冷却: %v", err)
	}
}

// 错误文案要能直接给用户看：说清是「都在冷却」以及最早什么时候恢复。
func TestAllCoolingErrorMentionsEarliestRecovery(t *testing.T) {
	until := time.Date(2026, 10, 24, 4, 7, 0, 0, time.UTC)
	err := &AllCoolingError{Until: until, Names: []string{"A", "B", "C"}}
	msg := err.Error()
	for _, want := range []string{"冷却", "3"} {
		if !containsStr(msg, want) {
			t.Errorf("错误文案 = %q 缺 %q", msg, want)
		}
	}
	if !containsStr(msg, FormatCooldown(until)) {
		t.Errorf("错误文案 = %q 缺最早恢复时刻 %q", msg, FormatCooldown(until))
	}
}

func containsStr(s, sub string) bool {
	return len(s) >= len(sub) && (len(sub) == 0 || indexStr(s, sub) >= 0)
}

func indexStr(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}
