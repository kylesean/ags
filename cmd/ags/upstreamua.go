package main

import (
	"context"
	"os/exec"
	"regexp"
	"runtime"
	"strings"
	"time"

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
