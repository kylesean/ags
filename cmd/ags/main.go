// ags — Antigravity (agy) 多账号切换器。
//
// 核心能力：
//   - 账号池私有持久化（目录 0700、文件 0600）
//   - 支持独立 OAuth 登录与系统 Keyring 凭据捕获
//   - 拦截探针支持网络协议与线格式分析
//   - 透传代理负责身份注入、协议信封双向改写与额度耗尽自动切号
//   - 终端界面支持中/英/日三语，按 locale 自动选择
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/kylesean/ags/internal/i18n"
)

func main() {
	// 语言必须在任何输出之前定下来，否则首屏（帮助、错误）会漏本地化。
	initLang()

	// -lang 是本工具的开关，必须先摘掉：留着它会让 `ags -lang en --help`
	// 走进 TUI 分支，然后被当成未知 flag 透传给 agy（打出 agy 的帮助）。
	argv := stripLangFlag(os.Args[1:])

	cmd := "tui"
	var args []string
	if len(argv) >= 1 {
		first := argv[0]
		if strings.HasPrefix(first, "-") && first != "-h" && first != "--help" {
			cmd = "tui"
			args = argv
		} else {
			cmd, args = first, argv[1:]
		}
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	var err error
	switch cmd {
	case "probe":
		err = cmdProbe(ctx, args)
	case "login":
		err = cmdLogin(ctx, args)
	case "add":
		err = cmdAdd(args)
	case "list":
		err = cmdList(args)
	case "status":
		err = cmdStatus(args)
	case "drop":
		err = cmdDrop(args)
	case "usage":
		err = cmdUsage(args)
	case "use":
		err = cmdUse(args)
	case "daemon":
		err = cmdDaemon(ctx, args)
	case "serve":
		err = cmdServe(ctx, args)
	case "tui":
		err = cmdTUI(ctx, args)
	case "-h", "--help", "help":
		fmt.Print(i18n.Tr("cli.usage"))
		return
	default:
		fmt.Fprintf(os.Stderr, i18n.Tr("cli.unknownCmd")+"\n", cmd, i18n.Tr("cli.usage"))
		os.Exit(2)
	}

	if err != nil {
		if err == flag.ErrHelp {
			os.Exit(0)
		}
		fmt.Fprintf(os.Stderr, i18n.Tr("cli.error")+"\n", err.Error())
		os.Exit(1)
	}
}

// initLang 决定进程语言。
//
// 优先级：-lang 参数 > AGS_LANG 环境变量 > 系统 locale > 默认。
//
// 为什么要在这里扫一遍 os.Args（而不是等各命令解析 flag）：
// 帮助与错误信息可能在**任何**子命令之前输出，语言必须提前定好。
// 这里只做「找 -lang」，不干扰各命令自己的 flag 解析。
func initLang() {
	if v := findLangFlag(os.Args[1:]); v != "" {
		i18n.SetLang(i18n.ParseLang(v))
		return
	}
	// 空串 → 让 i18n 自己按 locale 探测。
	i18n.SetLang("")
}

// findLangFlag 从参数里找 -lang/--lang（支持 -lang=en 与 -lang en 两种写法）。
//
// 只认这一个开关，其余参数原样留给子命令 —— 不能在这里做完整解析，
// 那会把 `ags --print hi` 这类 agy 透传参数吃掉。
func findLangFlag(args []string) string {
	_, v := locateLangFlag(args)
	return v
}

// locateLangFlag 返回 -lang 出现的位置与值。
//
// idx 是它在切片里的下标（指向 `-lang` 本身）；found 为 false 表示没有。
func locateLangFlag(args []string) (idx int, value string) {
	for i, a := range args {
		if a == "--" {
			// `--` 之后全归 agy，不要再当成我们的开关。
			return -1, ""
		}
		name, v, hasValue := strings.Cut(a, "=")
		if name != "-lang" && name != "--lang" {
			continue
		}
		if hasValue {
			return i, v
		}
		// `-lang en` 形式：取下一个参数。
		if i+1 < len(args) {
			return i, args[i+1]
		}
		return i, ""
	}
	return -1, ""
}

// stripLangFlag 从参数里摘掉 -lang（连同它的值），其余顺序保持不变。
//
// 摘掉而不是跳过：`ags -lang en usage` 里，`-lang en` 不能被留给 usage
// 子命令（它不认识），更不能透传给 agy。
func stripLangFlag(args []string) []string {
	idx, _ := locateLangFlag(args)
	if idx < 0 {
		return args
	}
	out := make([]string, 0, len(args))
	out = append(out, args[:idx]...)
	rest := args[idx+1:]
	// 无值形式（-lang=en）只占一个元素；有值形式（-lang en）占两个。
	if !strings.Contains(args[idx], "=") && len(rest) > 0 {
		rest = rest[1:]
	}
	out = append(out, rest...)
	return out
}
