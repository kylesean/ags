package main

import (
	"strings"
	"testing"

	"github.com/kylesean/ags/internal/i18n"
	"github.com/kylesean/ags/internal/term"
)

// 这是本次改造的核心诉求之一：切换语言后表格**不能错位**。
//
// 判定方法：把每行按列宽切出来后，检查每列内容的起始视觉列号完全一致。
// 只要有一个语言下对不齐，就说明列宽算错了（典型的「按 rune 数补空格」）。
func TestListColumnsAlignInEveryLanguage(t *testing.T) {
	rows := []listRow{
		{name: "名字", email: "a@example.com", expiry: "1h 后", refresh: "是", current: "●"},
		{name: "A", email: "verylongemail@example.com", expiry: "09-30 15:52", refresh: "否", current: ""},
		{name: "B", email: "b@x", expiry: "已过期", refresh: "是", current: ""},
	}

	for _, lang := range i18n.Supported() {
		t.Run(string(lang), func(t *testing.T) {
			old := i18n.Current()
			i18n.SetLang(lang)
			defer i18n.SetLang(old)

			widths := listColumnWidths(rows)
			const colGap = 2

			// 每个单元格 PadRight 之后的视觉宽度必须恰好等于 目标宽度+间距。
			for _, r := range rows {
				vals := [4]string{r.name, r.email, r.expiry, r.refresh}
				for i, v := range vals {
					cell := term.PadRight(v, widths[i]+colGap)
					if got := term.VisualWidth(cell); got != widths[i]+colGap {
						t.Errorf("列 %d 单元格 %q 宽 %d，想要 %d", i, v, got, widths[i]+colGap)
					}
				}
			}

			// 整行拼接后，第 2 列的起始列号必须对所有行一致。
			starts := make([]int, len(rows))
			for ri, r := range rows {
				line := term.PadRight(r.name, widths[0]+colGap)
				starts[ri] = term.VisualWidth(line)
			}
			for ri := 1; ri < len(starts); ri++ {
				if starts[ri] != starts[0] {
					t.Errorf("第 %d 行第 2 列起始于 %d，第 0 行在 %d —— 表格错位",
						ri, starts[ri], starts[0])
				}
			}
		})
	}
}

// 列宽必须取「所有语言列头的最大宽度」，否则切到更长的语言时表头被截断。
func TestListColumnWidthsCoverAllLanguages(t *testing.T) {
	rows := []listRow{{name: "A", email: "a@b", expiry: "-", refresh: "yes"}}
	widths := listColumnWidths(rows)

	headerKeys := [4]string{"col.name", "col.email", "col.expiry", "col.refresh"}
	for i, k := range headerKeys {
		for _, lang := range i18n.Supported() {
			hdr := i18n.T(lang, k)
			// 邮箱列有 32 列上限，其它列必须完整容纳所有语言的表头。
			if i == 1 {
				continue
			}
			if w := term.VisualWidth(hdr); w > widths[i] {
				t.Errorf("列 %d 宽度 %d 装不下 %s 的表头 %q（%d 列）",
					i, widths[i], lang, hdr, w)
			}
		}
	}
}

// 窄终端下表格仍要保持结构：不能因为邮箱太长就把后面的列挤没。
func TestListColumnsFitNarrowTerminal(t *testing.T) {
	t.Setenv("COLUMNS", "60")
	rows := []listRow{
		{name: "A", email: "extremely-long-email-address@some.example.com", expiry: "09-30 15:52", refresh: "yes"},
	}
	widths := listColumnWidths(rows)

	total := 0
	for _, w := range widths {
		total += w
	}
	total += 2 * 4 // 四个列间距
	if total > term.TermWidth() {
		t.Errorf("总宽 %d 超过终端 60 列 —— 窄终端会被折行打乱", total)
	}
	if widths[1] > 32 {
		t.Errorf("邮箱列宽 %d 超过上限 32", widths[1])
	}
}

// usage 的 description 实测 200+ 字符，必须折行而不是溢出。
func TestUsageDescriptionWraps(t *testing.T) {
	t.Setenv("COLUMNS", "80")
	long := "For paid Pro plans, Argon quota is separate. Higher Argon quota (included as part of the larger Gemini quota bucket) is available on Ultra plans."
	out := term.Wrap(long, 78)
	if len(out) < 2 {
		t.Fatalf("长描述没折行: %q", out)
	}
	for _, line := range out {
		if term.VisualWidth(line) > 78 {
			t.Errorf("行宽 %d 超过 78: %q", term.VisualWidth(line), line)
		}
	}
}

// 用法说明行不该在中英切换时消失（回归：字段描述曾被清空）。
func TestFlagDescriptionsAreLocalized(t *testing.T) {
	for _, lang := range i18n.Supported() {
		for _, k := range []string{"flag.account", "flag.upstream", "flag.usageTimeout", "flag.compact"} {
			if v := i18n.T(lang, k); v == "" || v == k {
				t.Errorf("%s 的 %q 没有本地化", lang, k)
			}
		}
	}
	// 中英必须真的不同
	if i18n.T(i18n.LangZH, "flag.account") == i18n.T(i18n.LangEN, "flag.account") {
		t.Error("flag.account 中英相同，没做本地化")
	}
}

var _ = strings.Repeat
