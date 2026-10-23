package i18n

// enCatalog is the English copy.
//
// Placeholders must match the zh table exactly (TestPlaceholdersMatch).
// Line widths are free to differ — layout adapts via internal/term.
var enCatalog = map[string]string{
	// ---- top level ----
	"cli.usage": `ags — multi-account switcher for agy

Usage:
  ags                 start the TUI and launch agy
  ags <command> [args]  run a specific subcommand

Commands:
  probe    start the intercepting probe (inspect wire format and protocol)
  login    run Google OAuth separately and store credentials in the pool
  add      capture the current system Keyring credentials into the pool
  list     list accounts in the pool
  status   show the current Keyring account and pool status
  drop     remove an account from the pool
  usage    query real quota for each account in the pool
  use      check quota then switch the system Keyring to an account (manual)
  daemon   stay in the foreground: poll quota and sync Keyring (lightweight)
  serve    start the reverse proxy (credential injection, envelope rewrite, auto-switch)
  tui      start the TUI and launch agy (default entry point, i.e. bare ags)

Run "<command> -h" for each command's flags.
`,
	"cli.unknownCmd": "unknown command %q\n\n%s",
	"cli.error":      "ags: %s",

	// ---- common ----
	"common.yes":        "yes",
	"common.no":         "no",
	"common.present":    "present",
	"common.absent":     "absent",
	"common.dash":       "-",
	"common.expired":    "expired",
	"common.inDuration": "in %s",
	"common.warning":    "warning",
	"common.cooling":    "cooling down",

	// ---- columns ----
	"col.name":    "name",
	"col.email":   "email",
	"col.expiry":  "expires",
	"col.refresh": "refresh",
	"col.current": "current",

	// ---- add ----
	"add.usage":      "usage: ags add <name>",
	"add.exists":     "pool already has %q; run ags drop %s first to replace it",
	"add.readFail":   "failed to read current credentials: %w",
	"add.emailChk":   "failed to check whether the email is already pooled: %w",
	"add.emailDup":   "email %s already exists (account %s); use a different Google account, or run ags drop %s first",
	"add.claimsFail": "failed to parse identity: %w",
	"add.captured":   "captured → %s",
	"add.noRefresh":  "no refresh_token; automatic renewal will not work",
	"add.stored":     "stored",

	// ---- login ----
	"login.usage":     "usage: ags login [-timeout 5m] [-no-browser] [-email you@example.com] <name>\n(flags must come before <name>)",
	"login.exists":    "pool already has %q; run ags drop %s first to replace it",
	"login.start":     "signing in → %s",
	"login.emailChk":  "sign-in succeeded but the email check failed (credentials not saved): %w",
	"login.emailDup":  "email %s already exists (account %s); use a different Google account, or run ags drop %s first",
	"login.saveFail":  "sign-in succeeded but saving to the pool failed (credentials not saved): %w",
	"login.saved":     "added to pool → %s",
	"login.noRefresh": "no refresh_token; automatic renewal will not work",
	"login.refreshOk": "yes (this account never needs signing in again)",
	"login.scope":     "scope %d entries",

	// ---- list ----
	"list.noArgs":            "list takes no arguments",
	"list.empty":             "the pool is empty (%s)",
	"list.emptyHint":         "usage: sign in with agy first, then run ags add <name>",
	"list.keyring":           "keyring current: %s",
	"list.keyringUnreadable": "keyring current: (unreadable or not signed in)",

	// ---- status ----
	"status.noArgs":      "status takes no arguments",
	"status.keyringHdr":  "== keyring identity ==",
	"status.readFail":    "read failed: %v",
	"status.email":       "email",
	"status.auth":        "auth",
	"status.expiry":      "expires",
	"status.refresh":     "refresh",
	"status.poolHdr":     "== account pool ==",
	"status.poolEmpty":   "(empty)",
	"status.accountLine": "  %s %s  expires %s%s",
	"status.poolDir":     "pool directory: %s",

	// ---- drop ----
	"drop.usage":   "usage: ags drop <name>",
	"drop.deleted": "deleted %s",

	// ---- use ----
	"use.usage":       "usage: ags use [--force] <name>",
	"use.timeout":     "-timeout must be greater than 0",
	"use.noToken":     "account %s has no usable token and no refresh_token",
	"use.refreshFail": "failed to refresh %s: %w",
	"use.quotaFail":   "failed to query quota for %s: %w",
	"use.exhausted":   "account %s quota is exhausted (%s, cooling until %s); refusing to switch — use --force to override",
	"use.switched":    "switched keyring → %s (%s)",
	"use.restart":     "restart agy to apply (run /resume in an interactive session)",

	// ---- usage ----
	"usage.account":       "Account: %s",
	"usage.pool":          "Pool: %s",
	"usage.noGroups":      "no quota groups found (upstream may have changed the API, or the UA is stale)",
	"usage.noGeminiGroup": "no GEMINI quota group found",
	"usage.queryFail":     "query failed: %v",
	"usage.failSummary":   "%d/%d accounts failed to query",
	"usage.window5h":      "Five Hour Limit Remaining",
	"usage.windowWeekly":  "Weekly Limit Remaining",
	"usage.available":     "Quota available",
	"usage.unknown":       "Quota unknown",
	"usage.refreshesIn":   "Refreshes in %s",
	"usage.refreshesSoon": "Refreshes soon",
	"usage.accountFail":   "Account: %s\nquery failed: %v\n",
	"usage.compactHeader": "account\temail\tquota",

	// Duration fragments. English pluralises, so keep the "s" form and let
	// the odd "1 hours" through: it is a countdown, not prose.
	"usage.durDaysHours": "%dd %dh",
	"usage.durDays":      "%dd",
	"usage.durHoursMins": "%dh %dm",
	"usage.durHours":     "%dh",
	"usage.durMins":      "%dm",

	// ---- tui ----
	"tui.oneShotNoRestart": "one-shot agy runs skip automatic Keyring restarts",
	"tui.agyNotFound":      "agy not found; install it and add it to PATH: %w",
	"tui.logOpenFail":      "failed to open the TUI log: %w",
	"tui.switchAccount":    "switching to %s (%s); syncing Keyring and restarting agy",
	"tui.agyRestarted":     "agy restarted; run /resume to continue your session",
	"tui.agyExit":          "agy exited: %w",
	"tui.gatewayExit":      "gateway exited: %w",
	"tui.gatewayStartFail": "failed to start the gateway: %w",
	"tui.gatewayExitEarly": "serve exited before the gateway became ready",

	// ---- pool / keyring ----
	"pool.syncFail":             "failed to sync account %s into Keyring: %w",
	"pool.loadFail":             "failed to load account to switch to %q: %w",
	"pool.identityChanged":      "account %s identity changed; refusing to write Keyring",
	"pool.noCredentials":        "account %s has no credentials that can be written",
	"pool.noCandidates":         "the pool is empty and reading keyring failed: %w",
	"pool.claimsFail":           "failed to parse identity: %w",
	"pool.loadAccountFail":      "failed to load account %q: %w",
	"pool.refreshPartial":       "forced refresh partially failed: %v",
	"pool.noUsableAfterRefresh": "still no usable account after refresh: %w",
	"pool.refreshedAll":         "force-refreshed all accounts",

	// ---- serve / daemon ----
	"serve.noAccount":      "no usable account",
	"serve.modelsLoaded":   "model mapping loaded",
	"serve.warning":        "%s",
	"serve.listening":      "listening on %s → %s",
	"serve.ready":          "Gateway ready: %s",
	"serve.quotaExhausted": "quota exhausted for %s: %s → cooling until %s",
	"serve.quotaOk":        "quota %s: %s",
	"serve.account":        "account %s (%s), token expires %s",
	"serve.candidates":     "%d pool candidates: %s",
	"serve.quotaInterval":  "quota check every %s for the GEMINI group (threshold %.3f)",
	"serve.quotaOff":       "quota checks disabled (-quota-interval=0); no automatic switching",
	"serve.allCooling":     "all accounts cooling down: %v",
	"serve.got429":         "upstream returned 429; account %s cooling for 1 minute",
	"serve.got401":         "upstream returned 401; account %s cooling for 30 seconds",
	"serve.retryNext":      "upstream 429; account %s cooling, retrying the next one",
	"serve.switched":       "switched keyring → %s (%s); restart agy to apply (run /resume in an interactive session)",

	// ---- probe ----
	"probe.httpHint":    "hint: AGY_LLM_GATEWAY_URL=http://%s agy --print 'hi'",
	"probe.connectHint": "hint: AGY_PROXY_URL=http://%s agy --print 'hi'",
	"probe.noParams":    "probe takes no positional arguments",
	"probe.badMode":     "unknown -mode %q (available: http | connect)",
	"probe.listen":      "probe listening on %s (upstream=%s)",
	"probe.listenFail":  "failed to listen on %s: %w",
	"probe.unset":       "(unset)",
	"probe.captured":    "captured %s %s%s (%d bytes, proto=%s)",
	"probe.forwarded":   "forwarded %s%s → %s [%d]",
	"probe.forwardFail": "forward failed %s%s: %v",
	"probe.writeFail":   "failed to write record: %v",
	"probe.logPath":     "log: %s",

	// ---- flag help ----
	"flag.account":         "only query the given account",
	"flag.upstream":        "CloudCode upstream base URL",
	"flag.usageTimeout":    "per-account quota query timeout",
	"flag.compact":         "single-line compact output (for scripts/pipes)",
	"flag.useTimeout":      "quota query timeout",
	"flag.quotaThreshold":  "quota exhaustion threshold",
	"flag.force":           "switch even if quota is exhausted",
	"flag.timeout":         "how long to wait for the browser sign-in",
	"flag.noBrowser":       "only print the auth URL, do not open a browser (SSH/headless)",
	"flag.email":           "prefilled Google email (optional)",
	"flag.listen":          "listen address",
	"flag.serveAccount":    "use only this pooled account; empty means all, falling back to keyring when the pool is empty",
	"flag.refresh":         "force one access_token refresh at startup",
	"flag.verbose":         "log account selection and forwarding for every request",
	"flag.logFile":         "log file; empty writes to stderr",
	"flag.userAgent":       "User-Agent sent upstream (license gate; leave it alone)",
	"flag.passthrough":     "disable envelope rewriting, pure passthrough (debug; upstream rejects agy's Gemini paths)",
	"flag.noModelAlias":    "skip the model table, model aliases and model discovery (debug)",
	"flag.quotaInterval":   "GEMINI quota poll interval; 0 disables quota checks (and automatic switching)",
	"flag.quotaThreshold0": "switch when remaining ratio <= this value (0 = only switch at zero, to use every last bit)",
	"flag.stripField":      "drop this top-level JSON field before forwarding (repeatable)",
	"flag.quotaIntervalD":  "quota poll interval; 0 runs a single round then exits",
	"flag.daemonAccount":   "watch only this account; empty watches the whole pool",
	"flag.probeMode":       "probe mode: http | connect",
	"flag.probeListen":     "listen address",
	"flag.probeUpstream":   "upstream address; empty records only and forwards nothing",
	"flag.probeOut":        "JSONL log path; empty creates a temp file",
	"flag.bodyMax":         "request-body bytes kept per record",
	"flag.quiet":           "write to the file only, do not print to stderr",
	"flag.tuiListen":       "gateway listen address",
	"flag.tuiAccount":      "use only this account",
	"flag.tuiLogFile":      "gateway/TUI log file",
	"flag.syncKeyring":     "sync Keyring and restart agy when switching accounts",

	// ---- quota ----
	"quota.argon":    "Argon weekly",
	"quota.week":     "weekly",
	"quota.fiveHour": "5h",
	"quota.resetAt":  "(resets %s)",
	"quota.unknown":  "unknown",
	"quota.notFound": "no GEMINI group found",
}
