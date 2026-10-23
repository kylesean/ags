package main

import (
	"flag"
	"fmt"
	"strings"
	"time"

	"github.com/kylesean/ags/internal/i18n"
	"github.com/kylesean/ags/internal/keyring"
	"github.com/kylesean/ags/internal/pool"
	"github.com/kylesean/ags/internal/term"
)

// cmdAdd 把当前 keyring 里的凭据捕获进账号池。
//
// 注册第二个号的流程：先在 agy 里正常登一次该号，然后 ags add <name>。
// 捕获之后那个号就永久免登了。
func cmdAdd(args []string) error {
	fs := flag.NewFlagSet("add", flag.ContinueOnError)
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 1 {
		return i18n.Errorf("add.usage")
	}
	name := fs.Arg(0)
	if err := pool.ValidateName(name); err != nil {
		return err
	}

	// 幂等：同名账号要求显式覆盖，避免误冲。
	if _, err := pool.Load(name); err == nil {
		return i18n.Errorf("add.exists", name, name)
	}

	sec, email, err := keyring.Current()
	if err != nil {
		return i18n.Errorf("add.readFail", err)
	}
	existing, err := pool.FindByEmail(email)
	if err != nil {
		return i18n.Errorf("add.emailChk", err)
	}
	if existing != nil {
		return i18n.Errorf("add.emailDup", email, existing.Name, existing.Name)
	}
	claims, err := sec.Claims()
	if err != nil {
		return i18n.Errorf("add.claimsFail", err)
	}

	a := &pool.Account{
		Name:         name,
		Email:        email,
		ClientID:     claims.Audience(),
		AddedAt:      time.Now().UTC(),
		AccessToken:  sec.Token.AccessToken,
		RefreshToken: sec.Token.RefreshToken,
		Expiry:       sec.Token.Expiry.Time,
		IDToken:      sec.IDToken,
		AuthMethod:   sec.AuthMethod,
	}
	if err := pool.Save(a); err != nil {
		return err
	}

	dir, _ := pool.Dir()
	// 标签列按视觉宽度对齐：中日韩标签占 2 列，英文占 1 列，硬编码宽度会错位。
	labelW := labelColumnWidth
	kv := func(label, value string) {
		fmt.Printf("  %s%s\n", term.PadRight(label, labelW), value)
	}

	fmt.Println(i18n.Trf("add.captured", name))
	kv(i18n.Tr("col.email"), email)
	kv(i18n.Tr("col.expiry"), fmtExpiry(a.Expiry))
	kv("client_id", orDash(a.ClientID))
	if a.RefreshToken == "" {
		kv(i18n.Tr("common.warning"), i18n.Tr("add.noRefresh"))
	}
	kv(i18n.Tr("add.stored"), fmt.Sprintf("%s (0600)", dir))
	return nil
}

// labelColumnWidth 是「标签: 值」里标签列的目标宽度。
//
// 取固定值而不是自动测：多语言下标签长度差异大（"过期时间" 8 列 vs
// "expires" 7 列），固定宽度让不同语言的输出看起来一致；内容超宽时
// term.PadRight 会截断，保证右侧值仍然对齐。
const labelColumnWidth = 14

