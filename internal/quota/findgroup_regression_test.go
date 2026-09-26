package quota

import (
	"encoding/json"
	"testing"
)

// RED: 精确组名应优先于模糊含名组（第三轮现为死代码）。
// 两组都无 bucketId gemini-5h（首轮空），真组靠 window 名可辨，
// 旧顺序模糊轮先命中 Fake 导致 Gemini() 整体落空。
func TestFindGeminiGroupPrefersExact(t *testing.T) {
	var s Summary
	body := `{"groups":[
		{"displayName":"Fake Gemini Addon","buckets":[{"bucketId":"other","window":"other"}]},
		{"displayName":"Gemini Models","buckets":[
			{"bucketId":"v1-5h","window":"5h","remainingFraction":0.5},
			{"bucketId":"v1-weekly","window":"weekly","remainingFraction":0.5}]}
	]}`
	if err := json.Unmarshal([]byte(body), &s); err != nil {
		t.Fatal(err)
	}
	g := s.Gemini()
	if g == nil || g.FiveHour == nil || g.Weekly == nil {
		t.Fatalf("应定位到真正的 Gemini Models 组（经 window 兜底），got %+v", g)
	}
}
