package main

import (
	"context"
	"flag"
	"io"
	"log"
	"os"
	"path/filepath"

	"github.com/kylesean/ags/internal/i18n"
	"github.com/kylesean/ags/internal/probe"
)

// cmdProbe 启动拦截探针。
//
// 两种模式：
//   - -mode http    反向代理形态，配 AGY_LLM_GATEWAY_URL 用，记录线格式
//   - -mode connect 极简 CONNECT 代理，配 AGY_PROXY_URL/http_proxy 用，记录上游 host:port
//
// 本命令只读，不修改系统 Keyring。
func cmdProbe(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("probe", flag.ContinueOnError)
	mode := fs.String("mode", "http", i18n.Tr("flag.probeMode"))
	listen := fs.String("listen", "127.0.0.1:7897", i18n.Tr("flag.probeListen"))
	upstream := fs.String("upstream", "", i18n.Tr("flag.probeUpstream"))
	out := fs.String("out", "", i18n.Tr("flag.probeOut"))
	bodyMax := fs.Int("body-max", 8192, i18n.Tr("flag.bodyMax"))
	quiet := fs.Bool("q", false, i18n.Tr("flag.quiet"))
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() > 0 {
		return i18n.Errorf("probe.noParams")
	}

	logPath := *out
	if logPath == "" {
		f, err := os.CreateTemp("", "ags-probe-*.jsonl")
		if err != nil {
			return i18n.Errorf("probe.tempFail", err)
		}
		logPath = f.Name()
		_ = f.Close()
	} else if dir := filepath.Dir(logPath); dir != "" && dir != "." {
		// -out 指向的目录可能还不存在。
		if err := os.MkdirAll(dir, 0o700); err != nil {
			return i18n.Errorf("probe.dirFail", err)
		}
	}

	// 必须先 OpenFile 建出文件，chmod 才有对象。
	f, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return i18n.Errorf("probe.openFail", err)
	}
	defer f.Close()

	// 日志含协议细节，收紧权限（OpenFile 的 mode 受 umask 影响，这里强制纠正）。
	if err := os.Chmod(logPath, 0o600); err != nil {
		return i18n.Errorf("probe.chmodFail", err)
	}
	if dir := filepath.Dir(logPath); dir != "" {
		_ = os.Chmod(dir, 0o700)
	}

	lg := log.New(os.Stderr, "[probe] ", log.LstdFlags|log.Lmsgprefix)
	if *quiet {
		lg.SetOutput(io.Discard)
	}

	rec := probe.New(f, *bodyMax)
	lg.Printf("%s", i18n.Trf("probe.logPath", logPath))

	switch *mode {
	case "http":
		lg.Printf("%s", i18n.Trf("probe.httpHint", *listen))
		s := &probe.HTTPServer{
			Listen:   *listen,
			Upstream: *upstream,
			Recorder: rec,
			Log:      lg,
		}
		return s.Run(ctx)
	case "connect":
		lg.Printf("%s", i18n.Trf("probe.connectHint", *listen))
		s := &probe.CONNECTServer{Listen: *listen, Recorder: rec, Log: lg}
		return s.Run(ctx)
	default:
		return i18n.Errorf("probe.badMode", *mode)
	}
}
