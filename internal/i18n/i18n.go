// Package i18n 提供极简的终端文案多语言支持。
//
// 设计取舍：
//   - 不用 go-i18n / x/text：它们带进整套 CLDR 数据（进二进制就是几 MB），
//     而 ags 的文案量只有几十条，手写表最省事也最可控。
//   - 不做运行时热切换：语言在进程启动时定一次，之后只读，避免并发问题。
//   - key 用点号命名空间（"usage.account"），与文案文件一一对应。
//   - 缺译文一律回落默认语言，再缺就返回 key 本身 —— 界面上永不出现空字符串。
package i18n

import (
	"os"
	"regexp"
	"sort"
	"strings"
)

// Lang 是语言标识。
type Lang string

const (
	LangZH Lang = "zh"
	LangEN Lang = "en"
	LangJA Lang = "ja"
)

// DefaultLang 是兜底语言。中文是项目原语言，缺译文时回落到它最能保持可读。
const DefaultLang = LangZH

// supported 是受支持的语言集合，顺序即 --help 里的展示顺序。
var supported = []Lang{LangZH, LangEN, LangJA}

// Supported 返回受支持的语言列表。
func Supported() []Lang { return append([]Lang(nil), supported...) }

// IsSupported 判断某语言是否受支持。
func IsSupported(l Lang) bool {
	for _, s := range supported {
		if s == l {
			return true
		}
	}
	return false
}

// langAliases 把常见的 locale 前缀映射到受支持语言。
//
// 有意把 zh_TW / zh_HK 也归到简体文案：ags 的文案里没有需要区分
// 简繁的词汇，而「给繁体用户看简体」远好过「给繁体用户看英文」。
var langAliases = map[string]Lang{
	"zh": LangZH,
	"en": LangEN,
	"ja": LangJA,
}

// ParseLang 解析 locale 字符串（"zh_CN.UTF-8"、"zh-Hans-CN"、"ja"…）。
// 认不出来时返回 DefaultLang。
func ParseLang(s string) Lang {
	s = strings.TrimSpace(strings.ToLower(s))
	if s == "" {
		return DefaultLang
	}
	// C / POSIX 是 POSIX 约定的「无本地化」，等同英文。
	if s == "c" || s == "posix" {
		return LangEN
	}
	// 去掉编码后缀（".UTF-8"）与修饰（"@modifier"）。
	if i := strings.IndexAny(s, ".@"); i >= 0 {
		s = s[:i]
	}
	// 统一分隔符，然后取主语言子标签。
	s = strings.ReplaceAll(s, "-", "_")
	primary := s
	if i := strings.IndexByte(s, '_'); i >= 0 {
		primary = s[:i]
	}
	if l, ok := langAliases[primary]; ok {
		return l
	}
	return DefaultLang
}

// langEnvVars 是探测顺序（POSIX 优先级）。
var langEnvVars = []string{"AGS_LANG", "LC_ALL", "LC_MESSAGES", "LANG", "LANGUAGE"}

// Detect 探测当前应使用的语言。
//
// 优先级：AGS_LANG > LC_ALL > LC_MESSAGES > LANG > LANGUAGE > 系统默认。
// AGS_LANG 放最前是因为它是我们自己的开关，必须能压过系统设置。
func Detect() Lang {
	for _, k := range langEnvVars {
		if v := strings.TrimSpace(os.Getenv(k)); v != "" {
			return ParseLang(v)
		}
	}
	return systemDefault()
}

// T 返回 key 在指定语言下的译文。
//
// 查找顺序：目标语言 → 默认语言 → key 本身。
// 返回 key 而不是空串，是为了让漏翻在界面上一眼可见（而不是变成空白）。
func T(lang Lang, key string) string {
	if m, ok := catalog[lang]; ok {
		if s, ok := m[key]; ok && s != "" {
			return s
		}
	}
	if m, ok := catalog[DefaultLang]; ok {
		if s, ok := m[key]; ok && s != "" {
			return s
		}
	}
	return key
}

// Keys 返回所有已注册的 key（按字母序，便于测试与 diff）。
func Keys() []string {
	seen := map[string]bool{}
	for _, m := range catalog {
		for k := range m {
			seen[k] = true
		}
	}
	out := make([]string, 0, len(seen))
	for k := range seen {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// MissingKeys 返回某语言缺译文的 key。
func MissingKeys(lang Lang) []string {
	m := catalog[lang]
	var out []string
	for _, k := range Keys() {
		if s, ok := m[k]; !ok || s == "" {
			out = append(out, k)
		}
	}
	return out
}

// placeholderRe 抓 printf 占位符（%s/%d/%v/%q/%f 及带宽度/精度的变体）。
var placeholderRe = regexp.MustCompile(`%[-+ #0-9.]*[a-zA-Z]`)

// placeholders 返回字符串里的占位符序列，用于校验各语言译文一致。
func placeholders(s string) []string {
	return placeholderRe.FindAllString(s, -1)
}
