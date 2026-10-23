package cloudcode

import (
	"encoding/json"
	"testing"
)

// 兜底路径本身是必要的（旧版上游确实没有 agentModelSorts），
// 但它是**危险**的：models 表里有十几条 displayName 与 key 错位的条目，
// agy 会按显示名反推 ID，发出上游不存在的模型名，回一句
// "no longer available"。用户只会觉得「模型又坏了」。
//
// 所以兜底必须留下证据，让 serve 能在启动日志里喊出来。
// 这里用 Warnings 承载，而不是直接打日志：cloudcode 不该依赖 log.Logger，
// 也不该在纯函数里产生副作用（buildModels 要能用纯数据单测）。
func TestBuildModelsWarnsOnCatalogFallback(t *testing.T) {
	r := &fetchAvailableModelsResponse{
		Models: map[string]json.RawMessage{
			"a-model": []byte(`{"displayName":"A"}`),
			"b-model": []byte(`{"displayName":"B"}`),
		},
		// 上游没给 agentModelSorts：旧版形态。
	}
	m := buildModels(r)

	if len(m.Catalog) != 2 {
		t.Fatalf("兜底应仍给出清单，got %+v", m.Catalog)
	}
	if len(m.Warnings) == 0 {
		t.Fatal("走兜底却没有告警 —— 用户会静默用上错位的模型名")
	}
	if !containsAny(m.Warnings, "排序表", "agentModelSorts") {
		t.Errorf("告警应点明缺的是排序表，got %v", m.Warnings)
	}
}

// 上游给了排序表且能对上时不能告警（正常路径）。
func TestBuildModelsNoWarnWhenSortsMatch(t *testing.T) {
	m := buildModels(mustParseCatalog(t))
	if len(m.Warnings) != 0 {
		t.Errorf("正常路径不该告警: %v", m.Warnings)
	}
}

// 这是最需要警惕的形态：上游**给了** agentModelSorts，但里面每个 id
// 都不在（或不合法的）models 表里 —— 说明字段改名/结构变了。
// 此时兜底会把错位模型放进清单，必须明确告警，而不是当作「旧版上游」。
func TestBuildModelsWarnsOnMismatchedSorts(t *testing.T) {
	const raw = `{
	  "models": {
	    "gemini-3-flash-agent": {"displayName": "Gemini 3.5 Flash (High)"},
	    "gemini-2.5-flash":     {"displayName": "Gemini 3.5 Flash Lite"}
	  },
	  "agentModelSorts": [
	    {"displayName": "Recommended", "groups": [
	      {"modelIds": ["some-renamed-id", "another-renamed-id"]}
	    ]}
	  ]
	}`
	var r fetchAvailableModelsResponse
	if err := json.Unmarshal([]byte(raw), &r); err != nil {
		t.Fatalf("解析失败: %v", err)
	}
	m := buildModels(&r)

	if len(m.Catalog) == 0 {
		t.Fatal("应仍给出兜底清单，别让 agy 完全没模型可选")
	}
	if len(m.Warnings) == 0 {
		t.Fatal("排序表对不上却没有告警 —— 这正是上游改结构时的形态")
	}
	// 告警要能区分「缺表」和「表对不上」：后者更紧急。
	joined := joinWarnings(m.Warnings)
	if !containsAny([]string{joined}, "对不上", "不匹配", "一个都对不上") {
		t.Errorf("告警应说明是「排序表与模型表对不上」，got %q", joined)
	}
}

// 上游给了排序表、部分能对上：用能对上的，不告警。
// 局部错位（新模型还没进排序表）是正常演进，不该狼来了。
func TestBuildModelsPartialSortsNoWarn(t *testing.T) {
	const raw = `{
	  "models": {
	    "good-model":   {"displayName": "Good"},
	    "unlisted":     {"displayName": "Unlisted"}
	  },
	  "agentModelSorts": [
	    {"displayName": "Recommended", "groups": [{"modelIds": ["good-model"]}]}
	  ]
	}`
	var r fetchAvailableModelsResponse
	if err := json.Unmarshal([]byte(raw), &r); err != nil {
		t.Fatal(err)
	}
	m := buildModels(&r)
	if len(m.Catalog) != 1 || m.Catalog[0].ID != "good-model" {
		t.Errorf("Catalog = %+v，只该有排序表里能对上的", m.Catalog)
	}
	if len(m.Warnings) != 0 {
		t.Errorf("局部对得上不该告警: %v", m.Warnings)
	}
}

// 空模型表（拉表失败/上游异常）不该刷告警 —— 那是另一个故障，
// serve 那边已经有「获取模型表失败」的日志。
func TestBuildModelsEmptyTableNoWarn(t *testing.T) {
	if m := buildModels(&fetchAvailableModelsResponse{}); len(m.Warnings) != 0 {
		t.Errorf("空表不该有告警: %v", m.Warnings)
	}
}

func joinWarnings(in []string) string {
	out := ""
	for _, s := range in {
		out += s + " "
	}
	return out
}

func containsAny(haystack []string, needles ...string) bool {
	for _, h := range haystack {
		for _, n := range needles {
			if len(n) > 0 && len(h) >= len(n) {
				for i := 0; i+len(n) <= len(h); i++ {
					if h[i:i+len(n)] == n {
						return true
					}
				}
			}
		}
	}
	return false
}
