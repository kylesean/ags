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

// RED: 缓存根可被 AGS_CACHE_DIR 覆盖，daemon 落盘与 TUI 日志都不得写到真实用户缓存。
func TestCacheDirOverridable(t *testing.T) {
	base := t.TempDir()
	t.Setenv("AGS_CACHE_DIR", base)

	if got, want := defaultDaemonStatePath(), filepath.Join(base, "ags", "state.json"); got != want {
		t.Fatalf("defaultDaemonStatePath() = %q, want %q", got, want)
	}
	if got, want := defaultTUILogPath(), filepath.Join(base, "ags", "tui.log"); got != want {
		t.Fatalf("defaultTUILogPath() = %q, want %q", got, want)
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
