package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net"
	"os"
	"os/exec"
	"runtime"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/kylesean/ags/internal/keyring"
	"github.com/kylesean/ags/internal/pool"
)

// gatewayURL 把监听地址转换成 agy 可用的本地 Gateway URL。
// 通配监听只对本机开放，客户端统一使用 127.0.0.1 连接。
func gatewayURL(listen string) string {
	host, port, err := net.SplitHostPort(listen)
	if err != nil {
		return "http://" + listen
	}
	if host == "" || host == "0.0.0.0" || host == "::" {
		host = "127.0.0.1"
	}
	return "http://" + net.JoinHostPort(host, port)
}

func csvContains(values, want string) bool {
	for _, value := range strings.Split(values, ",") {
		if strings.EqualFold(strings.TrimSpace(value), want) {
			return true
		}
	}
	return false
}

func setEnv(env []string, key, value string) []string {
	prefix := key + "="
	out := make([]string, 0, len(env)+1)
	replaced := false
	for _, item := range env {
		if strings.HasPrefix(item, prefix) {
			if !replaced {
				out = append(out, prefix+value)
				replaced = true
			}
			continue
		}
		out = append(out, item)
	}
	if !replaced {
		out = append(out, prefix+value)
	}
	return out
}

// cacheDir 返回 ags 的缓存根目录。AGS_CACHE_DIR 可覆盖（便于测试隔离与自定义存放），
// 未设置时回落到系统用户缓存目录。
func cacheDir() string {
	if base := os.Getenv("AGS_CACHE_DIR"); base != "" {
		return base
	}
	dir, err := os.UserCacheDir()
	if err != nil {
		dir = os.TempDir()
	}
	return dir
}

func defaultTUILogPath() string {
	return filepath.Join(cacheDir(), "ags", "tui.log")
}

func openLogFile(path string) (*os.File, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, err
	}
	return os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
}

// ensureNoProxy 确保本地 Gateway 不会被 HTTP(S)_PROXY 再次代理。
// 同时维护大小写两套变量，兼容不同 HTTP 客户端。
func ensureNoProxy(env []string, host string) []string {
	out := append([]string(nil), env...)
	for _, key := range []string{"NO_PROXY", "no_proxy"} {
		prefix := key + "="
		found := false
		for i, item := range out {
			if !strings.HasPrefix(item, prefix) {
				continue
			}
			found = true
			values := strings.TrimPrefix(item, prefix)
			if !csvContains(values, host) {
				if values == "" {
					out[i] = prefix + host
				} else {
					out[i] = prefix + values + "," + host
				}
			}
		}
		if !found {
			out = append(out, prefix+host)
		}
	}
	return out
}

func waitGateway(ctx context.Context, addr string, serverErr <-chan error) error {
	for {
		conn, err := net.DialTimeout("tcp", addr, 200*time.Millisecond)
		if err == nil {
			_ = conn.Close()
			return nil
		}
		select {
		case err := <-serverErr:
			if err == nil {
				err = fmt.Errorf("serve 在 Gateway ready 前退出")
			}
			return fmt.Errorf("启动 Gateway 失败: %w", err)
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(100 * time.Millisecond):
		}
	}
}

var storeKeyring = keyring.Store

type accountSwitch struct {
	name  string
	email string
}

func secretFromAccount(a *pool.Account) *keyring.Secret {
	if a == nil {
		return nil
	}
	return &keyring.Secret{
		Token: keyring.Token{
			AccessToken:  a.AccessToken,
			TokenType:    "Bearer",
			RefreshToken: a.RefreshToken,
			Expiry:       keyring.ExpiryTime{Time: a.Expiry},
		},
		AuthMethod: a.AuthMethod,
		IDToken:    a.IDToken,
	}
}

