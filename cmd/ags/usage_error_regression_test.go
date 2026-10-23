package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/kylesean/ags/internal/i18n"
)

// 某个账号查询失败时：错误要逐账号打印，但**不能**再被 main 重复打印一遍。
// 返回值只带「有几个失败」的摘要，退出码非零即可。
func TestUsageErrorIsNotDuplicated(t *testing.T) {
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"error":{"status":"SUBSCRIPTION_REQUIRED"}}`))
	}))
	defer up.Close()

	t.Setenv("AGS_DATA_DIR", t.TempDir())
	// 造一个池账号，避免走 keyring 回落路径。
	if err := writeTestAccount(t, "X", "x@example.com", "TOK"); err != nil {
		t.Fatal(err)
	}

	out := captureStdout(t, func() {
		err := cmdUsage([]string{"-upstream", up.URL, "-timeout", "5s"})
		if err == nil {
			t.Fatal("全部失败时应当返回非 nil（退出码非零）")
		}
		msg := err.Error()
		// 摘要里只说几个失败，不重复上游原文。
		if strings.Contains(msg, "SUBSCRIPTION_REQUIRED") || strings.Count(msg, "403") > 1 {
			t.Errorf("错误被重复展开: %q", msg)
		}
		if !strings.Contains(msg, "1/1") {
			t.Errorf("摘要应说明 1/1 失败，got %q", msg)
		}
	})

	// 账号级错误在 stdout 里出现恰好一次（用当前语言的失败标识，不绑死中文）。
	marker := i18n.Tr("usage.queryFail")
	marker = strings.TrimSuffix(marker, ": %v")
	if n := strings.Count(out, marker); n != 1 {
		t.Errorf("失败提示 %q 出现 %d 次（想要 1 次）:\n%s", marker, n, out)
	}
}
