// Package keyring 读取、解析并受控写入 agy 保存在 Secret Service 中的 OAuth 凭据。
//
// 读取路径供 status/add 使用；写入由 use/daemon/tui 在切换账号时调用。
// 本包使用 agy 同款库 zalando/go-keyring，保证与其字节级兼容。
package keyring

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	kr "github.com/zalando/go-keyring"
)

// Secret Service 中唯一条目的定位键。agy 升级可能改动，改这里即可。
const (
	Service  = "gemini"
	Username = "antigravity"
)

// setSecret 可注入，测试不得写真实系统 Keyring。
var setSecret = kr.Set

// getSecret 可注入，测试不得读真实系统 Keyring。
var getSecret = kr.Get

// keyringReachable 报告 Secret Service 后端是否可达。
// 取到值（nil）或条目不存在（ErrNotFound）都说明后端可达；其余错误视为
// 后端不可达（Termux / 无头服务器无 DBus Secret Service）。
// 只有后端不可达时才允许降级到本地 token 文件。
func keyringReachable() bool {
	_, err := getSecret(Service, Username)
	return err == nil || errors.Is(err, kr.ErrNotFound)
}

// fallbackPath 返回 agy 在无 Secret Service 环境下的本地凭据文件路径。
// agy 自身在没有 DBus Secret Service 时会退化为读写该文件（Termux / 无头服务器）。
// AGY_TOKEN_FILE 可覆盖，便于测试隔离或自定义存放。
func fallbackPath() string {
	if p := os.Getenv("AGY_TOKEN_FILE"); p != "" {
		return p
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".gemini", "antigravity-cli", "antigravity-oauth-token")
}

// writeFallback 原子性不足但足以满足「agy 启动时读取」的场景：
// 建目录 0700、写文件 0600，并强制纠正受 umask 影响的权限。
func writeFallback(path string, raw []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	if err := os.WriteFile(path, raw, 0o600); err != nil {
		return err
	}
	return os.Chmod(path, 0o600)
}

// Store 将完整凭据写回 agy 使用的存储：优先 Keyring；仅当 Secret Service
// 后端不可达时才降级写本地 token 文件。后端可达却写失败属于真实错误，如实上报。
// 调用方负责在写入后重启 agy，使进程重新读取凭据。
func Store(sec *Secret) error {
	if sec == nil {
		return errors.New("凭据为 nil")
	}
	raw, err := json.Marshal(sec)
	if err != nil {
		return fmt.Errorf("序列化凭据失败: %w", err)
	}
	if err := setSecret(Service, Username, string(raw)); err == nil {
		return nil
	} else if keyringReachable() {
		// 后端可达却写失败：不降级——否则 agy 仍读 keyring，切换会静默失效。
		return fmt.Errorf("写入 keyring 失败: %w", err)
	}
	// 后端不可达：降级写 agy 本地 token 文件。
	if fb := fallbackPath(); fb != "" {
		if werr := writeFallback(fb, raw); werr == nil {
			return nil
		}
	}
	return fmt.Errorf("写入 keyring 失败: %w", err)
}

// ExpiryTime 兼容 RFC3339 字符串、Unix 秒数字面量、null 与缺失。
type ExpiryTime struct{ time.Time }

func (t *ExpiryTime) UnmarshalJSON(b []byte) error {
	s := strings.TrimSpace(string(b))
	switch {
	case s == "" || s == "null":
		return nil
	case s[0] == '"':
		var str string
		if err := json.Unmarshal(b, &str); err != nil {
			return err
		}
		if str == "" {
			return nil
		}
		tt, err := time.Parse(time.RFC3339, str)
		if err != nil {
			return fmt.Errorf("解析 expiry %q: %w", str, err)
		}
		t.Time = tt
		return nil
	default:
		var f float64
		if err := json.Unmarshal(b, &f); err != nil {
			return err
		}
		t.Time = time.Unix(int64(f), 0).UTC()
		return nil
	}
}

func (t ExpiryTime) MarshalJSON() ([]byte, error) {
	if t.Time.IsZero() {
		return []byte("null"), nil
	}
	return json.Marshal(t.Time.UTC().Format(time.RFC3339))
}

