package main

import (
	"context"
	"log"
	"strings"
	"testing"

	"github.com/kylesean/ags/internal/cloudcode"
)

// UA 过期是「静默降级」：接口照常 200，只是少返回模型与额度组。
// 所以必须在启动时**主动**判定一次，而不是等用户发现额度不刷新。
//
// 判据：新代际的 UA 能拿到 Gemini 4 Argon 这一族 + Gemini Models 额度组；
// 拿到的是旧代际的窄集就说明 UA 过期了。
func TestUACapabilityCheckDetectsStaleGeneration(t *testing.T) {
	stale := &cloudcode.Models{
		Keys: 23,
		// 旧代际：只有 claude 一族与 gpt-oss，没有 gemini-4-argon-*
		Catalog: []cloudcode.CatalogEntry{
			{ID: "claude-opus-5-5-medium", DisplayName: "Claude Opus 5.5 (Medium)"},
			{ID: "gpt-oss-120b-medium", DisplayName: "GPT-OSS 120B (Medium)"},
		},
	}
	if msg := uaCapabilityWarning(stale); msg == "" {
		t.Error("旧代际的模型表应当触发 UA 告警")
	}

	fresh := &cloudcode.Models{
		Keys: 29,
		Catalog: []cloudcode.CatalogEntry{
			{ID: "gemini-4-argon-medium", DisplayName: "Gemini 4 Argon (Medium)"},
			{ID: "claude-opus-5-5-medium", DisplayName: "Claude Opus 5.5 (Medium)"},
		},
	}
	if msg := uaCapabilityWarning(fresh); msg != "" {
		t.Errorf("新代际不该告警，得到 %q", msg)
	}
}

// 模型表为空（拉取失败）不该告警成「UA 过期」—— 那是另一个故障，
// 混为一谈会把排查方向带偏。
func TestUACapabilityCheckIgnoresEmptyTable(t *testing.T) {
	if msg := uaCapabilityWarning(nil); msg != "" {
		t.Errorf("nil 不该告警，得到 %q", msg)
	}
	if msg := uaCapabilityWarning(&cloudcode.Models{}); msg != "" {
		t.Errorf("空表不该告警，得到 %q", msg)
	}
}

// 告警文案要能直接指导用户：说明现象、原因、以及原始 UA。
func TestUACapabilityWarningIsActionable(t *testing.T) {
	m := &cloudcode.Models{Keys: 23, Catalog: []cloudcode.CatalogEntry{
		{ID: "claude-opus-5-5-medium", DisplayName: "x"},
	}}
	msg := uaCapabilityWarning(m)
	for _, want := range []string{"User-Agent", "agy"} {
		if !strings.Contains(msg, want) {
			t.Errorf("告警 = %q 缺 %q", msg, want)
		}
	}
}

var _ = context.Background
var _ = log.New
