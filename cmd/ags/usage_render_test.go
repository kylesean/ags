package main

import (
	"strings"
	"testing"
	"time"

	"github.com/kylesean/ags/internal/quota"
)

func f64(v float64) *float64 { return &v }

// 进度条必须能表达三种极端值：满、空、越界钳制。
func TestUsageBar(t *testing.T) {
	cases := []struct {
		f      float64
		filled int
		note   string
	}{
		{1, barWidth, "满额"},
		{0, 0, "归零"},
		{0.5, 12, "一半"},
		{-0.5, 0, "负数钳到 0"},
		{1.5, barWidth, "超过 1 钳到满"},
	}
	for _, c := range cases {
		bar := usageBar(c.f)
		if !strings.HasPrefix(bar, "[") || !strings.HasSuffix(bar, "]") {
			t.Errorf("%s: 进度条 = %q 缺方括号", c.note, bar)
			continue
		}
		inner := bar[1 : len(bar)-1]
		// 中文/宽字符环境下宽度算错是常见坑：必须是 rune 计数而非字节数。
		runes := []rune(inner)
		if len(runes) != barWidth {
			t.Errorf("%s: 进度条内部 %d 格，想要 %d", c.note, len(runes), barWidth)
		}
		if got := strings.Count(inner, "█"); got != c.filled {
			t.Errorf("%s: 填充 %d 格，想要 %d（%q）", c.note, got, c.filled, bar)
		}
	}
}

// 与 agy 原生一致的时长文案：分/时/天三级，且向上取整。
func TestHumanizeReset(t *testing.T) {
	now := time.Date(2026, 10, 24, 12, 0, 0, 0, time.UTC)
	cases := []struct {
		in   time.Duration
		want string
	}{
		{30 * time.Second, "1m"},
		{45 * time.Minute, "45m"},
		{26*time.Hour + 12*time.Minute, "1d 2h"},
		{2 * time.Hour, "2h"},
		{90 * time.Minute, "1h 30m"},
		{6 * 24 * time.Hour, "6d"},
		{-time.Hour, ""},
	}
	for _, c := range cases {
		if got := humanizeReset(now.Add(c.in), now); got != c.want {
			t.Errorf("humanizeReset(+%s) = %q，想要 %q", c.in, got, c.want)
		}
	}
	if got := humanizeReset(time.Time{}, now); got != "" {
		t.Errorf("零值 = %q，想要空串", got)
	}
}

// 窗口标题与 agy 原生一致：靠 window/bucketId 认，而不是英文 displayName。
func TestWindowLabel(t *testing.T) {
	cases := []struct {
		b    quota.Bucket
		want string
	}{
		{quota.Bucket{Window: "5h", BucketID: "gemini-5h"}, "Five Hour Limit Remaining"},
		{quota.Bucket{Window: "weekly", BucketID: "gemini-weekly"}, "Weekly Limit Remaining"},
		// window 缺失时按 bucketId 后缀兜底
		{quota.Bucket{BucketID: "3p-5h"}, "Five Hour Limit Remaining"},
		{quota.Bucket{BucketID: "gemini-argon-wtu-weekly"}, "Weekly Limit Remaining"},
	}
	for _, c := range cases {
		if got := windowLabel(&c.b); got != c.want {
			t.Errorf("windowLabel(%+v) = %q，想要 %q", c.b, got, c.want)
		}
	}
}

// 有额度就说 available，归零就说还有多久刷新 —— 与 agy 一致。
func TestBucketStatus(t *testing.T) {
	now := time.Date(2026, 10, 24, 12, 0, 0, 0, time.UTC)
	if got := bucketStatus(&quota.Bucket{RemainingFraction: f64(0.5)}, now); got != "Quota available" {
		t.Errorf("有额度 = %q", got)
	}
	b := &quota.Bucket{RemainingFraction: f64(0), ResetTime: now.Add(90 * time.Minute).Format(time.RFC3339)}
	if got := bucketStatus(b, now); got != "Refreshes in 1h 30m" {
		t.Errorf("归零 = %q", got)
	}
	// 归零但没有 resetTime：不能空着，也不能说 available。
	if got := bucketStatus(&quota.Bucket{RemainingFraction: f64(0)}, now); got != "Refreshes soon" {
		t.Errorf("无 resetTime = %q", got)
	}
	// 字段缺失必须说 unknown，不能当成 0。
	if got := bucketStatus(&quota.Bucket{}, now); got != "Quota unknown" {
		t.Errorf("字段缺失 = %q", got)
	}
}

