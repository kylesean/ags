// cloudcode 是 daily-cloudcode-pa.googleapis.com 的最小客户端。
//
// 目前只做一件事：拉 fetchAvailableModels 并推导出「agy 网关模式的模型名 →
// 上游真实模型键」的别名表。
//
// 为什么需要它（实测）：
//
//	agy 网关模式发  /v1beta/models/gemini-3.8-flash:streamGenerateContent
//	上游只有键     gemini-3.8-flash-tiered
//	直接转过去      → 404 NOT_FOUND「Requested entity was not found.」
//
// 上游这份响应里没有现成的「plain 名 → tiered 名」映射，但两条规则都能推：
//
//  1. deprecatedModelIds 是官方重命名表（gemini-3.1-pro-high → gemini-pro-agent）
//  2. 以 -tiered 结尾的键，去掉后缀若不与已有键冲突，就是那个 plain 名的别名
//
// 这与 native agy 的做法同源：二进制里存在
// FetchAvailableModelsResponse 的 cache，以及 ResolveModel / resolveModelName。
package cloudcode

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
)

// MethodFetchAvailableModels 是取模型表的方法路径。
const MethodFetchAvailableModels = "/v1internal:fetchAvailableModels"

// maxBody 限制读入大小。实测完整响应 164962 字节，留足余量，
// 但上游真要回一整个页面也不会把内存吃光。
const maxBody = 8 << 20

// Models 是 fetchAvailableModels 的解析结果里我们关心的部分。
type Models struct {
	// Aliases 是「旧名/别名 → 上游真实键」，例如
	// gemini-3.8-flash → gemini-3.8-flash-tiered。
	Aliases map[string]string
	// Catalog 是给 agy 做模型发现用的清单（顺序即展示顺序）。
	Catalog []CatalogEntry
	// Warnings 是「能用，但很可能有坑」的提示，供 serve 打进启动日志。
	//
	// 为什么放返回值而不是直接打日志：buildModels 是纯函数，要能用纯数据
	// 单测；cloudcode 也不该绑死某个 log.Logger。调用方决定怎么说。
	Warnings []string
	// DefaultAgentModelID 是上游声明的默认模型键（实测 23 字节 =
	// "gemini-3.8-flash-tiered"），用来在启动日志里交叉验证别名推得对不对。
	DefaultAgentModelID string
	// Keys 是上游模型键总数，用于启动日志。
	Keys int
}

// CatalogEntry 是模型发现（GET /v1beta/models）里的一条。
//
// 为什么需要它：agy ≥1.4 启动时先向网关要模型列表（Tier 3）。网关回 404
// 它就退到内置默认目录，然后发 gemini-3.1-pro-preview 这类上游早已下线
// 的名字，表现为「一提问就 404 NOT_FOUND」。网关必须自己回答这份清单。
type CatalogEntry struct {
	ID               string
	DisplayName      string
	InputTokenLimit  int
	OutputTokenLimit int
	SupportsImages   bool
	SupportsThinking bool
}

// modelEntry 是 models 表里我们关心的字段。
type modelEntry struct {
	DisplayName      string `json:"displayName"`
	SupportsImages   bool   `json:"supportsImages"`
	SupportsThinking bool   `json:"supportsThinking"`
	MaxTokens        int    `json:"maxTokens"`
	MaxOutputTokens  int    `json:"maxOutputTokens"`
	APIProvider      string `json:"apiProvider"`
	IsInternal       bool   `json:"isInternal"`
}

type fetchAvailableModelsResponse struct {
	DefaultAgentModelID string                     `json:"defaultAgentModelId"`
	Models              map[string]json.RawMessage `json:"models"`
	DeprecatedModelIDs  map[string]struct {
		NewModelID string `json:"newModelId"`
	} `json:"deprecatedModelIds"`
	AgentModelSorts []struct {
		DisplayName string `json:"displayName"`
		Groups      []struct {
			ModelIDs []string `json:"modelIds"`
		} `json:"groups"`
	} `json:"agentModelSorts"`
	ImageGenerationModelIDs []string `json:"imageGenerationModelIds"`
}

// FetchModels 取回模型表并推导别名。upstream 必须带 scheme 与 host。
//
// 失败时返回 error 而不是空表：调用方（serve）把它当启动健康检查用 ——
// 拉不到模型表通常意味着 token、User-Agent 或上游三者有一个不对，
// 那时与其监听一个注定 404 的端口，不如立刻报出来。
func FetchModels(ctx context.Context, upstream, accessToken, userAgent string) (*Models, error) {
	if accessToken == "" {
		return nil, fmt.Errorf("access_token 为空")
	}
	if strings.HasSuffix(upstream, "/") {
		upstream = strings.TrimRight(upstream, "/")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		upstream+MethodFetchAvailableModels, bytes.NewReader([]byte("{}")))
	if err != nil {
		return nil, fmt.Errorf("构造请求失败: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+accessToken)
	// 许可证闸门：不带这个 UA 上游回 403，见 internal/proxy/upstreamUserAgent。
	req.Header.Set("User-Agent", userAgent)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("请求 %s 失败: %w", MethodFetchAvailableModels, err)
	}
	defer func() {
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
		_ = resp.Body.Close()
	}()

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxBody))
	if err != nil {
		return nil, fmt.Errorf("读响应失败: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("上游 %d: %s", resp.StatusCode, clip(body, 512))
	}

	var parsed fetchAvailableModelsResponse
	if err := json.Unmarshal(body, &parsed); err != nil {
		return nil, fmt.Errorf("解析模型表失败: %w", err)
	}
	return buildModels(&parsed), nil
}

