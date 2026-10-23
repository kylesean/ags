package term

import (
	"os"
	"strconv"
	"strings"
	"testing"
)

// 中日韩字符占 2 列、ASCII 占 1 列 —— 这是所有表格错位的根源。
func TestRuneWidth(t *testing.T) {
	cases := []struct {
		r    rune
		want int
		note string
	}{
		{'a', 1, "ASCII"},
		{' ', 1, "空格"},
		{'●', 1, "项目符号（窄）"},
		{'名', 2, "中文"},
		{'字', 2, "中文"},
		{'あ', 2, "日文假名"},
		{'한', 2, "韩文"},
		{'，', 2, "全角标点"},
		{'：', 2, "全角冒号"},
		{'…', 1, "省略号（窄）"},
		{'─', 1, "制表符（窄）"},
		{'█', 1, "块字符（窄）"},
		{'░', 1, "浅块（窄）"},
		{0x0301, 0, "组合重音（零宽）"},
	}
	for _, c := range cases {
		if got := RuneWidth(c.r); got != c.want {
			t.Errorf("RuneWidth(%q %U) = %d，想要 %d（%s）", c.r, c.r, got, c.want, c.note)
		}
	}
}

func TestVisualWidth(t *testing.T) {
	cases := map[string]int{
		"":                 0,
		"abc":              3,
		"名字":               4,
		"名字 abc":           8,
		"● A":              3,
		"[████░░░░]":       10,
		"a\u0301":          1, // a + 组合重音
		"Pool: A":          7,
		"Account: a@b.com": 16,
	}
	for in, want := range cases {
		if got := VisualWidth(in); got != want {
			t.Errorf("VisualWidth(%q) = %d，想要 %d", in, got, want)
		}
	}
}

// PadRight 的核心契约：结果**永远**是恰好 width 列宽（除非需要截断）。
// 旧实现「超宽就原样返回」会让整张表往右错位，是真实踩过的 bug。
func TestPadRightAlwaysExactWidth(t *testing.T) {
	cases := []struct {
		s     string
		width int
	}{
		{"abc", 10},
		{"名字", 10},
		{"a@example.com", 10}, // 超宽 → 必须截断
		{"很长的中文邮箱很长", 8},      // 超宽 CJK → 必须截断
		{"", 5},
		{"abc", 3},
	}
	for _, c := range cases {
		got := PadRight(c.s, c.width)
		if w := VisualWidth(got); w != c.width {
			t.Errorf("PadRight(%q, %d) = %q 宽 %d，想要恰好 %d", c.s, c.width, got, w, c.width)
		}
	}
}

// 截断不能把双宽字符切成半个：宽度必须严格 <= limit。
func TestTruncateRespectsWideChars(t *testing.T) {
	cases := []struct {
		s     string
		limit int
	}{
		{"名字很长很长", 5}, // 奇数宽度，最后一个中文字放不下
		{"名字很长很长", 4},
		{"abcdef", 5},
		{"名字", 1}, // 一个中文字都放不下
		{"名字", 0},
	}
	for _, c := range cases {
		got := Truncate(c.s, c.limit)
		if w := VisualWidth(got); w > c.limit {
			t.Errorf("Truncate(%q, %d) = %q 宽 %d，超过上限", c.s, c.limit, got, w)
		}
	}
}

// 放得下就必须原样返回，不能无缘无故加省略号。
func TestTruncateNoopWhenFits(t *testing.T) {
	for _, s := range []string{"abc", "名字", "", "a@b.com"} {
		if got := Truncate(s, 40); got != s {
			t.Errorf("Truncate(%q, 40) = %q，不该改动", s, got)
		}
	}
}

// 截断后要能看出「还有内容」——省略号本身也占宽度。
func TestTruncateAddsEllipsis(t *testing.T) {
	got := Truncate("abcdefghij", 5)
	if !strings.HasSuffix(got, "…") {
		t.Errorf("Truncate = %q，应带省略号", got)
	}
	if w := VisualWidth(got); w > 5 {
		t.Errorf("带省略号后宽 %d，超过 5", w)
	}
}

// Wrap 按视觉宽度折行：长英文描述不能在中文字符中间硬断，
// 也不能永远不折（Argon 的 description 实测 200+ 字符，会溢出终端）。
func TestWrapBreaksLongLines(t *testing.T) {
	long := "For paid Pro plans, Argon quota is separate. Higher Argon quota is available on Ultra plans."
	out := Wrap(long, 30)
	if len(out) < 2 {
		t.Fatalf("应折成多行，got %d 行: %q", len(out), out)
	}
	for _, line := range out {
		if w := VisualWidth(line); w > 30 {
			t.Errorf("行宽 %d 超过 30: %q", w, line)
		}
	}
}

func TestWrapKeepsShortLines(t *testing.T) {
	out := Wrap("short", 80)
	if len(out) != 1 || out[0] != "short" {
		t.Errorf("Wrap = %q", out)
	}
}

// 中文没有空格，必须能在字符间断行（否则整段溢出）。
func TestWrapBreaksCJKWithoutSpaces(t *testing.T) {
	out := Wrap("这是一段没有空格的中文描述应该能在字符之间换行", 12)
	if len(out) < 2 {
		t.Fatalf("中文应能按字符折行，got %q", out)
	}
	for _, line := range out {
		if w := VisualWidth(line); w > 12 {
			t.Errorf("行宽 %d 超过 12: %q", w, line)
		}
	}
	// 折行不该吞掉字符（去掉换行后应与原文一致）。
	if joined := strings.Join(out, ""); joined != "这是一段没有空格的中文描述应该能在字符之间换行" {
		t.Errorf("折行丢字符: %q", joined)
	}
}

// COLUMNS 环境变量是唯一可靠的跨平台来源（非 tty 时 ioctl 不可用）。
func TestTermWidthFromColumns(t *testing.T) {
	t.Setenv("COLUMNS", "120")
	if got := TermWidth(); got != 120 {
		t.Errorf("TermWidth = %d，想要 120", got)
	}

	t.Setenv("COLUMNS", "garbage")
	if got := TermWidth(); got != DefaultWidth {
		t.Errorf("非法 COLUMNS 应回落 %d，got %d", DefaultWidth, got)
	}

	t.Setenv("COLUMNS", "0")
	if got := TermWidth(); got != DefaultWidth {
		t.Errorf("COLUMNS=0 应回落 %d，got %d", DefaultWidth, got)
	}
}

// 极窄终端要保底：宽度小到画不出任何东西时，退到最小值，
// 否则所有列都会被截成空串，输出变成一片空白。
func TestTermWidthHasFloor(t *testing.T) {
	t.Setenv("COLUMNS", "5")
	if got := TermWidth(); got < MinWidth {
		t.Errorf("TermWidth = %d，应不低于 %d", got, MinWidth)
	}
}

var _ = os.Getenv
var _ = strconv.Atoi
