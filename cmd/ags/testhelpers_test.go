package main

import (
	"bytes"
	"io"
	"os"
	"testing"

	"github.com/kylesean/ags/internal/pool"
)

// captureStdout 在测试期间把 os.Stdout 换成管道，返回捕获到的内容。
// usage 这类命令直接写 stdout，不捕获就没法断言。
func captureStdout(t *testing.T, fn func()) string {
	t.Helper()

	old := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("建管道失败: %v", err)
	}
	os.Stdout = w

	done := make(chan string, 1)
	go func() {
		var buf bytes.Buffer
		_, _ = io.Copy(&buf, r)
		done <- buf.String()
	}()

	fn()

	_ = w.Close()
	os.Stdout = old
	return <-done
}

// writeTestAccount 往（已隔离的）账号池里写一个账号。
func writeTestAccount(t *testing.T, name, email, token string) error {
	t.Helper()
	return pool.Save(&pool.Account{Name: name, Email: email, AccessToken: token})
}