// cmdList 列出账号池。
func cmdList(args []string) error {
	fs := flag.NewFlagSet("list", flag.ContinueOnError)
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() > 0 {
		return i18n.Errorf("list.noArgs")
	}

	accounts, err := pool.List()
	if err != nil {
		return err
	}
	// 展示有效候选，和 serve 使用相同规则；旧重复文件仍可通过 drop 清理。
	accounts = pool.Unique(accounts)
	if len(accounts) == 0 {
		dir, _ := pool.Dir()
		fmt.Println(i18n.Trf("list.empty", dir))
		fmt.Println(i18n.Tr("list.emptyHint"))
		return nil
	}

	// 当前 keyring 账号用于标注，读不到也不该让 list 挂掉。
	curEmail := ""
	if _, email, err := keyring.Current(); err == nil {
		curEmail = email
	}

	rows := []listRow{{
		name:    i18n.Tr("col.name"),
		email:   i18n.Tr("col.email"),
		expiry:  i18n.Tr("col.expiry"),
		refresh: i18n.Tr("col.refresh"),
		current: i18n.Tr("col.current"),
	}}

	for _, a := range accounts {
		refresh := i18n.Tr("common.yes")
		if a.RefreshToken == "" {
			refresh = i18n.Tr("common.no")
		}
		mark := ""
		if a.Email == curEmail && curEmail != "" {
			mark = "●"
		}
		rows = append(rows, listRow{
			name:    a.Name,
			email:   a.Email,
			expiry:  fmtExpiry(a.Expiry),
			refresh: refresh,
			current: mark,
		})
	}

	// 列宽按「所有语言里最宽的」算，保证切换语言时表格结构不变形。
	widths := listColumnWidths(rows)

	const colGap = 2
	for _, r := range rows {
		fmt.Printf("%s%s%s%s%s\n",
			term.PadRight(r.name, widths[0]+colGap),
			term.PadRight(r.email, widths[1]+colGap),
			term.PadRight(r.expiry, widths[2]+colGap),
			term.PadRight(r.refresh, widths[3]+colGap),
			r.current,
		)
	}

	if curEmail != "" {
		fmt.Printf("\n%s\n", i18n.Trf("list.keyring", curEmail))
	} else {
		fmt.Printf("\n%s\n", i18n.Tr("list.keyringUnreadable"))
	}
	return nil
}

// listRow 是 list 的一行（抽成类型便于算列宽与测试）。
type listRow = struct {
	name    string
	email   string
	expiry  string
	refresh string
	current string
}

// listColumnWidths 返回四列的内容宽度（不含列间距）。
//
// 关键：宽度取「当前语言实际内容」与「所有语言列头」的较大者。
// 这样 en/ja 切换时列不会突然变窄或变宽，表格始终是同一副骨架。
//
// 窄终端下必须整体收缩，否则总宽超出屏幕会让终端自己折行，
// 表格结构就彻底乱了（这比截断更难读）。
func listColumnWidths(rows []listRow) [4]int {
	var w [4]int
	// 先按所有语言的列头取最大，保证切换语言时表头不被截断。
	headerKeys := [4]string{"col.name", "col.email", "col.expiry", "col.refresh"}
	for _, lang := range i18n.Supported() {
		for i, k := range headerKeys {
			if v := term.VisualWidth(i18n.T(lang, k)); v > w[i] {
				w[i] = v
			}
		}
	}
	for _, r := range rows {
		vals := [4]string{r.name, r.email, r.expiry, r.refresh}
		for i, v := range vals {
			if vw := term.VisualWidth(v); vw > w[i] {
				w[i] = vw
			}
		}
	}
	// 邮箱列很长（可达 30+ 列），窄终端下必须让位给其他列。
	w[1] = clampWidth(w[1], 12, 32)

	// 总宽（含列间距）压进终端宽度：从最宽的列开始砍。
	const colGap = 2
	budget := term.TermWidth() - colGap*4
	for {
		total := 0
		for _, v := range w {
			total += v
		}
		if total <= budget {
			break
		}
		// 找当前最宽的列，砍到比第二宽少一列；都砍到底就停（避免死循环）。
		widest, second := 0, 0
		for i, v := range w {
			if v > w[widest] {
				second = w[widest]
				widest = i
			} else if i != widest && v > second {
				second = v
			}
		}
		target := second
		if target < 4 {
			target = 4
		}
		if w[widest] <= target {
			// 全都压到底了，只能接受（极窄终端）。
			break
		}
		w[widest] = target
	}
	return w
}

// clampWidth 把列宽压进 [min,max]。
func clampWidth(v, min, max int) int {
	if v < min {
		return min
	}
	if v > max {
		return max
	}
	return v
}

