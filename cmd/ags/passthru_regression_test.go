package main

import (
	"errors"
	"flag"
	"testing"
	"time"
)

// RED: tui 不认识的 flag 应透传给 agy，而不是报错。
func TestTUIUnknownFlagsPassThrough(t *testing.T) {
	o, agyArgs, err := parseTUIArgs([]string{"--dangerously-skip-permissions"})
	if err != nil {
		t.Fatalf("未知 flag 不应报错，got %v", err)
	}
	if len(agyArgs) != 1 || agyArgs[0] != "--dangerously-skip-permissions" {
		t.Fatalf("agyArgs = %v", agyArgs)
	}
	_ = o

	o, agyArgs, err = parseTUIArgs([]string{"-v", "--print", "hi"})
	if err != nil {
		t.Fatalf("got %v", err)
	}
	if !o.verbose {
		t.Fatalf("tui 自有 flag 应先生效")
	}
	if len(agyArgs) != 2 || agyArgs[0] != "--print" {
		t.Fatalf("agyArgs = %v", agyArgs)
	}

	o, agyArgs, err = parseTUIArgs([]string{"-quota-interval", "5m", "--prompt=test"})
	if err != nil {
		t.Fatalf("got %v", err)
	}
	if o.interval != 5*time.Minute {
		t.Fatalf("interval = %s", o.interval)
	}
	if len(agyArgs) != 1 || agyArgs[0] != "--prompt=test" {
		t.Fatalf("agyArgs = %v", agyArgs)
	}

	// -- 后全部归 agy，即使名字撞上 tui flag。
	_, agyArgs, err = parseTUIArgs([]string{"--", "-v"})
	if err != nil {
		t.Fatalf("got %v", err)
	}
	if len(agyArgs) != 1 || agyArgs[0] != "-v" {
		t.Fatalf("agyArgs = %v", agyArgs)
	}

	// -h 保持帮助语义。
	_, _, err = parseTUIArgs([]string{"-h"})
	if !errors.Is(err, flag.ErrHelp) {
		t.Fatalf("-h 应返回 ErrHelp，got %v", err)
	}
}
