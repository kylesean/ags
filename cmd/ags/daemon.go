package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/kylesean/ags/internal/pool"
)

// daemonState 是 daemon 每次切换落盘的选中快照，供 status 拉取查证。
// 用户不看 daemon 终端时，跑一遍 status 即可知道是否要重启。
type daemonState struct {
	Name  string    `json:"name"`
	Email string    `json:"email"`
	At    time.Time `json:"at"`
}

func defaultDaemonStatePath() string {
	return filepath.Join(cacheDir(), "ags", "state.json")
}

func writeDaemonState(path, name, email string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	data, err := json.Marshal(daemonState{Name: name, Email: email, At: time.Now().UTC()})
	if err != nil {
		return err
	}
	data = append(data, '\n')
	tmp, err := os.CreateTemp(filepath.Dir(path), ".state-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	if err := tmp.Chmod(0o600); err != nil {
		tmp.Close()
		return err
	}
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmpName, path); err != nil {
		return err
	}
	return os.Chmod(path, 0o600)
}

func readDaemonState(path string) (*daemonState, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var st daemonState
	if err := json.Unmarshal(data, &st); err != nil {
		return nil, err
	}
	return &st, nil
}

// pendingRestartHint 比对 keyring 当前身份与 daemon 选中：不一致返回提示行，
// 一致或无状态返回空串（纯函数，供 status 与单测）。
func pendingRestartHint(curEmail string, st *daemonState) string {
	if st == nil || st.Email == "" {
		return ""
	}
	if strings.EqualFold(strings.TrimSpace(curEmail), strings.TrimSpace(st.Email)) && curEmail != "" {
		return ""
	}
	return fmt.Sprintf("提示: daemon 已选中 %s (%s)，keyring 仍是旧身份，请重启 agy 生效（/resume 恢复）",
		st.Name, st.Email)
}

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
			return fmt.Errorf("账号池为空，先 ags login <name> 或 ags add <name>")
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
	if err := writeDaemonState(defaultDaemonStatePath(), best.Name, best.Email); err != nil {
		lg.Printf("落盘选中状态失败: %v", err)
	}
	lg.Printf("已切换 keyring → %s (%s)，请重启 agy 生效（交互式 /resume 恢复）", best.Name, best.Email)
	return true, nil
}
