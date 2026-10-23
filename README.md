# ags

**中文** | [English](README.en.md)

`agy` 多账号切换工具：多个 Google 账号养在本地池里，daemon 自动把系统 Keyring 换成有额度的那个，原生 `agy` 照常用。

## 安装

```sh
# 一键脚本（Linux / macOS，推荐）
curl -fsSL https://raw.githubusercontent.com/kylesean/ags/main/install.sh | sh

# Go 工具链
go install github.com/kylesean/ags/cmd/ags@latest

# 预编译包：GitHub Releases 按系统架构下载解压到 PATH
# Windows 用户请用 Releases 的 zip 包
```

**Termux（Android）**：一键脚本会自动识别 Termux，下载 `android/arm64` 包并安装到 `$PREFIX/bin`。Termux 没有桌面 Secret Service，`ags` 会自动降级读写 agy 的本地凭据文件 `~/.gemini/antigravity-cli/antigravity-oauth-token`（可用 `AGY_TOKEN_FILE` 覆盖）。

## 多语言

终端界面支持简体中文 / English / 日本語，按系统 locale 自动选择：

```sh
ags                 # 跟随 LANG / LC_ALL
ags -lang en        # 强制英文
AGS_LANG=ja ags     # 强制日文
```

其它语言回落到中文。表格按视觉列宽（中文占 2 列）对齐，窄终端会自动收缩。

## 快速开始

账号只需入池一次：

```sh
ags login A && ags login B   # 独立 OAuth 登录入池
# 或：先在 agy 里登录某号，再 ags add <name> 从当前 Keyring 捕获
```

之后按需二选一（别同时跑）：

### 方式一（推荐）：`ags` 托管

```sh
ags
```

启动本地 Gateway 并拉起 `agy`。当前号额度耗尽时**自动同步 Keyring 并重启 agy**，无需手动干预；会话用 `/resume` 恢复。

### 方式二：`ags daemon` + 原生 `agy`

```sh
# 终端 A：常驻值班
ags daemon

# 终端 B：原生启动 agy，该怎么用怎么用
agy
```

daemon 会把 Keyring 换成还有额度的号，但**不会重启 agy**。额度用尽时 AI 回复报红字，到终端 A 看切换行（或跑 `ags status`），在终端 B 重启 `agy` 并 `/resume` 恢复。

### 两种方式怎么选

| | 方式一 `ags` | 方式二 `ags daemon` |
|---|---|---|
| 切号后重启 agy | **自动** | 手动 |
| agy 界面 | 显示 "Enterprise Gateway"，看不到邮箱 | 原生，看得到身份 |
| 原生 `/usage` | 被网关接管 | 可用 |

**为什么方式一看不到身份**：`ags` 给 `agy` 注入 `AGY_LLM_GATEWAY_URL`，`agy` 据此认为自己在连企业网关，于是把身份显示成空（它认为身份由网关提供）。这是 `agy` 对企业网关模式的既定行为，不影响使用 —— `ags` 启动时会打印当前账号，`ags usage` 也能查真实额度。

在意原生界面就用方式二，在意免手动重启就用方式一。

## 切换账号

- **自动**：方式一由 `ags` 自动切换并重启 agy；方式二由 `ags daemon` 同步 Keyring，需自行重启 agy。
- **手动立即切**：

  ```sh
  ags list            # 看账号名与当前 ●
  ags use B           # 先验额度再切到 B；额度耗尽会被拒绝
  ags use --force B   # 无视额度强行切
  ```

  切完重启 `agy`（`/resume` 恢复会话）。
- **查状态**：`ags status` 看当前 Keyring 身份与是否需重启。

## 查额度

```sh
ags usage            # 分组进度条：5h / 周 / Argon 各窗口
ags usage -compact   # 单行表格，给脚本用
```

样式对齐 `agy` 原生 `/usage`：组名、`Models within this group:` 描述、
`[████░░░░] 47.44%` 条形，以及 `额度可用` / `将于 26h 12m 后刷新`。

## 命令

| 命令          | 说明                                     |
| ------------- | ---------------------------------------- |
| `ags login`  | 独立 OAuth 登录入池（**新号首选**，`login [-email m@g] <name>`） |
| `ags add`    | 从当前 Keyring 捕获凭据入池（已在 agy 登录过时用） |
| `ags list` / `status` / `drop` / `usage` | 查池 / 查状态 / 删号 / 查各号真实额度 |
| `ags use B`  | 先验额再把 Keyring 切到 B（`--force` 强行）|
| `ags daemon` | 前台常驻：轮询额度并同步 Keyring（需手动重启 agy） |
| `ags serve`  | 只启动反向代理，不拉起 agy（高级用法）    |
| `ags probe`  | 抓包探针，分析 agy 与上游的线格式（排查用） |
| `ags`        | 托管模式：启动 Gateway、拉起并自动重启 `agy` |

## 常用选项

```sh
ags -quota-threshold 0.01          # 托管模式：剩余额度低于此提前切，默认 0（归零才切）
ags daemon -quota-interval 5m      # 轮询间隔，默认 3m（±25% 抖动）
ags daemon -quota-threshold 0.01   # daemon：剩余额度低于此提前切
ags daemon -account A              # 只看管指定账号
ags use -force C                   # 无视额度强行切换
ags -sync-keyring=false            # TUI 不写 Keyring、不自动重启
ags --print 'hi'                   # agy 参数直接透传（-- 后也行）
```

## 注意

- 切换后必须重启 `agy` 生效（`agy` 启动时读一次 Keyring 并缓存），交互式会话用 `/resume` 恢复。
- 账号池目录 `0700`、凭据文件 `0600`；多账号轮换请遵守平台服务条款。
- 无桌面 Secret Service 的环境（Termux / 无头服务器）自动降级到 agy 本地凭据文件；`ags login` 在无 `DISPLAY` 时不会尝试拉起浏览器，直接用打印出的链接登录即可。

## 开发

```sh
go build ./...
go test -race ./...
```
