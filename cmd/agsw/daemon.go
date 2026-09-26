package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"github.com/kylesean/agsw/internal/pool"
)

// cmdDaemon 前台常驻的轻量本体：定期轮询额度 → keyring 写健康号。
// 不碰流量、不托管 agy；切换后提醒重启（交互式 /resume 恢复）。
func cmdDaemon(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("daemon", flag.ContinueOnError)
	interval := fs.Duration("quota-interval", DefaultQuotaInterval, "额度轮询间隔；设为 0 则只跑一轮即退")
	threshold := fs.Float64("quota-threshold", 0, "额度耗尽阈值")
	account := fs.String("account", "", "只看管指定账号；留空则看管池中全部")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() > 0 {
		return fmt.Errorf("daemon 不接受位置参数")
	}
	if *interval < 0 {
		return fmt.Errorf("-quota-interval 不能为负（收到 %s）", *interval)
	}
	lg := log.New(os.Stderr, "[daemon] ", log.LstdFlags|log.Lmsgprefix)

	var cands []*pool.Account
	if *account != "" {
		a, err := pool.Load(*account)
		if err != nil {
			return err
		}
		cands = []*pool.Account{a}
	} else {
		accounts, err := pool.List()
		if err != nil {
			return err
		}
		cands = pool.Unique(accounts)
		if len(cands) == 0 {
			return fmt.Errorf("账号池为空，先 agsw login <name> 或 agsw add <name>")
		}
	}

	sel := pool.NewSelector(cands, refreshAccount)
	qw := newQuotaWatcher(sel, DefaultUpstream, defaultUserAgent, *threshold, lg)

	qw.check(ctx, true)
	if _, err := daemonSyncOnce(ctx, sel, lg); err != nil {
		lg.Printf("首轮无可用账号: %v", err)
	}
	if *interval <= 0 {
		return nil
	}
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-time.After(jitteredInterval(*interval)):
			qw.check(ctx, false)
			if _, err := daemonSyncOnce(ctx, sel, lg); err != nil {
				lg.Printf("暂无可用账号: %v", err)
			}
		}
	}
}

// daemonSyncOnce 单轮同步：Pick 最优健康号，与 keyring 当前身份比对，
// 不一致则写 keyring 并返回 true；一致返回 false；无可用号返回 error 且不写。
func daemonSyncOnce(ctx context.Context, sel *pool.Selector, lg *log.Logger) (bool, error) {
	best, err := sel.Pick(ctx)
	if err != nil {
		return false, err
	}
	_, curEmail, curErr := keyringCurrentAdapter()
	if curErr == nil && strings.EqualFold(strings.TrimSpace(curEmail), strings.TrimSpace(best.Email)) && curEmail != "" {
		return false, nil
	}
	if err := syncKeyringAccount(accountSwitch{name: best.Name, email: best.Email}); err != nil {
		return false, err
	}
	lg.Printf("已切换 keyring → %s (%s)，请重启 agy 生效（交互式 /resume 恢复）", best.Name, best.Email)
	return true, nil
}
