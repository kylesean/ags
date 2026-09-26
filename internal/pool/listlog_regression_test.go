package pool

import (
	"bytes"
	"log"
	"os"
	"path/filepath"
	"testing"
)

// RED: 损坏文件被跳过时应留下可观测痕迹（日志），而非静默消失。
func TestListLogsCorruptFile(t *testing.T) {
	dir := withTmpDir(t)
	if err := Save(&Account{Name: "ok", Email: "o@x"}); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "pool", "broken.json"), []byte("{not json"), 0o600); err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	log.SetOutput(&buf)
	defer log.SetOutput(os.Stderr)
	got, err := List()
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 {
		t.Fatalf("want 1, got %d", len(got))
	}
	if !bytes.Contains(buf.Bytes(), []byte("broken")) {
		t.Fatalf("应日志告警损坏文件，got %q", buf.String())
	}
}
