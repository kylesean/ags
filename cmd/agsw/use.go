package main

import (
	"context"
	"flag"
	"fmt"
	"time"

	"github.com/kylesean/agsw/internal/pool"
	"github.com/kylesean/agsw/internal/quota"
)

// cmdUse 把指定账号设为系统 Keyring 当前身份（agy 唯一授权槽）。
//
// 先验额度：无额度拒绝并提示（--force 强行）；写入后提醒重启 agy。
func cmdUse(args []string) error {
	fs := flag.NewFlagSet("use", flag.ContinueOnError)
	upstream := fs.String("upstream", DefaultUpstream, "CloudCode 上游地址")
	timeout := fs.Duration("timeout", 15*time.Second, "额度查询超时")
	threshold := fs.Float64("quota-threshold", 0, "额度耗尽阈值")
	force := fs.Bool("force", false, "无视额度耗尽强行切换")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 1 {
		return fmt.Errorf("用法: agsw use [--force] <name>")
	}
	if *timeout <= 0 {
		return fmt.Errorf("-timeout 必须大于 0")
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
			return fmt.Errorf("账号 %s 无可用 token 且无 refresh_token", name)
		}
		if err := refreshAccount(ctx, a); err != nil {
			return fmt.Errorf("刷新 %s 失败: %w", name, err)
		}
		// 刷新落盘后重读，保证 keyring 写入与磁盘一致。
		if a, err = pool.Load(name); err != nil {
			return err
		}
	}

	sum, err := quota.Fetch(ctx, *upstream, a.AccessToken, defaultUserAgent)
	if err != nil {
		return fmt.Errorf("查询 %s 额度失败: %w", name, err)
	}
	if g := sum.Gemini(); g != nil {
		if until, exhausted := g.Exhausted(*threshold, time.Now()); exhausted && !*force {
			return fmt.Errorf("账号 %s 额度耗尽 (%s，冷却至 %s)，拒绝切换；用 --force 强行",
				name, g.Describe(), pool.FormatCooldown(until))
		}
	}

	if err := syncKeyringAccount(accountSwitch{name: a.Name, email: a.Email}); err != nil {
		return err
	}
	fmt.Printf("已切换 keyring → %s (%s)\n", a.Name, a.Email)
	fmt.Println("请重启 agy 生效（交互式会话执行 /resume 恢复）")
	return nil
}
