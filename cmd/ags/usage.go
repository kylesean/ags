package main

import (
	"context"
	"flag"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/kylesean/ags/internal/pool"
	"github.com/kylesean/ags/internal/quota"
)

// barWidth 是进度条内部的格子数。agy 原生是 24 格左右，
// 这里取 24 让同一行在 80 列终端里放得下（含方括号与百分比）。
const barWidth = 24

// usageBar 画一条额度条：[████████░░░░░░░░░░░░░░░░] 47.44%
//
// 与 agy 原生 /usage 的视觉一致：方括号包住、填充表示「剩余」。
// filled 用 █ 而不是色块字符，避免中文宽字符环境下宽度算错。
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

// humanizeReset 把「还有多久重置」说成人话，与 agy 的
// "Refreshes in 26h 12m" 对齐。已经过期返回空串。
func humanizeReset(reset, now time.Time) string {
	if reset.IsZero() {
		return ""
	}
	d := reset.Sub(now)
	if d <= 0 {
		return ""
	}
	// 向上取整到分钟：剩 30 秒说 "1m"，比 "0m" 有用。
	mins := int64((d + time.Minute - 1) / time.Minute)
	days := mins / (24 * 60)
	mins -= days * 24 * 60
	hours := mins / 60
	mins -= hours * 60

	switch {
	case days > 0 && hours > 0:
		return fmt.Sprintf("%dd %dh", days, hours)
	case days > 0:
		return fmt.Sprintf("%dd", days)
	case hours > 0 && mins > 0:
		return fmt.Sprintf("%dh %dm", hours, mins)
	case hours > 0:
		return fmt.Sprintf("%dh", hours)
	default:
		return fmt.Sprintf("%dm", mins)
	}
}

// windowLabel 把上游的 bucket 名翻译成人话标题，与 agy 原生的
// "Weekly Limit Remaining" / "Five Hour Limit Remaining" 一致。
func windowLabel(b *quota.Bucket) string {
	if b == nil {
		return ""
	}
	// 优先按 window（语义键），bucketId 兜底。
	switch strings.ToLower(b.Window) {
	case "5h", "five_hour", "fivehour":
		return "Five Hour Limit Remaining"
	case "weekly", "week":
		return "Weekly Limit Remaining"
	}
	switch {
	case strings.HasSuffix(b.BucketID, "-5h"):
		return "Five Hour Limit Remaining"
	case strings.HasSuffix(b.BucketID, "-weekly"):
		return "Weekly Limit Remaining"
	}
	return b.DisplayName
}

// bucketStatus 是固定句式的一句状态，与 agy 原生一致：
// 归零说「什么时候刷新」，还有额度就说 "Quota available"。
func bucketStatus(b *quota.Bucket, now time.Time) string {
	if b == nil {
		return ""
	}
	rem, ok := b.Remaining()
	if !ok {
		return "Quota unknown"
	}
	if rem > 0 {
		return "Quota available"
	}
	if in := humanizeReset(b.Reset(), now); in != "" {
		return "Refreshes in " + in
	}
	return "Refreshes soon"
}

// groupTitle 把额度组标题转成 agy 的写法：全大写。
// 上游给的是 "Gemini Models" / "Claude and GPT models"。
func groupTitle(g quota.Group) string {
	name := strings.TrimSpace(g.DisplayName)
	if name == "" {
		return "UNKNOWN GROUP"
	}
	return strings.ToUpper(name)
}

// defaultGroupOrder 让输出顺序稳定：主 GEMINI 组在前，3p 组其次，
// Argon 最后 —— 与 agy 原生的排列一致。
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

// sortGroups 返回按 rank 稳定排序后的组。
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

// renderAccountUsage 渲染一个账号的完整额度块，模仿 agy 原生 /usage：
//
//	Account: a@example.com
//
//	GEMINI MODELS
//	  Models within this group: Gemini Flash, Gemini Pro
//
//	  Weekly Limit Remaining
//	  [██████████░░░░░░░░░░░░░░] 47.44%
//	  Refreshes in 26h 12m
//
//	Five Hour Limit Remaining
//	[████████████████████████] 100.00%
//	Quota available
func renderAccountUsage(name, email string, sum *quota.Summary, now time.Time) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Account: %s\n", email)
	if name != "" {
		fmt.Fprintf(&b, "Pool: %s\n", name)
	}

	groups := sortGroups(sum.Groups)
	if len(groups) == 0 {
		b.WriteString("\n未找到任何额度组（上游可能改接口或 UA 已过期）\n")
		return b.String()
	}

	for _, g := range groups {
		b.WriteString("\n")
		b.WriteString(groupTitle(g))
		b.WriteString("\n")
		if desc := strings.TrimSpace(g.Description); desc != "" {
			fmt.Fprintf(&b, "  %s\n", desc)
		}
		for i := range g.Buckets {
			bk := &g.Buckets[i]
			label := windowLabel(bk)
			if label == "" {
				continue
			}
			b.WriteString("\n")
			fmt.Fprintf(&b, "  %s\n", label)
			rem, ok := bk.Remaining()
			if !ok {
				b.WriteString("  [                        ] 未知\n")
				continue
			}
			fmt.Fprintf(&b, "  %s %.2f%%\n", usageBar(rem), rem*100)
			if st := bucketStatus(bk, now); st != "" {
				fmt.Fprintf(&b, "  %s\n", st)
			}
		}
	}
	return b.String()
}

// formatUsageLine 是紧凑的一行式输出（表格用），保留给脚本/管道。
func formatUsageLine(a *pool.Account, sum *quota.Summary) string {
	g := sum.Gemini()
	if g == nil {
		return fmt.Sprintf("%s\t%s\t未找到 GEMINI 额度组", a.Name, a.Email)
	}
	return fmt.Sprintf("%s\t%s\t%s", a.Name, a.Email, g.Describe())
}

func formatUsageError(a *pool.Account, err error) string {
	return fmt.Sprintf("%s\t%s\t查询失败: %v", a.Name, a.Email, err)
}

// cmdUsage 直接查询账号池的真实额度，不依赖 agy 的 /usage 命令。
func cmdUsage(args []string) error {
	fs := flag.NewFlagSet("usage", flag.ContinueOnError)
	account := fs.String("account", "", "只查询指定账号")
	upstream := fs.String("upstream", DefaultUpstream, "CloudCode 上游地址")
	timeout := fs.Duration("timeout", 15*time.Second, "每个账号的额度查询超时")
	compact := fs.Bool("compact", false, "单行紧凑输出（脚本/管道用）")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() > 0 {
		return fmt.Errorf("usage 不接受位置参数")
	}
	if *timeout <= 0 {
		return fmt.Errorf("-timeout 必须大于 0")
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

	var errs []error
	if *compact {
		fmt.Println("账号\t邮箱\t额度")
	}
	now := time.Now()
	failed := 0
	for _, r := range results {
		if r.err != nil {
			errs = append(errs, r.err)
			failed++
			if *compact {
				fmt.Println(formatUsageError(r.acc, r.err))
			} else {
				fmt.Printf("Account: %s\n查询失败: %v\n\n", r.acc.Email, r.err)
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

	// 错误已经逐账号打印过了：这里只回一个「有多少个失败」的摘要，
	// 让退出码非零，但别把同样的三条错误再刷一遍。
	if len(errs) > 0 {
		return fmt.Errorf("%d/%d 个账号查询失败", failed, len(accounts))
	}
	return nil
}
