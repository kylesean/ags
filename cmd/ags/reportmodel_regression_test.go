package main

import (
	"bytes"
	"log"
	"strings"
	"testing"

	"github.com/kylesean/ags/internal/cloudcode"
)

// 告警必须出现在**所有**打印模型表的路径上。
//
// 启动路径与后台重试路径各打一次模型表；任何一条漏了告警，
// 用户就会在「拉表失败后重试成功」这条更常见的路径上静默用上错位模型。
func TestReportModelTablePrintsWarnings(t *testing.T) {
	m := &cloudcode.Models{
		Keys: 29,
		Aliases: map[string]string{
			"gemini-3.8-flash": "gemini-3.8-flash-tiered",
		},
		Catalog: []cloudcode.CatalogEntry{
			{ID: "gemini-4-argon-medium", DisplayName: "Gemini 4 Argon (Medium)"},
		},
		Warnings: []string{"上游没有返回 agentModelSorts（旧版上游？）"},
	}

	var buf bytes.Buffer
	lg := log.New(&buf, "", 0)
	reportModelTable(m, false, lg, "模型映射已加载")

	out := buf.String()
	if !strings.Contains(out, "模型映射已加载") {
		t.Errorf("缺前缀日志: %q", out)
	}
	if !strings.Contains(out, "agentModelSorts") {
		t.Errorf("模型表告警没打出来: %q", out)
	}
}

// UA 告警也要在同一处打出来（两条路径共用这个函数）。
func TestReportModelTablePrintsUAWarning(t *testing.T) {
	m := &cloudcode.Models{
		Keys: 23,
		Catalog: []cloudcode.CatalogEntry{
			// 旧代际：没有 gemini-4-argon*
			{ID: "claude-opus-5-5-medium", DisplayName: "Claude Opus 5.5 (Medium)"},
		},
	}

	var buf bytes.Buffer
	lg := log.New(&buf, "", 0)
	reportModelTable(m, false, lg, "后台重试获取模型表成功")

	if !strings.Contains(buf.String(), "User-Agent") {
		t.Errorf("UA 告警没打出来: %q", buf.String())
	}
}

// 正常表（有排序表、有 Argon、无告警）不该刷任何「警告」。
func TestReportModelTableQuietWhenHealthy(t *testing.T) {
	m := &cloudcode.Models{
		Keys: 29,
		Catalog: []cloudcode.CatalogEntry{
			{ID: "gemini-4-argon-medium", DisplayName: "Gemini 4 Argon (Medium)"},
		},
	}

	var buf bytes.Buffer
	lg := log.New(&buf, "", 0)
	reportModelTable(m, false, lg, "模型映射已加载")

	if strings.Contains(buf.String(), "警告") {
		t.Errorf("健康模型表不该告警: %q", buf.String())
	}
}

// -v 时要带上别名表，否则排查「哪个别名映射错了」得重跑。
func TestReportModelTableVerboseShowsAliases(t *testing.T) {
	m := &cloudcode.Models{
		Keys:    29,
		Aliases: map[string]string{"gemini-4-argon": "gemini-4-argon-medium"},
		Catalog: []cloudcode.CatalogEntry{{ID: "gemini-4-argon-medium"}},
	}

	var buf bytes.Buffer
	lg := log.New(&buf, "", 0)
	reportModelTable(m, true, lg, "模型映射已加载")

	out := buf.String()
	if !strings.Contains(out, "gemini-4-argon") || !strings.Contains(out, "→") {
		t.Errorf("-v 应打印别名映射: %q", out)
	}
}
