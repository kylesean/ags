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

```sh
ags login A && ags login B   # 两个号入池（或 ags add <name> 从当前 Keyring 捕获）

# 终端 A：常驻值班
ags daemon

# 终端 B：原生启动 agy，该怎么用怎么用
agy
```

额度用尽时 AI 回复会报红字，到终端 A 确认切换行（或任意处跑 `ags status`），在终端 B 重启 `agy` 并 `/resume` 恢复。等不及就 `ags use B` 手动切。

## 命令

| 命令          | 说明                                     |
| ------------- | ---------------------------------------- |
| `ags login`  | 独立 OAuth 登录入池（`login [-email m@g] <name>`） |
| `ags add`    | 从当前系统 Keyring 捕获凭据入池          |
| `ags list` / `status` / `drop` / `usage` | 查池 / 查状态 / 删号 / 查各号真实额度 |
| `ags use B`  | 先验额再把 Keyring 切到 B（`--force` 强行）|
| `ags daemon` | 前台常驻：轮询额度并同步 Keyring         |
| `ags serve`  | 反向代理（429 换号重试一次，可选）        |
| `ags`        | TUI：启动 Gateway 并拉起 `agy`            |

## 常用选项

```sh
ags daemon -quota-interval 5m      # 轮询间隔，默认 3m（±25% 抖动）
ags daemon -quota-threshold 0.002  # 剩余额度低于此提前切，默认 0（归零才切）
ags daemon -account A              # 只看管指定账号
ags use -force C                   # 无视额度强行切换
ags -sync-keyring=false            # TUI 不写 Keyring、不自动重启
ags --print 'hi'                 # agy 参数直接透传（-- 后也行）
```

## 注意

- 切换后必须重启 `agy` 生效（`agy` 启动时读一次 Keyring 并缓存），交互式会话用 `/resume` 恢复。
- 账号池目录 `0700`、凭据文件 `0600`；多账号轮换请遵守平台服务条款。

## 开发

```sh
go build ./...
go test -race ./...
```
