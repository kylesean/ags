package main

import (
	"context"
	"flag"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/kylesean/ags/internal/i18n"
	"github.com/kylesean/ags/internal/pool"
	"github.com/kylesean/ags/internal/quota"
	"github.com/kylesean/ags/internal/term"
)

// barWidth 是进度条内部的格子数。
const barWidth = 24

// usageBar 画一条额度条：[████████░░░░░░░░░░░░░░░░] 47.44%
func usageBar(fraction float64) string {
	if fraction < 0 {
		fraction = 0
	}
	if fraction > 1 {
		fraction = 1
	}
	filled := int(fraction*float64(barWidth) + 0.5)
	if filled < 0 {
		filled = 0
	}
	if filled > barWidth {
		filled = barWidth
	}
	return "[" + strings.Repeat("█", filled) + strings.Repeat("░", barWidth-filled) + "]"
}

// humanizeReset 把「还有多久重置」说成人话。
//
// 片段按语言拼接：中文「1天2小时」、英文「1d 2h」、日文「1日2時間」，
// 数量词与语序都不同，所以不能用一个格式串硬套。
func humanizeReset(reset, now time.Time) string {
	if reset.IsZero() {
		return ""
	}
	d := reset.Sub(now)
	if d <= 0 {
		return ""
	}
	// 向上取整到分钟：剩 30 秒说「1 分钟」，比「0 分钟」有用。
	mins := int64((d + time.Minute - 1) / time.Minute)
	days := mins / (24 * 60)
	mins -= days * 24 * 60
	hours := mins / 60
	mins -= hours * 60

	switch {
	case days > 0 && hours > 0:
		return i18n.Trf("usage.durDaysHours", days, hours)
	case days > 0:
		return i18n.Trf("usage.durDays", days)
	case hours > 0 && mins > 0:
		return i18n.Trf("usage.durHoursMins", hours, mins)
	case hours > 0:
		return i18n.Trf("usage.durHours", hours)
	default:
		return i18n.Trf("usage.durMins", mins)
	}
}

// windowLabel 把上游的 bucket 翻成当前语言的窗口标题。
func windowLabel(b *quota.Bucket) string {
	if b == nil {
		return ""
	}
	switch strings.ToLower(b.Window) {
	case "5h", "five_hour", "fivehour":
		return i18n.Tr("usage.window5h")
	case "weekly", "week":
		return i18n.Tr("usage.windowWeekly")
	}
	switch {
	case strings.HasSuffix(b.BucketID, "-5h"):
		return i18n.Tr("usage.window5h")
	case strings.HasSuffix(b.BucketID, "-weekly"):
		return i18n.Tr("usage.windowWeekly")
	}
	return b.DisplayName
}

// bucketStatus 是窗口状态的一句话。
func bucketStatus(b *quota.Bucket, now time.Time) string {
	if b == nil {
		return ""
	}
	rem, ok := b.Remaining()
	if !ok {
		return i18n.Tr("usage.unknown")
	}
	if rem > 0 {
		return i18n.Tr("usage.available")
	}
	if in := humanizeReset(b.Reset(), now); in != "" {
		return i18n.Trf("usage.refreshesIn", in)
	}
	return i18n.Tr("usage.refreshesSoon")
}

// groupTitle 是组标题。上游给的 displayName 是英文专名（"Gemini Models"、
// "Claude and GPT models"），**不翻译** —— 它们与 agy 原生界面一致，
// 翻了反而对不上。只做大小写整形。
func groupTitle(g quota.Group) string {
	name := strings.TrimSpace(g.DisplayName)
	if name == "" {
		return "UNKNOWN GROUP"
	}
	return strings.ToUpper(name)
}

// groupRank 让输出顺序稳定：GEMINI → 3p → Argon。
func groupRank(displayName string) int {
	n := strings.ToLower(displayName)
	switch {
	case strings.Contains(n, "argon"):
		return 2
	case strings.Contains(n, "gemini"):
		return 0
	default:
		return 1
	}
}

func sortGroups(groups []quota.Group) []quota.Group {
	out := append([]quota.Group(nil), groups...)
	sort.SliceStable(out, func(i, j int) bool {
		ri, rj := groupRank(out[i].DisplayName), groupRank(out[j].DisplayName)
		if ri != rj {
			return ri < rj
		}
		return out[i].DisplayName < out[j].DisplayName
	})
	return out
}

// indent 是每行内容的前导空格（与标签同宽，便于阅读）。
const usageIndent = "  "

