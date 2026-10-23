// 信封改写：将客户端 Gemini REST 请求翻译为 CloudCode v1internal 信封，并将响应还原回客户端期待的格式。
//
//	客户端: POST /v1beta/models/{model}:{method}?alt=sse
//	        body:   {contents, systemInstruction, ...}
//	上游:   POST /v1internal:{method}?alt=sse
//	        body:   {"model":"<model>","request":<原 body 字节>}
//	响应:   解开上游 data: {"response":{candidates...}} 为 data: {candidates...}
package proxy

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"runtime"
	"strings"
)

// upstreamUserAgent 是访问上游 CloudCode 必须携带的客户端标识（许可证闸门）。
//
// 格式与约束都是实测的（2026-10-23）：
//
//	antigravity/cli/1.4.6 (aidev_client; os_type=linux; arch=amd64)
//
// 上游按这个头决定「你算哪一代客户端」，进而决定返回哪些模型与哪些额度组。
// 旧格式 antigravity-cli/1.2.9 仍然能用，但会被静默降级：模型表里没有
// Gemini 4 Argon，额度汇总里干脆没有 "Gemini Models" 组 —— ags 于是永远
// 显示「未找到 GEMINI 组」，自动切号失效。没有任何报错，这是最难查的一种。
//
// 所以这里必须跟着 agy 的版本走，见 DefaultUpstreamUserAgent。
const upstreamUserAgentPrefix = "antigravity/cli/"

// defaultUpstreamVersion 是内置兜底版本。取一个确认可用的值，
// 而不是 "0"：版本号过旧会重新掉回被降级的那一档。
const defaultUpstreamVersion = "1.4.6"

// UpstreamUserAgent 拼出当前格式的上游 UA。
//
// version 传 agy 的真实版本（见 cmd/ags 的 detectAgyVersion）；空则用兜底值。
// os/arch 用 GOOS/GOARCH，跟 agy 自报的写法一致。
func UpstreamUserAgent(goos, goarch, version string) string {
	version = strings.TrimSpace(version)
	if version == "" {
		version = defaultUpstreamVersion
	}
	if goos == "" {
		goos = "linux"
	}
	if goarch == "" {
		goarch = "amd64"
	}
	return upstreamUserAgentPrefix + version +
		" (aidev_client; os_type=" + goos + "; arch=" + goarch + ")"
}

// DefaultUpstreamUserAgent 是 serve/daemon 在没拿到 agy 版本时的默认值。
func DefaultUpstreamUserAgent() string {
	return UpstreamUserAgent(runtime.GOOS, runtime.GOARCH, "")
}

// geminiPathPrefix 是 agy 网关模式发来的生成请求路径前缀。
const geminiPathPrefix = "/v1beta/models/"

// modelsDiscoveryPath 是 agy ≥1.4 的模型发现路径：
//
//	GET /v1beta/models?pageSize=1000
//
// 上游 CloudCode 没有这个路由，必须由网关自己回答，见 serveModelDiscovery。
// 注意它和 geminiPathPrefix 只差一个末尾斜杠，别把两者混用。
const modelsDiscoveryPath = "/v1beta/models"

// openAIModelsDiscoveryPath 是同一件事的 OpenAI 兼容写法。
// agy 1.4.6 实测会连发两条（先 Gemini 再 OpenAI），哪个先答就用哪个。
const openAIModelsDiscoveryPath = "/v1/models"

// v1internalPrefix 是 cloudcode 信封方法的路径前缀。
const v1internalPrefix = "/v1internal:"

// parseGeminiPath 拆开 agy 网关模式的路径：
//
//	/v1beta/models/gemini-3.1-flash-lite:streamGenerateContent
//	  → model="gemini-3.1-flash-lite", method="streamGenerateContent"
//
// 不匹配的路径返回 ok=false，由调用方决定原样透传还是报错。
func parseGeminiPath(p string) (model, method string, ok bool) {
	if !strings.HasPrefix(p, geminiPathPrefix) {
		return "", "", false
	}
	rest := strings.TrimPrefix(p, geminiPathPrefix)
	colon := strings.IndexByte(rest, ':')
	if colon <= 0 || colon == len(rest)-1 {
		return "", "", false
	}
	model, method = rest[:colon], rest[colon+1:]
	// method 里不该再有斜杠；出现了说明这不是生成路径（例如模型名里带了冒号）。
	if strings.ContainsAny(model, "/") || strings.Contains(method, "/") || strings.Contains(model, ":") {
		return "", "", false
	}
	return model, method, true
}

// buildEnvelope 把 agy 的 body 套进 {"model":...,"request":...}。
//
// 内层逐字节原样嵌入（不重新 Unmarshal/Marshal），这样温度、token 数、
// thinkingBudget=-1 这些值不会被重编码成另一个字面量 ——
// 上游收到的必须与 agy 原意完全一致。
//
// 只先校验内层是合法 JSON 对象：不合法就尽早报错，
// 免得上游回一个难懂的 INVALID_ARGUMENT。
func buildEnvelope(model string, inner []byte) ([]byte, error) {
	if len(inner) == 0 {
		inner = []byte("{}")
	}
	var probe map[string]json.RawMessage
	if err := json.Unmarshal(inner, &probe); err != nil {
		return nil, fmt.Errorf("内层请求体不是 JSON 对象: %w", err)
	}
	m, err := json.Marshal(model)
	if err != nil {
		return nil, fmt.Errorf("序列化模型名失败: %w", err)
	}
	out := make([]byte, 0, len(inner)+len(m)+32)
	out = append(out, `{"model":`...)
	out = append(out, m...)
	out = append(out, `,"request":`...)
	out = append(out, inner...)
	out = append(out, '}')
	return out, nil
}

