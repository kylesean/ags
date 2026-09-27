# agsw

[中文](README.md) | **English**

Multi-account switcher for `agy`: keep several Google accounts in a local pool; the daemon rewrites the system Keyring to whichever account still has quota. Stock `agy`, used as usual.

## Install

```sh
# One-liner (Linux / macOS, recommended)
curl -fsSL https://raw.githubusercontent.com/kylesean/agsw/main/install.sh | sh

# Go toolchain
go install github.com/kylesean/agsw/cmd/agsw@latest

# Prebuilt archives: download from GitHub Releases and drop into PATH
# Windows users: grab the zip from Releases
```

## Quickstart

```sh
agsw login A && agsw login B   # Pool two accounts (or capture via agsw add <name>)

# Terminal A: stay on duty
agsw daemon

# Terminal B: launch stock agy, use it normally
agy
```

When quota runs out, the AI reply shows an error. Confirm the switch line in terminal A (or run `agsw status` anywhere), restart `agy` in terminal B, then `/resume` to restore. In a hurry: `agsw use B` switches manually.

## Commands

| Command       | Description                                              |
| ------------- | -------------------------------------------------------- |
| `agsw login`  | Standalone OAuth login into the pool (`login [-email m@g] <name>`) |
| `agsw add`    | Import current system Keyring credentials into the pool  |
| `agsw list` / `status` / `drop` / `usage` | List / inspect / remove / quota-check accounts |
| `agsw use B`  | Verify quota, then switch the Keyring to B (`--force` overrides) |
| `agsw daemon` | Foreground loop: poll quotas and sync the Keyring        |
| `agsw serve`  | Reverse proxy (one retry on 429, optional)               |
| `agsw`        | TUI: launch the Gateway and manage `agy`                 |

## Common options

```sh
agsw daemon -quota-interval 5m      # Poll interval, default 3m (±25% jitter)
agsw daemon -quota-threshold 0.002  # Switch early below this; default 0 (empty only)
agsw daemon -account A              # Watch a single account
agsw use -force C                   # Switch regardless of quota
agsw -sync-keyring=false            # TUI: never touch Keyring, no auto-restart
agsw -- --print 'hi'                # Everything after -- goes to agy
```

## Notes

- Restart `agy` after every switch (it reads the Keyring once at startup and caches it); resume interactive sessions with `/resume`.
- Pool dir `0700`, credential files `0600`; rotate accounts in compliance with platform terms.

## Development

```sh
go build ./...
go test -race ./...
```
