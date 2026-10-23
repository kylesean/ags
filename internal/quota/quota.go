// Package quota 负责请求与解析 Antigravity 只读额度汇总接口 retrieveUserQuotaSummary。
//
// 该接口为只读端点，不消耗模型生成配额，可获取 GEMINI 模型的 5 小时与周度配额窗口及重置时刻。
//
// 分组结构：
//
//	groups[]
//	├─ displayName: "Gemini Models"
//	│  ├─ gemini-5h      remainingFraction, resetTime
//	│  └─ gemini-weekly  remainingFraction, resetTime
//	├─ displayName: "Gemini 4 Argon"
//	│  └─ gemini-argon-wtu-weekly   （只有周窗口，独立计量）
//	└─ ... (其他模型组忽略)
package quota

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/kylesean/ags/internal/i18n"
)

// MethodRetrieveUserQuotaSummary 是取额度汇总的方法路径。
const MethodRetrieveUserQuotaSummary = "/v1internal:retrieveUserQuotaSummary"

// GEMINI 组的分组名与两个窗口的 bucketId（实测值）。
//
// 用 bucketId 认组比用 displayName 更硬：displayName 是展示字段，
// 上游哪天改文案（换语言、加前缀）就会静默认不出整组，
// 而 bucketId 是键，改了等于换接口。
const (
	geminiGroup        = "Gemini Models"
	geminiBucket5h     = "gemini-5h"
	geminiBucketWeekly = "gemini-weekly"

	// Argon 是 2026-10 起独立计量的一组（默认 Agent 模型
	// gemini-4-argon-medium 就在里面）。它只有周窗口，没有 5h。
	geminiBucketArgonWeekly = "gemini-argon-wtu-weekly"
)

// maxBody 限制读入大小。实测额度汇总只有几 KB，留足余量。
const maxBody = 1 << 20

// 冷却时刻的两条兜底，见 Gemini.Exhausted。
const (
	// minCooldown 是「resetTime 不可用或已过期」时的最少退避时间。
	// 太短会变成热循环：Pick 跳过 → 立刻到期 → 又挑中同一个没额度的号。
	minCooldown = 30 * time.Second
	// resetGrace 是在 resetTime 之后再多等的时间，等上游把额度落账。
	// 没有它，reset 临界点上的请求会拿到 429。
	resetGrace = 5 * time.Second
)

// Bucket 是一个额度窗口。remainingFraction 可能缺失，
// 因此用指针区分「剩余 0」与「压根没这个字段」。
type Bucket struct {
	BucketID          string   `json:"bucketId"`
	DisplayName       string   `json:"displayName"`
	Window            string   `json:"window"`
	ResetTime         string   `json:"resetTime"`
	RemainingFraction *float64 `json:"remainingFraction"`
	Description       string   `json:"description"`
}

// Remaining 返回剩余比例，以及「上游到底给没给这个字段」。
//
// 字段缺失时返回 (0, false)，调用方必须当未知处理 ——
// 把缺失误读成 0 会把一个明明还有额度的账号冷冻掉，
// 那比漏判严重得多（漏判最多撞一次 429，误判是整个号白扔）。
func (b *Bucket) Remaining() (float64, bool) {
	if b == nil || b.RemainingFraction == nil {
		return 0, false
	}
	return *b.RemainingFraction, true
}

// Reset 解析 resetTime（RFC3339 UTC）。解析不出返回零值。
func (b *Bucket) Reset() time.Time {
	if b == nil || b.ResetTime == "" {
		return time.Time{}
	}
	t, err := time.Parse(time.RFC3339, b.ResetTime)
	if err != nil {
		return time.Time{}
	}
	return t.UTC()
}

// Group 是额度汇总里的一组模型。
type Group struct {
	DisplayName string   `json:"displayName"`
	Description string   `json:"description"`
	Buckets     []Bucket `json:"buckets"`
}

// Summary 是 retrieveUserQuotaSummary 的完整响应。
type Summary struct {
	Description string  `json:"description"`
	Groups      []Group `json:"groups"`
}

