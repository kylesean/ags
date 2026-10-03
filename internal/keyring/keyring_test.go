package keyring

import (
	"encoding/base64"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	kr "github.com/zalando/go-keyring"
)

// stubKeyring 替换 keyring 读写 seam，测试结束自动还原。传 nil 表示不替换该项。
func stubKeyring(t *testing.T, get func(string, string) (string, error), set func(string, string, string) error) {
	t.Helper()
	oldGet, oldSet := getSecret, setSecret
	t.Cleanup(func() { getSecret, setSecret = oldGet, oldSet })
	if get != nil {
		getSecret = get
	}
	if set != nil {
		setSecret = set
	}
}

// tempTokenFile 把降级路径指向临时文件，隔离真实的 ~/.gemini 凭据。
func tempTokenFile(t *testing.T) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "antigravity-oauth-token")
	t.Setenv("AGY_TOKEN_FILE", p)
	return p
}

func TestRawFallsBackToTokenFileWhenBackendUnavailable(t *testing.T) {
	p := tempTokenFile(t)
	const raw = `{"token":{"access_token":"at","refresh_token":"rt"}}`
	if err := os.WriteFile(p, []byte(raw), 0o600); err != nil {
		t.Fatal(err)
	}
	stubKeyring(t, func(string, string) (string, error) { return "", errors.New("dbus 不可达") }, nil)

	got, err := Raw()
	if err != nil {
		t.Fatalf("Raw: %v", err)
	}
	if got != raw {
		t.Errorf("Raw = %q, want %q", got, raw)
	}
}