// unwrapEnvelope 拆掉上游信封，取出其中的 response 对象。
//
// 上游形状：{"response":{candidates...},"traceId":"...","metadata":{}}
// agy 期待：{candidates...}   （标准 Gemini GenerateContentResponse）
//
// traceId / metadata 直接丢弃：agy 的解析器不认识它们，
// 而且把它们并进顶层会让「哪个字段来自上游」变得含糊。
// 找不到 response 或它不是对象时返回 ok=false，调用方原样放行 ——
// 宁可多透传一段，也不能把一个正常响应改坏。
func unwrapEnvelope(s string) (string, bool) {
	var m map[string]json.RawMessage
	if err := json.Unmarshal([]byte(s), &m); err != nil {
		return "", false
	}
	inner, ok := m["response"]
	if !ok || len(inner) == 0 {
		return "", false
	}
	var probe map[string]json.RawMessage
	if err := json.Unmarshal(inner, &probe); err != nil {
		return "", false
	}
	return string(inner), true
}

// unwrapSSELine 改写一条 SSE 行。
//
// 只认 "data:" 开头的行；event:/id:/注释/空行原样放行。
// 解不开（非 JSON、没有 response 字段）时返回 ok=false，
// 由调用方保留原始行。
//
// 返回值不含行尾换行，是否补回由调用方按原行决定 ——
// EOF 处的最后一行本来就没有换行，不能凭空给它加一个。
func unwrapSSELine(line string) (string, bool) {
	if !strings.HasPrefix(line, "data:") {
		return "", false
	}
	payload := strings.TrimSpace(line[len("data:"):])
	// data: [DONE] 是常见终止哨兵，不是 JSON，保留原样。
	if payload == "" || payload == "[DONE]" {
		return "", false
	}
	out, ok := unwrapEnvelope(payload)
	if !ok {
		return "", false
	}
	return "data: " + out, true
}

// unwrapJSONBody 解开非流式 JSON 响应的信封。
//
// 上游对 :generateContent（不带 alt=sse）回 {"response":{...}}，
// agy 期待顶层就是 {candidates...}。limit 防止上游抽风时把内存吃光。
func unwrapJSONBody(b []byte, limit int64) ([]byte, bool) {
	if int64(len(b)) > limit {
		return nil, false
	}
	trimmed := strings.TrimSpace(string(b))
	out, ok := unwrapEnvelope(trimmed)
	if !ok {
		return nil, false
	}
	return []byte(out), true
}

// sseUnwrapper 是包在上游响应体外的按行改写读取器。
//
// 必须按行 yield：它下面就是 httputil.ReverseProxy 的
// FlushInterval=-1 实时刷写路径。一旦在这里攒够整段再返回，
// 59KB 上下文的流式回答就会被卡在缓冲里，表现为 agy 卡住不吐字
// （而上游还在等我们读）——那正是我们费劲用 ReverseProxy 规避的坑。
type sseUnwrapper struct {
	br      *bufio.Reader
	src     io.Closer // 底下的上游响应体，Close 时要透传下去
	pending []byte    // 当前行改写后比调用方缓冲还长时的余量
	changed int       // 改写过的行数，供测试与日志确认它确实工作了
}

func newSSEUnwrapper(rc io.ReadCloser) *sseUnwrapper {
	return &sseUnwrapper{br: bufio.NewReader(rc), src: rc}
}

func (u *sseUnwrapper) Read(p []byte) (int, error) {
	for len(u.pending) == 0 {
		line, err := u.br.ReadString('\n')
		if line != "" {
			hadNewline := strings.HasSuffix(line, "\n")

			// 统一归一化为 LF 换行，防止上游 CRLF 与改写后的 LF 产生混合换行导致客户端解析失败。
			core := strings.TrimSuffix(strings.TrimSuffix(line, "\n"), "\r")
			if out, ok := unwrapSSELine(core); ok {
				core = out
				u.changed++
			}
			line = core
			if hadNewline {
				line += "\n"
			}
			u.pending = []byte(line)
		}
		// 行非空就被写进 pending，循环到此为止；
		// 只有「读到空行且已到流尾」才需要把 EOF 交出去。
		if len(u.pending) == 0 && err != nil {
			return 0, err
		}
	}
	if len(u.pending) == 0 {
		return 0, io.EOF
	}
	n := copy(p, u.pending)
	u.pending = u.pending[n:]
	return n, nil
}

// Changed 返回已被改写的行数。
func (u *sseUnwrapper) Changed() int { return u.changed }

// Close 把关闭动作透传给底下的上游连接。
//
// ReverseProxy 会在拷贝完后关闭 resp.Body；如果这里不实现 Close，
// 底下的连接永远不会被释放 —— 长时间跑 serve 就会攒出一堆泄漏的连接。
func (u *sseUnwrapper) Close() error {
	if u.src == nil {
		return nil
	}
	return u.src.Close()
}

// readAllLimited 读取最多 limit+1 字节以判断是否超限。
func readAllLimited(r io.Reader, limit int64) (b []byte, exceeded bool, err error) {
	limited := io.LimitReader(r, limit+1)
	b, err = io.ReadAll(limited)
	if err != nil {
		return nil, false, err
	}
	if int64(len(b)) > limit {
		return b, true, nil
	}
	return b, false, nil
}

// discardClose 关掉读完的 body，忽略错误（调用方要么覆盖它，要么已无用）。
func discardClose(rc io.ReadCloser) {
	if rc == nil {
		return
	}
	_, _ = io.Copy(io.Discard, io.LimitReader(rc, 4096))
	_ = rc.Close()
}

// bytesReader 把字节切片还原成 ReadCloser，供替换 resp.Body。
func bytesReader(b []byte) io.ReadCloser {
	return io.NopCloser(bytes.NewReader(b))
}