// Gemini 是我们要参与耗尽判定的额度窗口。三者都可能为 nil。
type Gemini struct {
	FiveHour *Bucket
	Weekly   *Bucket
	// Argon 是「Gemini 4 Argon」组的周窗口。默认 Agent 模型
	// （gemini-4-argon-medium）只吃这一份额度，主组再满也不代表能用。
	Argon *Bucket
}

// Gemini 从汇总里取出主 GEMINI 组，并单独抓 Argon 周窗口。
// 主组找不到返回 nil（只有 Argon 的账号仍会返回非 nil，见下）。
//
// 认组三步：先找含 bucketId gemini-5h 的组（键匹配，最硬）；
// 再退回 displayName 精确等于 "Gemini Models" 的；
// 最后才用 displayName 模糊含 "gemini" 的（兼容上游改组名）。
// 模糊这步必须排除 Argon 组 —— 它的 displayName 同样含 "gemini"，
// 而它只有周窗口，误认会把主组判成「只剩周额度」。
func (s *Summary) Gemini() *Gemini {
	if s == nil {
		return nil
	}
	g := s.findGeminiGroup()

	out := &Gemini{}
	if g != nil {
		out.FiveHour = pickBucket(g, geminiBucket5h, "5h")
		out.Weekly = pickBucket(g, geminiBucketWeekly, "weekly")
	}
	if ag := s.findArgonGroup(); ag != nil {
		out.Argon = pickBucket(ag, geminiBucketArgonWeekly, "weekly")
	}

	// 一个窗口都没找到才算「没有额度信息」。
	// 只有 Argon 的账号（未授权 Gemini 3.x）也要参与判定：
	// 它的默认模型正是 Argon。
	if out.FiveHour == nil && out.Weekly == nil && out.Argon == nil {
		return nil
	}
	return out
}

// findArgonGroup 找「Gemini 4 Argon」组：先按 bucketId，再按组名。
func (s *Summary) findArgonGroup() *Group {
	if s == nil {
		return nil
	}
	for i := range s.Groups {
		for _, b := range s.Groups[i].Buckets {
			if b.BucketID == geminiBucketArgonWeekly {
				return &s.Groups[i]
			}
		}
	}
	for i := range s.Groups {
		if strings.Contains(strings.ToLower(s.Groups[i].DisplayName), "argon") {
			return &s.Groups[i]
		}
	}
	return nil
}

// findGeminiGroup 按「bucketId 键匹配」→「displayName 精确」→「displayName 模糊含名」的顺序认组。
// 精确必须排在模糊之前，否则 "Gemini Models" 必先被模糊命中，精确分支永为死代码。
func (s *Summary) findGeminiGroup() *Group {
	for i := range s.Groups {
		g := &s.Groups[i]
		for _, b := range g.Buckets {
			if b.BucketID == geminiBucket5h {
				return g
			}
		}
	}
	// 精确组名兜底（实测就是这个值）。
	for i := range s.Groups {
		if s.Groups[i].DisplayName == geminiGroup {
			return &s.Groups[i]
		}
	}
	for i := range s.Groups {
		g := &s.Groups[i]
		name := strings.ToLower(g.DisplayName)
		if !strings.Contains(name, "gemini") {
			continue
		}
		// Argon 是独立计量的另一组，不能顶替主组。
		if strings.Contains(name, "argon") {
			continue
		}
		return g
	}
	return nil
}

// pickBucket 在组内先按 bucketId 找，再按 window 找（同一语义的两套叫法）。
func pickBucket(g *Group, bucketID, window string) *Bucket {
	for i := range g.Buckets {
		if g.Buckets[i].BucketID == bucketID {
			return &g.Buckets[i]
		}
	}
	for i := range g.Buckets {
		if g.Buckets[i].Window == window {
			return &g.Buckets[i]
		}
	}
	return nil
}

