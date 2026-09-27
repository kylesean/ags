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

## Quickstart

```sh
ags login A && ags login B   # Pool two accounts (or capture via ags add <name>)

# Terminal A: stay on duty
ags daemon

# Terminal B: launch stock agy, use it normally
agy
```

When quota runs out, the AI reply shows an error. Confirm the switch line in terminal A (or run `ags status` anywhere), restart `agy` in terminal B, then `/resume` to restore. In a hurry: `ags use B` switches manually.

## Commands

| Command       | Description                                              |
| ------------- | -------------------------------------------------------- |
| `ags login`  | Standalone OAuth login into the pool (`login [-email m@g] <name>`) |
| `ags add`    | Import current system Keyring credentials into the pool  |
| `ags list` / `status` / `drop` / `usage` | List / inspect / remove / quota-check accounts |
| `ags use B`  | Verify quota, then switch the Keyring to B (`--force` overrides) |
| `ags daemon` | Foreground loop: poll quotas and sync the Keyring        |
| `ags serve`  | Reverse proxy (one retry on 429, optional)               |
| `ags`        | TUI: launch the Gateway and manage `agy`                 |

## Common options

```sh
ags daemon -quota-interval 5m      # Poll interval, default 3m (±25% jitter)
ags daemon -quota-threshold 0.002  # Switch early below this; default 0 (empty only)
ags daemon -account A              # Watch a single account
ags use -force C                   # Switch regardless of quota
ags -sync-keyring=false            # TUI: never touch Keyring, no auto-restart
ags --print 'hi'                 # agy flags pass through (-- also works)
```

## Notes

- Restart `agy` after every switch (it reads the Keyring once at startup and caches it); resume interactive sessions with `/resume`.
- Pool dir `0700`, credential files `0600`; rotate accounts in compliance with platform terms.

## Development

```sh
go build ./...
go test -race ./...
```
