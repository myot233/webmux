# 配置与持久化清单

本页记录当前实现，不把 OpenSpec 的目标行为当成已实现功能。后续变更必须同步运行时描述符、清单和源码引用。

## 当前服务端存储

以下路径除 `config.toml` 外均相对于 `state_dir`；`none` 表示没有对应跨进程锁。TOML 位置由 `-config` 或默认配置目录决定。运行时目录中的 socket 不是持久化 JSON，不能用备份记录替代活进程身份验证。

<!-- persistence-inventory:start -->

| ID                  | 路径                     | 锁                            | 所有者                       | 损坏或冲突            | 网页可改 |
| ------------------- | ------------------------ | ----------------------------- | ---------------------------- | --------------------- | -------- |
| bootstrap           | config.toml              | none                          | internal/config + cmd/webmux | 配置错误阻止启动      | 否       |
| launch              | launch-configs.json      | launch-configs.json.lock      | internal/launch              | unavailable           | 是       |
| repositories        | repositories.json        | repositories.json.lock        | internal/workspace           | unavailable           | 是       |
| tunnel              | tunnel-settings.json     | tunnel-settings.json.lock     | internal/tunnelsettings      | fail_closed           | 是       |
| tailscale-settings  | tailscale-settings.json  | tailscale-settings.json.lock  | internal/tailscalesettings   | fail_closed           | 是       |
| voice-settings      | voice-settings.json      | voice-settings.json.lock      | internal/voicesettings       | fail_closed           | 是       |
| p2p-settings        | p2p-settings.json        | p2p-settings.json.lock        | internal/p2psettings         | fail_closed           | 是       |
| tailscale-ownership | tailscale-ownership.json | tailscale-ownership.json.lock | cmd/webmux                   | block_start           | 否       |
| push                | push.json                | push.json.lock                | internal/push                | unavailable           | 是       |
| runtime             | run/server.json          | run/server.json.lock          | internal/runtime             | local_api_unavailable | 否       |
| settings            | settings.json            | settings.json.lock            | internal/settings            | unavailable           | 是       |
| hybrid              | hybrid-sessions.json     | hybrid-sessions.json.lock     | internal/hybrid              | unavailable           | 是       |
| agent-options       | agent-options.json       | agent-options.json.lock       | internal/hybrid              | unavailable           | 是       |

| conversation | conversations.sqlite | conversations.sqlite.lock | internal/conversation | unavailable | 是 |

<!-- persistence-inventory:end -->

这份表的机器来源是 `internal/archtest/persistence_inventory.json`；`make persistence-check` 精确比较表格各单元格（允许 Markdown 对齐空格变化），拒绝遗漏、重复或过时条目。

## 对话事务存储

`conversations.sqlite` 保存 Conversation、ProviderThread、单调 Run 序号，以及 handoff、delivery 和幂等 receipt 的事务基础。
ACP 聊天的规范化消息与工具内容在 UI 截断前保存为完整 Activity；服务端交接预览保存冻结来源、预算、选项与用户编辑。fork inline 投递先提交 receipt/pending，再按持久 checkpoint 准备资源；结果不明保留 unknown。
现有混合会话一对一迁移为稳定身份，旧记录标记 `historyOrigin=unknown`，不猜测升级前历史。

