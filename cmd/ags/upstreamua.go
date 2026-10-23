package main

import (
	"context"
	"fmt"
	"os/exec"
	"regexp"
	"runtime"
	"strings"
	"time"

	"github.com/kylesean/ags/internal/cloudcode"
	"github.com/kylesean/ags/internal/proxy"
)

// agyVersionRe 从 `agy --version` 的输出里抠版本号。
// 实测输出就是一行 "1.4.6"；也容忍 "agy 1.4.6" 这类带前缀的写法。
var agyVersionRe = regexp.MustCompile(`\d+\.\d+(?:\.\d+)?`)

// agyVersionTimeout 很短：这只是一次本地进程调用，
// 卡住时宁可退回内置默认版本，也别拖慢启动。
const agyVersionTimeout = 3 * time.Second

// detectAgyVersion 问出本机 agy 的版本，供上游 User-Agent 使用。
//
// 为什么要问：上游按 UA 版本决定返回哪些模型与哪些额度组（见
// internal/proxy.UpstreamUserAgent）。写死一个版本，agy 升级后就会
// 静默降级 —— 表现是模型表少了新模型、额度汇总里没有 GEMINI 组。
//
// 取不到就返回空串，由 UpstreamUserAgent 用内置兜底版本。
func detectAgyVersion(ctx context.Context) string {
	cctx, cancel := context.WithTimeout(ctx, agyVersionTimeout)
	defer cancel()

	out, err := exec.CommandContext(cctx, "agy", "--version").Output()
	if err != nil {
		return ""
	}
	return parseAgyVersion(string(out))
}

// parseAgyVersion 从命令输出里提取版本号，抽出来便于单测。
func parseAgyVersion(out string) string {
	out = strings.TrimSpace(out)
	if out == "" {
		return ""
	}
	// 只认第一行：--version 之后可能跟别的提示。
	if i := strings.IndexByte(out, '\n'); i >= 0 {
		out = out[:i]
	}
	return agyVersionRe.FindString(out)
}

// upstreamUserAgent 返回本次运行要用的上游 UA（已带本机 agy 版本）。
func upstreamUserAgent(ctx context.Context) string {
	return proxy.UpstreamUserAgent(runtime.GOOS, runtime.GOARCH, detectAgyVersion(ctx))
}

// uaCapabilityProbe 是「新代际 UA 才能看到」的模型前缀。
//
// 上游按 UA 决定给哪一批模型。旧代际（antigravity-cli/1.2.9）只能看到
// claude/gpt-oss 一族；Gemini 4 Argon 与 Gemini 3.8 Flash 这些是新代际
// 才有的。用它们当探针，比盯版本号更可靠：上游可能改了分代规则而版本号
// 没动。
const uaCapabilityProbe = "gemini-4-argon"

// uaCapabilityWarning 在模型表看起来是「旧代际窄集」时返回告警文案。
//
// 为什么值得单独检查：UA 过期不会报错，接口照常 200，只是悄悄少给模型
// 和额度组。用户看到的是「额度不刷新了」「模型少了一半」，而日志里
// 一行错都没有 —— 这是最费时间的一类问题。
func uaCapabilityWarning(m *cloudcode.Models) string {
	if m == nil || len(m.Catalog) == 0 {
		// 拉表失败是另一个故障，别混进来。
		return ""
	}
	for _, e := range m.Catalog {
		if strings.HasPrefix(e.ID, uaCapabilityProbe) {
			return ""
		}
	}
	return fmt.Sprintf(
		"上游只返回了 %d 个模型且不含 %s*，User-Agent 可能已过期（当前代际看不到完整模型与额度组）"+
			"；若 agy 已升级，请更新 ags，或临时用 -user-agent 指定新版 agy 的 UA",
		len(m.Catalog), uaCapabilityProbe)
}
