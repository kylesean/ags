package i18n

import (
	"fmt"
	"os"
	"sync"
)

// 包级当前语言。进程启动时定一次，之后只读，所以用 RWMutex 保护即可
// （测试要能切，生产中不会变）。
var (
	curMu sync.RWMutex
	cur   Lang
	once  sync.Once
)

// Current 返回当前语言。首次调用时自动探测。
func Current() Lang {
	curMu.RLock()
	l := cur
	curMu.RUnlock()
	if l != "" {
		return l
	}

	once.Do(func() {
		curMu.Lock()
		if cur == "" {
			cur = Detect()
		}
		curMu.Unlock()
	})

	curMu.RLock()
	l = cur
	curMu.RUnlock()
	return l
}

// SetLang 强制设置当前语言（供命令行开关与测试使用）。
// 传空串表示恢复自动探测。
func SetLang(l Lang) {
	curMu.Lock()
	defer curMu.Unlock()
	if l == "" {
		cur = ""
		once = sync.Once{}
		return
	}
	cur = l
}

// Tr 用当前语言查一条文案。
func Tr(key string) string {
	return T(Current(), key)
}

// Trf 用当前语言查文案并格式化。
//
// 用 Sprintf 而不是自己实现占位符替换：这样 %w 之外的所有 fmt 语法
// 都能直接用，且测试能靠 %!s(MISSING) 立刻发现占位符对不上。
func Trf(key string, args ...any) string {
	return fmt.Sprintf(T(Current(), key), args...)
}

// Printf 用当前语言打印到 stdout。
func Printf(key string, args ...any) {
	fmt.Fprintf(os.Stdout, T(Current(), key), args...)
}

// Println 打印一条文案并换行。
func Println(key string) {
	fmt.Fprintln(os.Stdout, Tr(key))
}

// Errorf 返回一个已本地化的 error。
func Errorf(key string, args ...any) error {
	return fmt.Errorf(T(Current(), key), args...)
}

// Fprintf 用当前语言写到一个 writer（供需要自定义输出流的调用方使用）。
func Fprintf(w interface{ Write([]byte) (int, error) }, key string, args ...any) {
	fmt.Fprintf(w, T(Current(), key), args...)
}
