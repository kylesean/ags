# ags

[中文](README.md) | **English**

Multi-account switcher for `agy`: keep several Google accounts in a local pool; the daemon rewrites the system Keyring to whichever account still has quota. Stock `agy`, used as usual.

## Install

```sh
# One-liner (Linux / macOS, recommended)
curl -fsSL https://raw.githubusercontent.com/kylesean/ags/main/install.sh | sh

# Go toolchain
go install github.com/kylesean/ags/cmd/ags@latest

# Prebuilt archives: download from GitHub Releases and drop into PATH
# Windows users: grab the zip from Releases
```

**Termux (Android)**: the install script detects Termux, downloads the `android/arm64` archive, and installs it to `$PREFIX/bin`. Termux has no desktop Secret Service, so `ags` automatically falls back to agy's local credential file `~/.gemini/antigravity-cli/antigravity-oauth-token` (override with `AGY_TOKEN_FILE`).

## Languages

Terminal output is available in Simplified Chinese / English / Japanese,
selected automatically from your locale:

```sh
ags                 # follows LANG / LC_ALL
ags -lang en        # force English
AGS_LANG=ja ags     # force Japanese
```

Other languages fall back to Chinese. Tables align by visual width (CJK takes
two columns) and shrink to fit narrow terminals.

## Quickstart

Pool your accounts once:

```sh
ags login A && ags login B   # Standalone OAuth login into the pool
# Or: log in a account in agy first, then ags add <name> to capture the current Keyring
```

Then pick one of the two modes (don't run both):

### Mode 1 (recommended): `ags` managed

```sh
ags
```

Starts the local Gateway and launches `agy`. When the current account runs out of quota, `ags` **syncs the Keyring and restarts agy automatically** — no manual step; use `/resume` to restore a session.

### Mode 2: `ags daemon` + stock `agy`

```sh
# Terminal A: stay on duty
ags daemon

# Terminal B: launch stock agy, use it normally
agy
```

The daemon rewrites the Keyring to an account that still has quota, but it does **not** restart agy. When quota runs out, the AI reply shows an error; confirm the switch line in terminal A (or run `ags status`), restart `agy` in terminal B, then `/resume`.

### Which mode?

| | Mode 1 `ags` | Mode 2 `ags daemon` |
|---|---|---|
| Restart agy after switching | **automatic** | manual |
| agy UI | shows "Enterprise Gateway", no email | stock, identity visible |
| Native `/usage` | taken over by the gateway | available |

**Why Mode 1 hides the identity**: `ags` injects `AGY_LLM_GATEWAY_URL` into `agy`, which then assumes it is talking to an enterprise gateway and renders the identity as empty (it expects the gateway to supply it). That is agy's own behaviour for enterprise gateway mode and does not affect functionality — `ags` prints the active account at startup and `ags usage` still reports real quota.

Pick Mode 2 for the stock UI, Mode 1 to avoid manual restarts.

## Switching accounts

- **Automatic**: Mode 1 switches and restarts agy for you; Mode 2 syncs the Keyring only, so you restart agy yourself.
- **Switch manually right away**:

  ```sh
  ags list            # Show account names and the current ●
  ags use B           # Verify quota, then switch the Keyring to B; refuses if exhausted
  ags use --force B   # Switch regardless of quota
  ```

  Restart `agy` after switching (`/resume` to restore a session).
- **Inspect**: `ags status` shows the current Keyring identity and whether a restart is needed.

## Quota

```sh
ags usage            # grouped progress bars: 5h / weekly / Argon
ags usage -compact   # one-line table, for scripts
```

Styled after agy's native `/usage`: group names, the `Models within this group:` line,
`[████░░░░] 47.44%` bars, and `Quota available` / `Refreshes in 26h 12m`.

## Commands

| Command       | Description                                              |
| ------------- | -------------------------------------------------------- |
| `ags login`  | Standalone OAuth login into the pool (**first choice for a new account**; `login [-email m@g] <name>`) |
| `ags add`    | Import current Keyring credentials (use when already signed in via agy) |
| `ags list` / `status` / `drop` / `usage` | List / inspect / remove / quota-check accounts |
| `ags use B`  | Verify quota, then switch the Keyring to B (`--force` overrides) |
| `ags daemon` | Foreground loop: poll quotas and sync the Keyring (restart agy yourself) |
| `ags serve`  | Start the reverse proxy only, without launching agy (advanced) |
| `ags probe`  | Intercepting probe to inspect agy↔upstream wire format (debugging) |
| `ags`        | Managed mode: start the Gateway, launch and auto-restart `agy` |

## Common options

```sh
ags -quota-threshold 0.01          # Managed mode: switch early below this; default 0 (empty only)
ags daemon -quota-interval 5m      # Poll interval, default 3m (±25% jitter)
ags daemon -quota-threshold 0.01   # Daemon: switch early below this
ags daemon -account A              # Watch a single account
ags use -force C                   # Switch regardless of quota
ags -sync-keyring=false            # TUI: never touch Keyring, no auto-restart
ags --print 'hi'                   # agy flags pass through (-- also works)
```

## Notes

- Restart `agy` after every switch (it reads the Keyring once at startup and caches it); resume interactive sessions with `/resume`.
- Pool dir `0700`, credential files `0600`; rotate accounts in compliance with platform terms.
- Without a desktop Secret Service (Termux / headless servers), `ags` falls back to agy's local credential file; `ags login` skips launching a browser when `DISPLAY` is unset — just open the printed URL yourself.

## Development

```sh
go build ./...
go test -race ./...
```
