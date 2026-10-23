package quota

import (
	"strings"
	"testing"
	"time"
)

// 旧的三级认组会把 Argon 组当成主 GEMINI 组：主组缺失时（未授权 3.x
// 的账号）模糊匹配命中 Argon，于是 Argon 的周额度被塞进 Weekly 槽位，
// 冷却判定就变成「主组周额度 2%」——日志里也看不出是哪个组。
func TestGeminiKeepsArgonOutOfMainWindows(t *testing.T) {
	raw := `{"groups":[
	  {"displayName":"Gemini 4 Argon","description":"Models within this group: Gemini 4 Argon",
	   "buckets":[{"bucketId":"gemini-argon-wtu-weekly","window":"weekly","remainingFraction":0.02,
	               "resetTime":"2026-10-30T14:29:46Z"}]}
	]}`
	g := mustParse(t, raw).Gemini()
	if g == nil {
		t.Fatal("只有 Argon 组时也应拿到 Argon 窗口，而不是整体判成无额度信息")
	}
	if g.FiveHour != nil || g.Weekly != nil {
		t.Errorf("Argon 的周额度被塞进主组槽位: 5h=%+v 周=%+v", g.FiveHour, g.Weekly)
	}
	if g.Argon == nil || g.Argon.BucketID != geminiBucketArgonWeekly {
		t.Errorf("Argon 窗口 = %+v", g.Argon)
	}
}

// 只有 Argon 组时，Argon 见底必须判成耗尽（默认模型就用它）。
func TestArgonOnlyAccountExhausts(t *testing.T) {
	now := time.Date(2026, 10, 23, 12, 0, 0, 0, time.UTC)
	raw := `{"groups":[{"displayName":"Gemini 4 Argon","buckets":[
	  {"bucketId":"gemini-argon-wtu-weekly","window":"weekly","remainingFraction":0}]}]}`
	g := mustParse(t, raw).Gemini()
	if _, exhausted := g.Exhausted(0, now); !exhausted {
		t.Error("Argon 归零却没判成耗尽")
	}
}

// Argon 是独立额度：它耗尽时该号也接不了默认模型，必须参与冷却判定。
// 但它不能占用主组的 5h / 周窗口。
func TestGeminiCapturesArgonBucket(t *testing.T) {
	raw := `{"groups":[
	  {"displayName":"Gemini Models","buckets":[
	    {"bucketId":"gemini-weekly","window":"weekly","remainingFraction":0.8,"resetTime":"2026-10-30T14:25:43Z"},
	    {"bucketId":"gemini-5h","window":"5h","remainingFraction":1,"resetTime":"2026-10-23T19:25:43Z"}]},
	  {"displayName":"Gemini 4 Argon","buckets":[
	    {"bucketId":"gemini-argon-wtu-weekly","window":"weekly","remainingFraction":0.4,"resetTime":"2026-10-30T14:29:46Z"}]}
	]}`
	g := mustParse(t, raw).Gemini()
	if g == nil {
		t.Fatal("主 GEMINI 组没认出来")
	}
	if g.Weekly == nil || g.Weekly.BucketID != geminiBucketWeekly {
		t.Fatalf("周窗口 = %+v，不该是 Argon 的 bucket", g.Weekly)
	}
	if g.Argon == nil {
		t.Fatal("没抓到 Argon 周额度")
	}
	if rem, ok := g.Argon.Remaining(); !ok || rem != 0.4 {
		t.Errorf("Argon 剩余 = (%v,%v)，want (0.4,true)", rem, ok)
	}
}

// 主组还有额度但 Argon 归零 → 默认模型用不了，仍算耗尽，且冷却到 Argon 重置。
func TestExhaustedIncludesArgon(t *testing.T) {
	now := time.Date(2026, 10, 23, 12, 0, 0, 0, time.UTC)
	five := 1.0
	week := 0.9
	argon := 0.0
	argonReset := now.Add(6 * 24 * time.Hour)
	g := &Gemini{
		FiveHour: &Bucket{BucketID: geminiBucket5h, RemainingFraction: &five},
		Weekly:   &Bucket{BucketID: geminiBucketWeekly, RemainingFraction: &week},
		Argon: &Bucket{
			BucketID:          geminiBucketArgonWeekly,
			RemainingFraction: &argon,
			ResetTime:         argonReset.Format(time.RFC3339),
		},
	}

	until, exhausted := g.Exhausted(0, now)
	if !exhausted {
		t.Fatal("Argon 归零却没判成耗尽")
	}
	want := argonReset.Add(resetGrace)
	if !until.Equal(want) {
		t.Errorf("冷却至 %v，want %v", until, want)
	}
}

// Argon 有额度时不能影响主组判定。
func TestArgonWithQuotaDoesNotExhaust(t *testing.T) {
	now := time.Date(2026, 10, 23, 12, 0, 0, 0, time.UTC)
	one := 1.0
	g := &Gemini{
		FiveHour: &Bucket{BucketID: geminiBucket5h, RemainingFraction: &one},
		Weekly:   &Bucket{BucketID: geminiBucketWeekly, RemainingFraction: &one},
		Argon:    &Bucket{BucketID: geminiBucketArgonWeekly, RemainingFraction: &one},
	}
	if _, exhausted := g.Exhausted(0, now); exhausted {
		t.Error("全部有额度却判成耗尽")
	}
}

// Describe 要能看出 Argon 这一路：切号日志只打这一行。
func TestDescribeMentionsArgon(t *testing.T) {
	raw := `{"groups":[
	  {"displayName":"Gemini Models","buckets":[
	    {"bucketId":"gemini-weekly","window":"weekly","remainingFraction":0.8},
	    {"bucketId":"gemini-5h","window":"5h","remainingFraction":1}]},
	  {"displayName":"Gemini 4 Argon","buckets":[
	    {"bucketId":"gemini-argon-wtu-weekly","window":"weekly","remainingFraction":0.4}]}
	]}`
	g := mustParse(t, raw).Gemini()
	if g == nil {
		t.Fatal("主组没认出来")
	}
	desc := g.Describe()
	for _, want := range []string{"Argon", "40.00%"} {
		if !strings.Contains(desc, want) {
			t.Errorf("Describe() = %q 缺 %q", desc, want)
		}
	}
}