// renderAccountUsage 渲染一个账号的完整额度块。
//
// 布局要点（多语言下最容易崩的地方）：
//   - description 实测 200+ 字符，必须按终端宽度折行，否则溢出
//   - 窗口标题与状态行长度随语言变化，一律走 Wrap
//   - 进度条用固定格数，不随语言变化，保证各账号之间可纵向对比
func renderAccountUsage(name, email string, sum *quota.Summary, now time.Time) string {
	var b strings.Builder
	b.WriteString(i18n.Trf("usage.account", email) + "\n")
	if name != "" {
		b.WriteString(i18n.Trf("usage.pool", name) + "\n")
	}

	groups := sortGroups(sum.Groups)
	if len(groups) == 0 {
		b.WriteString("\n" + i18n.Tr("usage.noGroups") + "\n")
		return b.String()
	}

	// 可用宽度要扣掉缩进，否则折行后右侧会被终端截断。
	width := term.TermWidth()
	bodyWidth := width - len(usageIndent)
	if bodyWidth < 20 {
		bodyWidth = 20
	}

	for _, g := range groups {
		b.WriteString("\n" + groupTitle(g) + "\n")
		if desc := strings.TrimSpace(g.Description); desc != "" {
			for _, line := range term.Wrap(desc, bodyWidth) {
				b.WriteString(usageIndent + line + "\n")
			}
		}
		for i := range g.Buckets {
			bk := &g.Buckets[i]
			label := windowLabel(bk)
			if label == "" {
				continue
			}
			b.WriteString("\n")
			for _, line := range term.Wrap(label, bodyWidth) {
				b.WriteString(usageIndent + line + "\n")
			}

			rem, ok := bk.Remaining()
			if !ok {
				b.WriteString(usageIndent + "[" + strings.Repeat(" ", barWidth) + "] " +
					i18n.Tr("usage.unknown") + "\n")
				continue
			}
			b.WriteString(fmt.Sprintf("%s%s %.2f%%\n", usageIndent, usageBar(rem), rem*100))
			if st := bucketStatus(bk, now); st != "" {
				for _, line := range term.Wrap(st, bodyWidth) {
					b.WriteString(usageIndent + line + "\n")
				}
			}
		}
	}
	return b.String()
}

// formatUsageLine 是紧凑的一行式输出（表格用），保留给脚本/管道。
//
// 注意：这里是**机器可读**格式，刻意不本地化 —— 中英混排的表格会让
// awk/cut 的列切分变得不可预测。表头只在 -compact 时本地化。
func formatUsageLine(a *pool.Account, sum *quota.Summary) string {
	g := sum.Gemini()
	if g == nil {
		return fmt.Sprintf("%s\t%s\t%s", a.Name, a.Email, i18n.Tr("usage.noGeminiGroup"))
	}
	return fmt.Sprintf("%s\t%s\t%s", a.Name, a.Email, g.Describe())
}

func formatUsageError(a *pool.Account, err error) string {
	return fmt.Sprintf("%s\t%s\t%s: %v", a.Name, a.Email, i18n.Tr("common.failed"), err)
}

// cmdUsage 直接查询账号池的真实额度，不依赖 agy 的 /usage 命令。
func cmdUsage(args []string) error {
	fs := flag.NewFlagSet("usage", flag.ContinueOnError)
	account := fs.String("account", "", i18n.Tr("flag.account"))
	upstream := fs.String("upstream", DefaultUpstream, i18n.Tr("flag.upstream"))
	timeout := fs.Duration("timeout", 15*time.Second, i18n.Tr("flag.usageTimeout"))
	compact := fs.Bool("compact", false, i18n.Tr("flag.compact"))
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() > 0 {
		return i18n.Errorf("usage.noArgs")
	}
	if *timeout <= 0 {
		return i18n.Errorf("use.timeout")
	}

	accounts, err := resolveCandidates(*account)
	if err != nil {
		return err
	}
	// 额度组同样受 UA 版本影响：旧版本只回 3p 组，看起来像「没有 GEMINI 组」。
	ua := upstreamUserAgent(context.Background())

	type result struct {
		acc *pool.Account
		sum *quota.Summary
		err error
	}
	results := make([]result, len(accounts))
	var wg sync.WaitGroup

	for i, a := range accounts {
		wg.Add(1)
		go func(idx int, acc *pool.Account) {
			defer wg.Done()
			results[idx].acc = acc

			accountCtx, cancel := context.WithTimeout(context.Background(), *timeout)
			defer cancel()

			needsRefresh := acc.AccessToken == "" ||
				(!acc.Expiry.IsZero() && !time.Now().Before(acc.Expiry.Add(-5*time.Minute)))
			if needsRefresh && acc.RefreshToken != "" {
				if err := refreshAccount(accountCtx, acc); err != nil {
					results[idx].err = err
					return
				}
			}
			sum, err := quota.Fetch(accountCtx, *upstream, acc.AccessToken, ua)
			if err != nil {
				results[idx].err = err
				return
			}
			results[idx].sum = sum
		}(i, a)
	}
	wg.Wait()

	if *compact {
		fmt.Println(i18n.Tr("usage.compactHeader"))
	}
	now := time.Now()
	failed := 0
	for _, r := range results {
		if r.err != nil {
			failed++
			if *compact {
				fmt.Println(formatUsageError(r.acc, r.err))
			} else {
				fmt.Print(i18n.Trf("usage.accountFail", r.acc.Email, r.err))
				fmt.Println()
			}
			continue
		}
		if *compact {
			fmt.Println(formatUsageLine(r.acc, r.sum))
			continue
		}
		fmt.Print(renderAccountUsage(r.acc.Name, r.acc.Email, r.sum, now))
		fmt.Println()
	}

	// 错误已经逐账号打印过了：只回一句摘要，别让 main 再刷一遍。
	if failed > 0 {
		return i18n.Errorf("usage.failSummary", failed, len(accounts))
	}
	return nil
}
