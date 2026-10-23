// Package term 提供终端宽度计算、按视觉列宽对齐与折行。
//
// 存在的理由：中文/日文/韩文占 2 个终端列，ASCII 占 1 列。任何
// 「按字节数或 rune 数补空格」的表格在多语言下都会错位 —— 这不是
// 理论问题，是 ags 真实踩过的 bug（list/status 的列在中文表头下会歪）。
package term

import (
	"os"
	"strconv"
	"strings"
	"unicode"
)

// DefaultWidth 是在拿不到终端宽度时的保守假设。
// 80 是 POSIX 以来的事实标准，也足够画下 ags 的所有表格。
const DefaultWidth = 80

// MinWidth 是布局下限。窄于此宽度时所有列都会被截成空串，
// 不如退到这个值 —— 让用户看到「被截断但结构还在」的输出。
const MinWidth = 40

// RuneWidth 返回一个 rune 占用的终端列数（0/1/2）。
//
// 覆盖范围按 Unicode East Asian Width 的常用子集，够 ags 用：
//   - 0：组合记号（U+0300-036F 等）—— 叠在前一个字符上，不占列
//   - 2：CJK 表意、假名、谚文、全角符号、Emoji
//   - 1：其余（含 ●、█、░、…、─ 这些在多数终端里是窄的）
func RuneWidth(r rune) int {
	// 零宽：组合附加符号、变体选择符、零宽连接符。
	if unicode.Is(unicode.Mn, r) || unicode.Is(unicode.Me, r) {
		return 0
	}
	switch r {
	case 0x200B, 0x200C, 0x200D, 0xFEFF: // ZWSP/ZWNJ/ZWJ/BOM
		return 0
	}

	// 双宽区段（EAW = Wide / Fullwidth 的常用范围）。
	if (r >= 0x1100 && r <= 0x115F) || // 谚文字母
		(r >= 0x2E80 && r <= 0x303E) || // CJK 部首、标点
		(r >= 0x3041 && r <= 0x33FF) || // 假名、CJK 兼容
		(r >= 0x3400 && r <= 0x4DBF) || // CJK 扩展 A
		(r >= 0x4E00 && r <= 0x9FFF) || // CJK 统一表意
		(r >= 0xA000 && r <= 0xA4CF) || // 彝文
		(r >= 0xA960 && r <= 0xA97F) || // 谚文扩展 A
		(r >= 0xAC00 && r <= 0xD7A3) || // 谚文音节
		(r >= 0xF900 && r <= 0xFAFF) || // CJK 兼容表意
		(r >= 0xFE10 && r <= 0xFE19) || // 竖排标点
		(r >= 0xFE30 && r <= 0xFE6F) || // CJK 兼容形式
		(r >= 0xFF00 && r <= 0xFF60) || // 全角 ASCII
		(r >= 0xFFE0 && r <= 0xFFE6) || // 全角符号
		(r >= 0x1F300 && r <= 0x1F64F) || // Emoji
		(r >= 0x1F900 && r <= 0x1F9FF) || // 补充 Emoji
		(r >= 0x20000 && r <= 0x3FFFD) { // CJK 扩展 B+
		return 2
	}
	return 1
}

// VisualWidth 返回字符串在终端里占用的列数。
func VisualWidth(s string) int {
	w := 0
	for _, r := range s {
		w += RuneWidth(r)
	}
	return w
}

// Truncate 把字符串截到最多 limit 列宽，超出部分用省略号表示。
//
// 关键约束：不会把双宽字符切成半个 —— 中文被截断时宁可少放一个字，
// 也要保证返回值的视觉宽度 <= limit。
func Truncate(s string, limit int) string {
	if limit <= 0 {
		return ""
	}
	if VisualWidth(s) <= limit {
		return s
	}
	// 省略号本身占 1 列，先给它留位置。
	budget := limit - 1
	if budget < 0 {
		budget = 0
	}

	var b strings.Builder
	w := 0
	for _, r := range s {
		rw := RuneWidth(r)
		if w+rw > budget {
			break
		}
		b.WriteRune(r)
		w += rw
	}
	return b.String() + "…"
}

// PadRight 把字符串补齐到恰好 width 列宽（右补空格）；
// 已经超宽时先截断 —— 这是「表格绝不能错位」的硬保证。
//
// 旧实现「超宽就原样返回」会让整行往右偏，表头与数据对不上。
func PadRight(s string, width int) string {
	if width <= 0 {
		return ""
	}
	s = Truncate(s, width)
	if w := VisualWidth(s); w < width {
		return s + strings.Repeat(" ", width-w)
	}
	return s
}

// Wrap 按视觉列宽把文本折成多行。
//
// 折行策略：
//   - 优先在空格处断行（英文可读性）
//   - 没有空格可断时按字符断（中文段落本来就没有空格）
//   - 不吞字符，去掉换行拼回去应与原文一致
func Wrap(s string, width int) []string {
	if width < 1 {
		width = 1
	}
	if VisualWidth(s) <= width {
		return []string{s}
	}

	var lines []string
	var cur strings.Builder
	curW := 0
	// lastSpace 记录当前行最后一个断点的字节位置，便于回退到词边界。
	lastSpace := -1

	flush := func() {
		lines = append(lines, cur.String())
		cur.Reset()
		curW = 0
		lastSpace = -1
	}

	for _, r := range s {
		rw := RuneWidth(r)
		if curW+rw > width && cur.Len() > 0 {
			// 有空格断点且不是刚开头：把最后一个词挪到下一行。
			if lastSpace >= 0 && lastSpace < cur.Len() {
				line := cur.String()
				head, tail := line[:lastSpace], line[lastSpace+1:]
				lines = append(lines, head)
				cur.Reset()
				cur.WriteString(tail)
				curW = VisualWidth(tail)
				lastSpace = -1
			} else {
				flush()
			}
		}
		if r == ' ' {
			lastSpace = cur.Len()
		}
		cur.WriteRune(r)
		curW += rw
	}
	if cur.Len() > 0 || len(lines) == 0 {
		flush()
	}
	return lines
}

// TermWidth 返回当前终端宽度（列）。
//
// 数据来源优先级：
//  1. COLUMNS 环境变量 —— 唯一跨平台可靠来源，且非 tty（管道/CI）下也有效
//  2. 兜底 DefaultWidth
//
// 不调 ioctl：那要引入 golang.org/x/sys 的按平台分支，而我们只需要
// 「大致多宽」，且 COLUMNS 在交互式 shell 里总是被导出。
func TermWidth() int {
	if v := strings.TrimSpace(os.Getenv("COLUMNS")); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			if n < MinWidth {
				return MinWidth
			}
			return n
		}
	}
	return DefaultWidth
}
