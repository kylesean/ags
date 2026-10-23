//go:build !windows && !darwin

package i18n

// readOSLocale 在 Linux/BSD 上几乎没有额外信息源：
// 桌面会话的 locale 已经通过环境变量传进来了，而 /etc/locale.conf
// 是系统级默认（多用户机器上不该覆盖用户自己的 LANG）。
//
// 所以这里不读文件，交给 env 探测即可 —— 少一个「行为不可预测」的来源。
func readOSLocale() string { return "" }
