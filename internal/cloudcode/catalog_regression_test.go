package cloudcode

import (
	"encoding/json"
	"testing"
)

// catalogSample 取自 2026-10-23 实测 fetchAvailableModels 的形状（新 UA）。
// 关键点：
//   - 主 Agent 模型都有 displayName；
//   - chat_/tab_ 是内部模型（isInternal 或没有 displayName）；
//   - gemini-3.1-flash-image 是图像生成模型，出现在 imageGenerationModelIds；
//   - gemini-3.8-flash-tiered 没有 displayName（它只是别名目标）。
const catalogSample = `{
  "defaultAgentModelId": "gemini-4-argon-medium",
  "models": {
    "chat_20706":              {"model": "MODEL_CHAT", "isInternal": true},
    "tab_flash_lite_preview":  {"model": "MODEL_TAB"},
    "gemini-3.1-flash-image":  {"displayName": "Gemini 3.1 Flash Image", "maxTokens": 32768, "maxOutputTokens": 8192},
    "gemini-3.8-flash-tiered": {"model": "MODEL_PLACEHOLDER_M322", "maxTokens": 1048576, "maxOutputTokens": 65536},
    "gemini-3.8-flash-high":   {"displayName": "Gemini 3.8 Flash (High)", "maxTokens": 1048576, "maxOutputTokens": 65536, "supportsImages": true, "supportsThinking": true},
    "gemini-4-argon-medium":   {"displayName": "Gemini 4 Argon (Medium)", "maxTokens": 1048576, "maxOutputTokens": 65536, "supportsThinking": true},
    "claude-opus-5-5-medium":  {"displayName": "Claude Opus 5.5 (Medium)", "maxTokens": 1000000, "maxOutputTokens": 128000, "supportsImages": true}
  },
  "agentModelSorts": [
    {"displayName": "Recommended", "groups": [
      {"modelIds": ["gemini-4-argon-medium", "gemini-3.8-flash-high", "claude-opus-5-5-medium"]}
    ]}
  ],
  "imageGenerationModelIds": ["gemini-3.1-flash-image"]
}`

func mustParseCatalog(t *testing.T) *fetchAvailableModelsResponse {
	t.Helper()
	var r fetchAvailableModelsResponse
	if err := json.Unmarshal([]byte(catalogSample), &r); err != nil {
		t.Fatalf("解析 catalog 样例失败: %v", err)
	}
	return &r
}

// 发现接口拿到的模型清单必须只含「agy 真能选、上游真认」的条目。
// 漏了这层，agy 会退到内置目录并发 gemini-3.1-pro-preview（上游 404）。
func TestBuildModelsBuildsCatalog(t *testing.T) {
	m := buildModels(mustParseCatalog(t))

	want := []string{"gemini-4-argon-medium", "gemini-3.8-flash-high", "claude-opus-5-5-medium"}
	if len(m.Catalog) != len(want) {
		t.Fatalf("Catalog = %+v，想要 %v", m.Catalog, want)
	}
	for i, id := range want {
		if m.Catalog[i].ID != id {
			t.Errorf("Catalog[%d].ID = %q，想要 %q（按 agentModelSorts 排序）", i, m.Catalog[i].ID, id)
		}
	}

	first := m.Catalog[0]
	if first.DisplayName != "Gemini 4 Argon (Medium)" {
		t.Errorf("DisplayName = %q", first.DisplayName)
	}
	if first.InputTokenLimit != 1048576 || first.OutputTokenLimit != 65536 {
		t.Errorf("token 上限 = %d/%d", first.InputTokenLimit, first.OutputTokenLimit)
	}
	if !first.SupportsThinking {
		t.Error("supportsThinking 没带上，agy 的思考档位会退化")
	}
}

func TestBuildCatalogFiltersInternalAndImageModels(t *testing.T) {
	m := buildModels(mustParseCatalog(t))
	got := map[string]bool{}
	for _, c := range m.Catalog {
		got[c.ID] = true
	}
	for _, bad := range []string{
		"chat_20706",              // isInternal
		"tab_flash_lite_preview",  // 无 displayName
		"gemini-3.8-flash-tiered", // 别名目标，无 displayName
		"gemini-3.1-flash-image",  // 图像生成模型
	} {
		if got[bad] {
			t.Errorf("%q 不该出现在发现清单里", bad)
		}
	}
}

// 发现清单必须**严格等于** agentModelSorts 的成员。
//
// 实测踩过的坑：上游 models 表里有十几条 displayName 与 key 对不上的条目
// （gemini-3-flash-agent 显示为 "Gemini 3.5 Flash (High)"、gemini-2.5-flash
// 显示为 "Gemini 3.5 Flash Lite"）。把它们放进清单，agy 会按显示名反推 ID，
// 发出上游不存在的模型名，用户看到的是 "no longer available"。
// 真 agy 自己（agy models）也只列 agentModelSorts 这 13 个。
func TestBuildCatalogMatchesAgentModelSortsExactly(t *testing.T) {
	m := buildModels(mustParseCatalog(t))
	got := map[string]bool{}
	for _, c := range m.Catalog {
		got[c.ID] = true
	}
	// catalogSample 的 agentModelSorts 只有这三条。
	if len(got) != 3 {
		t.Fatalf("Catalog 应严格等于 agentModelSorts（3 条），实际 %d 条: %+v", len(got), m.Catalog)
	}
	for _, want := range []string{"gemini-4-argon-medium", "gemini-3.8-flash-high", "claude-opus-5-5-medium"} {
		if !got[want] {
			t.Errorf("Catalog 缺 %q", want)
		}
	}
}

// displayName 与 key 不符的条目即使合法也不能进清单。
func TestBuildCatalogSkipsMismatchedDisplayNames(t *testing.T) {
	const raw = `{
	  "models": {
	    "gemini-3-flash-agent": {"displayName": "Gemini 3.5 Flash (High)"},
	    "good-model":           {"displayName": "Good Model"}
	  },
	  "agentModelSorts": [
	    {"displayName": "Recommended", "groups": [
	      {"modelIds": ["good-model"]}
	    ]}
	  ]
	}`
	var r fetchAvailableModelsResponse
	if err := json.Unmarshal([]byte(raw), &r); err != nil {
		t.Fatalf("解析失败: %v", err)
	}
	m := buildModels(&r)
	if len(m.Catalog) != 1 || m.Catalog[0].ID != "good-model" {
		t.Errorf("Catalog = %+v，只该有 good-model", m.Catalog)
	}
}

// 上游没给 agentModelSorts（旧版形态）时才退回全部合法条目。
func TestBuildCatalogFallsBackWhenNoSorts(t *testing.T) {
	r := &fetchAvailableModelsResponse{
		Models: map[string]json.RawMessage{
			"b-model": []byte(`{"displayName":"B"}`),
			"a-model": []byte(`{"displayName":"A"}`),
			"c-model": []byte(`{"displayName":"C"}`),
		},
	}
	m := buildModels(r)
	want := []string{"a-model", "b-model", "c-model"}
	if len(m.Catalog) != len(want) {
		t.Fatalf("Catalog = %+v", m.Catalog)
	}
	for i, id := range want {
		if m.Catalog[i].ID != id {
			t.Errorf("Catalog[%d] = %q，想要 %q", i, m.Catalog[i].ID, id)
		}
	}
}
