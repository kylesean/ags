package main

import (
	"testing"
)

// `agy --version` 的输出形态会随版本变，这里钉住几种实测/常见写法。
func TestParseAgyVersion(t *testing.T) {
	cases := map[string]string{
		"1.4.6":                        "1.4.6",
		"1.4.6\n":                      "1.4.6",
		"agy 1.4.6":                    "1.4.6",
		"antigravity/cli/1.4.6":        "1.4.6",
		"1.4.6 (build abc)\nmore: 2.0": "1.4.6",
		"":                             "",
		"no version here":              "",
		"1.5":                          "1.5",
	}
	for in, want := range cases {
		if got := parseAgyVersion(in); got != want {
			t.Errorf("parseAgyVersion(%q) = %q，想要 %q", in, got, want)
		}
	}
}

// 默认 UA 必须带版本号；拿不到 agy 版本时仍要是合法的新格式。
func TestDefaultUserAgentHasVersion(t *testing.T) {
	ua := defaultUserAgent
	if ua == "" {
		t.Fatal("defaultUserAgent 为空")
	}
	if got := parseAgyVersion(ua); got == "" {
		t.Errorf("defaultUserAgent = %q 里没有版本号", ua)
	}
}
