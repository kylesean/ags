package main

import (
	"context"
	"log"
	"testing"
	"time"

	"github.com/kylesean/agsw/internal/keyring"
	"github.com/kylesean/agsw/internal/pool"
)

// : 单轮同步——keyring 与最优号不一致时写入，一致时不动。
func TestDaemonSyncOnce(t *testing.T) {
	t.Setenv("AGSW_DATA_DIR", t.TempDir())
	now := time.Now()
	sel := pool.NewSelector([]*pool.Account{
		{Name: "B", Email: "b@x", AccessToken: "AT", RefreshToken: "rt", Expiry: now.Add(time.Hour)},
	}, nil)
	old := storeKeyring
	t.Cleanup(func() { storeKeyring = old })
	var got *keyring.Secret
	storeKeyring = func(sec *keyring.Secret) error { got = sec; return nil }

	// B 入池，daemon 才能按名同步。
	if err := pool.Save(&pool.Account{Name: "B", Email: "b@x", AccessToken: "AT",
		RefreshToken: "rt", Expiry: now.Add(time.Hour)}); err != nil {
		t.Fatal(err)
	}
	switched, err := daemonSyncOnce(context.Background(), sel, log.New(discardWriter{}, "", 0))
	if err != nil {
		t.Fatalf("daemonSyncOnce: %v", err)
	}
	// 本机 keyring 身份大概率不是 b@x → 应切换写入；若恰好是则不断言方向，只断言一致性。
	_, curEmail, curErr := keyring.Current()
	if curErr != nil || curEmail != "b@x" {
		if !switched || got == nil {
			t.Fatalf("身份不一致时应写入，switched=%v got=%+v", switched, got)
		}
	} else if switched {
		t.Fatalf("身份一致时不应写入")
	}
}

// : 全部冷却时不写、不崩。
func TestDaemonSyncOnceAllCooling(t *testing.T) {
	t.Setenv("AGSW_DATA_DIR", t.TempDir())
	now := time.Now()
	sel := pool.NewSelector([]*pool.Account{
		{Name: "A", Email: "a@x", AccessToken: "AT", Expiry: now.Add(time.Hour)},
	}, nil)
	sel.SetCooldown("A", now.Add(time.Hour))
	old := storeKeyring
	t.Cleanup(func() { storeKeyring = old })
	called := false
	storeKeyring = func(sec *keyring.Secret) error { called = true; return nil }
	_, err := daemonSyncOnce(context.Background(), sel, log.New(discardWriter{}, "", 0))
	if err == nil {
		t.Fatalf("无可用号时应报错")
	}
	if called {
		t.Fatalf("无可用号时不应写 keyring")
	}
}
