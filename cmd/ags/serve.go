package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/kylesean/ags/internal/cloudcode"
	"github.com/kylesean/ags/internal/i18n"
	"github.com/kylesean/ags/internal/pool"
	"github.com/kylesean/ags/internal/proxy"
	"github.com/kylesean/ags/internal/token"
)

// stringList 让 -strip-field 可以重复出现，例如
//
//	-strip-field sessionId -strip-field somethingElse
type stringList []string

func (s *stringList) String() string { return strings.Join(*s, ", ") }

func (s *stringList) Set(v string) error {
	v = strings.TrimSpace(v)
	if v == "" {
		return errors.New("-strip-field 不能是空字符串")
	}
	*s = append(*s, v)
	return nil
}

// DefaultUpstream 是 Antigravity CloudCode 服务端点。
// 方法路径为 v1internal:streamGenerateContent，顶层协议为 {model, request}。
const DefaultUpstream = "https://daily-cloudcode-pa.googleapis.com"

// defaultUserAgent 是发给上游的客户端标识（许可证闸门），已带本机 agy 版本。
//
// 别写死版本号：上游按 UA 版本决定给哪一批模型与哪几个额度组，
// 版本写死会让 agy 升级后静默降级（见 internal/proxy.UpstreamUserAgent）。
// 运行期取值见 upstreamUserAgent(ctx)；这里是包级默认值，供 flag 默认值使用。
var defaultUserAgent = proxy.DefaultUpstreamUserAgent()

// DefaultQuotaInterval 是额度轮询默认间隔。5 小时窗口下额度变化缓慢，
// 突发切换由 429 即时触发兜底，常规轮询 3 分钟足够；实际等待叠加 ±25% 抖动。
const DefaultQuotaInterval = 3 * time.Minute

// cmdServe 启动反向代理服务。
// 职责：挑选有效账号 → 注入 Authorization 与专用 User-Agent → 双向改写请求/响应信封
// → 转发上游 → 后台监听配额并在耗尽或收到 429 时自动切号。
type serveHooks struct {
	onSwitch func(name, email string)
}

// cmdServe 保留原有 CLI 入口；gui 通过 cmdServeWithHooks 注入账号切换事件。
func cmdServe(ctx context.Context, args []string) error {
	return cmdServeWithHooks(ctx, args, serveHooks{})
}

