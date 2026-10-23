package i18n

import (
	"os"
	"strings"
	"testing"
)

// 语言探测优先级：显式配置 > LC_ALL > LC_MESSAGES > LANG > 系统默认。
// 这是 POSIX 约定的顺序，弄反了会让用户「设了 LANG 却不生效」。
func TestDetectPriority(t *testing.T) {
	cases := []struct {
		name string
		env  map[string]string
		want Lang
	}{
		{"LC_ALL 最优先", map[string]string{"AGS_LANG": "", "LC_ALL": "en_US.UTF-8", "LANG": "zh_CN.UTF-8"}, LangEN},
		{"LC_MESSAGES 次之", map[string]string{"LC_MESSAGES": "zh_CN.UTF-8", "LANG": "en_US.UTF-8"}, LangZH},
		{"LANG 兜底", map[string]string{"LANG": "ja_JP.UTF-8"}, LangJA},
		{"AGS_LANG 覆盖一切", map[string]string{"AGS_LANG": "zh", "LC_ALL": "en_US.UTF-8"}, LangZH},
		{"C 是英文", map[string]string{"LANG": "C"}, LangEN},
		{"POSIX 是英文", map[string]string{"LANG": "POSIX"}, LangEN},
		{"空环境用默认", map[string]string{"LANG": ""}, DefaultLang},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			clearLangEnv(t)
			for k, v := range c.env {
				if v != "" {
					t.Setenv(k, v)
				}
			}
			if got := Detect(); got != c.want {
				t.Errorf("Detect() = %q，想要 %q", got, c.want)
			}
		})
	}
}

// "zh_CN.UTF-8" / "zh-Hans-CN" / "zh" 都要认成中文。
func TestParseLangTag(t *testing.T) {
	cases := map[string]Lang{
		"zh":          LangZH,
		"zh_CN.UTF-8": LangZH,
		"zh-Hans-CN":  LangZH,
		"zh_TW":       LangZH,
		"en":          LangEN,
		"en_US.UTF-8": LangEN,
		"en_GB":       LangEN,
		"ja_JP.UTF-8": LangJA,
		"ja":          LangJA,
		"fr_FR.UTF-8": DefaultLang, // 未支持 → 默认
		"de":          DefaultLang,
		"":            DefaultLang,
	}
	for in, want := range cases {
		if got := ParseLang(in); got != want {
			t.Errorf("ParseLang(%q) = %q，想要 %q", in, got, want)
		}
	}
}

// 不支持的语言必须优雅回落到默认，而不是返回空（空会让所有文案变成 key）。
func TestUnsupportedFallsBack(t *testing.T) {
	if ParseLang("ko_KR") == "" {
		t.Fatal("不支持的语言不该返回空")
	}
	if !IsSupported(DefaultLang) {
		t.Fatal("默认语言必须受支持")
	}
}

func clearLangEnv(t *testing.T) {
	t.Helper()
	for _, k := range []string{"AGS_LANG", "LC_ALL", "LC_MESSAGES", "LANG", "LANGUAGE"} {
		t.Setenv(k, "")
		_ = os.Unsetenv(k)
	}
}

// 每个 key 在所有语言里都必须有译文 —— 缺一条就会在界面上露出 key，
// 或者掉回另一种语言（中英混排是典型的「多语言没做完」）。
func TestAllKeysTranslated(t *testing.T) {
	keys := Keys()
	if len(keys) == 0 {
		t.Fatal("没有注册任何 key")
	}
	for _, lang := range Supported() {
		missing := MissingKeys(lang)
		if len(missing) > 0 {
			t.Errorf("语言 %q 缺 %d 条译文: %v", lang, len(missing), missing)
		}
	}
}

// 已经废弃（不在 Keys() 里）的译文要被清掉，否则会越积越多。
func TestNoOrphanTranslations(t *testing.T) {
	known := map[string]bool{}
	for _, k := range Keys() {
		known[k] = true
	}
	for _, lang := range Supported() {
		for k := range catalog[lang] {
			if !known[k] {
				t.Errorf("语言 %q 有孤立译文 %q（文案表里没这个 key）", lang, k)
			}
		}
	}
}

// 译文里的占位符必须与原文一致：少一个 %s 会让格式化输出变成 %!s(MISSING)。
func TestPlaceholdersMatch(t *testing.T) {
	for _, lang := range Supported() {
		for _, k := range Keys() {
			want := placeholders(T(DefaultLang, k))
			got := placeholders(T(lang, k))
			if strings.Join(want, ",") != strings.Join(got, ",") {
				t.Errorf("key %q 在 %q 里占位符不一致：默认 %v vs %q %v",
					k, lang, want, lang, got)
			}
		}
	}
}

// 缺 key 时返回 key 本身（而不是空串），方便定位是漏翻了还是文案错了。
func TestMissingKeyReturnsKey(t *testing.T) {
	if got := T(LangZH, "no.such.key"); got != "no.such.key" {
		t.Errorf("T(缺失) = %q，想要 key 本身", got)
	}
}

// 语言不区分大小写与分隔符。
func TestLangNormalization(t *testing.T) {
	if got := ParseLang("ZH_cn.utf8"); got != LangZH {
		t.Errorf("ParseLang(ZH_cn.utf8) = %q", got)
	}
}
