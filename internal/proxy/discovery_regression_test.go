package proxy

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// agy ≥1.4 的模型发现走 Tier 3：先 GET <gateway>/v1beta/models?pageSize=1000。
// 拿不到就退到 Tier 4「内置默认目录」，然后发 gemini-3.1-pro-preview 这种
// 上游早已不认的名字 —— 表现为「一提问就 404 NOT_FOUND」，且 serve 日志里
// 只有一行 404，看不出是发现失败引起的。
//
// 所以网关必须自己回答发现请求。
func TestServesModelDiscoveryWhenEnvelopeMode(t *testing.T) {
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("发现请求不该被转发到上游: %s %s", r.Method, r.URL.Path)
	}))
	t.Cleanup(up.Close)

	s := newEnvelopeServer(t, up.URL, Static(&Account{AccessToken: "T"}))
	s.SetModelCatalog([]CatalogEntry{
		{ID: "gemini-4-argon-medium", DisplayName: "Gemini 4 Argon (Medium)",
			InputTokenLimit: 1048576, OutputTokenLimit: 65536, SupportsThinking: true},
		{ID: "claude-opus-5-5-medium", DisplayName: "Claude Opus 5.5 (Medium)",
			InputTokenLimit: 1000000, OutputTokenLimit: 128000},
	})

	req := httptest.NewRequest("GET", "http://gw/v1beta/models?pageSize=1000", nil)
	// 与实测 agy 1.4.6 的 Tier-3 请求同构。
	req.Header.Set("User-Agent", "antigravity/cli/1.4.6 (aidev_client; os_type=linux; arch=amd64)")
	rec := httptest.NewRecorder()
	s.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("状态码 = %d, body=%s", rec.Code, rec.Body.String())
	}
	if ct := rec.Header().Get("Content-Type"); !strings.Contains(ct, "application/json") {
		t.Errorf("Content-Type = %q", ct)
	}

	var got struct {
		Models []struct {
			Name                       string   `json:"name"`
			DisplayName                string   `json:"displayName"`
			SupportedGenerationMethods []string `json:"supportedGenerationMethods"`
			InputTokenLimit            int      `json:"inputTokenLimit"`
			OutputTokenLimit           int      `json:"outputTokenLimit"`
		} `json:"models"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("响应不是合法 JSON: %v\n%s", err, rec.Body.String())
	}
	if len(got.Models) != 2 {
		t.Fatalf("模型数 = %d，想要 2: %s", len(got.Models), rec.Body.String())
	}
	first := got.Models[0]
	// agy 对 name 同时接受 "models/x" 与裸 "x"；用裸名最省事，但要保证不加前缀。
	if first.Name != "gemini-4-argon-medium" {
		t.Errorf("name = %q，想要裸模型名", first.Name)
	}
	if first.DisplayName != "Gemini 4 Argon (Medium)" {
		t.Errorf("displayName = %q", first.DisplayName)
	}
	if len(first.SupportedGenerationMethods) == 0 {
		t.Error("缺 supportedGenerationMethods，agy 会当成不可用模型")
	}
	if first.InputTokenLimit != 1048576 || first.OutputTokenLimit != 65536 {
		t.Errorf("token 上限 = %d/%d", first.InputTokenLimit, first.OutputTokenLimit)
	}
}

// agy 1.4.6 会连发 /v1beta/models 与 /v1/models 两条发现请求。
// 只答 Gemini 那条不够：less 日志里 OpenAI 那条拿到 404 时，
// 它在某些 wireProtocol 下会以 OpenAI 格式解析失败并退回内置目录。
func TestServesOpenAICompatibleDiscovery(t *testing.T) {
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("发现请求不该被转发到上游: %s %s", r.Method, r.URL.Path)
	}))
	t.Cleanup(up.Close)

	s := newEnvelopeServer(t, up.URL, Static(&Account{AccessToken: "T"}))
	s.SetModelCatalog([]CatalogEntry{
		{ID: "gemini-4-argon-medium", DisplayName: "Gemini 4 Argon (Medium)"},
		{ID: "claude-opus-5-5-medium", DisplayName: "Claude Opus 5.5 (Medium)"},
	})

	rec := httptest.NewRecorder()
	s.ServeHTTP(rec, httptest.NewRequest("GET", "http://gw/v1/models", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("状态码 = %d", rec.Code)
	}
	var got struct {
		Object string `json:"object"`
		Data   []struct {
			ID     string `json:"id"`
			Object string `json:"object"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("响应不是合法 JSON: %v\n%s", err, rec.Body.String())
	}
	if got.Object != "list" {
		t.Errorf("object = %q，OpenAI 兼容格式要求 list", got.Object)
	}
	if len(got.Data) != 2 || got.Data[0].ID != "gemini-4-argon-medium" {
		t.Errorf("data = %+v", got.Data)
	}
	if got.Data[0].Object != "model" {
		t.Errorf("data[0].object = %q", got.Data[0].Object)
	}
}

// 目录为空时必须原样透传（例如 -no-model-alias 或拉表失败），
// 绝不能自己编一份出来 —— 那会把请求导向不存在的模型。
func TestDiscoveryPassesThroughWithoutCatalog(t *testing.T) {
	var gotPath string
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"models":[]}`)
	}))
	t.Cleanup(up.Close)

	s := newEnvelopeServer(t, up.URL, Static(&Account{AccessToken: "T"}))

	rec := httptest.NewRecorder()
	s.ServeHTTP(rec, httptest.NewRequest("GET", "http://gw/v1beta/models", nil))

	if gotPath != "/v1beta/models" {
		t.Errorf("目录为空时应透传，上游收到 %q", gotPath)
	}
}

// 只认 GET /v1beta/models；:streamGenerateContent 这类生成路径不能被劫持。
func TestDiscoveryOnlyInterceptsModelList(t *testing.T) {
	var gotPath string
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(w, `data: {"response":{"candidates":[]}}`+"\n\n")
	}))
	t.Cleanup(up.Close)

	s := newEnvelopeServer(t, up.URL, Static(&Account{AccessToken: "T"}))
	s.SetModelCatalog([]CatalogEntry{{ID: "m", DisplayName: "M"}})

	req := httptest.NewRequest("POST", "http://gw/v1beta/models/m:streamGenerateContent?alt=sse",
		strings.NewReader(`{"contents":[]}`))
	req.Header.Set("Content-Type", "application/json")
	s.ServeHTTP(httptest.NewRecorder(), req)

	if gotPath != "/v1internal:streamGenerateContent" {
		t.Errorf("生成请求被劫持了，上游收到 %q", gotPath)
	}
}

// 关掉信封（-passthrough）时就不该接管发现：那是纯透传调试模式。
func TestDiscoveryDisabledInPassthrough(t *testing.T) {
	var gotPath string
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"models":[]}`)
	}))
	t.Cleanup(up.Close)

	s := newTestServer(t, up.URL, Static(&Account{AccessToken: "T"}))
	s.SetModelCatalog([]CatalogEntry{{ID: "m", DisplayName: "M"}})

	s.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("GET", "http://gw/v1beta/models", nil))

	if gotPath != "/v1beta/models" {
		t.Errorf("passthrough 模式不该接管发现，上游收到 %q", gotPath)
	}
}