func syncKeyringAccount(sw accountSwitch) error {
	a, err := pool.Load(sw.name)
	if err != nil {
		// 池空回落的内存伪账号（Name=="keyring" 但磁盘无文件）：
		// 其凭据本就来自 keyring，无源可同步，直接跳过而非让 tui 启动失败。
		// 真名为 keyring 的磁盘账号走正常路径，不受影响。
		if sw.name == "keyring" {
			if exists, existsErr := pool.Exists(sw.name); existsErr == nil && !exists {
				return nil
			}
		}
		return fmt.Errorf("加载切换账号 %q 失败: %w", sw.name, err)
	}
	if !strings.EqualFold(strings.TrimSpace(a.Email), strings.TrimSpace(sw.email)) {
		return fmt.Errorf("账号 %s 身份已变化，拒绝写入 Keyring", sw.name)
	}
	if a.RefreshToken == "" && a.IDToken == "" {
		return fmt.Errorf("账号 %s 缺少可写入的凭据", sw.name)
	}
	if err := storeKeyring(secretFromAccount(a)); err != nil {
		return fmt.Errorf("同步账号 %s 到 Keyring 失败: %w", sw.name, err)
	}
	return nil
}

type managedAgy struct {
	cmd  *exec.Cmd
	done chan error
}

func startAgy(env []string, args []string) *managedAgy {
	cmd := exec.Command("agy", args...)
	cmd.Env = env
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	p := &managedAgy{cmd: cmd, done: make(chan error, 1)}
	go func() { p.done <- cmd.Run() }()
	return p
}

func stopAgy(p *managedAgy, timeout time.Duration) {
	if p == nil || p.cmd == nil || p.cmd.Process == nil {
		return
	}
	// Windows 的 Process.Signal 只支持 Kill：跳过注定失败的 Interrupt，
	// 短等待后直接 Kill（调用方传的超时仅作上限）。
	interrupt, wait := agyStopPlan(runtime.GOOS)
	if wait < timeout {
		timeout = wait
	}
	if interrupt {
		_ = p.cmd.Process.Signal(os.Interrupt)
	}
	select {
	case <-p.done:
		return
	case <-time.After(timeout):
		_ = p.cmd.Process.Kill()
		<-p.done
	}
}

// agyStopPlan 按 OS 决定停止策略，供单测按 goos 断言。
func agyStopPlan(goos string) (interrupt bool, wait time.Duration) {
	if goos == "windows" {
		return false, 2 * time.Second
	}
	return true, 5 * time.Second
}

// cmdTUI 统一启动 Gateway 和 agy，避免用户手动设置 AGY_GATEWAY_URL。

func isOneShotAgy(args []string) bool {
	for _, arg := range args {
		name, value, hasValue := strings.Cut(arg, "=")
		switch name {
		case "-p", "--print", "--prompt":
			if !hasValue {
				return true
			}
			// --print=false 显式关闭时不是一次性；--prompt=xxx 携值时是一次性。
			v := strings.ToLower(strings.TrimSpace(value))
			switch v {
			case "false", "0", "no", "off":
				continue
			default:
				return true
			}
		}
	}
	return false
}

// tuiOptions 是 tui 的全部开关（serve 透传项 + tui 独有项）。
// 抽出以便单测只验解析不拉起 agy 进程。
type tuiOptions struct {
	listen       string
	upstream     string
	account      string
	refresh      bool
	threshold    float64
	interval     time.Duration
	verbose      bool
	logFile      string
	ua           string
	passthrough  bool
	noModelAlias bool
	syncKeyring  bool
	stripFields  stringList
}

func registerTUIFlags(fs *flag.FlagSet, o *tuiOptions) {
	fs.StringVar(&o.listen, "listen", "127.0.0.1:7897", "Gateway 监听地址")
	fs.StringVar(&o.upstream, "upstream", DefaultUpstream, "上游地址")
	fs.StringVar(&o.account, "account", "", "只使用指定账号")
	fs.BoolVar(&o.refresh, "refresh", false, "启动时先强制刷新一次 access_token")
	fs.Float64Var(&o.threshold, "quota-threshold", 0, "额度耗尽阈值")
	fs.DurationVar(&o.interval, "quota-interval", DefaultQuotaInterval, "额度轮询间隔")
	fs.BoolVar(&o.verbose, "v", false, "打印每个请求的详细选号日志")
	fs.StringVar(&o.logFile, "log-file", defaultTUILogPath(), "Gateway/TUI 日志文件")
	fs.StringVar(&o.ua, "user-agent", defaultUserAgent, "发给上游的 User-Agent")
	fs.BoolVar(&o.passthrough, "passthrough", false, "关闭信封改写，纯透传（调试用）")
	fs.BoolVar(&o.noModelAlias, "no-model-alias", false, "不拉取模型表")
	fs.BoolVar(&o.syncKeyring, "sync-keyring", true, "切换账号时同步 Keyring 并重启 agy")
	fs.Var(&o.stripFields, "strip-field", "转发前从请求体顶层删掉这个 JSON 字段，可重复")
}

