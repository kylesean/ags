package i18n

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// 这是「文案表漏 key」的守卫。
//
// 之前只测了「表里有的 key 各语言是否齐全」，漏掉了反向：
// **代码里用到的 key 是否都在表里**。结果 serve.modelsFail 这类 key
// 直接以 `serve.modelsFail%!(EXTRA ...)` 的形式打进了用户日志 ——
// 比缺译文更难发现，因为它在日志里而不是界面上。
//
// 扫描范围是整个仓库的 .go 文件（含测试）：测试里引用的 key 也必须是
// 真实存在的，否则测试自己就在验证一个不存在的文案。
func TestAllUsedKeysExist(t *testing.T) {
	root, err := moduleRoot()
	if err != nil {
		t.Skipf("找不到模块根目录，跳过: %v", err)
	}

	used, err := collectStrictKeys(root)
	if err != nil {
		t.Fatalf("扫描 key 失败: %v", err)
	}
	if len(used) == 0 {
		t.Fatal("没扫描到任何 key，扫描逻辑可能坏了")
	}

	known := map[string]bool{}
	for _, k := range Keys() {
		known[k] = true
	}

	var missing []string
	for k := range used {
		if !known[k] {
			missing = append(missing, k)
		}
	}
	if len(missing) > 0 {
		t.Errorf("代码里用了 %d 个文案表里不存在的 key（会以 key%%!s(MISSING) 的形式出现在界面上）:\n  %s",
			len(missing), strings.Join(missing, "\n  "))
	}
}

// 文案表里不该有代码完全不用的 key —— 那会让「这份表是权威」失效，
// 也会让 translator 白翻。
func TestNoUnusedKeys(t *testing.T) {
	root, err := moduleRoot()
	if err != nil {
		t.Skipf("找不到模块根目录，跳过: %v", err)
	}
	used, err := collectAllKeyLiterals(root)
	if err != nil {
		t.Fatalf("扫描 key 失败: %v", err)
	}

	var unused []string
	for _, k := range Keys() {
		if !used[k] {
			unused = append(unused, k)
		}
	}
	if len(unused) > 0 {
		t.Errorf("文案表里有 %d 个没人用的 key:\n  %s",
			len(unused), strings.Join(unused, "\n  "))
	}
}

// keyCallRe 抓 Tr/Trf/Errorf/Println/Printf 的真实调用。
//
// 要求 key 至少含一个点（我们的 key 都是「命名空间.名称」），
// 这样注释里形如 Tr("x") 的示意写法与单段占位符不会被误当成 key。
var keyCallRe = regexp.MustCompile(
	`i18n\.(?:Tr|Trf|Errorf|Println|Printf)\(\s*"([A-Za-z0-9_]+\.[A-Za-z0-9._]+)"`)

// keyLiteralRe 抓任何形如 "namespace.name" 的字符串字面量。
//
// 为什么范围放这么宽：key 可能不直接传给 i18n.Tr，而是作为参数传给
// 辅助函数（reportModelTable(..., "serve.modelsLoaded")）。只匹配直接
// 调用会把这些判成「无人使用」，从而误删掉真实的 key。
//
// 收窄条件是「首段小写字母开头 + 至少一个点」，这足以排除文件路径
// （含斜杠）、格式串（含 %）、域名（含多个点且首段像域名）里的常见形态；
// 即便偶尔多算一个，副作用也只是「不报 unused」，不会漏报 missing。
var keyLiteralRe = regexp.MustCompile(`"([a-z][A-Za-z0-9_]*\.[A-Za-z0-9_]+)"`)

// collectStrictKeys 只收「确定被 i18n 解析」的 key：
// 直接调用 Tr / Trf / Errorf / Println / Printf 且带完整命名空间。
//
// 用于「用了但表里没有」的检查 —— 这里必须精确，否则会把 state.json
// 这类文件名当成 key，报出一堆假缺失。
func collectStrictKeys(root string) (map[string]bool, error) {
	return scanGoFiles(root, func(text string) map[string]bool {
		out := map[string]bool{}
		for _, m := range keyCallRe.FindAllStringSubmatch(text, -1) {
			out[m[1]] = true
		}
		// 辅助函数的 key 参数（已知的少数几处，显式列出比宽松正则更可靠）。
		for _, m := range keyParamRe.FindAllStringSubmatch(text, -1) {
			out[m[1]] = true
		}
		return out
	})
}

// collectAllKeyLiterals 收「所有形如 key 的字符串字面量」。
//
// 用于「表里有但没人用」的检查 —— 这里宁可宽松：多算一个只会让某个
// 真实废弃 key 晚一点被发现，而漏算一个会导致真实的 key 被误删。
func collectAllKeyLiterals(root string) (map[string]bool, error) {
	return scanGoFiles(root, func(text string) map[string]bool {
		out := map[string]bool{}
		for _, m := range keyCallRe.FindAllStringSubmatch(text, -1) {
			out[m[1]] = true
		}
		for _, m := range keyParamRe.FindAllStringSubmatch(text, -1) {
			out[m[1]] = true
		}
		for _, m := range keyLiteralRe.FindAllStringSubmatch(text, -1) {
			out[m[1]] = true
		}
		return out
	})
}

// keyParamRe 抓「key 作为参数传给辅助函数」的已知形态。
var keyParamRe = regexp.MustCompile(
	`reportModelTable\([^)]*"([A-Za-z0-9_]+\.[A-Za-z0-9._]+)"`)

// scanGoFiles 遍历模块下所有 .go 文件，对每个文件调用 extract 取 key 集合。
func scanGoFiles(root string, extract func(string) map[string]bool) (map[string]bool, error) {
	used := map[string]bool{}
	err := filepath.Walk(root, func(path string, info os.FileInfo, walkErr error) error {
		if walkErr != nil {
			return nil
		}
		if info.IsDir() {
			switch info.Name() {
			case ".git", "vendor", "dist", "node_modules":
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") {
			return nil
		}
		b, readErr := os.ReadFile(path)
		if readErr != nil {
			return nil
		}
		for k := range extract(string(b)) {
			used[k] = true
		}
		return nil
	})
	return used, err
}

// moduleRoot 从当前目录向上找 go.mod。
//
// internal/i18n 的测试工作目录是包目录，需要往上走三层才到模块根。
func moduleRoot() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", os.ErrNotExist
		}
		dir = parent
	}
}
