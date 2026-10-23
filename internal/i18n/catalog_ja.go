package i18n

// jaCatalog は日本語の文言。
//
// プレースホルダは zh 表と完全に一致させること（TestPlaceholdersMatch）。
// 行幅は言語ごとに違ってよい — レイアウトは internal/term が吸収する。
var jaCatalog = map[string]string{
	// ---- トップレベル ----
	"cli.usage": `ags — agy マルチアカウント切り替えツール

使い方:
  ags                 TUI を起動して agy を立ち上げる
  ags <コマンド> [引数]  指定したサブコマンドを実行する

コマンド:
  probe    傍受プローブを起動（通信形式とプロトコルを観察）
  login    Google OAuth を単独で実行し、資格情報をプールに保存
  add      現在のシステム Keyring の資格情報をプールに取り込む
  list     プール内のアカウントを一覧表示
  status   現在の Keyring アカウントとプールの状態を表示
  drop     指定したアカウントをプールから削除
  usage    プール内の各アカウントの実際のクォータを照会
  use      クォータを確認してから Keyring を切り替え（手動）
  daemon   常駐：クォータをポーリングして Keyring を同期（軽量版）
  serve    リバースプロキシを起動（資格情報注入・エンベロープ変換・自動切り替え）
  tui      TUI を起動して agy を立ち上げる（既定の入口、ags のみでも同じ）

各コマンドの引数は "<コマンド> -h" で確認してください。
`,
	"cli.unknownCmd": "不明なコマンド %q\n\n%s",
	"cli.error":      "ags: %s",

	// ---- 共通 ----
	"common.yes":        "あり",
	"common.no":         "なし",
	"common.present":    "あり",
	"common.absent":     "なし",
	"common.dash":       "-",
	"common.expired":    "期限切れ",
	"common.inDuration": "%s 後",
	"common.warning":    "警告",
	"common.cooling":    "クールダウン中",

	// ---- 列見出し ----
	"col.name":    "名前",
	"col.email":   "メール",
	"col.expiry":  "有効期限",
	"col.refresh": "refresh",
	"col.current": "現在",

	// ---- add ----
	"add.usage":      "使い方: ags add <名前>",
	"add.exists":     "プールに %q が既にあります。上書きするには先に ags drop %s を実行してください",
	"add.readFail":   "現在の資格情報の読み取りに失敗しました: %w",
	"add.emailChk":   "メールが既にプールにあるかの確認に失敗しました: %w",
	"add.emailDup":   "メール %s は既に存在します（アカウント %s）。別の Google アカウントを使ってください。置き換えるには先に ags drop %s を実行してください",
	"add.claimsFail": "身元情報の解析に失敗しました: %w",
	"add.captured":   "取り込みました → %s",
	"add.noRefresh":  "refresh_token がないため、自動更新は行われません",
	"add.stored":     "保存先",

	// ---- login ----
	"login.usage":     "使い方: ags login [-timeout 5m] [-no-browser] [-email you@example.com] <名前>\n（フラグは <名前> より前に書いてください）",
	"login.exists":    "プールに %q が既にあります。上書きするには先に ags drop %s を実行してください",
	"login.start":     "ログインを開始 → %s",
	"login.emailChk":  "ログインは成功しましたがメールの確認に失敗しました（資格情報は未保存）: %w",
	"login.emailDup":  "メール %s は既に存在します（アカウント %s）。別の Google アカウントを使ってください。置き換えるには先に ags drop %s を実行してください",
	"login.saveFail":  "ログインは成功しましたがプールへの保存に失敗しました（資格情報は未保存）: %w",
	"login.saved":     "プールに追加しました → %s",
	"login.noRefresh": "refresh_token がないため、自動更新は行われません",
	"login.refreshOk": "あり（このアカウントは再ログイン不要になります）",
	"login.scope":     "scope %d 件",

	// ---- list ----
	"list.noArgs":            "list は引数を取りません",
	"list.empty":             "プールは空です（%s）",
	"list.emptyHint":         "使い方: 先に agy でログインし、次に ags add <名前> を実行してください",
	"list.keyring":           "keyring の現在値: %s",
	"list.keyringUnreadable": "keyring の現在値: (読み取り失敗、または未ログイン)",

	// ---- status ----
	"status.noArgs":      "status は引数を取りません",
	"status.keyringHdr":  "== keyring の現在の身元 ==",
	"status.readFail":    "読み取りに失敗しました: %v",
	"status.email":       "メール",
	"status.auth":        "auth",
	"status.expiry":      "有効期限",
	"status.refresh":     "refresh",
	"status.poolHdr":     "== アカウントプール ==",
	"status.poolEmpty":   "(空)",
	"status.accountLine": "  %s %s  期限 %s%s",
	"status.poolDir":     "プールのディレクトリ: %s",

	// ---- drop ----
	"drop.usage":   "使い方: ags drop <名前>",
	"drop.deleted": "%s を削除しました",

	// ---- use ----
	"use.usage":       "使い方: ags use [--force] <名前>",
	"use.timeout":     "-timeout は 0 より大きい必要があります",
	"use.noToken":     "アカウント %s には有効なトークンも refresh_token もありません",
	"use.refreshFail": "%s の更新に失敗しました: %w",
	"use.quotaFail":   "%s のクォータ照会に失敗しました: %w",
	"use.exhausted":   "アカウント %s のクォータは枯渇しています（%s、%s までクールダウン）。切り替えを拒否しました — --force で強行できます",
	"use.switched":    "keyring を切り替えました → %s (%s)",
	"use.restart":     "agy を再起動してください（対話セッションでは /resume で復帰）",

	// ---- usage ----
	"usage.account":       "Account: %s",
	"usage.pool":          "Pool: %s",
	"usage.noGroups":      "クォータグループが見つかりません（上流の API 変更、または UA が古い可能性）",
	"usage.noGeminiGroup": "GEMINI クォータグループが見つかりません",
	"usage.queryFail":     "照会に失敗しました: %v",
	"usage.failSummary":   "%d/%d 件のアカウントで照会に失敗しました",
	"usage.window5h":      "5時間リミットの残量",
	"usage.windowWeekly":  "週間リミットの残量",
	"usage.available":     "クォータあり",
	"usage.unknown":       "クォータ不明",
	"usage.refreshesIn":   "%s 後にリセット",
	"usage.refreshesSoon": "まもなくリセット",
	"usage.accountFail":   "Account: %s\n照会に失敗しました: %v\n",
	"usage.compactHeader": "アカウント\tメール\tクォータ",

	// 長さの断片。日本語は助数詞が異なるため、文単位ではなく断片で結合する。
	"usage.durDaysHours": "%d日%d時間",
	"usage.durDays":      "%d日",
	"usage.durHoursMins": "%d時間%d分",
	"usage.durHours":     "%d時間",
	"usage.durMins":      "%d分",

	// ---- tui ----
	"tui.oneShotNoRestart": "1回限りの agy 実行では Keyring の自動再起動を行いません",
	"tui.agyNotFound":      "agy が見つかりません。インストールして PATH に追加してください: %w",
	"tui.logOpenFail":      "TUI ログを開けませんでした: %w",
	"tui.switchAccount":    "アカウント %s (%s) に切り替え、Keyring を同期して agy を再起動します",
	"tui.agyRestarted":     "agy を再起動しました。セッションを復帰するには /resume を実行してください",
	"tui.agyExit":          "agy が終了しました: %w",
	"tui.gatewayExit":      "ゲートウェイが終了しました: %w",
	"tui.gatewayStartFail": "ゲートウェイの起動に失敗しました: %w",
	"tui.gatewayExitEarly": "ゲートウェイの準備完了前に serve が終了しました",

	// ---- プール / keyring ----
	"pool.syncFail":             "アカウント %s の Keyring への同期に失敗しました: %w",
	"pool.loadFail":             "切り替え先アカウント %q の読み込みに失敗しました: %w",
	"pool.identityChanged":      "アカウント %s の身元が変わったため、Keyring への書き込みを拒否しました",
	"pool.noCredentials":        "アカウント %s に書き込める資格情報がありません",
	"pool.noCandidates":         "プールが空で、keyring の読み取りにも失敗しました: %w",
	"pool.claimsFail":           "身元情報の解析に失敗しました: %w",
	"pool.loadAccountFail":      "アカウント %q の読み込みに失敗しました: %w",
	"pool.refreshPartial":       "強制更新の一部が失敗しました: %v",
	"pool.noUsableAfterRefresh": "更新後も利用可能なアカウントがありません: %w",
	"pool.refreshedAll":         "全アカウントを強制更新しました",

	// ---- serve / daemon ----
	"serve.noAccount":      "利用可能なアカウントがありません",
	"serve.listening":      "%s で待ち受け → %s",
	"serve.ready":          "Gateway ready: %s",
	"serve.quotaExhausted": "%s のクォータが枯渇: %s → %s までクールダウン",
	"serve.quotaOk":        "クォータ %s: %s",
	"serve.account":        "アカウント %s (%s)、トークン期限 %s",
	"serve.candidates":     "プールの候補 %d 件: %s",
	"serve.quotaInterval":  "%s ごとに GEMINI グループのクォータを確認（しきい値 %.3f）",
	"serve.quotaOff":       "クォータ確認は無効です（-quota-interval=0）。自動切り替えは行いません",
	"serve.allCooling":     "全アカウントがクールダウン中です: %v",
	"serve.got429":         "上流が 429 を返しました。アカウント %s を 1 分間クールダウンします",
	"serve.got401":         "上流が 401 を返しました。アカウント %s を 30 秒間クールダウンします",
	"serve.retryNext":      "上流 429 のためアカウント %s をクールダウンし、次のアカウントで再試行します",
	"serve.switched":       "keyring を切り替えました → %s (%s)。agy を再起動してください（対話セッションでは /resume で復帰）",

	// ---- probe ----
	"probe.httpHint":    "接続: AGY_LLM_GATEWAY_URL=http://%s agy --print 'hi'",
	"probe.connectHint": "接続: AGY_PROXY_URL=http://%s agy --print 'hi'",
	"probe.noParams":    "probe は位置引数を取りません",
	"probe.badMode":     "不明な -mode %q（利用可能: http | connect）",
	"probe.listen":      "プローブが %s で待ち受けています（上流=%s）",
	"probe.listenFail":  "%s で待ち受けに失敗しました: %w",
	"probe.captured":    "捕捉 %s %s%s (%d bytes, proto=%s)",
	"probe.forwarded":   "転送 %s%s → %s [%d]",
	"probe.forwardFail": "転送に失敗 %s%s: %v",
	"probe.writeFail":   "記録の書き込みに失敗しました: %v",
	"probe.logPath":     "ログ: %s",

	// ---- フラグの説明 ----
	"flag.account":         "指定したアカウントのみ照会",
	"flag.upstream":        "CloudCode 上流のベース URL",
	"flag.usageTimeout":    "アカウントごとのクォータ照会タイムアウト",
	"flag.compact":         "1 行のコンパクト出力（スクリプト/パイプ用）",
	"flag.useTimeout":      "クォータ照会のタイムアウト",
	"flag.quotaThreshold":  "クォータ枯渇のしきい値",
	"flag.force":           "クォータが枯渇していても強制的に切り替え",
	"flag.timeout":         "ブラウザでのログイン完了を待つ時間",
	"flag.noBrowser":       "認証 URL を表示するだけでブラウザを開かない（SSH/ヘッドレス）",
	"flag.email":           "事前入力する Google メール（任意）",
	"flag.listen":          "待ち受けアドレス",
	"flag.serveAccount":    "プール内のこのアカウントのみ使用。空なら全て、プールが空なら keyring にフォールバック",
	"flag.refresh":         "起動時に access_token を 1 回強制更新",
	"flag.verbose":         "リクエストごとにアカウント選択と転送を記録",
	"flag.logFile":         "ログファイル。空なら stderr に出力",
	"flag.userAgent":       "上流に送る User-Agent（ライセンスゲート。通常は変更しない）",
	"flag.passthrough":     "エンベロープ変換を無効化し純粋なパススルーにする（デバッグ用。上流は agy の Gemini パスを受け付けない）",
	"flag.noModelAlias":    "モデル表・モデル別名・モデル検出を行わない（デバッグ用）",
	"flag.quotaInterval":   "GEMINI クォータのポーリング間隔。0 で無効化（自動切り替えも停止）",
	"flag.quotaThreshold0": "残量比率がこの値以下なら切り替え（0 = ゼロになってから切り替え、残量を無駄にしない）",
	"flag.stripField":      "転送前にリクエスト本文のトップレベルからこの JSON 項目を削除（複数指定可）",
	"flag.quotaIntervalD":  "クォータのポーリング間隔。0 なら 1 回だけ実行して終了",
	"flag.daemonAccount":   "このアカウントのみ監視。空ならプール全体を監視",
	"flag.probeMode":       "プローブのモード: http | connect",
	"flag.probeListen":     "待ち受けアドレス",
	"flag.probeUpstream":   "上流アドレス。空なら記録のみで転送しない",
	"flag.probeOut":        "JSONL ログのパス。空なら一時ファイルを作成",
	"flag.bodyMax":         "1 レコードあたりに保持するリクエスト本文のバイト数",
	"flag.quiet":           "ファイルにのみ書き、stderr には出力しない",
	"flag.tuiListen":       "ゲートウェイの待ち受けアドレス",
	"flag.tuiAccount":      "このアカウントのみ使用",
	"flag.tuiLogFile":      "ゲートウェイ/TUI のログファイル",
	"flag.syncKeyring":     "アカウント切り替え時に Keyring を同期して agy を再起動",

	// ---- クォータ ----
	"quota.argon":    "Argon 週間",
	"quota.week":     "週間",
	"quota.fiveHour": "5h",
	"quota.resetAt":  "(リセット %s)",
	"quota.unknown":  "不明",
	"quota.notFound": "GEMINI グループが見つかりません",
	// ---- 以前漏れていた key（TestAllUsedKeysExist が検出）----
	"common.failed":          "失敗",
	"usage.noArgs":           "usage は位置引数を取りません",
	"serve.noPositional":     "serve は位置引数を取りません",
	"serve.intervalNegative": "-quota-interval は負の値にできません（%s）",
	"serve.thresholdRange":   "-quota-threshold は 0〜1 の範囲で指定してください（%v）",
	"serve.stripFields":      "リクエスト本文から削除する項目: %s",
	"serve.modelsFail":       "モデル表の取得に失敗しました。バックグラウンドで再試行します（一部モデルが 404 になる可能性）: %v",
	"serve.pickAccount":      "%s (%s) を選択、期限 %s",
	"serve.inflightSwitch":   "実行中のリクエストが全て完了しました。アカウントを切り替えて同期します: %s (%s)",
	"serve.repickFail":       "起動後の再選択に失敗しました: %v",
	"serve.shutdown":         "終了シグナルを受信しました。安全に終了します…",
	"serve.shutdownFail":     "終了に失敗しました: %w",
	"serve.daoPoolEmpty":     "プールが空です。先に ags login <名前> か ags add <名前> を実行してください",
	"serve.daoFirstFail":     "初回に利用可能なアカウントがありません: %v",
	"serve.daoNone":          "利用可能なアカウントがありません: %v",
	"serve.daoStateFail":     "選択状態の保存に失敗しました: %v",
	"probe.tempFail":         "ログファイルの作成に失敗しました: %w",
	"probe.dirFail":          "ログディレクトリの作成に失敗しました: %w",
	"probe.openFail":         "ログを開けませんでした: %w",
	"probe.chmodFail":        "ログの権限設定に失敗しました: %w",
	"probe.connectListen":    "CONNECT プローブが %s で待ち受けています",
	"probe.unsetNoForward":   "(未設定、記録のみで転送しない)",
	"serve.tokenFail":        "%s のクォータ確認でトークン取得に失敗しました（状態は変更しません）: %v",
	"serve.quotaFail":        "%s のクォータ確認に失敗しました（状態は変更しません）: %v",
	"serve.forward":          "→ %s (%s) %s%s",
	"serve.forwardFail":      "転送に失敗 %s%s: %v",
	"serve.upstreamErr":      "上流 %d %s: %s",
	"serve.rewriteFail":      "リクエスト本文の書き換えに失敗 %s%s: %v",
	"serve.envelopeFail":     "エンベロープ変換に失敗 %s%s: %v",
	"serve.readBodyFail":     "リクエスト本文の読み取りに失敗 %s%s: %v",
	"serve.pickFail":         "アカウント選択に失敗しました: %v",
	"serve.discovery":        "モデル検出 %s → %d 件",
	// ---- 低レベルエラー（ユーザーの対応が必要なもの）----
	"keyring.noEntry":         "keyring に service=%s / username=%s の項目がありません（agy に未ログイン？）",
	"keyring.readFail":        "keyring の読み取りに失敗しました: %w",
	"keyring.badJSON":         "資格情報が正しい JSON ではありません: %w",
	"keyring.idTokenSegments": "id_token のセグメント数が %d です（3 であるべき）",
	"picker.cooling":          "%s: %s までクールダウン",
	"picker.refreshFail":      "%s の更新に失敗しました: %w",
	"picker.noUsableToken":    "%s: 有効なトークンがなく更新もできません",
	"picker.emptyToken":       "%s: access_token が空です",
	"picker.noneUsable":       "利用可能なアカウントがありません: %w",
	"picker.notInPool":        "アカウント %q は候補プールにありません",
	"flag.badValue":           "-%s の値が不正です: %q",
	"flag.missingValue":       "-%s には値が必要です",
	"serve.envelope":          "エンベロープ変換 %s、User-Agent %s",
	"serve.envelopeOn":        "有効",
	"serve.envelopeOff":       "無効（純粋なパススルー）",
	"serve.defaultModel":      "上流の既定モデル %s",
	"serve.retryFail":         "モデル表の再取得に失敗しました（%s 後に再試行）: %v",
}