func cmdServeWithHooks(ctx context.Context, args []string, hooks serveHooks) error {
	fs := flag.NewFlagSet("serve", flag.ContinueOnError)
	listen := fs.String("listen", "127.0.0.1:7897", i18n.Tr("flag.listen"))
	upstream := fs.String("upstream", DefaultUpstream, i18n.Tr("flag.upstream"))
	account := fs.String("account", "", i18n.Tr("flag.serveAccount"))
	refresh := fs.Bool("refresh", false, i18n.Tr("flag.refresh"))
	verbose := fs.Bool("v", false, i18n.Tr("flag.verbose"))
	logFile := fs.String("log-file", "", i18n.Tr("flag.logFile"))
	ua := fs.String("user-agent", defaultUserAgent, i18n.Tr("flag.userAgent"))
	passthrough := fs.Bool("passthrough", false,
		"关闭信封改写，纯透传（调试用；此时上游不会接受 agy 的 Gemini 路径）")
	noModelAlias := fs.Bool("no-model-alias", false,
		"不拉模型表、不做模型名映射/模型发现（调试用；agy 会退回内置目录并发已下线的模型名）")
	quotaInterval := fs.Duration("quota-interval", DefaultQuotaInterval, i18n.Tr("flag.quotaInterval"))
	quotaThreshold := fs.Float64("quota-threshold", 0, i18n.Tr("flag.quotaThreshold0"))
	var stripFields stringList
	fs.Var(&stripFields, "strip-field", i18n.Tr("flag.stripField"))
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() > 0 {
		return i18n.Errorf("usage.noArgs")
	}
	if *quotaThreshold < 0 || *quotaThreshold > 1 {
		return i18n.Errorf("serve.thresholdRange", *quotaThreshold)
	}
	if *quotaInterval < 0 {
		return i18n.Errorf("serve.intervalNegative", *quotaInterval)
	}

	var lg *log.Logger
	if strings.TrimSpace(*logFile) == "" {
		lg = log.New(os.Stderr, "[serve] ", log.LstdFlags|log.Lmsgprefix)
	} else {
		if err := os.MkdirAll(filepath.Dir(*logFile), 0o700); err != nil {
			return i18n.Errorf("probe.dirFail", err)
		}
		f, err := os.OpenFile(*logFile, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
		if err != nil {
			return i18n.Errorf("probe.openFail", err)
		}
		defer f.Close()
		lg = log.New(f, "[serve] ", log.LstdFlags|log.Lmsgprefix)
	}

	cands, err := resolveCandidates(*account)
	if err != nil {
		return err
	}

	// 刷新逻辑交给 Selector，它在锁内做，防止并发请求触发刷新风暴。
	sel := pool.NewSelector(cands, refreshAccount)
	adapter := &selectorPicker{sel: sel, verbose: *verbose, log: lg, onSwitch: hooks.onSwitch}

	if *refresh {
		refreshCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
		err := sel.RefreshAll(refreshCtx)
		cancel()
		if err != nil {
			// 一个账号失败不该阻止整个 serve 启动，
			// 但如果连一个可用账号都挑不出来，那就必须报错。
			lg.Printf("%s", i18n.Trf("pool.refreshPartial", err))
			if _, pickErr := sel.Pick(ctx); pickErr != nil {
				return i18n.Errorf("pool.noUsableAfterRefresh", pickErr)
			}
		} else {
			lg.Printf("%s", i18n.Tr("pool.refreshedAll"))
		}
	}

	// UA 决定上游给哪一批模型与额度组，必须带本机 agy 的真实版本；
	// 用户在命令行显式指定时以用户为准。
	if *ua == defaultUserAgent {
		*ua = upstreamUserAgent(ctx)
	}

	srv, err := proxy.New(*upstream, adapter, lg)
	if err != nil {
		return err
	}
	srv.SetUserAgent(*ua)
	if !*passthrough {
		srv.EnableEnvelope()
	}
	srv.StripFields(stripFields...)
	if n := len(stripFields); n > 0 {
		lg.Printf("%s", i18n.Trf("serve.stripFields", strings.Join(stripFields, ", ")))
	}

	// 启动时先挑一次，让错误在监听前就暴露出来，而不是等第一个请求 503。
	first, err := sel.Pick(ctx)
	if err != nil {
		return err
	}
	adapter.setInitialAccount(first.Name, first.Email)

	// 模型别名 + 模型发现清单。两条都来自同一份 fetchAvailableModels：
	//
	//   - 别名：agy 网关模式发的 gemini-3.8-flash / gemini-4-argon 上游并不认，
	//     真实键是 gemini-3.8-flash-tiered / gemini-4-argon-medium。
	//   - 发现清单：agy ≥1.4 启动时先 GET /v1beta/models 问网关要模型表，
	//     拿不到就退回内置目录并发已下线的名字（一提问就 404）。
	//
	// 失败只告警不退出：模型表拿不到时仍可能有一部分模型是直接命中的
	//（实测 gemini-3.1-flash-lite 就是），不该因此把整个 serve 判死。
	if srv.Envelope() && !*noModelAlias {
		aliasCtx, cancel := context.WithTimeout(ctx, 20*time.Second)
		models, aliasErr := cloudcode.FetchModels(aliasCtx, *upstream,
			first.AccessToken, srv.UserAgentForUpstream())
		cancel()
		if aliasErr != nil {
			lg.Printf("%s", i18n.Trf("serve.modelsFail", aliasErr))
			go retryFetchModelAliases(ctx, sel, *upstream, srv, *verbose, lg, 10*time.Second)
		} else {
			srv.SetModelAliases(models.Aliases)
			// 模型发现清单：agy ≥1.4 会先 GET /v1beta/models 问网关。
			// 没有它 agy 就退回内置目录并发已下线的模型名（一提问就 404）。
			srv.SetModelCatalog(toProxyCatalog(models.Catalog))
			reportModelTable(models, *verbose, lg, "serve.modelsLoaded")
			if models.DefaultAgentModelID != "" {
				lg.Printf("%s", i18n.Trf("serve.defaultModel", models.DefaultAgentModelID))
			}
		}
	}
	lg.Printf("%s", i18n.Trf("serve.listening", *listen, srv.Upstream()))
	lg.Printf("%s", i18n.Trf("serve.envelope", envelopeState(srv.Envelope()), *ua))
	lg.Printf("%s", i18n.Trf("serve.account", first.Name, first.Email, fmtExpiry(first.Expiry)))
	if n := len(cands); n > 1 {
		lg.Printf("%s", i18n.Trf("serve.candidates", n, joinNames(sel.Candidates())))
	}
	lg.Printf("%s", i18n.Trf("serve.ready", gatewayURL(*listen)))

	// 额度检测：启动先同步跑一轮，让第一个请求就已经知道哪些号没额度；
	// 之后交给后台按 interval 轮询。首轮失败只告警，不拦启动 ——
	// 额度接口抖动不该让代理起不来。
	if *quotaInterval > 0 {
		qw := newQuotaWatcher(sel, *upstream, srv.UserAgentForUpstream(), *quotaThreshold, lg)
		adapter.on429 = func(name string) {
			qw.triggerAccount(name)
		}
		qw.check(ctx, true)
		// 启动首轮额度检查可能已经冷却了 first；重新 Pick 一次，
		// 让 gui 的 Keyring 同步拿到真正可用的账号。
		if next, pickErr := sel.Pick(ctx); pickErr != nil {
			lg.Printf("%s", i18n.Trf("serve.repickFail", pickErr))
		} else if next.Name != first.Name {
			first = next
			adapter.setInitialAccount(first.Name, first.Email)
		}
		go qw.run(ctx, *quotaInterval)
		lg.Printf("%s", i18n.Trf("serve.quotaInterval", *quotaInterval, *quotaThreshold))
	} else {
		lg.Printf("%s", i18n.Tr("serve.quotaOff"))
	}

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		adapter.beginRequest()
		defer adapter.endRequest()
		srv.ServeHTTP(w, r)
	})

	httpSrv := &http.Server{
		Addr:              *listen,
		Handler:           handler,
		ReadHeaderTimeout: 15 * time.Second,
		// 不设 WriteTimeout / ReadTimeout：SSE 长流可能持续数分钟，
		// 任何全局超时都会把它拦腰砍断。
	}
	errCh := make(chan error, 1)
	go func() { errCh <- httpSrv.ListenAndServe() }()

	select {
	case <-ctx.Done():
		lg.Printf("%s", i18n.Tr("serve.shutdown"))
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := httpSrv.Shutdown(shutdownCtx); err != nil {
			return i18n.Errorf("serve.shutdownFail", err)
		}
		return nil
	case err := <-errCh:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	}
}