- 纯 Go SQLite（modernc.org/sqlite），支持 `CGO_ENABLED=0`；schema 迁移与版本号在同一事务提交。
- 交接选择的完整计数与草稿在同一事务维护，旧记录升级时从 selection 条目回填；按 handoff/position 建索引，小页读取不重新统计或排序整个历史。
- 数据库、生命周期锁与崩溃恢复 sidecar 均为 0600，状态目录 0700；拒绝文件自身的符号链接。
- SQLite 使用 FULL 同步和默认 DELETE rollback journal；不使用长期保留正文的 WAL；删除启用 `secure_delete`。
- 损坏、未知未来版本和锁冲突只禁用依赖对话存储的创建、恢复和忘记操作，以及历史/交接入口；普通终端仍可运行，原数据库不重建。
- 删除最后一条运行绑定（`forget=1`）或成功淘汰关联归档记录，清理其 Conversation、Run、Activity 分块和来源 handoff/命令记录；同一 Conversation 尚有其他 provider 记录时继续保留共享历史。
- 目标独立的 dispatch 摘要与用户 receipt 受目标生命周期管理：来源删除不能解除可能已经发送的 unknown，也不清空目标占用估算；精确原生回放只更新目标事实，不恢复来源操作。
- fork 目标单独保存回放所需的用户新请求、精确 native ID/hash 与目标附件引用，不复制来源历史。忘记来源后仍能正常显示目标用户消息；忘记目标时一并删除，来源的操作记录不会被重新建立。
- JSON 记录与 SQLite 不共享事务；受管 runtime 有未发布/已发布/正在删除标记。创建窗口保留预留资源，删除前先写意图；中断后用成功读取的 JSON 清单对账，不让已发布的 managed runtime 永久跳过清理。
- 未开始执行的孤立 fork 草稿一并清理；有目标原生事实、已 claim 的 prompt 或独立历史时继续保留目标事实。
- worktree 释放与重建保留身份，配置删除不隐式删除对话。备份时先停服务，并备份整个状态目录；不得只复制活跃数据库。
- 流式消息追加分块；重复前缀的工具快照只改写变化尾部，读取按 UTF-8 字节分页，不把整条历史装入内存。
- 完成后才发布持久 position；崩溃尾部保持不可见并标 gap，服务重启不把半条内容视为已提交。
- replay 按 ProviderThread + item ID 或有序完整项对账；无法判定时保留 `reconcile_required`，相同文本不会被全局 hash 合并。
- TUI 时段只记录缺口，不解析 PTY 伪造历史；不保存 reasoning、raw ACP、环境、原生工具状态或附件 blob，只保留附件元数据和引用。
- 投递 attempt 保留精确 worktree、runtime 与 dispatch 身份，旧 receipt 不跟随新重试改变；unknown 只能通过精确原生回放确认，不自动重发。
- 预览、独立用户任务、冻结来源工作位置、手工编辑与 revision 在同一数据库保存；服务重启不丢草稿。预览不启动 agent 或创建 worktree，SQLite 不存附件正文。
- 生成持久预览前，受限暂存附件复制到已验证来源 runtime 的私有 uploads，保留原暂存文件供其他表单使用；已有来源上传不重复复制。副本沿用该 runtime 的保留、清扫和忘记生命周期，可跨服务重启，但不保证抵御操作系统临时目录清理。
- 来源草稿快照以已验证内容和原暂存文件名共同生成固定长度 SHA-256 身份；同一暂存文件相同内容幂等复用，不同文件即使字节相同也保留独立引用。同名不同内容仍可并存，不覆盖普通 uploads 或旧预览；显示名称与实际路径授权、内容探测分离。
- 投递时以持久 copy intention 和内容 hash 发布独立目标 uploads；目标副本不依赖来源或暂存区继续存在。
- Codex 原生注入保存独立 pending/injected/unknown、原生身份及有序项目摘要；目标持有回放摘要和占用估算，来源删除不导致再次导入。用户 prompt 的结果不能撤销已确认的原生注入。
- 原生注入已保存但确认丢失时，即使已忘记来源，目标仍可用完整、有序且身份一致的原生回放持久确认注入及项目 ID；不重建来源投递，不重发历史或来源中尚未发送的用户请求。
- v10 升级与已验证的 v11 重开在启动事务中修复孤立派发 owner，分别恢复历史 ProviderThread/native 身份与当前 runtime 绑定；不要求旧线程仍有活绑定。历史 unknown 仅按原 native 身份对账，不能借新的原生线程恢复绕过；不增加 schema 版本或逐请求修复。
- 取消原生注入不会丢弃已排队的写者准入结果；转换始终收回 ACP 写者并完成清理，关闭会话不因遗留的打开中状态而挂起。
- 数据库错误不把 SQL 值或正文交给应用日志；当前版本不可用时保留原文件供人工诊断。
- `history_capture=false` 关闭新正文捕获、历史 REST、handoff 与 MCP，但仍维护身份和未决投递保护；已保存正文不删除。关闭区间不在重新开启后的 load 中自动回填。
- SQLite 随完整历史增长，不受终端 `scrollback_bytes` 淘汰；当前没有自动历史期限或数据库配额。需要删除时使用“忘记”，并监测磁盘空间。
- `secure_delete` 清除当前数据库空闲页中的删除内容，文件大小可能保留；不能擦除旧备份、文件系统快照、浏览器离线草稿或外部 agent transcript。