// buildModels 从解析结果推导别名表。拆出来是为了能用纯数据做单测。
//
// 三条规则，作用域不同：
//
//  1. 官方重命名表 deprecatedModelIds：old → newModelId，**无条件**生效。
//     实测旧键往往仍列在 models 里（如 gemini-3.1-pro-high），那正是
//     「已弃用但还在列表」的形状，按「old 存在就跳过」会把整张表废掉。
//  2. -tiered 推导：gemini-3.8-flash-tiered 的 plain 名 gemini-3.8-flash
//     只有在上游**没有**同名键时才认作别名。这条是我们自己推的，标准要更严：
//     覆盖一个真实键会把请求悄悄换到别的模型，那比报 404 更糟。
//  3. 努力档位推导：agy 1.4.6 会把 "Gemini 4 Argon (Low)" 的选择摘成裸名
//     gemini-4-argon 发出来（档位改走 thinkingConfig），上游只有
//     gemini-4-argon-{low,medium,high}。没有这条，Argon 一族全是 404。
//
// 先跑官方表，再跑 tiered，最后跑档位；后两条不覆盖已有别名（先到先得），
// 也不覆盖真实键。
func buildModels(r *fetchAvailableModelsResponse) *Models {
	catalog, catalogWarnings := buildCatalog(r)
	aliases := make(map[string]string)

	// 规则 1：官方重命名，无条件。
	for old, entry := range r.DeprecatedModelIDs {
		if entry.NewModelID == "" || old == "" {
			continue
		}
		aliases[old] = entry.NewModelID
	}

	// 规则 2：-tiered 推导，且不得覆盖真实键或已有别名。
	for key := range r.Models {
		if !strings.HasSuffix(key, "-tiered") {
			continue
		}
		plain := strings.TrimSuffix(key, "-tiered")
		if plain == "" {
			continue
		}
		if _, exists := r.Models[plain]; exists {
			continue // plain 是真实键，不能覆盖
		}
		if _, taken := aliases[plain]; taken {
			continue // 官方表已接管
		}
		aliases[plain] = key
	}

	// 规则 3：努力档位（-low/-medium/-high）推导。
	for base, target := range effortAliases(r) {
		if _, exists := r.Models[base]; exists {
			continue // 裸名是真实键
		}
		if _, taken := aliases[base]; taken {
			continue // tiered / 官方表优先
		}
		aliases[base] = target
	}

	return &Models{
		Aliases:             aliases,
		Catalog:             catalog,
		Warnings:            catalogWarnings,
		DefaultAgentModelID: r.DefaultAgentModelID,
		Keys:                len(r.Models),
	}
}

// effortSuffixes 是 agy 摘掉后只留在模型名里的努力档位后缀。
// 顺序即偏好：上游没指定默认档时优先 medium（与 defaultAgentModelId
// 的实测取值一致），再退 high、low。
var effortSuffixes = []string{"medium", "high", "low"}

// effortAliases 推导「裸族名 → 具体档位键」。
//
// 只处理**同一族至少有两个档位**的名字：单档位的（如 gemini-3.5-flash-extra-low）
// 多半是历史遗留命名，凭空造一条 base 别名只会污染映射表。
func effortAliases(r *fetchAvailableModelsResponse) map[string]string {
	if r == nil || len(r.Models) == 0 {
		return nil
	}

	// family → suffix → 完整键
	families := make(map[string]map[string]string)
	for key := range r.Models {
		for _, suf := range effortSuffixes {
			full := "-" + suf
			if !strings.HasSuffix(key, full) {
				continue
			}
			base := strings.TrimSuffix(key, full)
			if base == "" {
				continue
			}
			if families[base] == nil {
				families[base] = make(map[string]string, len(effortSuffixes))
			}
			families[base][suf] = key
			break
		}
	}

	out := make(map[string]string, len(families))
	for base, tiers := range families {
		if len(tiers) < 2 {
			continue // 单档位不算一族
		}
		// 上游声明的默认模型若正好属于这一族，就以它为准。
		if def := r.DefaultAgentModelID; def != "" {
			if strings.HasPrefix(def, base+"-") {
				if _, ok := tiers[strings.TrimPrefix(def, base+"-")]; ok {
					out[base] = def
					continue
				}
			}
		}
		for _, suf := range effortSuffixes {
			if key, ok := tiers[suf]; ok {
				out[base] = key
				break
			}
		}
	}
	return out
}