// retryFetchModelAliases 在后台周期性重试拉取模型表，直到成功或 ctx 取消。
// 启动时若因临时网络抖动、DNS 故障失败，此协程可确保网络恢复后自动加载模型映射，
// 避免整个 serve 生命周期内模型别名映射永久缺失导致 404。
func retryFetchModelAliases(ctx context.Context, sel *pool.Selector, upstream string, srv *proxy.Server, verbose bool, lg *log.Logger, retryInterval time.Duration) {
	if retryInterval <= 0 {
		retryInterval = 10 * time.Second
	}
	ticker := time.NewTicker(retryInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			var tok string
			for _, c := range sel.Candidates() {
				if t, err := sel.FreshToken(ctx, c.Name); err == nil && t != "" {
					tok = t
					break
				}
			}
			if tok == "" {
				continue
			}
			fetchCtx, cancel := context.WithTimeout(ctx, 20*time.Second)
			models, err := cloudcode.FetchModels(fetchCtx, upstream, tok, srv.UserAgentForUpstream())
			cancel()
			if err != nil {
				lg.Printf("%s", i18n.Trf("serve.retryFail", retryInterval, err))
				continue
			}
			srv.SetModelAliases(models.Aliases)
			srv.SetModelCatalog(toProxyCatalog(models.Catalog))
			reportModelTable(models, verbose, lg, "serve.retryLoaded")
			if models.DefaultAgentModelID != "" {
				lg.Printf("%s", i18n.Trf("serve.defaultModel", models.DefaultAgentModelID))
			}
			return
		}
	}
}

// envelopeState 把信封开关翻成当前语言的「开/关」。
func envelopeState(on bool) string {
	if on {
		return i18n.Tr("serve.envelopeOn")
	}
	return i18n.Tr("serve.envelopeOff")
}