// parseTUIArgs 容忍式解析：tui 认识的 flag 归 tui，不认识的（含 agy 的）
// 一律透传给 agy；`--` 之后全部归 agy。已知 flag 的值非法时仍报错，
// 避免把 `-quota-interval=abc` 这类笔误悄悄送进 agy。
func parseTUIArgs(args []string) (tuiOptions, []string, error) {
	o := tuiOptions{
		listen:      "127.0.0.1:7897",
		upstream:    DefaultUpstream,
		logFile:     defaultTUILogPath(),
		ua:          defaultUserAgent,
		interval:    DefaultQuotaInterval,
		syncKeyring: true,
	}
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if arg == "--" {
			return o, append([]string{}, args[i+1:]...), nil
		}
		if !strings.HasPrefix(arg, "-") || arg == "-" {
			// 首个位置参数：与 flag 包语义一致，余下全部归 agy。
			return o, append([]string{arg}, args[i+1:]...), nil
		}
		name, value, hasValue := strings.Cut(strings.TrimLeft(arg, "-"), "=")
		switch name {
		case "h", "help":
			usage := flag.NewFlagSet("tui", flag.ContinueOnError)
			registerTUIFlags(usage, &tuiOptions{})
			fmt.Fprintln(os.Stderr, "Usage of tui:")
			usage.PrintDefaults()
			return o, nil, flag.ErrHelp
		case "v", "refresh", "passthrough", "no-model-alias", "sync-keyring":
			b := true
			if hasValue {
				var err error
				if b, err = strconv.ParseBool(value); err != nil {
					return o, nil, fmt.Errorf("-%s 值非法: %q", name, value)
				}
			}
			switch name {
			case "v":
				o.verbose = b
			case "refresh":
				o.refresh = b
			case "passthrough":
				o.passthrough = b
			case "no-model-alias":
				o.noModelAlias = b
			case "sync-keyring":
				o.syncKeyring = b
			}
		case "listen", "upstream", "account", "log-file", "user-agent", "strip-field":
			if !hasValue {
				i++
				if i >= len(args) {
					return o, nil, fmt.Errorf("-%s 缺少值", name)
				}
				value = args[i]
			}
			switch name {
			case "listen":
				o.listen = value
			case "upstream":
				o.upstream = value
			case "account":
				o.account = value
			case "log-file":
				o.logFile = value
			case "user-agent":
				o.ua = value
			case "strip-field":
				if err := o.stripFields.Set(value); err != nil {
					return o, nil, err
				}
			}
		case "quota-interval":
			if !hasValue {
				i++
				if i >= len(args) {
					return o, nil, fmt.Errorf("-%s 缺少值", name)
				}
				value = args[i]
			}
			d, err := time.ParseDuration(value)
			if err != nil {
				return o, nil, fmt.Errorf("-%s 值非法: %q", name, value)
			}
			o.interval = d
		case "quota-threshold":
			if !hasValue {
				i++
				if i >= len(args) {
					return o, nil, fmt.Errorf("-%s 缺少值", name)
				}
				value = args[i]
			}
			f, err := strconv.ParseFloat(value, 64)
			if err != nil {
				return o, nil, fmt.Errorf("-%s 值非法: %q", name, value)
			}
			o.threshold = f
		default:
			// 不认识：一律归 agy，本参数与其后续位置参数由 agy 自行解释。
			return o, append([]string{arg}, args[i+1:]...), nil
		}
	}
	return o, nil, nil
}

