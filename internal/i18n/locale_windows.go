//go:build windows

package i18n

import (
	"syscall"
	"unsafe"
)

var (
	kernel32                     = syscall.NewLazyDLL("kernel32.dll")
	procGetUserDefaultUILanguage = kernel32.NewProc("GetUserDefaultUILanguage")
)

// readOSLocale 在 Windows 上读用户界面语言。
//
// GetUserDefaultUILanguage 返回 LANGID，低 10 位是主语言 ID。
// 0x04 是中文（含简繁），0x09 是英文，0x11 是日文。
//
// 用 UI 语言而不是系统默认语言（GetSystemDefaultUILanguage）：
// 用户可以在「语言首选项」里把显示语言改成与系统安装语言不同。
//
// 直接用 syscall 而不是 x/sys/windows：后者没有封装这个 API，
// 而这里只需要一次无参调用，不值得为此加一层。
func readOSLocale() string {
	r, _, _ := procGetUserDefaultUILanguage.Call()
	return langIDToTag(uint32(r))
}

// langIDToTag 把 Windows LANGID 的主语言 ID 翻成 BCP-47 主标签。
//
// 只映射我们支持的三门语言，其余返回空串（交给上层回落默认语言）。
func langIDToTag(langid uint32) string {
	switch langid & 0x3ff {
	case 0x04: // LANG_CHINESE
		return "zh"
	case 0x09: // LANG_ENGLISH
		return "en"
	case 0x11: // LANG_JAPANESE
		return "ja"
	default:
		return ""
	}
}

var _ = unsafe.Pointer(nil)
