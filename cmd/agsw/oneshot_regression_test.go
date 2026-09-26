package main

import "testing"

// RED: =value 形态的一次性 flag 应被识别。
func TestIsOneShotForms(t *testing.T) {
	for _, args := range [][]string{
		{"--print=true", "hi"},
		{"-p=true"},
		{"--prompt=test"},
		{"--print=false"},
	} {
		_ = args
	}
	cases := map[string]bool{
		"--print=true":  true,
		"--prompt=test": true,
		"-p=true":       true,
	}
	for arg, want := range cases {
		if got := isOneShotAgy([]string{arg}); got != want {
			t.Errorf("isOneShotAgy(%q) = %v, want %v", arg, got, want)
		}
	}
}