// 后端可达但无条目：不得降级读文件（避免读到与 keyring 不一致的陈旧凭据）。
func TestRawDoesNotFallBackOnErrNotFound(t *testing.T) {
	p := tempTokenFile(t)
	if err := os.WriteFile(p, []byte(`{"token":{"access_token":"from-file"}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	stubKeyring(t, func(string, string) (string, error) { return "", kr.ErrNotFound }, nil)

	got, err := Raw()
	if err == nil || !strings.Contains(err.Error(), "keyring 中没有") {
		t.Fatalf("err = %v, want 提及 keyring 中没有", err)
	}
	if got != "" {
		t.Errorf("Raw = %q, want 空（不应回落到文件）", got)
	}
}

// 后端可达但条目为空：同样不降级。
func TestRawDoesNotFallBackOnEmptyEntry(t *testing.T) {
	p := tempTokenFile(t)
	if err := os.WriteFile(p, []byte(`{"token":{"access_token":"from-file"}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	stubKeyring(t, func(string, string) (string, error) { return "   ", nil }, nil)

	_, err := Raw()
	if err == nil || !strings.Contains(err.Error(), "内容为空") {
		t.Fatalf("err = %v, want 提及内容为空", err)
	}
}

func TestKeyringReachable(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want bool
	}{
		{"命中条目", nil, true},
		{"条目不存在", kr.ErrNotFound, true},
		{"后端不可达", errors.New("dial unix: no such file"), false},
	}
	for _, c := range cases {
		stubKeyring(t, func(string, string) (string, error) { return "", c.err }, nil)
		if got := keyringReachable(); got != c.want {
			t.Errorf("%s: keyringReachable() = %v, want %v", c.name, got, c.want)
		}
	}
}

func TestRawPrefersKeyringOverTokenFile(t *testing.T) {
	p := tempTokenFile(t)
	if err := os.WriteFile(p, []byte(`{"token":{"access_token":"from-file"}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	stubKeyring(t, func(string, string) (string, error) {
		return `{"token":{"access_token":"from-keyring"}}`, nil
	}, nil)

	got, err := Raw()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, "from-keyring") {
		t.Errorf("Raw = %q，keyring 可用时应以 keyring 为准", got)
	}
}

func TestRawErrorsWhenBothSourcesMissing(t *testing.T) {
	tempTokenFile(t) // 文件不存在
	stubKeyring(t, func(string, string) (string, error) { return "", kr.ErrNotFound }, nil)

	_, err := Raw()
	if err == nil || !strings.Contains(err.Error(), "keyring 中没有") {
		t.Fatalf("err = %v, want 提及 keyring 中没有", err)
	}
}

func TestStoreFallsBackToTokenFileWhenBackendUnavailable(t *testing.T) {
	p := tempTokenFile(t)
	unavailable := func() error { return errors.New("dbus 不可达") }
	stubKeyring(t,
		func(string, string) (string, error) { return "", unavailable() },
		func(string, string, string) error { return unavailable() })

	sec := &Secret{
		Token:      Token{AccessToken: "at", RefreshToken: "rt", TokenType: "Bearer"},
		AuthMethod: "consumer", IDToken: "h.p.s",
	}
	if err := Store(sec); err != nil {
		t.Fatalf("Store: %v", err)
	}
	data, err := os.ReadFile(p)
	if err != nil {
		t.Fatalf("token 文件未写入: %v", err)
	}
	got, err := Parse(string(data))
	if err != nil {
		t.Fatalf("落盘 JSON 非法: %v", err)
	}
	if got.Token.AccessToken != "at" || got.Token.RefreshToken != "rt" || got.IDToken != "h.p.s" {
		t.Errorf("落盘凭据 = %+v", got)
	}
	// Windows 不支持 POSIX 权限位。
	if runtime.GOOS != "windows" {
		fi, err := os.Stat(p)
		if err != nil {
			t.Fatal(err)
		}
		if perm := fi.Mode().Perm(); perm != 0o600 {
			t.Errorf("token 文件权限 = %o, want 600", perm)
		}
	}
}

// 后端可达却写失败：必须如实报错，不得降级写文件，
// 否则 agy 仍读 keyring，切换会静默失效。
func TestStoreDoesNotFallBackWhenBackendReachable(t *testing.T) {
	p := tempTokenFile(t)
	stubKeyring(t,
		func(string, string) (string, error) { return "", kr.ErrNotFound }, // 后端可达
		func(string, string, string) error { return errors.New("collection locked") })

	err := Store(&Secret{Token: Token{AccessToken: "at"}})
	if err == nil {
		t.Fatal("后端可达时写失败应报错")
	}
	if _, statErr := os.Stat(p); !os.IsNotExist(statErr) {
		t.Errorf("后端可达时不应写文件, stat err = %v", statErr)
	}
}

func TestStorePrefersKeyringOverTokenFile(t *testing.T) {
	p := tempTokenFile(t)
	called := false
	stubKeyring(t, nil, func(string, string, string) error { called = true; return nil })

	if err := Store(&Secret{Token: Token{AccessToken: "at"}}); err != nil {
		t.Fatal(err)
	}
	if !called {
		t.Error("keyring 可用时应写 keyring")
	}
	if _, err := os.Stat(p); !os.IsNotExist(err) {
		t.Errorf("keyring 成功时不应写文件, stat err = %v", err)
	}
}

func TestStoreRawRoundTripViaTokenFile(t *testing.T) {
	tempTokenFile(t)
	unavailable := func() error { return errors.New("dbus 不可达") }
	stubKeyring(t,
		func(string, string) (string, error) { return "", unavailable() },
		func(string, string, string) error { return unavailable() })

	sec := &Secret{
		Token:      Token{AccessToken: "at", RefreshToken: "rt", TokenType: "Bearer"},
		AuthMethod: "consumer", IDToken: "h.p.s",
	}
	if err := Store(sec); err != nil {
		t.Fatal(err)
	}
	raw, err := Raw()
	if err != nil {
		t.Fatal(err)
	}
	got, err := Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	if got.Token.AccessToken != "at" || got.IDToken != "h.p.s" {
		t.Errorf("往返 = %+v", got)
	}
}

func TestFallbackPathHonorsEnvAndHome(t *testing.T) {
	t.Setenv("AGY_TOKEN_FILE", "/custom/token")
	if got := fallbackPath(); got != "/custom/token" {
		t.Errorf("fallbackPath = %q, want /custom/token", got)
	}

	t.Setenv("AGY_TOKEN_FILE", "")
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home) // Windows 的 os.UserHomeDir 读 USERPROFILE
	want := filepath.Join(home, ".gemini", "antigravity-cli", "antigravity-oauth-token")
	if got := fallbackPath(); got != want {
		t.Errorf("fallbackPath = %q, want %q", got, want)
	}
}

// makeJWT 造一个只有 header+payload+假签名的 JWT，payload 是合法 claims。
func makeJWT(payload string) string {
	enc := func(s string) string {
		return base64.RawURLEncoding.EncodeToString([]byte(s))
	}
	return strings.Join([]string{
		enc(`{"alg":"RS256","typ":"JWT"}`),
		enc(payload),
		"signature",
	}, ".")
}

func TestDecodeJWTEmailAndAudience(t *testing.T) {
	payload := `{"email":"me@example.com","sub":"123","aud":"999.apps.googleusercontent.com"}`
	c, err := decodeJWT(makeJWT(payload))
	if err != nil {
		t.Fatalf("decodeJWT: %v", err)
	}
	if c.Email != "me@example.com" {
		t.Errorf("Email = %q", c.Email)
	}
	if got := c.Audience(); got != "999.apps.googleusercontent.com" {
		t.Errorf("Audience = %q", got)
	}
}

func TestAudienceAcceptsArrayForm(t *testing.T) {
	// Google 有时把 aud 下发成数组。
	c := &Claims{Aud: []byte(`["first.apps.googleusercontent.com","second"]`)}
	if got := c.Audience(); got != "first.apps.googleusercontent.com" {
		t.Errorf("Audience = %q", got)
	}
}

func TestDecodeJWTRejectsGarbage(t *testing.T) {
	bad := []string{
		"",
		"onlyonesegment",
		"a.b",                 // payload 不是 base64
		"a.!!!notbase64!!!.c", // 非法字符
		"a." + base64.RawURLEncoding.EncodeToString([]byte("{bad json")) + ".c",
	}
	for _, tok := range bad {
		if _, err := decodeJWT(tok); err == nil {
			t.Errorf("decodeJWT(%q) 应当报错", tok)
		}
	}
}

func TestParseRejectsEmptyTokens(t *testing.T) {
	if _, err := Parse(`{"token":{},"id_token":"x"}`); err == nil {
		t.Error("access_token 与 refresh_token 均为空时应当报错")
	}
	if _, err := Parse(`not json`); err == nil {
		t.Error("非法 JSON 应当报错")
	}
}

func TestParseAcceptsFullSecret(t *testing.T) {
	raw := `{
		"token": {
			"access_token": "at",
			"token_type": "Bearer",
			"refresh_token": "rt",
			"expiry": "2026-09-24T12:00:00Z"
		},
		"auth_method": "consumer",
		"id_token": "h.p.s"
	}`
	sec, err := Parse(raw)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if sec.Token.RefreshToken != "rt" {
		t.Errorf("RefreshToken = %q", sec.Token.RefreshToken)
	}
	if sec.AuthMethod != "consumer" {
		t.Errorf("AuthMethod = %q", sec.AuthMethod)
	}
	want := time.Date(2026, 9, 24, 12, 0, 0, 0, time.UTC)
	if !sec.Token.Expiry.Equal(want) {
		t.Errorf("Expiry = %v, want %v", sec.Token.Expiry, want)
	}
}

func TestExpiryTimeAcceptsUnixSeconds(t *testing.T) {
	// 有些实现把 expiry 下发成数字。
	sec, err := Parse(`{"token":{"access_token":"at","refresh_token":"rt","expiry":1790000000}}`)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	got := sec.Token.Expiry.Unix()
	if got != 1790000000 {
		t.Errorf("Expiry.Unix() = %d, want 1790000000", got)
	}
}

func TestExpiryTimeToleratesNullAndMissing(t *testing.T) {
	for _, raw := range []string{
		`{"token":{"access_token":"a","refresh_token":"r","expiry":null}}`,
		`{"token":{"access_token":"a","refresh_token":"r"}}`,
		`{"token":{"access_token":"a","refresh_token":"r","expiry":""}}`,
	} {
		sec, err := Parse(raw)
		if err != nil {
			t.Fatalf("Parse(%s): %v", raw, err)
		}
		if !sec.Token.Expiry.IsZero() {
			t.Errorf("expiry 应当为零值, 得到 %v", sec.Token.Expiry)
		}
	}
}

func TestExpiryTimeRoundTrip(t *testing.T) {
	in := ExpiryTime{Time: time.Date(2026, 9, 24, 12, 0, 0, 0, time.UTC)}
	b, err := in.MarshalJSON()
	if err != nil {
		t.Fatalf("MarshalJSON: %v", err)
	}
	var out ExpiryTime
	if err := out.UnmarshalJSON(b); err != nil {
		t.Fatalf("UnmarshalJSON: %v", err)
	}
	if !out.Equal(in.Time) {
		t.Errorf("往返不一致: %v vs %v", out, in)
	}
}

func TestClaimsEmailMissingIsAnError(t *testing.T) {
	sec := &Secret{IDToken: makeJWT(`{"sub":"123"}`)}
	if _, err := sec.Email(); err == nil {
		t.Error("缺 email 时应当报错")
	}
}

func TestClaimsRequiresIDToken(t *testing.T) {
	sec := &Secret{}
	if _, err := sec.Claims(); err == nil {
		t.Error("缺 id_token 时应当报错")
	}
	if _, err := sec.Email(); err == nil {
		t.Error("缺 id_token 时 Email 应当报错")
	}
}

// TestServiceConstantsDocumentsTheContract 把逆向出来的定位键钉死。
// agy 升级若改动这些值，本测试会在 status/add 上先炸，而不是静默读错条目。
func TestServiceConstantsDocumentsTheContract(t *testing.T) {
	if Service != "gemini" {
		t.Errorf("Service = %q, 想要 gemini", Service)
	}
	if Username != "antigravity" {
		t.Errorf("Username = %q, 想要 antigravity", Username)
	}
}

func TestStoreSerializesSecretForAgyKeyring(t *testing.T) {
	oldSet := setSecret
	t.Cleanup(func() { setSecret = oldSet })
	var gotService, gotUser, gotRaw string
	setSecret = func(service, user, raw string) error {
		gotService, gotUser, gotRaw = service, user, raw
		return nil
	}

	sec := &Secret{
		Token: Token{
			AccessToken: "at", RefreshToken: "rt", TokenType: "Bearer",
			Expiry: ExpiryTime{Time: time.Date(2026, 9, 24, 12, 0, 0, 0, time.UTC)},
		},
		AuthMethod: "consumer", IDToken: "h.p.s",
	}
	if err := Store(sec); err != nil {
		t.Fatalf("Store: %v", err)
	}
	if gotService != Service || gotUser != Username {
		t.Fatalf("keyring location = %q/%q", gotService, gotUser)
	}
	got, err := Parse(gotRaw)
	if err != nil {
		t.Fatalf("stored JSON invalid: %v\n%s", err, gotRaw)
	}
	if got.Token.AccessToken != "at" || got.Token.RefreshToken != "rt" || got.IDToken != "h.p.s" {
		t.Errorf("stored credentials = %+v", got)
	}
}