// buildCatalog 生成给 agy 的模型发现清单。
//
// 关键结论（对着实测数据定）：只有 agentModelSorts 里列出的模型才是
// agy 该展示、上游真能用的。models 表里另外十几条不能用：
//
//   - isInternal 的 chat_/tab_ 是 CLI 内部用途；
//   - 没有 displayName 的是别名目标（gemini-3.8-flash-tiered）；
//   - 图像生成模型（imageGenerationModelIds）走另一条链路；
//   - 更要命的是**有 displayName 但名字与 key 不符**的一批：
//     gemini-3-flash-agent 的 displayName 是 "Gemini 3.5 Flash (High)"，
//     gemini-2.5-flash 是 "Gemini 3.5 Flash Lite"……agy 会按显示名反推
//     ID，于是选 "Gemini 3.5 Flash (High)" 时发出上游并不存在的
//     gemini-3.5-flash-high，回一句 "no longer available"。
//
// 真 agy 自己也只展示 agentModelSorts 那 13 个（实测 agy models）。
// 所以这里**以 agentModelSorts 为准**，不再兜底补全其余模型。
//
// 只有上游没给 agentModelSorts 时才退回「全部合法条目按 ID 排序」，
// 那是旧版上游的形态。但这条兜底路径**有坑**（见上），所以必须回报
// 一条 Warnings 让 serve 喊出来：静默用上错位模型名，用户只会觉得
// 「模型又坏了」，而日志里一行错都没有。
//
// 返回值第二个是告警列表（可能为空）。
func buildCatalog(r *fetchAvailableModelsResponse) ([]CatalogEntry, []string) {
	if r == nil || len(r.Models) == 0 {
		return nil, nil
	}

	entries := make(map[string]CatalogEntry, len(r.Models))
	imageOnly := make(map[string]bool, len(r.ImageGenerationModelIDs))
	for _, id := range r.ImageGenerationModelIDs {
		imageOnly[id] = true
	}

	for id, raw := range r.Models {
		if imageOnly[id] {
			continue
		}
		var e modelEntry
		if err := json.Unmarshal(raw, &e); err != nil {
			continue
		}
		if e.IsInternal || e.DisplayName == "" {
			continue
		}
		entries[id] = CatalogEntry{
			ID:               id,
			DisplayName:      e.DisplayName,
			InputTokenLimit:  e.MaxTokens,
			OutputTokenLimit: e.MaxOutputTokens,
			SupportsImages:   e.SupportsImages,
			SupportsThinking: e.SupportsThinking,
		}
	}
	if len(entries) == 0 {
		return nil, nil
	}

	// 首选：严格按 agentModelSorts 的顺序与成员，一个不多。
	out := make([]CatalogEntry, 0, len(entries))
	seen := make(map[string]bool, len(entries))
	sortIDs := 0
	for _, sort := range r.AgentModelSorts {
		for _, g := range sort.Groups {
			for _, id := range g.ModelIDs {
				sortIDs++
				if seen[id] {
					continue
				}
				if e, ok := entries[id]; ok {
					out = append(out, e)
					seen[id] = true
				}
			}
		}
	}
	if len(out) > 0 {
		return out, nil
	}

	// 兜底：上游没给排序表（旧版形态）或给了但对不上（结构变了）。
	ids := make([]string, 0, len(entries))
	for id := range entries {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		out = append(out, entries[id])
	}

	// 两种兜底的危险程度不同，告警也要分开说：
	//   - 没给排序表：可能是旧版上游，尚可接受
	//   - 给了却一条都对不上：上游改了字段名/结构，几乎肯定要出问题
	var warnings []string
	switch {
	case sortIDs > 0:
		warnings = append(warnings, fmt.Sprintf(
			"上游的 agentModelSorts 里有 %d 个 id，但与 models 表一个都对不上"+
				"（上游很可能改了字段名或结构）；已退回按 id 排序的 %d 个模型，"+
				"其中可能有 displayName 与 key 错位的条目 —— agy 会按显示名反推 id，"+
				"选到这类模型会得到 no longer available",
			sortIDs, len(out)))
	default:
		warnings = append(warnings, fmt.Sprintf(
			"上游没有返回 agentModelSorts（旧版上游？），已退回按 id 排序的 %d 个模型；"+
				"其中可能有 displayName 与 key 错位的条目 —— agy 会按显示名反推 id，"+
				"选到这类模型会得到 no longer available",
			len(out)))
	}
	return out, warnings
}

// clip 截断并压平超长文本，避免把上游整页错误写进日志。
func clip(b []byte, limit int) string {
	s := strings.Join(strings.Fields(string(b)), " ")
	if len(s) > limit {
		return s[:limit] + "…"
	}
	return s
}
