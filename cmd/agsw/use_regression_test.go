package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/kylesean/agsw/internal/keyring"
	"github.com/kylesean/agsw/internal/pool"
)

func healthyUpstream(t *testing.T) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"groups":[{"displayName":"Gemini Models","buckets":[
			{"bucketId":"gemini-5h","remainingFraction":0.9,"resetTime":"2099-01-01T00:00:00Z"},
			{"bucketId":"gemini-weekly","remainingFraction":0.9,"resetTime":"2099-01-01T00:00:00Z"}]}]}`))
	}))
}

func exhaustedUpstream(t *testing.T) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"groups":[{"displayName":"Gemini Models","buckets":[
			{"bucketId":"gemini-5h","remainingFraction":0,"resetTime":"2099-01-01T00:00:00Z"},
			{"bucketId":"gemini-weekly","remainingFraction":0.9,"resetTime":"2099-01-01T00:00:00Z"}]}]}`))
	}))
}

func seedUseAccount(t *testing.T, name string) {
	t.Helper()
	a := &pool.Account{
		Name: name, Email: name + "@x", AccessToken: "AT",
		RefreshToken: "rt", Expiry: time.Now().Add(time.Hour),
	}
	if err := pool.Save(a); err != nil {
		t.Fatal(err)
	}
}

// : 有额度时 use 应写入 keyring。
func TestUseHealthyWritesKeyring(t *testing.T) {
	t.Setenv("AGSW_DATA_DIR", t.TempDir())
	up := healthyUpstream(t)
	defer up.Close()
	seedUseAccount(t, "B")
	old := storeKeyring
	t.Cleanup(func() { storeKeyring = old })
	var got *keyring.Secret
	storeKeyring = func(sec *keyring.Secret) error { got = sec; return nil }
	if err := cmdUse([]string{"-upstream", up.URL, "B"}); err != nil {
		t.Fatalf("cmdUse: %v", err)
	}
	if got == nil || got.Token.AccessToken != "AT" {
		t.Fatalf("未写入 keyring，got %+v", got)
	}
}

// : 无额度时 use 应拒绝（除非 --force），且不写 keyring。
func TestUseExhaustedRefuses(t *testing.T) {
	t.Setenv("AGSW_DATA_DIR", t.TempDir())
	up := exhaustedUpstream(t)
	defer up.Close()
	seedUseAccount(t, "B")
	old := storeKeyring
	t.Cleanup(func() { storeKeyring = old })
	called := false
	storeKeyring = func(sec *keyring.Secret) error { called = true; return nil }
	err := cmdUse([]string{"-upstream", up.URL, "B"})
	if err == nil || !strings.Contains(err.Error(), "耗尽") {
		t.Fatalf("应拒绝耗尽账号，got %v", err)
	}
	if called {
		t.Fatalf("拒绝时不应写 keyring")
	}
	if err := cmdUse([]string{"-upstream", up.URL, "-force", "B"}); err != nil {
		t.Fatalf("--force 应强行切换，got %v", err)
	}
	if !called {
		t.Fatalf("--force 应写入 keyring")
	}
}
