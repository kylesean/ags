package main

import "testing"

// RED: 空池 + 伪 keyring 切换应跳过同步（返回 nil），而不是让 gui 启动失败。
func TestSyncKeyringSkipsFallback(t *testing.T) {
	t.Setenv("AGSW_DATA_DIR", t.TempDir())
	if err := syncKeyringAccount(accountSwitch{name: "keyring", email: "x@y"}); err != nil {
		t.Fatalf("伪 keyring 回落应跳过同步，got err: %v", err)
	}
}
