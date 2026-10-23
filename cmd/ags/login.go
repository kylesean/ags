package main

import (
	"context"
	"flag"
	"fmt"
	"time"

	"github.com/kylesean/ags/internal/i18n"
	"github.com/kylesean/ags/internal/oauth"
	"github.com/kylesean/ags/internal/pool"
	"github.com/kylesean/ags/internal/term"
)

// cmdLogin 自己走一遍 Google OAuth（授权码 + PKCE + 本地回环回调），把号直接写进池。
//
// 这是注册新账号的推荐路径：不碰 keyring、不依赖 agy TUI、
// 也没有「必须在登录后立刻 add」的顺序坑。
//
// 万一 Antigravity 的 client_id/secret 哪天被轮换，回退路径是
// 「agy 登录 → ags add」，那条路保留不删。
func cmdLogin(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("login", flag.ContinueOnError)
	timeout := fs.Duration("timeout", 5*time.Minute, i18n.Tr("flag.timeout"))
	noBrowser := fs.Bool("no-browser", false, i18n.Tr("flag.noBrowser"))
	hint := fs.String("email", "", i18n.Tr("flag.email"))
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 1 {
		// Go 的 flag 包遇到第一个非 flag 参数即停止解析，
		// 所以标志必须写在 <name> 前面 —— 用法串必须如实反映。
		return i18n.Errorf("login.usage")
	}
	name := fs.Arg(0)
	if err := pool.ValidateName(name); err != nil {
		return err
	}

	// 与 add 同样防覆盖：已存在的名字要求显式 drop，避免误冲。
	if _, err := pool.Load(name); err == nil {
		return i18n.Errorf("login.exists", name, name)
	}

	fmt.Println(i18n.Trf("login.start", name))
	res, err := oauth.Login(ctx, oauth.Config{}, oauth.Options{
		Timeout:      *timeout,
		NoBrowser:    *noBrowser,
		LoginHint:    *hint,
		PrintAuthURL: nil, // oauth 包默认打到 stdout
	})
	if err != nil {
		return err
	}

	existing, err := pool.FindByEmail(res.Email)
	if err != nil {
		return i18n.Errorf("login.emailChk", err)
	}
	if existing != nil {
		return i18n.Errorf("login.emailDup", res.Email, existing.Name, existing.Name)
	}

	// 与 add 完全一致的池文件 schema，serve/pool/quota 零改动。
	a := &pool.Account{
		Name:         name,
		Email:        res.Email,
		ClientID:     oauth.DefaultClientID(),
		AddedAt:      time.Now().UTC(),
		AccessToken:  res.AccessToken,
		RefreshToken: res.RefreshToken,
		Expiry:       res.Expiry,
		IDToken:      res.IDToken,
		AuthMethod:   oauth.AuthMethod,
	}
	if err := pool.Save(a); err != nil {
		return i18n.Errorf("login.saveFail", err)
	}

	labelW := labelColumnWidth
	kv := func(label, value string) {
		fmt.Printf("  %s%s\n", term.PadRight(label, labelW), value)
	}
	fmt.Printf("\n%s\n", i18n.Trf("login.saved", name))
	kv(i18n.Tr("col.email"), a.Email)
	kv(i18n.Tr("col.expiry"), fmtExpiry(a.Expiry))
	kv("client_id", orDash(a.ClientID))
	kv("scope", i18n.Trf("login.scope", len(oauth.DefaultScopes)))
	if a.RefreshToken == "" {
		kv(i18n.Tr("common.warning"), i18n.Tr("login.noRefresh"))
	} else {
		kv("refresh", i18n.Tr("login.refreshOk"))
	}
	dir, _ := pool.Dir()
	kv(i18n.Tr("add.stored"), fmt.Sprintf("%s (0600)", dir))
	return nil
}
