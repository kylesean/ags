package i18n

// catalog 是所有语言的文案表。
//
// 约定：
//   - key 用点号分层：<命令>.<用途>，例如 "usage.account"、"cli.unknownCmd"
//   - 占位符用 %s/%d/%v，各语言的占位符序列必须完全一致
//     （TestPlaceholdersMatch 会强制检查，少一个就是 %!s(MISSING)）
//   - 不带标点差异：中文用「」，英文用 "" —— 这不影响布局
//   - 长度可以不同，布局由 internal/term 按视觉宽度自适应
var catalog = map[Lang]map[string]string{
	LangZH: zhCatalog,
	LangEN: enCatalog,
	LangJA: jaCatalog,
}
