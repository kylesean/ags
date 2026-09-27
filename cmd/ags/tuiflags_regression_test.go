package main

import (
	"strings"
	"testing"
)

// gui 应接受 serve 的调试/上游开关（统一启动器自称 [serve flags]）。
// 只验解析与透传，不拉起 agy 进程。
func TestTUIFlagsParity(t *testing.T) {
	for _, args := range [][]string{
		{"--upstream", "http://127.0.0.1:9"},
		{"--refresh"},
		{"--user-agent", "test-ua"},
		{"--passthrough"},
		{"--strip-field", "sessionId"},
	} {
		o, _, err := parseTUIArgs(args)
		if err != nil {
			t.Errorf("gui %v 应被接受，got: %v", args, err)
			continue
		}
		joined := strings.Join(o.serveArgs(), "\x00")
		for _, want := range args {
			if strings.HasPrefix(want, "--") && want != "--refresh" && want != "--passthrough" {
				continue // 值单独断言
			}
			if want == "--refresh" && !o.refresh {
				t.Errorf("refresh 未生效")
			}
			if want == "--passthrough" && !o.passthrough {
				t.Errorf("passthrough 未生效")
			}
		}
		_ = joined
	}
	o, _, err := parseTUIArgs([]string{"--upstream", "http://u:9", "--user-agent", "UA", "--refresh", "--passthrough", "--strip-field", "a", "--strip-field", "b"})
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	got := strings.Join(o.serveArgs(), " ")
	for _, want := range []string{"-upstream http://u:9", "-user-agent UA", "-refresh", "-passthrough", "-strip-field a", "-strip-field b"} {
		if !strings.Contains(got, want) {
			t.Errorf("serveArgs 缺 %q，got: %s", want, got)
		}
	}
}