// Token 是凭据里的 token 子对象。
type Token struct {
	AccessToken  string     `json:"access_token"`
	TokenType    string     `json:"token_type"`
	RefreshToken string     `json:"refresh_token"`
	Expiry       ExpiryTime `json:"expiry"`
}

// Secret 是 keyring 中存放的完整 JSON。
type Secret struct {
	Token      Token  `json:"token"`
	AuthMethod string `json:"auth_method"`
	IDToken    string `json:"id_token"`
}

// Claims 是 id_token JWT payload 的最小可解子集。
// 只做 base64 解码，不做签名校验（读取的是本机自家凭据）。
type Claims struct {
	Email string          `json:"email"`
	Sub   string          `json:"sub"`
	Aud   json.RawMessage `json:"aud"`
}

// Audience 返回 aud 字段的首个值（可能是单字符串或数组）。
// 对已安装应用而言，这就是 Google OAuth client_id。
func (c Claims) Audience() string {
	var s string
	if err := json.Unmarshal(c.Aud, &s); err == nil && s != "" {
		return s
	}
	var list []string
	if err := json.Unmarshal(c.Aud, &list); err == nil && len(list) > 0 {
		return list[0]
	}
	return ""
}

// Raw 取出原始凭据 JSON：优先 Secret Service；仅当后端不可达时回落
// agy 本地 token 文件。后端可达但无条目/条目为空一律如实报错，不回落到文件。
func Raw() (string, error) {
	s, err := getSecret(Service, Username)
	if err == nil {
		if strings.TrimSpace(s) == "" {
			return "", errors.New("keyring 条目内容为空")
		}
		return s, nil
	}
	if !errors.Is(err, kr.ErrNotFound) {
		// 后端不可达：降级读 agy 本地 token 文件。
		if fb := fallbackPath(); fb != "" {
			if data, rerr := os.ReadFile(fb); rerr == nil && strings.TrimSpace(string(data)) != "" {
				return string(data), nil
			}
		}
		return "", fmt.Errorf("读取 keyring 失败: %w", err)
	}
	return "", fmt.Errorf("keyring 中没有 service=%s / username=%s 条目（是否尚未在 agy 登录？）",
		Service, Username)
}

// Parse 解析凭据 JSON。
func Parse(raw string) (*Secret, error) {
	var sec Secret
	if err := json.Unmarshal([]byte(raw), &sec); err != nil {
		return nil, fmt.Errorf("凭据不是合法 JSON: %w", err)
	}
	if sec.Token.AccessToken == "" && sec.Token.RefreshToken == "" {
		return nil, errors.New("凭据里 access_token 与 refresh_token 均为空")
	}
	return &sec, nil
}

// Claims 解出 id_token 里的身份。
func (s *Secret) Claims() (*Claims, error) {
	if s.IDToken == "" {
		return nil, errors.New("凭据缺少 id_token，无法识别账号身份")
	}
	return decodeJWT(s.IDToken)
}

// Email 返回当前登录账号的邮箱。
func (s *Secret) Email() (string, error) {
	c, err := s.Claims()
	if err != nil {
		return "", err
	}
	if c.Email == "" {
		return "", errors.New("id_token 中没有 email 字段")
	}
	return c.Email, nil
}

// Current 一步拿到「解析后的凭据 + 邮箱」。
// 任一环节失败都会返回 err，但不会写任何东西。
func Current() (*Secret, string, error) {
	raw, err := Raw()
	if err != nil {
		return nil, "", err
	}
	sec, err := Parse(raw)
	if err != nil {
		return nil, "", err
	}
	email, err := sec.Email()
	if err != nil {
		return nil, "", err
	}
	return sec, email, nil
}

// decodeJWT 解出 JWT 第二段 payload。缺 padding 也容忍。
func decodeJWT(tok string) (*Claims, error) {
	parts := strings.Split(tok, ".")
	if len(parts) < 2 {
		return nil, fmt.Errorf("id_token 段数异常（%d 段，应为 3）", len(parts))
	}
	payload, err := base64.RawURLEncoding.DecodeString(strings.TrimRight(parts[1], "="))
	if err != nil {
		return nil, fmt.Errorf("解码 id_token payload 失败: %w", err)
	}
	var c Claims
	if err := json.Unmarshal(payload, &c); err != nil {
		return nil, fmt.Errorf("解析 id_token claims 失败: %w", err)
	}
	return &c, nil
}
