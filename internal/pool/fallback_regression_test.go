package pool

import "testing"

// RED: KeyringFallback 必须为纯内存标记，不持久化。
func TestKeyringFallbackNotPersisted(t *testing.T) {
	withTmpDir(t)
	a := &Account{Name: "tmp", Email: "t@x", KeyringFallback: true}
	if err := Save(a); err != nil {
		t.Fatalf("Save: %v", err)
	}
	got, err := Load("tmp")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got.KeyringFallback {
		t.Errorf("KeyringFallback 不应持久化到磁盘")
	}
}
