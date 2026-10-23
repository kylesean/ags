package main

import (
	"context"
	"flag"
	"fmt"
	"time"

	"github.com/kylesean/ags/internal/i18n"

	"github.com/kylesean/ags/internal/pool"
	"github.com/kylesean/ags/internal/quota"
)

// cmdUse 把指定账号设为系统 Keyring 当前身份（agy 唯一授权槽）。
//
// 先验额度：无额度拒绝并提示（--force 强行）；写入后提醒重启 agy。
func cmdUse(args []string) error {
	fs := flag.NewFlagSet("use", flag.ContinueOnError)
	upstream := fs.String("upstream", DefaultUpstream, i18n.Tr("flag.upstream"))
	timeout := fs.Duration("timeout", 15*time.Second, i18n.Tr("flag.useTimeout"))
	threshold := fs.Float64("quota-threshold", 0, i18n.Tr("flag.quotaThreshold"))
	force := fs.Bool("force", false, i18n.Tr("flag.force"))
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 1 {
		return i18n.Errorf("use.usage")
	}
	if *timeout <= 0 {
		return i18n.Errorf("use.timeout")
	}
	name := fs.Arg(0)

	a, err := pool.Load(name)
	if err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(context.Background(), *timeout)
	defer cancel()

	// token 临近过期先续，保证查到的是该号当下真实额度。
	if a.AccessToken == "" || (!a.Expiry.IsZero() && !time.Now().Before(a.Expiry.Add(-pool.DefaultRefreshSkew))) {
		if a.RefreshToken == "" {
			return i18n.Errorf("use.noToken", name)
		}
		if err := refreshAccount(ctx, a); err != nil {
			return i18n.Errorf("use.refreshFail", name, err)
		}
		// 刷新落盘后重读，保证 keyring 写入与磁盘一致。
		if a, err = pool.Load(name); err != nil {
			return err
		}
	}

	sum, err := quota.Fetch(ctx, *upstream, a.AccessToken, upstreamUserAgent(ctx))
	if err != nil {
		return i18n.Errorf("use.quotaFail", name, err)
	}
	if g := sum.Gemini(); g != nil {
		if until, exhausted := g.Exhausted(*threshold, time.Now()); exhausted && !*force {
			return i18n.Errorf("use.exhausted",
				name, g.Describe(), pool.FormatCooldown(until))
		}
	}

	if err := syncKeyringAccount(accountSwitch{name: a.Name, email: a.Email}); err != nil {
		return err
	}
	fmt.Println(i18n.Trf("use.switched", a.Name, a.Email))
	fmt.Println(i18n.Tr("use.restart"))
	return nil
}
