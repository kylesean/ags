package main

import (
	"path/filepath"
	"testing"
)

// RED: daemon 选中落盘 status 可读；不一致时提示重启。
func TestDaemonStateRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	if err := writeDaemonState(path, "B", "b@x"); err != nil {
		t.Fatalf("write: %v", err)
	}
	st, err := readDaemonState(path)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if st.Name != "B" || st.Email != "b@x" || st.At.IsZero() {
		t.Fatalf("got %+v", st)
	}
	if _, err := readDaemonState(filepath.Join(t.TempDir(), "missing.json")); err == nil {
		t.Fatalf("缺文件应报错")
	}
}

func TestPendingRestartHint(t *testing.T) {
	if got := pendingRestartHint("a@x", &daemonState{Name: "B", Email: "b@x"}); got == "" {
		t.Fatalf("不一致时应提示重启")
	}
	if got := pendingRestartHint("B@X", &daemonState{Name: "B", Email: "b@x"}); got != "" {
		t.Fatalf("一致时不应提示，got %q", got)
	}
	if got := pendingRestartHint("a@x", nil); got != "" {
		t.Fatalf("无状态时不应提示，got %q", got)
	}
}
