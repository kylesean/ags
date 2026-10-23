package proxy

import (
	"runtime"
	"strings"
	"testing"
)

// 上游按 User-Agent 决定「你是谁、能看哪些模型」。
//
// 实测（2026-10-23，同一 token）：
//
//	antigravity-cli/1.2.9  → 23 个模型，额度汇总只有 "Claude and GPT models"
//	antigravity/cli/1.4.6  → 29 个模型（含 Gemini 4 Argon），额度汇总含
//	                         "Gemini Models" / "Claude and GPT models" / "Gemini 4 Argon"
//
// 旧格式不是「403」，而是**静默降级**：模型表少了 Gemini 组，
// 额度接口也不再返回 GEMINI 组 —— ags 因此永远显示「未找到 GEMINI 组」，
// 自动切号彻底失效。这比报错更难发现，所以必须钉死新格式。
func TestUpstreamUserAgentUsesCurrentFormat(t *testing.T) {
	ua := UpstreamUserAgent("linux", "amd64", "1.4.6")

	if !strings.HasPrefix(ua, "antigravity/cli/1.4.6 ") {
		t.Errorf("UA = %q，缺少 antigravity/cli/<version> 前缀", ua)
	}
	if strings.HasPrefix(ua, "antigravity-cli/") {
		t.Errorf("UA = %q，还是旧格式（会被上游静默降级）", ua)
	}
	for _, want := range []string{"aidev_client", "os_type=linux", "arch=amd64"} {
		if !strings.Contains(ua, want) {
			t.Errorf("UA = %q 缺 %q", ua, want)
		}
	}
}

// 版本号缺失时退回内置默认版本：宁可用一个已知可用的版本，
// 也不能拼出 "antigravity/cli/ " 这种必然被降级的头。
func TestUpstreamUserAgentFallsBackWithoutVersion(t *testing.T) {
	ua := UpstreamUserAgent("linux", "amd64", "")
	if !strings.HasPrefix(ua, "antigravity/cli/") {
		t.Fatalf("UA = %q", ua)
	}
	if strings.Contains(ua, "cli/ ") || strings.Contains(ua, "cli/)") {
		t.Errorf("空版本拼出了畸形 UA: %q", ua)
	}
}

// 版本号必须被清理：agy --version 的输出可能带换行/前后空白。
func TestUpstreamUserAgentTrimsVersion(t *testing.T) {
	ua := UpstreamUserAgent("linux", "amd64", " 1.4.6\n")
	if !strings.HasPrefix(ua, "antigravity/cli/1.4.6 ") {
		t.Errorf("UA = %q，版本号没被 trim", ua)
	}
}

// 默认 UA 必须就是新格式，且带本机平台 —— serve/daemon 不依赖调用方记得传。
func TestDefaultUserAgentIsCurrentFormat(t *testing.T) {
	ua := DefaultUpstreamUserAgent()
	if strings.HasPrefix(ua, "antigravity-cli/") {
		t.Errorf("默认 UA 仍是旧格式: %q", ua)
	}
	if !strings.Contains(ua, "os_type="+runtime.GOOS) {
		t.Errorf("默认 UA 没带本机平台: %q", ua)
	}
	if !strings.Contains(ua, "arch="+runtime.GOARCH) {
		t.Errorf("默认 UA 没带本机架构: %q", ua)
	}
}