// reportModelTable 统一输出「模型表已就绪」这段日志，并带上所有告警。
//
// 抽出来是因为启动路径与后台重试路径都要打同一套信息：任何一条告警
// 漏在某条路径上，都会变成「静默降级」—— 正是这次要根治的那类问题。
func reportModelTable(m *cloudcode.Models, verbose bool, lg *log.Logger, prefixKey string) {
	prefix := i18n.Tr(prefixKey)
	if verbose {
		lg.Printf("%s (%d keys, %d aliases, %d discovered): %s",
			prefix, m.Keys, len(m.Aliases), len(m.Catalog), formatAliases(m.Aliases))
	} else {
		lg.Printf("%s (%d aliases, %d discovered models)",
			prefix, len(m.Aliases), len(m.Catalog))
	}

	// 兜底清单的告警（displayName 与 key 错位会让 agy 选到不存在的模型）。
	for _, w := range m.Warnings {
		lg.Printf("%s: %s", i18n.Tr("common.warning"), w)
	}
	// UA 过期是静默降级：接口 200 但少给模型与额度组。
	if warn := uaCapabilityWarning(m); warn != "" {
		lg.Printf("%s: %s", i18n.Tr("common.warning"), warn)
	}
}

// selectorPicker 把 pool.Selector 适配成 proxy.Picker 与 proxy.StatusReporter。
type selectorPicker struct {
	sel      *pool.Selector
	verbose  bool
	log      *log.Logger
	on429    func(name string)
	onSwitch func(name, email string)

	mu            sync.Mutex
	lastName      string
	inFlight      int
	pendingSwitch *accountSwitch
}

// CandidateCount 实现 proxy.CandidateCounter：告诉网关这个池有几个号。
// 网关据此决定 429 最多换几次号 —— 池里有 3 个号就该试 3 次。
func (p *selectorPicker) CandidateCount() int {
	if p == nil || p.sel == nil {
		return 0
	}
	return p.sel.CandidateCount()
}

func (p *selectorPicker) beginRequest() {
	p.mu.Lock()
	p.inFlight++
	p.mu.Unlock()
}

func (p *selectorPicker) endRequest() {
	var sw *accountSwitch
	p.mu.Lock()
	p.inFlight--
	if p.inFlight <= 0 {
		p.inFlight = 0
		if p.pendingSwitch != nil {
			sw = p.pendingSwitch
			p.pendingSwitch = nil
		}
	}
	p.mu.Unlock()

	if sw != nil && p.onSwitch != nil {
		if p.log != nil {
			p.log.Printf("%s", i18n.Trf("serve.inflightSwitch", sw.name, sw.email))
		}
		p.onSwitch(sw.name, sw.email)
	}
}

func (p *selectorPicker) setInitialAccount(name, email string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.lastName = name
	if p.onSwitch != nil {
		p.onSwitch(name, email)
	}
}

func (p *selectorPicker) ReportStatus(name string, statusCode int) {
	if statusCode == http.StatusTooManyRequests {
		if p.log != nil {
			p.log.Printf("%s", i18n.Trf("serve.got429", name))
		}
		p.sel.SetCooldown(name, time.Now().Add(time.Minute))
		if p.on429 != nil {
			p.on429(name)
		}
		return
	}
	if statusCode == http.StatusUnauthorized {
		// token 被吊销/失效但本地 Expiry 尚远时，靠 skew 自愈太慢：
		// 先让该号避让 30s 并唤醒额度轮询，下次 Pick 即换号。
		if p.log != nil {
			p.log.Printf("%s", i18n.Trf("serve.got401", name))
		}
		p.sel.SetCooldown(name, time.Now().Add(30*time.Second))
		if p.on429 != nil {
			p.on429(name)
		}
	}
}

func (p *selectorPicker) Pick(ctx context.Context) (*proxy.Account, error) {
	a, err := p.sel.Pick(ctx)
	if err != nil {
		return nil, err
	}
	p.mu.Lock()
	changed := a.Name != p.lastName
	p.lastName = a.Name
	var immediateSwitch bool
	if changed && p.onSwitch != nil {
		if p.inFlight > 0 {
			// 在飞请求处理中：暂存切换，等本次及并发请求平滑完成后再通知，绝不切断正在生成的流式回答
			p.pendingSwitch = &accountSwitch{name: a.Name, email: a.Email}
		} else {
			immediateSwitch = true
		}
	}
	p.mu.Unlock()

	if p.log != nil && (p.verbose || changed) {
		p.log.Printf("%s", i18n.Trf("serve.pickAccount", a.Name, a.Email, fmtExpiry(a.Expiry)))
	}
	if immediateSwitch {
		p.onSwitch(a.Name, a.Email)
	}
	return &proxy.Account{Name: a.Name, Email: a.Email, AccessToken: a.AccessToken}, nil
}

