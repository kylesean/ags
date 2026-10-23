package i18n

import (
	"strings"
	"sync"
	"testing"
)

func resetLang(t *testing.T) {
	t.Helper()
	SetLang("")
	t.Cleanup(func() { SetLang("") })
}

// Current 必须返回受支持的语言（不能是空串，否则所有文案都变成 key）。
func TestCurrentIsAlwaysSupported(t *testing.T) {
	resetLang(t)
	if l := Current(); !IsSupported(l) {
		t.Errorf("Current() = %q，不是受支持的语言", l)
	}
}

// SetLang 要能强制切换（命令行 -lang 与测试都靠它）。
func TestSetLangForces(t *testing.T) {
	resetLang(t)
	SetLang(LangEN)
	if got := Tr("common.yes"); got != "yes" {
		t.Errorf("切到英文后 common.yes = %q", got)
	}
	SetLang(LangJA)
	if got := Tr("common.yes"); got != "あり" {
		t.Errorf("切到日文后 common.yes = %q", got)
	}
}

// SetLang("") 要恢复自动探测。
func TestSetLangEmptyRestoresAuto(t *testing.T) {
	resetLang(t)
	SetLang(LangEN)
	SetLang("")
	t.Setenv("AGS_LANG", "ja")
	if got := Current(); got != LangJA {
		t.Errorf("恢复自动探测后 Current() = %q，想要 ja", got)
	}
}

// Trf 必须正确格式化占位符 —— 占位符对不上时 Go 会输出 %!s(MISSING)，
// 那是最典型的「多语言没做完」的痕迹。
func TestTrfFormatsPlaceholders(t *testing.T) {
	resetLang(t)
	for _, lang := range Supported() {
		SetLang(lang)
		got := Trf("usage.account", "a@example.com")
		if strings.Contains(got, "%!") {
			t.Errorf("%s: Trf 输出含格式化错误: %q", lang, got)
		}
		if !strings.Contains(got, "a@example.com") {
			t.Errorf("%s: Trf 丢了参数: %q", lang, got)
		}
	}
}

// 同时访问 Current/Tr 不能有数据竞争（-race 下跑）。
func TestConcurrentAccess(t *testing.T) {
	resetLang(t)
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 200; j++ {
				_ = Tr("usage.available")
				_ = Current()
			}
		}()
	}
	wg.Wait()
}