// Exhausted 判断额度是否已耗尽，并给出「该冷却到什么时候」。
//
// threshold 是耗尽判定线：remainingFraction <= threshold 就算耗尽。
// 默认 0，即「归零才切号」，不浪费剩余的一点点额度。
//
// 参与判定的是主组 5h + 主组周 + Argon 周：三个窗口任意一个见底，
// 默认模型就用不了，账号必须让位。
//
// 冷却时刻取所有耗尽窗口里**最晚**的 resetTime：
// 5h 归零但周窗口还满 → 只等 22 分钟；周窗口也归零了 → 得等一周。
// 取 max 才不会提前把号捞回来又撞一次 429。
//
// 三条兜底，缺一条就会退化成热循环：
//   - resetTime 解析不出（上游换格式）→ 走 minCooldown
//   - resetTime 已经过去（上游缓存滞后）→ 走 minCooldown
//   - 命中的 resetTime 再加 resetGrace，等上游落账
func (g *Gemini) Exhausted(threshold float64, now time.Time) (time.Time, bool) {
	if g == nil {
		return time.Time{}, false
	}
	var until time.Time
	hit := false
	for _, b := range []*Bucket{g.FiveHour, g.Weekly, g.Argon} {
		rem, ok := b.Remaining()
		if !ok || rem > threshold {
			continue // 字段缺失当未知，剩余量高于线就是没耗尽
		}
		hit = true
		if t := b.Reset(); t.After(until) {
			until = t
		}
	}
	if !hit {
		return time.Time{}, false
	}
	if floor := now.Add(minCooldown); until.Before(floor) {
		until = floor
	}
	return until.Add(resetGrace), true
}

// Describe 生成给日志看的一行摘要，例如
// 「5h 0.00% (重置 09-23 19:24), 周 99.99% (重置 09-30 15:52)」。
// Argon 有额度窗口时追加一段，便于解释「主组还有额度却切号」。
func (g *Gemini) Describe() string {
	if g == nil {
		return i18n.Tr("quota.notFound")
	}
	s := fmt.Sprintf("%s %s, %s %s",
		i18n.Tr("quota.fiveHour"), g.FiveHour.describe(),
		i18n.Tr("quota.week"), g.Weekly.describe())
	if g.Argon != nil {
		s += fmt.Sprintf(", %s %s", i18n.Tr("quota.argon"), g.Argon.describe())
	}
	return s
}

func (b *Bucket) describe() string {
	if b == nil {
		return i18n.Tr("common.dash")
	}
	rem, ok := b.Remaining()
	if !ok {
		return i18n.Tr("quota.unknown")
	}
	s := fmt.Sprintf("%.2f%%", rem*100)
	if t := b.Reset(); !t.IsZero() {
		s += " " + i18n.Trf("quota.resetAt", t.Local().Format("01-02 15:04"))
	}
	return s
}

// Fetch 调 retrieveUserQuotaSummary 取回额度汇总。
// upstream 必须带 scheme 与 host；accessToken 不能为空。
//
// 与 cloudcode.FetchModels 同样的三条请求纪律：Content-Type、
// Authorization、以及**许可证闸门 UA**。失败一律返回 error 而不是空汇总 ——
// 空汇总会被读成「没有 GEMINI 组」，从而静默不冷却。
func Fetch(ctx context.Context, upstream, accessToken, userAgent string) (*Summary, error) {
	if accessToken == "" {
		return nil, fmt.Errorf("access_token 为空")
	}
	if strings.HasSuffix(upstream, "/") {
		upstream = strings.TrimRight(upstream, "/")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		upstream+MethodRetrieveUserQuotaSummary, bytes.NewReader([]byte("{}")))
	if err != nil {
		return nil, fmt.Errorf("构造请求失败: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("User-Agent", userAgent)

	// http.DefaultClient 走 http.ProxyFromEnvironment —— 必须如此：
	// 本机到 Google 的直连不通，一切出网靠环境变量里的本地代理。
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("请求 %s 失败: %w", MethodRetrieveUserQuotaSummary, err)
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

	var sum Summary
	if err := json.Unmarshal(body, &sum); err != nil {
		return nil, fmt.Errorf("解析额度汇总失败: %w", err)
	}
	return &sum, nil
}

// clip 截断并压平超长文本，避免把上游整页错误写进日志。
func clip(b []byte, limit int) string {
	s := strings.Join(strings.Fields(string(b)), " ")
	if len(s) > limit {
		return s[:limit] + "…"
	}
	return s
}
