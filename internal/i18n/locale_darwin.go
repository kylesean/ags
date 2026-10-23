//go:build darwin

package i18n

import "os/exec"

// readOSLocale 在 macOS 上读用户偏好语言。
//
// 实测：即使 LANG 未设置，macOS 也会把用户语言记在
// AppleLanguages 默认值里（系统设置 → 语言与地区）。
// 用 `defaults read -g AppleLanguages` 读第一个元素。
//
// 失败就返回空串 —— 语言探测不该因为取不到而卡住或报错。
func readOSLocale() string {
	out, err := exec.Command("defaults", "read", "-g", "AppleLanguages").Output()
	if err != nil {
		return ""
	}
	return parseAppleLanguages(string(out))
}

// parseAppleLanguages 从 `defaults` 的输出里取第一个语言标签。
//
// 输出形如：
//
//	(
//	    "zh-Hans-CN",
//	    "en-US"
//	)
func parseAppleLanguages(out string) string {
	start := -1
	for i, r := range out {
		if r == '"' {
			if start < 0 {
				start = i
				continue
			}
			return out[start+1 : i]
		}
	}
	return ""
}
