package main

import (
	"strings"
	"testing"

	"github.com/kylesean/ags/internal/i18n"
)

// -lang 必须能被识别，且不干扰其它参数（agy 的透传参数尤其不能被吃掉）。
func TestFindLangFlag(t *testing.T) {
	cases := []struct {
		args []string
		want string
	}{
		{[]string{"-lang", "en"}, "en"},
		{[]string{"--lang=en"}, "en"},
		{[]string{"-lang=ja"}, "ja"},
		{[]string{"-v", "-lang", "en", "--print", "hi"}, "en"},
		// agy 的参数不能被误当成本工具的开关
		{[]string{"--print", "hi"}, ""},
		{[]string{"-p", "hello"}, ""},
		// `--` 之后全归 agy，不能把 -lang 也认走
		{[]string{"--", "-lang", "en"}, ""},
		// 缺值时不 panic，返回空
		{[]string{"-lang"}, ""},
		{[]string{}, ""},
	}
	for _, c := range cases {
		if got := findLangFlag(c.args); got != c.want {
			t.Errorf("findLangFlag(%v) = %q，想要 %q", c.args, got, c.want)
		}
	}
}

// -lang 必须被**摘掉**，不能只识别：留着它会让 `ags -lang en --help`
// 走进 TUI 分支，被当成未知 flag 透传给 agy（打出 agy 的帮助）。
func TestStripLangFlag(t *testing.T) {
	cases := []struct {
		in   []string
		want []string
	}{
		{[]string{"-lang", "en", "--help"}, []string{"--help"}},
		{[]string{"--lang=ja", "usage"}, []string{"usage"}},
		{[]string{"-lang=zh"}, []string{}},
		{[]string{"-v", "-lang", "en", "usage"}, []string{"-v", "usage"}},
		// 没有 -lang 时原样返回
		{[]string{"--print", "hi"}, []string{"--print", "hi"}},
		// `--` 之后全归 agy，里面的 -lang 不能被摘
		{[]string{"--", "-lang", "en"}, []string{"--", "-lang", "en"}},
		{[]string{"-lang"}, []string{}},
	}
	for _, c := range cases {
		got := stripLangFlag(c.in)
		if len(got) != len(c.want) {
			t.Errorf("stripLangFlag(%v) = %v，想要 %v", c.in, got, c.want)
			continue
		}
		for i := range got {
			if got[i] != c.want[i] {
				t.Errorf("stripLangFlag(%v) = %v，想要 %v", c.in, got, c.want)
				break
			}
		}
	}
}

// 帮助文案必须随语言切换。这是「多语言真的生效了」的最直接证据。
func TestUsageTextSwitch(t *testing.T) {
	old := i18n.Current()
	t.Cleanup(func() { i18n.SetLang(old) })

	zh := i18n.T(i18n.LangZH, "cli.usage")
	en := i18n.T(i18n.LangEN, "cli.usage")
	if zh == en {
		t.Fatal("中英帮助文案相同，说明没做本地化")
	}
	if !strings.Contains(zh, "用法") {
		t.Errorf("中文帮助缺「用法」: %q", zh)
	}
	if !strings.Contains(en, "Usage") {
		t.Errorf("英文帮助缺 Usage: %q", en)
	}

	// 帮助里必须列出全部子命令，否则用户找不到入口。
	for _, cmd := range []string{"probe", "login", "add", "list", "status", "drop", "usage", "use", "daemon", "serve", "tui"} {
		if !strings.Contains(zh, cmd) {
			t.Errorf("中文帮助缺子命令 %q", cmd)
		}
		if !strings.Contains(en, cmd) {
			t.Errorf("英文帮助缺子命令 %q", cmd)
		}
	}
}

// 所有语言的帮助都必须是「多行且结构一致」（都有 Usage/用法 段与命令段）。
func TestUsageTextAllLanguagesWellFormed(t *testing.T) {
	for _, lang := range i18n.Supported() {
		body := i18n.T(lang, "cli.usage")
		if strings.Count(body, "\n") < 10 {
			t.Errorf("%s 的帮助太短，可能没写完:\n%s", lang, body)
		}
		for _, cmd := range []string{"serve", "usage", "login"} {
			if !strings.Contains(body, cmd) {
				t.Errorf("%s 的帮助缺子命令 %q", lang, cmd)
			}
		}
	}
}
