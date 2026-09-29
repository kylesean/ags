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

## 切换账号

- **自动**：方式一由 `ags` 自动切换并重启 agy；方式二由 `ags daemon` 同步 Keyring，需自行重启 agy。
- **手动立即切**：

  ```sh
  ags list            # 看账号名与当前 ●
  ags use B           # 先验额度再切到 B；额度耗尽会被拒绝
  ags use --force B   # 无视额度强行切
  ```

  切完重启 `agy`（`/resume` 恢复会话）。
- **查状态**：`ags status` 看当前 Keyring 身份与是否需重启；`ags usage` 看各号真实额度。

## 命令

| 命令          | 说明                                     |
| ------------- | ---------------------------------------- |
| `ags login`  | 独立 OAuth 登录入池（`login [-email m@g] <name>`） |
| `ags add`    | 从当前系统 Keyring 捕获凭据入池          |
| `ags list` / `status` / `drop` / `usage` | 查池 / 查状态 / 删号 / 查各号真实额度 |
| `ags use B`  | 先验额再把 Keyring 切到 B（`--force` 强行）|
| `ags daemon` | 前台常驻：轮询额度并同步 Keyring（需手动重启 agy） |
| `ags serve`  | 反向代理（429 换号重试一次，可选）        |
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

## 开发

```sh
go build ./...
go test -race ./...
```
