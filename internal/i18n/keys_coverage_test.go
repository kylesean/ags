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

	used, err := collectUsedKeys(root)
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
	used, err := collectUsedKeys(root)
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

// collectUsedKeys 扫描模块下所有 .go 文件里引用的 key。
func collectUsedKeys(root string) (map[string]bool, error) {
	used := map[string]bool{}
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil // 单个文件读不了不该让整轮失败
		}
		if info.IsDir() {
			// 跳过 vendor / .git / 构建产物
			switch info.Name() {
			case ".git", "vendor", "dist", "node_modules":
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") {
			return nil
		}
		b, err := os.ReadFile(path)
		if err != nil {
			return nil
		}
		for _, m := range keyCallRe.FindAllStringSubmatch(string(b), -1) {
			used[m[1]] = true
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
