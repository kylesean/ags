package cloudcode

import (
	"encoding/json"
	"testing"
)

// effortSample 是与 2026-10-23 实测的 models 表同构的子集。
//
// agy 1.4.6 在网关模式下的行为（实测，见 cli.log 与抓包）：
//   - 选 "Gemini 4 Argon (Low)"    → 发 path .../models/gemini-4-argon:...
//   - 选 "Gemini 4 Argon (Medium)" → 发 path .../models/gemini-4-argon:...
//   - 选 "Gemini 3.8 Flash (High)" → 发 path .../models/gemini-3.8-flash:...
//
// 也就是它把努力档位从模型名里摘掉，靠 thinkingConfig 传档位，
// 然后指望网关把裸名解析成上游真实键。gemini-3.8-flash 由 -tiered
// 规则兜住了，Argon 没有 tiered 键，于是裸名 gemini-4-argon 404。
const effortSample = `{
  "defaultAgentModelId": "gemini-4-argon-medium",
  "models": {
    "gemini-4-argon-low":    {"displayName": "Gemini 4 Argon (Low)"},
    "gemini-4-argon-medium": {"displayName": "Gemini 4 Argon (Medium)"},
    "gemini-4-argon-high":   {"displayName": "Gemini 4 Argon (High)"},
    "gemini-3.8-flash-low":  {"displayName": "Gemini 3.8 Flash (Low)"},
    "gemini-3.8-flash-medium": {"displayName": "Gemini 3.8 Flash (Medium)"},
    "gemini-3.8-flash-high": {"displayName": "Gemini 3.8 Flash (High)"},
    "gemini-3.8-flash-tiered": {"model": "MODEL_PLACEHOLDER_M322"},
    "claude-opus-5-5-low":    {"displayName": "Claude Opus 5.5 (Low)"},
    "claude-opus-5-5-medium": {"displayName": "Claude Opus 5.5 (Medium)"}
  }
}`

func mustParseEffort(t *testing.T) *fetchAvailableModelsResponse {
	t.Helper()
	var r fetchAvailableModelsResponse
	if err := json.Unmarshal([]byte(effortSample), &r); err != nil {
		t.Fatalf("解析样例失败: %v", err)
	}
	return &r
}

// 裸名必须能解析到上游真实键，否则 agy 一选 Argon 就是 404。
func TestBuildModelsDerivesEffortAlias(t *testing.T) {
	m := buildModels(mustParseEffort(t))

	if got := m.Aliases["gemini-4-argon"]; got != "gemini-4-argon-medium" {
		t.Errorf("gemini-4-argon → %q，想要上游声明的默认档 gemini-4-argon-medium", got)
	}
}

// 上游没声明默认档时退到 medium，再退到排序后的第一个。
func TestBuildModelsEffortAliasPrefersMedium(t *testing.T) {
	r := &fetchAvailableModelsResponse{
		Models: map[string]json.RawMessage{
			"m-low":  []byte(`{"displayName":"M (Low)"}`),
			"m-high": []byte(`{"displayName":"M (High)"}`),
		},
	}
	if got := buildModels(r).Aliases["m"]; got != "m-high" {
		t.Errorf("没有 medium 时 m → %q，想要按字典序取 -high", got)
	}

	r = &fetchAvailableModelsResponse{
		Models: map[string]json.RawMessage{
			"m-low":    []byte(`{"displayName":"M (Low)"}`),
			"m-medium": []byte(`{"displayName":"M (Medium)"}`),
			"m-high":   []byte(`{"displayName":"M (High)"}`),
		},
	}
	if got := buildModels(r).Aliases["m"]; got != "m-medium" {
		t.Errorf("m → %q，想要 m-medium", got)
	}
}

// -tiered 规则优先：gemini-3.8-flash 指向 tiered 而不是 -high。
// 反过来会把 agy 原本正确的努力档位映射给废掉。
func TestBuildModelsTieredWinsOverEffort(t *testing.T) {
	m := buildModels(mustParseEffort(t))

	if got := m.Aliases["gemini-3.8-flash"]; got != "gemini-3.8-flash-tiered" {
		t.Errorf("gemini-3.8-flash → %q，想要 -tiered", got)
	}
}

// 裸名本身就是真实键时不许覆盖（与 -tiered 规则同一条底线）。
func TestBuildModelsEffortDoesNotShadowRealKey(t *testing.T) {
	r := &fetchAvailableModelsResponse{
		Models: map[string]json.RawMessage{
			"x":        []byte(`{"displayName":"X"}`),
			"x-low":    []byte(`{"displayName":"X (Low)"}`),
			"x-high":   []byte(`{"displayName":"X (High)"}`),
			"x-medium": []byte(`{"displayName":"X (Medium)"}`),
		},
	}
	m := buildModels(r)
	if _, ok := m.Aliases["x"]; ok {
		t.Errorf("x 是真实键却进了别名表: x → %q", m.Aliases["x"])
	}
}

// 不带档位后缀的模型不能凭空长出一条别名。
func TestBuildModelsEffortIgnoresUnrelatedNames(t *testing.T) {
	r := &fetchAvailableModelsResponse{
		Models: map[string]json.RawMessage{
			"gemini-2.5-pro": []byte(`{"displayName":"Gemini 2.5 Pro"}`),
			"gpt-oss-120b":   []byte(`{"displayName":"GPT-OSS 120B"}`),
			"cli-low-level":  []byte(`{"displayName":"Not a tier"}`),
		},
	}
	m := buildModels(r)
	for _, bad := range []string{"gemini-2.5", "gpt-oss-120b", "cli"} {
		if _, ok := m.Aliases[bad]; ok {
			t.Errorf("凭空生成了别名 %s → %q", bad, m.Aliases[bad])
		}
	}
}