// resolveCandidates 决定反向代理的候选账号集合：
//   - name 非空：显式使用指定账号
//   - name 为空：使用账号池中全部可用账号（支持自动轮转），若池为空则回落至当前系统 Keyring 凭据
func resolveCandidates(name string) ([]*pool.Account, error) {
	if name != "" {
		a, err := pool.Load(name)
		if err != nil {
			return nil, i18n.Errorf("pool.loadAccountFail", name, err)
		}
		return []*pool.Account{a}, nil
	}

	accounts, err := pool.List()
	if err != nil {
		return nil, err
	}
	// 池文件可能来自旧版本或人工复制；服务层再次按邮箱去重，
	// 防止同一 Google 账号被重复轮换、重复刷新和重复查额度。
	accounts = pool.Unique(accounts)
	if len(accounts) > 0 {
		return accounts, nil
	}

	// 池为空：直接用 keyring 里那一份，让链路先跑通（只读，不写 keyring）。
	sec, email, err := keyringCurrentAdapter()
	if err != nil {
		return nil, i18n.Errorf("pool.noCandidates", err)
	}
	claims, claimsErr := sec.Claims()
	if claimsErr != nil {
		return nil, i18n.Errorf("pool.claimsFail", claimsErr)
	}
	return []*pool.Account{{
		Name:            "keyring",
		Email:           email,
		ClientID:        claims.Audience(),
		AccessToken:     sec.Token.AccessToken,
		RefreshToken:    sec.Token.RefreshToken,
		Expiry:          sec.Token.Expiry.Time,
		AuthMethod:      sec.AuthMethod,
		KeyringFallback: true,
	}}, nil
}

// toProxyCatalog 把 cloudcode 的清单转成 proxy 的（避免 proxy 依赖 cloudcode）。
func toProxyCatalog(in []cloudcode.CatalogEntry) []proxy.CatalogEntry {
	if len(in) == 0 {
		return nil
	}
	out := make([]proxy.CatalogEntry, 0, len(in))
	for _, e := range in {
		out = append(out, proxy.CatalogEntry{
			ID:               e.ID,
			DisplayName:      e.DisplayName,
			InputTokenLimit:  e.InputTokenLimit,
			OutputTokenLimit: e.OutputTokenLimit,
			SupportsImages:   e.SupportsImages,
			SupportsThinking: e.SupportsThinking,
		})
	}
	return out
}

// resolverMap 是给日志用的：别名表可能有几十条，全打出来会把启动日志淹掉。
func formatAliases(m map[string]string) string {
	if len(m) == 0 {
		return "(空)"
	}
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var b strings.Builder
	for i, k := range keys {
		if i > 0 {
			b.WriteString(", ")
		}
		b.WriteString(k)
		b.WriteString("→")
		b.WriteString(m[k])
		if i >= 7 && len(keys) > 9 {
			b.WriteString(fmt.Sprintf(" …(共 %d 条)", len(keys)))
			break
		}
	}
	return b.String()
}

// refreshAccount 刷新账号的 access_token 并落盘（keyring 那份是临时态，不落盘）。
// 这是「每号只登一次，之后永久免登」的兑现点。
func refreshAccount(ctx context.Context, a *pool.Account) error {
	if a.RefreshToken == "" {
		return errors.New("该账号没有 refresh_token")
	}
	if _, hasDeadline := ctx.Deadline(); !hasDeadline {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, 20*time.Second)
		defer cancel()
	}

	res, err := token.Refresh(ctx, a.ClientID, a.RefreshToken)
	if err != nil {
		return err
	}
	a.AccessToken = res.AccessToken
	a.Expiry = res.Expiry().UTC()
	if res.RefreshToken != "" {
		a.RefreshToken = res.RefreshToken
	}
	if a.KeyringFallback {
		// 内存伪账号（池空回落）：本就来自 keyring，无文件可落盘。
		return nil
	}
	exists, err := pool.Exists(a.Name)
	if err == nil && !exists {
		// 账号已被外部删除（如 ags drop），不再落盘复活
		return nil
	}
	return pool.Save(a)
}

func joinNames(cands []*pool.Account) string {
	names := make([]string, 0, len(cands))
	for _, a := range cands {
		names = append(names, a.Name)
	}
	out := ""
	for i, n := range names {
		if i > 0 {
			out += ", "
		}
		out += n
	}
	return out
}
