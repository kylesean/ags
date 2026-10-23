package proxy

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/kylesean/ags/internal/pool"
)

// coolingPicker 返回一个全池冷却错误，模拟 5h 窗口刚见底的情形。
type coolingPicker struct{ until time.Time }

func (p *coolingPicker) Pick(context.Context) (*Account, error) {
	return nil, &pool.AllCoolingError{Until: p.until, Names: []string{"A", "B"}}
}

// 全池冷却时，客户端该被明确告知「等多久再来」，而不是收到一个
// 语义模糊的 503。agy 会把 Retry-After 透给用户，也会自己按它退避。
func TestAllCoolingReturnsRetryAfter(t *testing.T) {
	s := newTestServer(t, "http://127.0.0.1:1", &coolingPicker{until: time.Now().Add(90 * time.Second)})

	rec := httptest.NewRecorder()
	s.ServeHTTP(rec, httptest.NewRequest("POST", "http://gw/v1beta/models/m:streamGenerateContent", nil))

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("状态码 = %d，想要 503", rec.Code)
	}
	ra := rec.Header().Get("Retry-After")
	if ra == "" {
		t.Fatal("缺 Retry-After 头")
	}
	secs, err := strconv.Atoi(ra)
	if err != nil {
		t.Fatalf("Retry-After = %q 不是秒数", ra)
	}
	if secs < 1 || secs > 90 {
		t.Errorf("Retry-After = %d，应在 1..90 之间", secs)
	}

	var body struct {
		Error struct {
			Message string `json:"message"`
			Type    string `json:"type"`
		} `json:"error"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("响应不是 JSON: %v\n%s", err, rec.Body.String())
	}
	if !strings.Contains(body.Error.Message, "冷却") {
		t.Errorf("message = %q，应说明是冷却", body.Error.Message)
	}
}

// 普通失败（token 全废）不该给 Retry-After：等下去也不会好。
func TestNonCoolingFailureHasNoRetryAfter(t *testing.T) {
	s := newTestServer(t, "http://127.0.0.1:1", noAccountPicker{})

	rec := httptest.NewRecorder()
	s.ServeHTTP(rec, httptest.NewRequest("POST", "http://gw/x", nil))

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("状态码 = %d", rec.Code)
	}
	if ra := rec.Header().Get("Retry-After"); ra != "" {
		t.Errorf("不该有 Retry-After，得到 %q", ra)
	}
}

type noAccountPicker struct{}

func (noAccountPicker) Pick(context.Context) (*Account, error) {
	return nil, errors.New("没有可用账号: token 全废")
}

// 429 重试不能只看一次：账号池有 N 个号时，应该换到下一个继续试，
// 直到用完候选。旧实现 attempt >= 1 就放弃，多号池等于白养。
func Test429RetriesAcrossMultipleAccounts(t *testing.T) {
	var seen []string
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen = append(seen, r.Header.Get("Authorization"))
		if len(seen) < 3 {
			w.WriteHeader(http.StatusTooManyRequests)
			_, _ = io.WriteString(w, `{"error":{"message":"quota"}}`)
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(w, "data: {\"response\":{\"candidates\":[]}}\n\n")
	}))
	t.Cleanup(up.Close)

	s := newEnvelopeServer(t, up.URL, &rotatingPicker{tokens: []string{"T1", "T2", "T3", "T4", "T5"}})

	rec := httptest.NewRecorder()
	s.ServeHTTP(rec, httptest.NewRequest("POST", "http://gw/v1beta/models/m:streamGenerateContent?alt=sse",
		strings.NewReader(`{"contents":[]}`)))

	if len(seen) != 3 {
		t.Fatalf("上游被请求 %d 次，想要 3 次（换 3 个号）: %v", len(seen), seen)
	}
	if rec.Code != http.StatusOK {
		t.Errorf("最终状态码 = %d，想要 200", rec.Code)
	}
}

// 候选用完仍 429 时，把最后一次的 429 如实交给客户端，而不是无限重试。
func Test429StopsAfterCandidatesExhausted(t *testing.T) {
	var calls int
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = io.WriteString(w, `{"error":{"message":"quota"}}`)
	}))
	t.Cleanup(up.Close)

	s := newEnvelopeServer(t, up.URL, &rotatingPicker{tokens: []string{"T1", "T2", "T3", "T4", "T5"}})

	rec := httptest.NewRecorder()
	s.ServeHTTP(rec, httptest.NewRequest("POST", "http://gw/v1beta/models/m:streamGenerateContent?alt=sse",
		strings.NewReader(`{"contents":[]}`)))

	if calls < 2 {
		t.Errorf("只请求了 %d 次，应该换号重试到候选用完", calls)
	}
	if calls > 8 {
		t.Errorf("上游被请求 %d 次，重试没有上限", calls)
	}
	if rec.Code != http.StatusTooManyRequests {
		t.Errorf("状态码 = %d，想要 429", rec.Code)
	}
}

// 保证测试里用到的 picker 实现都满足接口（编译期断言，避免签名漂移）。
var (
	_ Picker = (*coolingPicker)(nil)
	_ Picker = noAccountPicker{}
	_        = log.New
	_        = strconv.Itoa
)
