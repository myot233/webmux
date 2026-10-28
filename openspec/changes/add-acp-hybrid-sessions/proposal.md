## Why

webmux 现在只把 CLI agent 当作 PTY 字节流托管。手机上阅读输出、点按批准、切换模型和排队都要经过终端与软键盘，门槛高；桌面端的 TUI 重绘也不够理想。[Agent Client Protocol（ACP）](https://agentclientprotocol.com) v1 已经稳定，OpenCode 与 omp 都原生提供 `acp` 子命令，并且实测可以在 ACP 与原生 TUI 之间用同一个 agent 会话 ID 交替接手（见 issue #181 的“研究记录”）。

issue #181 已定稿 D1–D32 共 32 条决策，PR #186 已在 Storybook 中给出聊天视图、切换、首页卡片的可交互原型。这次提案把这些决策落成规格与架构：确定在哪一层引入 ACP、会话身份与进程归谁所有、线上契约怎样扩展，并让后续接入 Claude/Codex 适配器、omp RPC 模式或 ACP v2 时不需要推翻第一期的结构。

## What Changes

- **混合会话（hybrid session）**：一种新的会话形态。webmux 会话 ID 在其整个生命周期内不变，同一时刻只在 `tui`（PTY 中的原生 TUI）或 `acp`（ACP 子进程 + 聊天视图）一端运行，切换由服务端统一协调（D1、D3、D10）。普通 PTY 会话的行为完全不变（D8）。
- **会话外壳与运行分离**：`internal/session` 把“会话身份、列表信息、关注状态”与“一次 PTY 运行”拆开。混合会话的外壳可以在没有 PTY 时存在（聊天端），也可以在同一 ID 下启动新的 PTY 运行（新运行代际）。会话还可以托管不带 PTY 的伴随进程（ACP 子进程），与 PTY 共用环境构造、进程组和清理逻辑。
- **ACP 客户端**：新增只依赖标准库的 `internal/acp`，按 ACP v1 稳定子集实现 JSON-RPC 2.0 over stdio 客户端；不声明客户端 `fs`/`terminal` 能力（D7）。私有扩展（omp 的 steering）只经 Agent 档案的能力开关使用。
- **Agent 档案**：新增内置档案（第一期 OpenCode、omp），描述可执行文件、两种可识别的命令写法（D18）、ACP 启动参数与透传参数（D19）、TUI 恢复模板与参数白名单（D15）、中断键、终端信号映射与实测版本（D30）。档案是纯数据，新增 agent 只需新增档案。
- **启动配置的“混合模式”开关**：`launch.Config` 增加可选布尔字段 `hybrid`，只允许对档案能识别的命令开启，否则返回 400（D9）。Web 端新建会话时可选择起始端（D17）；`webmux run --launch-config` 引用混合配置时按混合会话启动（D16）；`webmux run -- …` 不变（D14）。
- **切换编排**：服务端为每个混合会话维护单写者状态机（`tui`、`acp`、`switching`、`dormant`），按 issue 中场景 A/B 的步骤完成中断、退出、启动、`session/load`、重新应用配置，失败自动回滚（D21、D22），并处理 TUI 自行退出、ACP 崩溃、空闲休眠与 `auth_required`（D23、D24、D27、D32）。
- **会话身份跟踪与切换资格**：从 Warp cli-agent 事件与 OSC 7501 中额外提取 agent 身份、agent 会话 ID 与 cwd（只用于混合会话与占用检测，不改变现有关注/状态逻辑），按 D13 的五个条件实时计算 [聊天] 是否可用，并检测 `/new`、`/resume`、`/fork` 带来的 ID 变化（D11、D26）。
- **聊天流与命令接口**：新增 `/ws/s/{id}/chat` JSON 事件流（快照 + 带序号的规范化事件，可按代际与序号续接），以及挂在 `/api/sessions/{id}/` 下的 REST 命令（发送、排队、插话、取消、答复权限与表单、设置配置项、切换、恢复）；可恢复记录单独使用 `/api/hybrid-sessions`（列出、移除）。事件日志的上限复用 `scrollback_bytes`（D28）。
- **服务端队列与多设备**：每个会话一个 FIFO，多设备共享；回合被取消或出错时暂停；切到 TUI 时退回草稿（D6、D20）。
- **混合会话记录与恢复**：新增状态文件 `hybrid-sessions.json`，保存会话之间的对应关系与 argv 快照，不保存环境变量值、消息或队列（D12、D29）。重启、崩溃或主动关闭后，首页“可恢复”分组可以恢复到任一端或移除记录（D31）。
- **关注与推送**：聊天端的回合状态、权限请求与表单提问映射到现有 `attention`，沿用现有 Web Push，不新增推送通道。
- **CLI**：混合会话在聊天端或切换中时，`webmux attach` 被拒绝并给出提示；会话从终端端切走时，已附着的本地终端视为被动分离：收到提示后退出 0，与按分离键相同。新增 WebSocket 关闭码 4409（会话已移到另一端），浏览器与本机客户端都不会因此重连。`webmux ls` 显示所在端。
- **前端**：把 PR #186 的原型组件提升到 `web/src`，新增聊天事件 reducer、聊天流 hook、混合会话页面编排、首页“可恢复”分组、启动配置勾选框与起始端选择、空闲休眠设置。
- **产品边界**：“会话不跨服务重启”改为“普通 PTY 会话不跨服务重启；混合会话的对应关系可以从记录恢复，完整历史由 agent 自己保存”。

## Capabilities

### New Capabilities

- `hybrid-sessions`：混合会话的创建、起始端、所在端与单写者、双向切换流程、切换失败回滚、中断超时、TUI 自行退出、ACP 崩溃重连、空闲休眠与唤醒、关闭语义。
- `hybrid-session-recovery`：混合会话记录的内容与持久化、重启后的“可恢复”状态、恢复到任一端、配置被修改或删除后的处理、残留进程检查、移除记录、首页“可恢复”分组。
- `hybrid-agent-profiles`：内置 agent 档案，命令写法识别、混合模式可用性判定、ACP 启动参数、TUI 恢复命令生成、环境一致性与版本提示。
- `agent-session-identity`：从终端信号识别 agent 身份、当前 agent 会话 ID 与 cwd，ID 变化提示，切换资格的五个条件，跨会话占用检测。
- `acp-agent-connection`：与 ACP 子进程交互的协议边界：协商的版本与能力、使用的方法子集、stdout 纯净与行长上限、错误与取消语义、日志不记录内容。
- `chat-view`：聊天视图的事件流与续接、消息/思考/工具调用/计划/压缩/用量/提示的展示、权限与表单答复、配置项下拉、斜杠命令菜单、多设备一致、历史上限提示、`auth_required` 引导。
- `chat-prompt-queue`：发送、服务端排队、编辑/删除/上移/立即发送、暂停与继续、插话、请求去重、退回草稿。

### Modified Capabilities

- `launch-configs`：增加混合模式开关的数据模型、校验、接口字段与设置页勾选框。
- `terminal-session-cli`：`webmux run --launch-config` 对混合配置的行为；`webmux attach` 在混合会话各端的接入规则与退出码；`webmux ls` 的所在端字段。
- `agent-attention`：结构化事件的安全边界放宽为“可以提取有界的标识字段（会话 ID、agent 名、版本、cwd）供混合会话使用，正文仍不保存”；新增聊天端回合状态到关注的映射。
- `workspace-overview`：会话卡片标出混合会话所在端，聊天端以最新 agent 消息代替终端预览；新增“可恢复”分组。
- `terminal-draft-persistence`：可恢复的混合会话视为存在，其草稿不因列表快照或 4409 被删除。
- `program-status`（由在途变更 `add-program-status-signals` 新建）：“其他值不解析”增加身份标识例外，即根记录的 `app` 与子记录 `id` 的首段，只供混合会话使用。

## Impact

- **后端（新包）**：`internal/acp`（共享层，协议客户端）、`internal/agentprofile`（共享层，纯数据与纯函数）、`internal/hybrid`（Service，编排、队列、事件日志、记录存储）。均需在 `internal/archtest/layers_test.go` 中归类。
- **后端（修改）**：
  - `internal/session`：外壳/运行分离、伴随进程、ID 预留、身份信号提取、根进程 cwd 查询（Linux `/proc/<pid>/cwd`，macOS `proc_pidinfo`）、`Info.Hybrid` 覆盖层与驱动接口。
  - `internal/launch`：`hybrid` 字段与校验；`internal/workspace`：按配置把创建请求交给 `hybrid`。
  - `internal/statefile`：新增 `hybrid` 描述符；`internal/archtest/persistence_inventory.json` 与 `docs/persistence.md` 同步登记。
  - `internal/settings`：新增聊天端空闲休眠分钟数。
  - `internal/contract`、`internal/wire`、`internal/hub`、`internal/api`：新端点、新帧、新错误映射；`internal/api/testdata/wire` 新增并重录快照。
  - `internal/attach`、`internal/sessioncmd`：新的关闭码与退出码处理。
  - `cmd/webmux`：构造并注入 `hybrid`，关机顺序先于 `session.Manager`。
- **前端**：`web/src/schemas.ts`（或由它重新导出的聊天 schema 模块）、`web/src/ws.ts` 复用、新 hooks 与组件、`pages/Terminal.svelte`、`pages/SessionList.svelte`、`pages/Settings.svelte`、新建会话弹层。
- **测试**：Go 侧新增假 agent（同时模拟 TUI 信号与 ACP 服务端，以临时目录为会话库），覆盖协议、状态机、并发与故障注入；Playwright E2E 使用该假 agent，不连接真实模型；真实 OpenCode/omp 只在实施基线中用隔离 HOME 实测。
- **文档**：`docs/architecture.md`（非目标与生命周期）、`docs/protocol/`（新端点与帧）、`docs/persistence.md`、`docs/usage.md`、用户故事（新增混合会话组，修订 B、C、D09/D10、M01/M03、L10）。
- **依赖**：不新增 Go 或前端第三方依赖。
- **协作**：在途变更 `add-program-status-signals` 的任务已全部完成、代码已合入，但尚未归档。它新建的 `program-status` 规格规定 OSC 7501 除 `state`/`kind` 外不解析其他值。本变更的 `specs/program-status` 增量修改了这一条，因此须先归档 `add-program-status-signals`（tasks 0.1），本变更才能归档；`openspec validate` 目前已通过，只有归档步骤会检查目标规格是否存在。
