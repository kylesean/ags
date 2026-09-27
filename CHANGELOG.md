# Changelog

本项目遵循 [Keep a Changelog](https://keepachangelog.com/zh-CN/1.1.0/)，
版本号遵循 [语义化版本](https://semver.org/lang/zh-CN/)。

## [v0.5.0] - 2026-09-27

### Changed
- **彻底改名 `agsw` → `ags`**：二进制、仓库、Go module、环境变量（`AGS_*`，兼容旧 `AGSW_*`）、数据目录（旧 `~/.local/share/agsw` 自动迁走）全统一；`agsw gui` 早已移除，历史版本见下。

## [v0.4.2] - 2026-09-27

### Fixed
- **TUI 未知 flag 透传 agy**：`agsw --dangerously-skip-permissions` 这类 agy 参数不再报错进帮助界面；只有 `--` 仍可强制其后全归 agy。

## [v0.4.1] - 2026-09-27

### Added
- **daemon 选中落盘与 `status` 重启提示**：`state.json` 记录选中账号，`status` 在 Keyring 与选中不一致时直接提示重启。

### Fixed
- **`stopAgy` 按 OS 区分**：Windows 跳过注定失败的 `Interrupt`，短等待后直接 Kill；Unix 保持优雅中断。

## [v0.4.0] - 2026-09-27

### Added
- **`agsw daemon`（轻量本体）**：前台常驻，只做额度轮询 + Keyring 同步 + 重启提醒，不劫持流量、不托管 agy；原生 `/usage` 与同步状态天然一致。
- **`agsw use <name> [--force]`（手动范式）**：先验额度再写 Keyring，无额度拒绝（`--force` 强行）。
- **429 单号即查**：触发只查当事账号（1 次请求），常规轮询才全量。
- **轮询抖动**：每轮叠加 ±25%，打散机械周期节拍。

### Changed
- **默认轮询间隔 1m → 3m**（`DefaultQuotaInterval`）；`serve`/`tui`/`daemon` 共用。
- **401 触发自愈**：上游 401 打 30s 冷却并唤醒额度轮询，对齐 429。
- **429 冷却保持**：健康轮询不再洗掉未过期的 429/401 避让（`SetCooldownMax`/`ClearExpiredCooldown` 原子语义）；耗尽分支取最晚。
- **`RefreshAll` 并发化**：不再持全局锁跨网络刷新，各号持细粒度锁并发。
- **CLI 改名**：裸 `agsw` 即 TUI，移除 `agsw gui` 子命令（TUI 日志改 `~/.cache/agsw/tui.log`）；400 错误体改 JSON 编码；损坏池文件跳过时打日志；组识别精确优先于模糊；`--print=` 等值形态识别。

### Fixed
- **空池 TUI 启动死路**：池空回落伪账号不再导致 `syncKeyringAccount` 失败退出；伪账号标记化，刷新不落盘，真名 `keyring` 账号不受影响。
- **`gui` flag 对等**：TUI 透传 `upstream/refresh/user-agent/passthrough/strip-field`。

## [v0.3.0] - 2026-09-24

### Added
- **`Selector.FreshToken` 与闲置/冷却账号凭据保鲜**：即使账号处于业务冷却状态也能独立进行 Token 刷新，彻底解决配额监控协程对闲置及冷却账号轮询时遭遇 401 报错的死循环问题，确保配额重置即刻感知并解冻。
- **活跃账号粘性（Active Account Affinity）**：选号策略优先沿用当前健康账号，避免旧账号解冻时突发“反抢”导致频繁切号。
- **模型别名后台重试与并发安全**：启动期 `FetchModels` 遇网络故障时转入后台指数退避重试，并为 `proxy.Server` 的模型别名映射表增加读写锁，消除并发竞态。
- **并发额度查询 (`agsw usage`)**：并发拉取账号池各账号实时配额信息，保持稳定显示顺序，消除串行网络延迟。
- **在飞请求保护与平滑切号**：GUI 模式下在飞请求处理期间切号自动暂存，待当前 Prompt 的 SSE 流完整输出并交付后再触发 Keyring 同步与重启，绝不中断生成。

### Fixed
- **429 故障重试时的响应头污染**：`retryResponseWriter` 独立缓冲响应头，仅在最终交付时同步，避免首轮 429 错误头泄漏至重试后的成功响应中。
- **超限响应与重试请求体的连接泄漏**：引入 `multiReadCloser` 保留底层 `Close()` 能力，防止连接与文件描述符泄漏。
- **响应体截断时的 `Content-Length` 不一致**：修复上游错误响应及 429 响应体截断后 `Content-Length` 头未同步校正导致客户端 `unexpected EOF` 的问题。
- **CONNECT 探针单向关闭挂起**：双向流量搬运均接入通道通知，任一端收到 EOF 立即关闭双向连接，解决上游先断开时连接挂死的问题。
- **已被删除账号的僵尸复活**：Token 刷新落盘前校验文件存在性，已被 `agsw drop` 的账号不再回写落盘。
- **GUI 模式子进程退出与启动等待异常**：修复子进程崩溃退出错误被吞问题，并修复 `serve` 异常退出时 `waitGateway` 盲目等待超时的缺陷。

## [v0.2.0] - 2026-09-24

### Added
- **统一启动器 (`agsw` / `agsw gui`)**：直接拉起本地 Gateway 并托管 `agy` 进程，自动注入 `AGY_GATEWAY_URL` 与 `NO_PROXY`。
- **自动 Keyring 同步与会话恢复**：账号切换后自动写入系统 Keyring，用户通过 `/resume` 即可无缝切换凭据。

## [v0.1.0] - 2026-09-23

### Added
- **`agsw login <name>`**：独立 OAuth2 授权码流程（PKCE S256 + 127.0.0.1 本地回环回调），支持多平台默认浏览器自动拉起与凭据直写账号池。
- **`agsw serve`**：反向代理服务，支持 Gemini 与 Cloud Code 双向信封改写、SSE 流式解壳、模型别名转换与请求级凭据注入。
- **额度感知与轮换**：后台轻量轮询上游配额接口，配额耗尽自动标记账号冷却；配合代理上游 429 响应即时触发快速探测与换号。
- **`agsw probe`**：流量探针工具，支持 HTTP 网关与 CONNECT 代理透明抓包诊断，敏感凭据自动脱敏。
- **账号池管理**：`add`、`list`、`status`、`drop` 命令，凭据文件采用严苛权限控制与原子写入。

### Changed
- **选号器细粒度并发控制**：Token 网络刷新重构为单账号互斥与 double-checked locking，解除全局锁对其他健康账号的读写阻塞。
- **代理 429 响应被动联动**：新增 `StatusReporter` 接口，代理层拦截 429 立即触发账号临时避让并唤醒配额轮询协程。
- **全平台浏览器拉起适配**：抽离操作系统分发逻辑，支持 Linux (`xdg-open`)、macOS (`open`) 与 Windows (`rundll32`)。

### Fixed
- **超限/异常响应体泄漏与截断修复**：`readAllLimited` 修复边界字节截断问题，超限时利用 `io.MultiReader` 保留已读字节并无缝回退透传流，避免连接泄漏。
- **OAuth 回调服务平滑退出**：回调处理器显式刷新 HTTP 响应并在后台优雅关闭，消除高并发下本地 TCP 提前断开 (EOF) 竞争。
- **SSE 换行符跨端归一化**：规范化换行处理，防止上游混合 CRLF/LF 导致客户端事件解析停滞。

### Security
- 凭据文件采用 0600 权限与临时文件原子重命名写入，账号名称严格校验防止路径穿越。
- 诊断探针自动脱敏 `Authorization`、`Cookie` 等关键凭据。
- 额度冷却状态保持纯内存维护，避免污染持久化凭据。
