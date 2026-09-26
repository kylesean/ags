package pool

import (
	"context"
	"testing"
	"time"
)

// RED: RefreshAll 跨网络刷新时不应堵住全局读方法。
func TestRefreshAllDoesNotBlockGlobals(t *testing.T) {
	block := make(chan struct{})
	t.Cleanup(func() { select { case <-block: default: close(block) } })
	refresh := func(_ context.Context, ac *Account) error {
		<-block
		return nil
	}
	a := &Account{Name: "a", RefreshToken: "RT", AccessToken: "A"}
	s := NewSelector([]*Account{a}, refresh)

	done := make(chan error, 1)
	go func() { done <- s.RefreshAll(context.Background()) }()

	// 等 RefreshAll 进入刷新
	time.Sleep(50 * time.Millisecond)

	finished := make(chan struct{})
	go func() {
		defer close(finished)
		_ = s.CooldownOf("a")
		s.SetCooldown("a", baseTime.Add(time.Minute))
		// 注意：Candidates 需持单号锁做快照，正在刷新的账号会短暂阻塞它，
		// 这与 tryPick 刷新路径一致；此处只断言纯全局方法不被堵。
	}()

	select {
	case <-finished:
	case <-time.After(300 * time.Millisecond):
		t.Fatalf("RefreshAll 网络刷新阻塞了全局方法")
	}

	close(block)
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("RefreshAll: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatalf("RefreshAll 未返回")
	}
}
