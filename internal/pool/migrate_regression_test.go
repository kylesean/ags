package pool

import (
	"os"
	"path/filepath"
	"testing"
)

// RED: 旧 ags 数据目录应一次性自动迁移到 ags。
func TestDataDirMigratesFromLegacy(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_DATA_HOME", "")
	t.Setenv("AGS_DATA_DIR", "")
	t.Setenv("AGS_DATA_DIR", "")
	legacy := filepath.Join(home, ".local", "share", "agsw", "pool")
	if err := os.MkdirAll(legacy, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(legacy, "a.json"), []byte("{}"), 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := Dir()
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(home, ".local", "share", "ags", "pool")
	if got != want {
		t.Fatalf("Dir() = %q, want %q", got, want)
	}
	if _, err := os.Stat(legacy); !os.IsNotExist(err) {
		t.Fatalf("旧目录应已迁走")
	}
	if _, err := os.Stat(filepath.Join(want, "a.json")); err != nil {
		t.Fatalf("数据应随目录迁移: %v", err)
	}
}
