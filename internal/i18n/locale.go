package i18n

import "runtime"

// systemDefault 在没有语言环境变量时决定语言。
//
// 为什么不用「操作系统」判断：一个中文用户完全可能把 Windows/macOS/Linux
// 装在英文界面下，也可能在英文系统上想用中文。locale 环境变量是用户
// 真实意图的最直接表达，操作系统只是它的一个（较弱的）代理。
//
// 所以这里只在前面的 env 探测全部落空时，才看一眼系统区域设置。
func systemDefault() Lang {
	if l, ok := osLocale(); ok {
		return ParseLang(l)
	}
	return DefaultLang
}

// osLocale 返回操作系统层面的用户语言（可能取不到）。
//
// macOS / Windows / 部分 Linux 桌面下，进程环境里不一定有 LANG，
// 但系统本身记着用户语言。这里按平台读一次，取不到就报 false。
func osLocale() (string, bool) {
	if v := readOSLocale(); v != "" {
		return v, true
	}
	return "", false
}

// readOSLocale 是平台相关的，见 locale_unix.go / locale_windows.go。
var _ = runtime.GOOS