func (o tuiOptions) serveArgs() []string {
	serveArgs := []string{
		"-listen", o.listen,
		"-upstream", o.upstream,
		"-log-file", o.logFile,
		"-user-agent", o.ua,
		"-quota-threshold", strconv.FormatFloat(o.threshold, 'g', -1, 64),
		"-quota-interval", o.interval.String(),
	}
	if o.refresh {
		serveArgs = append(serveArgs, "-refresh")
	}
	if o.passthrough {
		serveArgs = append(serveArgs, "-passthrough")
	}
	for _, f := range o.stripFields {
		serveArgs = append(serveArgs, "-strip-field", f)
	}
	if o.verbose {
		serveArgs = append(serveArgs, "-v")
	}
	if o.account != "" {
		serveArgs = append(serveArgs, "-account", o.account)
	}
	if o.noModelAlias {
		serveArgs = append(serveArgs, "-no-model-alias")
	}
	return serveArgs
}

// 用法：ags [serve flags] [-- agy flags]
func cmdTUI(ctx context.Context, args []string) error {
	o, agyArgs, err := parseTUIArgs(args)
	if err != nil {
		return err
	}
	syncKeyring := o.syncKeyring
	oneShot := isOneShotAgy(agyArgs)
	if syncKeyring && oneShot {
		fmt.Fprintln(os.Stderr, "[tui] 一次性 agy 命令不启用 Keyring 自动重启")
	}
	if _, err := exec.LookPath("agy"); err != nil {
		return fmt.Errorf("找不到 agy，请先安装并加入 PATH: %w", err)
	}
	tuiLogFile, err := openLogFile(o.logFile)
	if err != nil {
		return fmt.Errorf("打开 TUI 日志失败: %w", err)
	}
	defer tuiLogFile.Close()
	tuiLog := log.New(tuiLogFile, "[tui] ", log.LstdFlags|log.Lmsgprefix)

	serveArgs := o.serveArgs()

	syncOnSwitch := syncKeyring && !oneShot
	switchCh := make(chan accountSwitch, 1)
	hooks := serveHooks{}
	var switchMu sync.Mutex
	if syncOnSwitch {
		hooks.onSwitch = func(name, email string) {
			sw := accountSwitch{name: name, email: email}
			switchMu.Lock()
			defer switchMu.Unlock()
			// 只保留最新状态：启动时 A 可能先到，额度检查后 B 必须覆盖它。
			select {
			case <-switchCh:
			default:
			}
			switchCh <- sw
		}
	}

	runCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	serverErr := make(chan error, 1)
	go func() {
		err := cmdServeWithHooks(runCtx, serveArgs, hooks)
		serverErr <- err
		cancel()
	}()

	gateway := gatewayURL(o.listen)
	if err := waitGateway(runCtx, o.listen, serverErr); err != nil {
		return err
	}

	// 启动前的首次 Pick 也可能已经把首选账号从 Keyring 当前账号切走。
	if syncOnSwitch {
		select {
		case sw := <-switchCh:
			if err := syncKeyringAccount(sw); err != nil {
				cancel()
				return err
			}
		default:
		}
	}

	env := ensureNoProxy(os.Environ(), "127.0.0.1")
	env = setEnv(env, "AGY_GATEWAY_URL", gateway)
	agy := startAgy(env, agyArgs)

	for {
		select {
		case sw := <-switchCh:
			tuiLog.Printf("切换账号 %s (%s)，同步 Keyring 并重启 agy", sw.name, sw.email)
			if err := syncKeyringAccount(sw); err != nil {
				stopAgy(agy, 5*time.Second)
				cancel()
				return err
			}
			stopAgy(agy, 5*time.Second)
			agy = startAgy(env, agyArgs)
			tuiLog.Println("agy 已重启，请执行 /resume 恢复会话")
		case err := <-agy.done:
			cancel()
			if err != nil && ctx.Err() == nil {
				return fmt.Errorf("agy 退出: %w", err)
			}
			return nil
		case err := <-serverErr:
			stopAgy(agy, 5*time.Second)
			cancel()
			return fmt.Errorf("Gateway 退出: %w", err)
		case <-ctx.Done():
			stopAgy(agy, 5*time.Second)
			cancel()
			return nil
		}
	}
}
