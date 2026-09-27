package main

import (
	"testing"
	"time"
)

// RED: Windows 跳过注定失败的 Interrupt、短等待直接 Kill；Unix 走优雅中断。
func TestAgyStopPlan(t *testing.T) {
	interrupt, wait := agyStopPlan("windows")
	if interrupt {
		t.Errorf("windows 不应尝试 Interrupt")
	}
	if wait != 2*time.Second {
		t.Errorf("windows wait = %s, want 2s", wait)
	}
	interrupt, wait = agyStopPlan("linux")
	if !interrupt {
		t.Errorf("linux 应先尝试 Interrupt")
	}
	if wait != 5*time.Second {
		t.Errorf("linux wait = %s, want 5s", wait)
	}
}
