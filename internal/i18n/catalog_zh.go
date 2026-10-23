package i18n

// zhCatalog 是简体中文文案（项目原语言，也是缺译文时的兜底）。
//
// key 集合在这里是权威：其它语言缺 key 会被 TestAllKeysTranslated 判失败。
var zhCatalog = map[string]string{
	// ---- 顶层 ----
	"cli.usage": `ags — agy 多账号切换器

用法:
  ags                 直接启动 TUI 并拉起 agy
  ags <命令> [参数]  执行指定子命令

命令:
  probe    启动拦截探针（观察线格式与协议交互）
  login    独立执行 Google OAuth 登录并将凭据存入账号池
  add      捕获当前系统 Keyring 凭据入池
  list     列出账号池中的账号
  status   显示当前系统 Keyring 账号与账号池状态
  drop     从池中移除指定账号
  usage    查询账号池中各账号的真实额度
  use      验额后切换系统 Keyring 到指定账号（手动范式）
  daemon   前台常驻：轮询额度并同步 Keyring（轻量本体）
  serve    启动反向代理（凭据注入、信封改写与自动切号）
  tui      启动 TUI 并自动拉起 agy（默认入口，即裸 ags）

各命令用 "<命令> -h" 查看参数。
`,
	"cli.unknownCmd": "未知命令 %q\n\n%s",
	"cli.error":      "ags: %s",

	// ---- 通用 ----
	"common.yes":        "是",
	"common.no":         "否",
	"common.present":    "有",
	"common.absent":     "无",
	"common.dash":       "-",
	"common.expired":    "已过期",
	"common.inDuration": "%s 后",
	"common.warning":    "警告",
	"common.cooling":    "冷却中",

	// ---- 列头 ----
	"col.name":    "名字",
	"col.email":   "邮箱",
	"col.expiry":  "过期时间",
	"col.refresh": "refresh",
	"col.current": "当前",

	// ---- add ----
	"add.usage":      "用法: ags add <name>",
	"add.exists":     "池里已有 %q，要覆盖请先 ags drop %s",
	"add.readFail":   "读取当前凭据失败: %w",
	"add.emailChk":   "检查邮箱是否已在账号池失败: %w",
	"add.emailDup":   "邮箱 %s 已存在（账号 %s），请使用不同 Google 账号；如需替换请先 ags drop %s",
	"add.claimsFail": "解析身份失败: %w",
	"add.captured":   "已捕获 → %s",
	"add.noRefresh":  "无 refresh_token，之后无法自动续期",
	"add.stored":     "存放",

	// ---- login ----
	"login.usage":     "用法: ags login [-timeout 5m] [-no-browser] [-email you@example.com] <name>\n（标志要写在 <name> 前面）",
	"login.exists":    "池里已有 %q，要覆盖请先 ags drop %s",
	"login.start":     "开始登录 → %s",
	"login.emailChk":  "登录成功但检查邮箱失败（凭据未保存）: %w",
	"login.emailDup":  "邮箱 %s 已存在（账号 %s），请使用不同 Google 账号；如需替换请先 ags drop %s",
	"login.saveFail":  "登录成功但入池失败（凭据未保存）: %w",
	"login.saved":     "已入池 → %s",
	"login.noRefresh": "无 refresh_token，之后无法自动续期",
	"login.refreshOk": "有（该号从此永久免登）",
	"login.scope":     "scope %d 项",

	// ---- list ----
	"list.noArgs":            "list 不接受参数",
	"list.empty":             "账号池为空（%s）",
	"list.emptyHint":         "用法: 先在 agy 登录某号，再 ags add <name>",
	"list.keyring":           "keyring 当前: %s",
	"list.keyringUnreadable": "keyring 当前: (读取失败或未登录)",

	// ---- status ----
	"status.noArgs":      "status 不接受参数",
	"status.keyringHdr":  "== keyring 当前身份 ==",
	"status.readFail":    "读取失败: %v",
	"status.email":       "邮箱",
	"status.auth":        "auth",
	"status.expiry":      "过期时间",
	"status.refresh":     "refresh",
	"status.poolHdr":     "== 账号池 ==",
	"status.poolEmpty":   "(空)",
	"status.accountLine": "  %s %s  过期 %s%s",
	"status.poolDir":     "池目录: %s",

	// ---- drop ----
	"drop.usage":   "用法: ags drop <name>",
	"drop.deleted": "已删除 %s",

	// ---- use ----
	"use.usage":       "用法: ags use [--force] <name>",
	"use.timeout":     "-timeout 必须大于 0",
	"use.noToken":     "账号 %s 无可用 token 且无 refresh_token",
	"use.refreshFail": "刷新 %s 失败: %w",
	"use.quotaFail":   "查询 %s 额度失败: %w",
	"use.exhausted":   "账号 %s 额度耗尽 (%s，冷却至 %s)，拒绝切换；用 --force 强行",
	"use.switched":    "已切换 keyring → %s (%s)",
	"use.restart":     "请重启 agy 生效（交互式会话执行 /resume 恢复）",

	// ---- usage ----
	"usage.account":       "Account: %s",
	"usage.pool":          "Pool: %s",
	"usage.noGroups":      "未找到任何额度组（上游可能改接口或 UA 已过期）",
	"usage.noGeminiGroup": "未找到 GEMINI 额度组",
	"usage.queryFail":     "查询失败: %v",
	"usage.failSummary":   "%d/%d 个账号查询失败",
	"usage.window5h":      "五小时额度剩余",
	"usage.windowWeekly":  "每周额度剩余",
	"usage.available":     "额度可用",
	"usage.unknown":       "额度未知",
	"usage.refreshesIn":   "将于 %s 后刷新",
	"usage.refreshesSoon": "即将刷新",
	"usage.accountFail":   "Account: %s\n查询失败: %v\n",
	"usage.compactHeader": "账号\t邮箱\t额度",

	// 时长片段：中英日的数量词差异很大，必须按片段拼，不能整句替换。
	"usage.durDaysHours": "%d天%d小时",
	"usage.durDays":      "%d天",
	"usage.durHoursMins": "%d小时%d分",
	"usage.durHours":     "%d小时",
	"usage.durMins":      "%d分",

	// ---- tui ----
	"tui.oneShotNoRestart": "一次性 agy 命令不启用 Keyring 自动重启",
	"tui.agyNotFound":      "找不到 agy，请先安装并加入 PATH: %w",
	"tui.logOpenFail":      "打开 TUI 日志失败: %w",
	"tui.switchAccount":    "切换账号 %s (%s)，同步 Keyring 并重启 agy",
	"tui.agyRestarted":     "agy 已重启，请执行 /resume 恢复会话",
	"tui.agyExit":          "agy 退出: %w",
	"tui.gatewayExit":      "Gateway 退出: %w",
	"tui.gatewayStartFail": "启动 Gateway 失败: %w",
	"tui.gatewayExitEarly": "serve 在 Gateway ready 前退出",

	// ---- 账号池 / keyring ----
	"pool.syncFail":             "同步账号 %s 到 Keyring 失败: %w",
	"pool.loadFail":             "加载切换账号 %q 失败: %w",
	"pool.identityChanged":      "账号 %s 身份已变化，拒绝写入 Keyring",
	"pool.noCredentials":        "账号 %s 缺少可写入的凭据",
	"pool.noCandidates":         "账号池为空，且读取 keyring 失败: %w",
	"pool.claimsFail":           "解析身份失败: %w",
	"pool.loadAccountFail":      "加载账号 %q 失败: %w",
	"pool.refreshPartial":       "强制刷新部分失败: %v",
	"pool.noUsableAfterRefresh": "刷新后仍无可用账号: %w",
	"pool.refreshedAll":         "已强制刷新全部账号",

	// ---- serve / daemon ----
	"serve.noAccount":      "没有可用账号",
	"serve.listening":      "监听 %s → %s",
	"serve.ready":          "Gateway ready: %s",
	"serve.quotaExhausted": "额度耗尽 %s: %s → 冷却至 %s",
	"serve.quotaOk":        "额度 %s: %s",
	"serve.account":        "账号 %s (%s)，token 过期 %s",
	"serve.candidates":     "池中候选 %d 个：%s",
	"serve.quotaInterval":  "额度检测 每 %s 查一次 GEMINI 组（阈值 %.3f）",
	"serve.quotaOff":       "额度检测 已关闭（-quota-interval=0），不会自动换号",
	"serve.allCooling":     "全部账号冷却中：%v",
	"serve.got429":         "上游返回 429，账号 %s 进入临时冷却 (1 分钟)",
	"serve.got401":         "上游返回 401，账号 %s 进入临时冷却 (30 秒)",
	"serve.retryNext":      "上游 429，账号 %s 冷却，重试下一个账号",
	"serve.switched":       "已切换 keyring → %s (%s)，请重启 agy 生效（交互式 /resume 恢复）",

	// ---- probe ----
	"probe.httpHint":    "接入: AGY_LLM_GATEWAY_URL=http://%s agy --print 'hi'",
	"probe.connectHint": "接入: AGY_PROXY_URL=http://%s agy --print 'hi'",
	"probe.noParams":    "probe 不接受位置参数",
	"probe.badMode":     "未知 -mode %q（可用: http | connect）",
	"probe.listen":      "探针已监听 %s（上游=%s）",
	"probe.listenFail":  "监听 %s 失败: %w",
	"probe.captured":    "捕获 %s %s%s (%d bytes, proto=%s)",
	"probe.forwarded":   "转发 %s%s → %s [%d]",
	"probe.forwardFail": "转发失败 %s%s: %v",
	"probe.writeFail":   "写记录失败: %v",
	"probe.logPath":     "日志: %s",

	// ---- flag 说明 ----
	"flag.account":         "只查询指定账号",
	"flag.upstream":        "CloudCode 上游地址",
	"flag.usageTimeout":    "每个账号的额度查询超时",
	"flag.compact":         "单行紧凑输出（脚本/管道用）",
	"flag.useTimeout":      "额度查询超时",
	"flag.quotaThreshold":  "额度耗尽阈值",
	"flag.force":           "无视额度耗尽强行切换",
	"flag.timeout":         "等待浏览器完成登录的超时",
	"flag.noBrowser":       "只打印授权链接，不自动打开浏览器（SSH/无头场景）",
	"flag.email":           "预填的 Google 邮箱（可选）",
	"flag.listen":          "监听地址",
	"flag.serveAccount":    "指定用池中哪个账号；留空则用池里全部，池空则回落 keyring 当前账号",
	"flag.refresh":         "启动时先强制刷新一次 access_token",
	"flag.verbose":         "打印每个请求的选号与转发结果",
	"flag.logFile":         "日志文件；留空输出到 stderr",
	"flag.userAgent":       "发给上游的 User-Agent（许可证闸门，一般别改）",
	"flag.passthrough":     "关闭信封改写，纯透传（调试用；此时上游不会接受 agy 的 Gemini 路径）",
	"flag.noModelAlias":    "不拉模型表、不做模型名映射/模型发现（调试用）",
	"flag.quotaInterval":   "GEMINI 额度轮询间隔；设为 0 关闭额度检测（关了就不会自动换号）",
	"flag.quotaThreshold0": "GEMINI 剩余比例 ≤ 该值即判耗尽并换号（0 = 归零才切，不浪费残量）",
	"flag.stripField":      "转发前从请求体顶层删掉这个 JSON 字段，可重复",
	"flag.quotaIntervalD":  "额度轮询间隔；设为 0 则只跑一轮即退",
	"flag.daemonAccount":   "只看管指定账号；留空则看管池中全部",
	"flag.probeMode":       "探针模式: http | connect",
	"flag.probeListen":     "监听地址",
	"flag.probeUpstream":   "上游地址；留空则仅记录不转发",
	"flag.probeOut":        "JSONL 日志路径；留空则新建临时文件",
	"flag.bodyMax":         "单条记录保留的请求体字节数",
	"flag.quiet":           "只写文件，不往 stderr 刷",
	"flag.tuiListen":       "Gateway 监听地址",
	"flag.tuiAccount":      "只使用指定账号",
	"flag.tuiLogFile":      "Gateway/TUI 日志文件",
	"flag.syncKeyring":     "切换账号时同步 Keyring 并重启 agy",

	// ---- 额度 ----
	"quota.argon":    "Argon 周",
	"quota.week":     "周",
	"quota.fiveHour": "5h",
	"quota.resetAt":  "(重置 %s)",
	"quota.unknown":  "未知",
	"quota.notFound": "未找到 GEMINI 组",
	// ---- 之前漏掉的 key（由 TestAllUsedKeysExist 抓出）----
	"common.failed":          "失败",
	"usage.noArgs":           "usage 不接受位置参数",
	"serve.noPositional":     "serve 不接受位置参数",
	"serve.intervalNegative": "-quota-interval 不能为负（收到 %s）",
	"serve.thresholdRange":   "-quota-threshold 需在 0~1 之间（收到 %v）",
	"serve.stripFields":      "剥离请求体字段: %s",
	"serve.modelsFail":       "获取模型表失败，将在后台重试（当前部分模型可能 404）: %v",
	"serve.pickAccount":      "选号 %s (%s) 过期 %s",
	"serve.inflightSwitch":   "在飞请求全部完成，触发账号切换并同步: %s (%s)",
	"serve.repickFail":       "启动后重新选号失败: %v",
	"serve.shutdown":         "收到退出信号，优雅关闭…",
	"serve.shutdownFail":     "关闭失败: %w",
	"serve.daoPoolEmpty":     "账号池为空，先 ags login <name> 或 ags add <name>",
	"serve.daoFirstFail":     "首轮无可用账号: %v",
	"serve.daoNone":          "暂无可用账号: %v",
	"serve.daoStateFail":     "落盘选中状态失败: %v",
	"probe.tempFail":         "建日志文件失败: %w",
	"probe.dirFail":          "创建日志目录失败: %w",
	"probe.openFail":         "打开日志失败: %w",
	"probe.chmodFail":        "设置日志权限失败: %w",
	"probe.connectListen":    "CONNECT 探针已监听 %s",
	"probe.unsetNoForward":   "(未设置，仅记录不转发)",
	"serve.tokenFail":        "查额度 %s 获取 token 失败（保持原状态）: %v",
	"serve.quotaFail":        "查额度 %s 失败（保持原状态）: %v",
	"serve.forward":          "→ %s (%s) %s%s",
	"serve.forwardFail":      "转发失败 %s%s: %v",
	"serve.upstreamErr":      "上游 %d %s: %s",
	"serve.rewriteFail":      "改写请求体失败 %s%s: %v",
	"serve.envelopeFail":     "信封改写失败 %s%s: %v",
	"serve.readBodyFail":     "读取请求体失败 %s%s: %v",
	"serve.pickFail":         "选号失败：%v",
	"serve.discovery":        "模型发现 %s → %d 个模型",
	// ---- 底层错误（用户可能需要行动的那些）----
	"keyring.noEntry":         "keyring 中没有 service=%s / username=%s 条目（是否尚未在 agy 登录？）",
	"keyring.readFail":        "读取 keyring 失败: %w",
	"keyring.badJSON":         "凭据不是合法 JSON: %w",
	"keyring.idTokenSegments": "id_token 段数异常（%d 段，应为 3）",
	"picker.cooling":          "%s: 冷却至 %s",
	"picker.refreshFail":      "%s 刷新失败: %w",
	"picker.noUsableToken":    "%s: 无可用 token 且无法刷新",
	"picker.emptyToken":       "%s: access_token 为空",
	"picker.noneUsable":       "没有可用账号: %w",
	"picker.notInPool":        "账号 %q 不在候选池中",
	"flag.badValue":           "-%s 值非法: %q",
	"flag.missingValue":       "-%s 缺少值",
	"serve.envelope":          "信封改写 %s，User-Agent %s",
	"serve.envelopeOn":        "开",
	"serve.envelopeOff":       "关（纯透传）",
	"serve.defaultModel":      "上游默认模型 %s",
	"serve.retryFail":         "后台重试获取模型表失败（将在 %s 后重试）: %v",
	"tui.usingAccount":        "当前账号: %s（agy 里看不到身份属正常：网关模式下它认为身份由网关提供）",
	"serve.modelsLoaded":      "模型映射已加载",
	"serve.retryLoaded":       "后台重试获取模型表成功",
}