// cmdStatus 展示当前 keyring 账号与池状态。
func cmdStatus(args []string) error {
	fs := flag.NewFlagSet("status", flag.ContinueOnError)
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() > 0 {
		return i18n.Errorf("status.noArgs")
	}

	fmt.Println(i18n.Tr("status.keyringHdr"))
	sec, email, err := keyring.Current()
	if err != nil {
		fmt.Println(i18n.Trf("status.readFail", err))
	} else {
		labelW := labelColumnWidth
		kv := func(label, value string) {
			fmt.Printf("  %s%s\n", term.PadRight(label, labelW), value)
		}
		kv(i18n.Tr("status.email"), email)
		kv(i18n.Tr("status.auth"), orDash(sec.AuthMethod))
		kv(i18n.Tr("status.expiry"), fmtExpiry(sec.Token.Expiry.Time))
		kv(i18n.Tr("status.refresh"), present(sec.Token.RefreshToken != ""))
	}

	printRestartHint := func() {
		if st, err := readDaemonState(defaultDaemonStatePath()); err == nil {
			if hint := pendingRestartHint(email, st); hint != "" {
				fmt.Printf("\n%s\n", hint)
			}
		}
	}

	fmt.Printf("\n%s\n", i18n.Tr("status.poolHdr"))
	accounts, err := pool.List()
	if err != nil {
		return err
	}
	// 状态展示同样隐藏旧池中的同邮箱重复文件。
	accounts = pool.Unique(accounts)
	if len(accounts) == 0 {
		fmt.Printf("  %s\n", i18n.Tr("status.poolEmpty"))
		printRestartHint()
		return nil
	}

	// 名字列按最长的名字对齐（旧实现硬编码 12，名字一长就错位）。
	nameW := 0
	for _, a := range accounts {
		if w := term.VisualWidth(a.Name); w > nameW {
			nameW = w
		}
	}
	nameW = clampWidth(nameW, 4, 20)
	nameW += 2 // 列间距

	for _, a := range accounts {
		extra := ""
		if !a.CooldownUntil.IsZero() && time.Now().Before(a.CooldownUntil) {
			extra = " [" + i18n.Tr("common.cooling") + "]"
		}
		fmt.Println(i18n.Trf("status.accountLine",
			term.PadRight(a.Name, nameW), a.Email, fmtExpiry(a.Expiry), extra))
	}
	dir, _ := pool.Dir()
	fmt.Printf("\n%s\n", i18n.Trf("status.poolDir", dir))
	printRestartHint()
	return nil
}

// cmdDrop 删除池中一个账号。
func cmdDrop(args []string) error {
	fs := flag.NewFlagSet("drop", flag.ContinueOnError)
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 1 {
		return i18n.Errorf("drop.usage")
	}
	name := fs.Arg(0)
	if err := pool.Delete(name); err != nil {
		return err
	}
	fmt.Println(i18n.Trf("drop.deleted", name))
	return nil
}

// fmtExpiry 把过期时刻说成人话。
func fmtExpiry(t time.Time) string {
	if t.IsZero() {
		return i18n.Tr("common.dash")
	}
	d := time.Until(t)
	if d < 0 {
		return i18n.Tr("common.expired")
	}
	if d < time.Hour {
		// Round 到分钟后直接格式化，避免出现 "38m0s" 这种带秒的残留。
		rounded := d.Round(time.Minute)
		if rounded < time.Minute {
			rounded = time.Minute
		}
		return i18n.Trf("common.inDuration", fmt.Sprintf("%dm", int(rounded.Minutes())))
	}
	return t.Local().Format("01-02 15:04")
}

func orDash(s string) string {
	if s == "" {
		return i18n.Tr("common.dash")
	}
	return s
}

func present(ok bool) string {
	if ok {
		return i18n.Tr("common.present")
	}
	return i18n.Tr("common.absent")
}

// 兼容层：旧代码直接调这些函数，现在全部委托给 internal/term。
// 保留它们是为了让 TUI/其它包不必一次性改完，也便于逐步迁移。
func runeWidth(r rune) int            { return term.RuneWidth(r) }
func visualWidth(s string) int        { return term.VisualWidth(s) }
func padRight(s string, n int) string { return term.PadRight(s, n) }

var _ = strings.Repeat