## 统一行为与故障隔离

- 每个 JSON 存储通过 `statefile.Open[T]` 取得注册 handle 和独立生命周期锁；数据与锁均为 0600、目录 0700。正常关闭显式解锁但不删除锁文件。`state_dir` 可为指向真实目录的符号链接；数据文件与锁自身仍拒绝符号链接。
- 每个 JSON 存储都有条数与字节双重上限：写入前用 `Handle.Fits` 判断，超限时淘汰可丢弃的记录（hybrid 记录、agent 选项缓存）或以无效输入拒绝（启动配置、仓库登记），不会让存储进入 `TooLarge` 失败。延迟合并写入失败时记录错误日志，I/O 类失败每 30 秒重试。
- 读取有大小限制，拒绝符号链接、非普通文件、重复/未知/大小写变体键、缺失必需字段、非法 UTF-8、错误版本、尾随值与过深嵌套。损坏原文件保留。
- Commit 和 Remove 强制目录同步。发布前失败保留旧状态；发布后同步失败报告 `commit_uncertain` 并冻结写入，不广播成功、不用旧内存覆盖已发布文件。
- `unavailable` 只禁用对应功能；push 不再改名重建密钥。首次确认文件不存在时仍可初始化。
- `fail_closed` 表示覆盖文件异常时关闭对应入口，不能从 TOML 回退为启用；紧急关闭仍可停止当前入口，但写盘失败必须报告未持久化。
- `tunnel-settings.json`、`tailscale-settings.json`、`voice-settings.json` 与 `p2p-settings.json` 共用 `internal/settingsstore` 的打开、读取、提交与 fail-closed 骨架；文件结构、校验与关闭方式由各自包注入，盘点中登记为 `entry-settings` 例外。
- `tailscale-settings.json` 保存用户希望的 mode/port/provider；`tailscale-ownership.json` 保存本实例实际创建的外部入口（精确目标、端口、pending/active），供撤销和重启对账。它们的职责、权限和失败策略不同。
- 旧 `tailscale.json` 即使损坏或与新文件并存，也在监听前阻止启动。先停止旧版本并核实/撤销其精确外部入口，再整理旧文件；不执行全局 reset。旧 `tunnel.json` 使 Cloudflare 保持关闭。新版不自动迁移文件或旧浏览器偏好。
- `run/server.json.lock` 仅保护运行时记录和本机控制面；锁冲突退化为 TCP 服务，不能与 Tailscale 安全阻止启动混淆。
- 升级前停止全部旧进程并备份；新旧版本不得共用状态目录并发运行，改名后的锁无法与旧锁互斥。
- Windows 托盘、日志、下载缓存、会话临时文件夹（含上传文件与快速开始的暂存上传）、代码树文件操作（写入用户工作区或临时文件夹）、`webmux repo settings --edit` 交给编辑器的临时文件与 baseline 结果属于独立生命周期，保持逐项登记的例外。
- 外部 agent 的锁与 Claude 进程注册记录不属于 WebMux 存储；适配器只进行有界只读核对，不创建或修改这些文件，也不把路径、记录正文或进程事实写入 WebMux 状态。
- Kimi ACP 会话切到 TUI 前是写入例外：适配器只把当前会话 `wire.jsonl` 的 `acp:<会话ID>` 环境绑定改为 `local`，避免 2.5.2 工具注册丢失。首次修改保留 `wire.jsonl.before-local-binding` 备份，临时文件原子替换，其他事件不变；这不属于 WebMux 存储。
- 例外是 `agent_pretrust`（默认开启），见 [CLI agent 接入](cli-agents.md#文件夹信任预写)：
  - 启动 Claude Code、Codex、Copilot CLI、Antigravity CLI、Qoder 或 QoderCN 会话前，`internal/agenttrust` 在该 agent 自己的配置文件里追加启动目录的“信任此文件夹”记录。
  - 启动 Kimi 时改为注入进程级 `KIMI_CODE_TRUST_WORKSPACE=1`。
  - 配置写入走临时文件加改名，保留原权限与其他内容。
  - 不触碰 WebMux 状态目录；Kimi 不写持久信任记录。
  - 主目录、其上级目录和文件系统根永不写入或注入。

## 磁盘与活进程诊断

`webmux config show [--json]` 只读展示磁盘解析结果：不创建目录/锁、不 chmod、不修复。

- `--live` 经同 UID socket 查询已验证进程的内存快照；磁盘漂移不改变该快照，不可达则报错。
- 字段来源为 default/toml/web/cli/generated；凭据只返回是否配置。
- 存储健康另有状态、提交阶段与观察时间；离线锁标为 not_checked，不能据此声称可写。

## 机械校验

```sh
make persistence-check
```

守卫位于现有 `internal/archtest`，直接解析 `cmd/` 与 `internal/` 所有平台的生产 Go 文件，不受本机 GOOS 限制。测试文件、testdata 和前端输出不参与生产扫描。

- 每个 `.json` / `.lock` 字符串及选定的文件创建、替换、删除、底层打开和锁函数引用，都登记到具体文件、函数、符号与出现次数。
- 解析导入别名；引用函数值也计数；拒绝对受检查包使用点导入。新增入口、重复调用、删除后遗留登记、无主登记、无理由例外或缺失来源文件都会失败。
- 每个存储的数据路径、派生锁名、所有者与失败策略必须匹配运行时 descriptor；数据路径必须出现在登记引用中。临时 JSON 模式与设备文件也需要登记，不能凭命名自动豁免。
- 负向样例验证新存储、旧函数中的新增调用、别名、函数值、Windows 文件、清单漏行及错误策略漂移确实被拒绝。
- `make check` 和 CI 的格式/构建 job 显式执行，常规 Go 测试也会运行。失败时根据诊断更新具体登记与本表，不能扩大目录级白名单。

## 校验边界

这是当前架构的防漂移入口守卫，不是完整的数据流证明。它无法推断已经登记函数内部任意动态路径的新含义，也不能证明 chmod、fsync、锁时序、错误策略描述与所有执行路径一致；外部进程和反射写文件不在扫描能力内。更换未覆盖的 I/O 库必须扩展扫描规则与负向样例。

所有领域使用已注册 typed handle；除本机 socket 清理外，领域原始写/锁入口已经撤销。逐 descriptor 契约测试覆盖权限、锁与重开；共享故障注入覆盖创建、chmod、短写、写入、Sync、Close、rename 和目录同步阶段，以及删除后同步不确定。领域与 API 测试进一步验证广播时机、降级与脱敏。

## 变更流程

新增存储前明确所属层、生命周期、数据/锁名、所有者、敏感性、失败后果和网页可编辑边界。登记真实读写入口及对应测试后同步清单；仍在规划的能力只写 OpenSpec，不登记为已实现。

详细目标、破坏性改名和分阶段验收见 [配置与状态治理提案](../openspec/changes/archive/2026-10-29-govern-config-state/proposal.md)。部署总览保留在 [部署指南](deployment.md)，架构分层见 [架构说明](architecture.md)。