// 组标题全大写，与 agy 原生一致。
func TestGroupTitle(t *testing.T) {
	g := quota.Group{DisplayName: "Claude and GPT models"}
	if got := groupTitle(g); got != "CLAUDE AND GPT MODELS" {
		t.Errorf("groupTitle = %q", got)
	}
}

// 组顺序必须稳定：GEMINI → 3p → Argon，与 agy 原生排列一致。
// 不排序的话 Go map 迭代顺序会让每次输出都变。
func TestSortGroupsOrder(t *testing.T) {
	in := []quota.Group{
		{DisplayName: "Gemini 4 Argon"},
		{DisplayName: "Claude and GPT models"},
		{DisplayName: "Gemini Models"},
	}
	got := sortGroups(in)
	want := []string{"Gemini Models", "Claude and GPT models", "Gemini 4 Argon"}
	for i, name := range want {
		if got[i].DisplayName != name {
			t.Errorf("位置 %d = %q，想要 %q", i, got[i].DisplayName, name)
		}
	}
}

// 端到端渲染：把一份真实形状的汇总渲染出来，检查关键要素都在。
func TestRenderAccountUsage(t *testing.T) {
	now := time.Date(2026, 10, 24, 4, 0, 0, 0, time.UTC)
	sum := &quota.Summary{Groups: []quota.Group{
		{
			DisplayName: "Gemini Models",
			Description: "Models within this group: Gemini Flash, Gemini Pro",
			Buckets: []quota.Bucket{
				{BucketID: "gemini-weekly", Window: "weekly", RemainingFraction: f64(0.4744),
					ResetTime: now.Add(26*time.Hour + 12*time.Minute).Format(time.RFC3339)},
				{BucketID: "gemini-5h", Window: "5h", RemainingFraction: f64(1)},
			},
		},
		{
			DisplayName: "Gemini 4 Argon",
			Buckets: []quota.Bucket{
				{BucketID: "gemini-argon-wtu-weekly", Window: "weekly", RemainingFraction: f64(0.02),
					ResetTime: now.Add(6 * 24 * time.Hour).Format(time.RFC3339)},
			},
		},
	}}
	out := renderAccountUsage("A", "a@example.com", sum, now)

	for _, want := range []string{
		"Account: a@example.com",
		"GEMINI MODELS",
		"Models within this group: Gemini Flash, Gemini Pro",
		"Weekly Limit Remaining",
		"47.44%",
		"Quota available",
		"GEMINI 4 ARGON",
		"2.00%",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("渲染结果缺 %q:\n%s", want, out)
		}
	}

	// 组顺序：GEMINI 在 Argon 之前。
	gi := strings.Index(out, "GEMINI MODELS")
	ai := strings.Index(out, "GEMINI 4 ARGON")
	if gi < 0 || ai < 0 || gi > ai {
		t.Errorf("组顺序不对（GEMINI@%d Aragon@%d）:\n%s", gi, ai, out)
	}
}

// 归零的窗口必须给出「还有多久刷新」，这是用户最关心的信息。
func TestRenderAccountUsageShowsRefreshWhenEmpty(t *testing.T) {
	now := time.Date(2026, 10, 24, 4, 0, 0, 0, time.UTC)
	sum := &quota.Summary{Groups: []quota.Group{{
		DisplayName: "Gemini Models",
		Buckets: []quota.Bucket{{
			BucketID: "gemini-5h", Window: "5h", RemainingFraction: f64(0),
			ResetTime: now.Add(22 * time.Minute).Format(time.RFC3339),
		}},
	}}}
	out := renderAccountUsage("A", "a@x", sum, now)
	if !strings.Contains(out, "Refreshes in 22m") {
		t.Errorf("归零窗口缺刷新时间:\n%s", out)
	}
	if strings.Contains(out, "Quota available") {
		t.Errorf("归零窗口不该说 available:\n%s", out)
	}
}

// 没有额度组时要说清楚，不能静默打印一个空块。
func TestRenderAccountUsageEmptyGroups(t *testing.T) {
	out := renderAccountUsage("A", "a@x", &quota.Summary{}, time.Now())
	if !strings.Contains(out, "未找到任何额度组") {
		t.Errorf("空汇总 = %q", out)
	}
}
