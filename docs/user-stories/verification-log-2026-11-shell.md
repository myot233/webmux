# 核对记录：脚本与 agent 控制（2026-11-05 起）

本文件记录 [脚本与 agent 控制](shell-control.md)（S 组）及其关联故事的局部核对，条目按日期升序。前一份记录见 [verification-log-2026-11](verification-log-2026-11.md)。

本次局部核对（2026-11-05）：实现 OpenSpec 变更 `add-agent-shell-control` 的第一部分（会话发现、会话元数据与帮助框架），新增 S 组 US-S01–US-S04，更新 US-M01、US-M05 的命令帮助与 `ls` 筛选说明及覆盖索引。

- 范围：
  - 会话环境注入 `WEBMUX_SOCKET` 与 `WEBMUX_BIN`（聊天端 ACP 进程共用同一入口），二者与 `WEBMUX_SESSION_ID` 一样保留；
  - CLI 的 `--socket`、`self`、退出码 124 常量与版本过旧提示，本机状态新增 `bin`；
  - `GET /api/sessions/{id}/details`、`PATCH /api/sessions/{id}/labels`，创建请求的 `labels`，本机套接字创建时自动记录 `parentId`；
  - `webmux get`、`webmux label`、`ls --label/--parent/--status`；
  - 命令表、`--help`、`help --json`，运维命令迁入命令表，选项可写在位置参数之后。
- 依据：
  - `go test -race` 覆盖以下包：
    - `internal/session`：标签校验与原子修改、父会话、环境注入、进程所属会话；
    - `internal/api`：本机创建记录父会话与标签、TCP 不记录、非法标签不创建、详情列出子会话，并重录 wire 快照；
    - `internal/sessioncmd`：get、label、ls 筛选、交错解析、旧服务端提示、帮助输出；
    - `cmd/webmux`：帮助写到标准输出、分组与用法错误、运维命令选项与命令表一致、套接字定位优先级；
  - `internal/archtest`、`internal/docguard`、`golangci-lint`（v2.15.0）通过；Windows 客户端交叉编译通过。
- 限制：
  - 本地 git 2.43 输出的提交时间格式使 `TestWireSnapshots` 的 `repository-commits` 快照在本机不一致，未修改该快照，以 CI 结果为准；
  - 未在 macOS 上运行；未在原生 Windows 终端中执行帮助命令；Web 端不显示父会话与标签。

本次局部核对（2026-11-05）：实现 `add-agent-shell-control` 的第二部分（llms.txt 手册与 `{{webmux}}`），新增 US-S05、US-S06，更新 US-G06、US-J12 的初始化指令说明与覆盖索引。

- 范围：
  - 新增共享包 `internal/agentguide`：`/llms.txt`、`/llms/<主题>.md`、`/llms-full.txt` 只在内存中渲染，命令参考由命令表生成；TCP 上不含实例信息，本机套接字附加版本、可执行文件与套接字；
  - `POST /api/sessions/{id}/agent-guide` 返回 `{{webmux}}` 说明；初始化指令先展开 `{{dir}}` 再插入说明，说明不可用时预检 503；
  - CLI 新增 `webmux guide`（服务未运行时输出内嵌副本）与 `webmux dir`；
  - 前端：快捷指令支持 `{{webmux}}`，预设“临时文件夹”换为“WebMux 工具”，初始化指令“可插入”新增按钮，快速开始预览说明 `{{webmux}}`，移除保留变量提示。
- 依据：
  - `go test -race` 覆盖 `internal/agentguide`（索引结构、链接、公开内容无实例信息、简述）、`internal/api`（手册路由与 Host 检查、说明接口；wire 快照新增 `session-agent-guide`）、`internal/workspace`（说明插入顺序与预检）、`internal/sessioncmd`（guide 与 dir）、`cmd/webmux`（命令参考与命令表同步）；
  - 前端单元测试 `quick-commands.test.ts`，以及受影响的浏览器与 E2E 用例（预设菜单、快捷指令、临时文件夹）。
- 限制：未在 macOS 与真机浏览器上核对；以前保存的“临时文件夹”预设不迁移。

本次局部核对（2026-11-05）：实现 `add-agent-shell-control` 的第三部分（启动命令），新增 US-S07、US-S08，更新 US-M01 与覆盖索引。

- 范围：`run` 的 worktree 位置、新建 worktree 与主页显示、混合起始端与首条消息、`--json`/`--wait`/`--dry-run`/`--label`、无终端自动分离与 `--attach`、`--follow`；`chat new` 与 `launch-config options`。
- 依据：
  - `cmd/webmux` 的端到端测试（真实服务进程）：
    - `--follow --strip` 的输出与退出码、会话中的 `WEBMUX_SOCKET`/`WEBMUX_BIN`；
    - `--wait exited` 与超时 124、自动分离、`--dry-run` 隐藏环境值、会话内创建的会话记录父会话；
    - 在 worktree 中启动与主页显示、假 agent 上的 `chat new --wait` 与 `launch-config options`；
  - `internal/sessioncmd` 单元测试：控制序列去除、状态解析、分支名生成，以及无终端时的自动分离。
- 限制：`--follow` 不做断线续接；服务端本来按启动目录关联会话，因此没有提供关闭关联的选项；未在 macOS 上运行。

本次局部核对（2026-11-05）：实现 `add-agent-shell-control` 的第四部分（会话控制与补全），新增 US-S09–US-S12，更新覆盖索引。

- 范围：`api`、`kill`、`rm`、`rename`、`send`（带回执的提交、命名按键、`--wait`）、`wait`、`logs`（`--strip`、`--tail`、`--follow`）、`screen`、`events` 与 shell 补全。
- 依据：
  - `cmd/webmux` 端到端测试（真实服务进程）覆盖上述命令，包括 `send --wait exited` 读到退出码、`wait --changed` 不被旧状态满足、`screen` 渲染全屏绘制、`events` 的首行快照、补全的动态会话 ID；
  - 在 bash 中加载生成的补全脚本并检查 `COMPREPLY`；`internal/sessioncmd` 的单元测试。
- 限制：本机没有 zsh 与 fish，只在 bash 中运行了补全脚本；`screen` 不输出颜色；未在 macOS 上运行。

本次局部核对（2026-11-05）：实现 `add-agent-shell-control` 的第五部分（提醒与聊天命令），新增 US-S13–US-S15，更新 US-C06、US-C12 与覆盖索引。

- 范围：
  - 服务端：`POST`/`DELETE /api/sessions/{id}/attention`，提醒来源 `cli` 与内存中的消息，推送正文与页面内弹窗说明行使用该消息；
  - 聊天接口：发送结果新增 `turnId`、`promptId`，`turn.start` 新增 `queueItemId`，使脚本能跟踪自己提示的回合；
  - CLI：`notify`、`chat send|watch|status|cancel|answer|switch|ls|resume|forget|set|permission|queue ...`，以及 `rm` 先结束运行中的会话。
- 依据：
  - `go test -race` 覆盖 `internal/session`（覆盖与清除、分层、输入清除、聊天端保留到 agent 新状态、已退出拒绝）、`internal/push`（消息正文与 `include_title`）、
    `internal/hybrid`、`internal/api`（wire 快照新增 `session-attention*`，更新 `chat-prompt-sent`）；
  - `cmd/webmux` 端到端测试（真实服务进程与假 ACP agent）：`notify` 与清除、`chat send --wait` 的回复、授权请求处的退出、排队与队列编辑、
    排队提示被清空时 `--wait` 以 1 退出、`chat answer`、`chat set`、`chat watch --json` 跳过历史、终端与聊天之间的切换、`chat ls`、关闭后的 `chat resume` 与 `chat forget`；
  - 前端单元测试 `attention-toast.test.ts` 与 wire 契约测试。
- 限制：`chat switch` 等待选择的分支（终端 TUI 忙）只做了代码审查，没有构造忙碌的 TUI；未在 macOS 上运行。

本次局部核对（2026-11-05）：实现 `add-agent-shell-control` 的第六部分（工作区命令），新增 US-S16–US-S18，更新覆盖索引。

- 范围：`worktree ls|show|create|rm|pin|unpin|home`、`repo ls|add|rm|show|refresh|branches|commits|base|settings`、
  `launch-config ls|show|add|edit|rm|presets|restore`、`forward ls|add|rm`，以及它们的补全。
- 依据：
  - `cmd/webmux` 端到端测试（真实服务进程）：仓库登记幂等、初始化指令的设置与引用警告、上限与互斥、worktree 的创建与主页状态（pinned、hidden、sessions）、
    `home --set`/`--reset`、`worktree show`、分支与提交、带会话时 `rm` 的阻止与 `--yes` 要求、`repo rm` 清理主页设置；
    启动配置的增删改（隐藏变量值、只改给出的字段、改名、混合模式检查）、预设恢复与端口转发；
  - `internal/sessioncmd` 单元测试：主页状态与 `web/src/domains/workspace/home-worktrees.ts` 规则对照、设置冲突后的重放与放弃。
- 限制：`repo settings --edit` 需要真实终端与编辑器，只做了代码审查；`repo base` 在没有上游的仓库中没有覆盖取回成功的路径；未在 macOS 上运行。

本次局部核对（2026-11-06）：按在途变更 `unify-session-surface` 与 `unify-session-cli` 的规格更新协议文档与故事，不新增或退役故事。

- 更新的故事：US-Q01、US-Q02、US-Q03、US-Q08、US-Q12、US-Q13、US-Q17、US-Q18、US-Q19；US-S01、US-S03、US-S04、US-S05、US-S07、US-S08、US-S09、US-S10、US-S11、US-S12、US-S13、US-S14、US-S15、US-S16、US-S17、US-S18；US-M03、US-M05。
- 依据：两个变更的 `specs/` 与工作树中 `internal/contract`（`sessions.go`、`views.go`、`chat.go`、`launch.go`）及 `internal/api` 路由的对照阅读；只运行了 `make docs-check`。
- 限制：实现仍在进行中，未运行 Go、前端或端到端测试，也未用真实服务进程冒烟；CLI 部分（`run --view`、`send --reply`、`interrupt`、`view`、`resume`、`ls --archived`、`rm --forget`、
  命令目录在运行时渲染）只按规格描述，待变更的验证任务完成后再核对。

本次局部核对（2026-11-06，终端尺寸控制）：`run --cols/--rows`（创建时的初始尺寸）、`screen --scrollback`、`POST /api/sessions/{id}/resize` 与 `webmux resize`。

- 更新的故事：US-S07、US-S09、US-S10、新增 US-S19；US-D06 只增加指向 US-S19 的说明。
- 已运行：`go test -race` 覆盖 `internal/session`、`workspace`、`hybrid`、`api`（含 `TestWireSnapshots` 与新增的认证、Origin、400/404/409 用例）、`wire`、`sessioncmd`、`agentguide`、`archtest`；
  `make check`（含 `make docs-check`、`format-check`、`golangci-lint` v2.15.0、`check:arch`）；`web/test/api.test.ts`。
- 冒烟：用本分支构建的二进制起独立实例（独立状态目录与端口），`run --cols 200 --rows 50` 后 `ls --json` 与 `screen --json` 均为 200×50；
  输出 300 行的会话上 `screen` 只有 23 行，`--scrollback 200` 取得 223 行；`send` 之后 `resize` 成功，WebSocket 连接控制时 `resize` 以 1 结束（409），`--take` 成功。
- 限制：控制端是真实浏览器或 `attach`（PTY）的路径只用 WebSocket 连接模拟，未在浏览器中运行；未在 macOS 上运行；在 WebMux 会话内运行测试时（环境中有 `WEBMUX_*`）`internal/session` 的 `TestProcessSessionFindsCreator` 与 `cmd/webmux` 的隧道测试会失败，
  取消这些变量后 `internal/session`、`workspace`、`cmd/webmux` 全部通过。

本次局部核对（2026-11-06，快速开始页快捷指令）：快速开始页 `/new` 加入快捷指令菜单，`{{dir}}`、`{{webmux}}` 经请求字段 `expandPrompt` 由服务端在会话创建后替换。

- 更新的故事：US-G02、US-G06、US-Q18、US-J12、US-S06；覆盖索引同步，故事数量不变。
- 已运行：`go test -race` 覆盖 `internal/workspace`（新增 `TestQuickStartExpandPrompt*`）、`api`、`wire`、`agentguide`、`contract`、`sessioncmd`、`archtest`；
  `web/test/browser/QuickStart.test.ts`；`pnpm --dir web check`、`lint`、`check:arch`；`make format-check docs-check`。
- 限制：按约定完整测试交给 CI；本地未运行全量 vitest、Playwright E2E 与 Storybook。

本次局部核对（2026-11-06，`webmux screen` 不再等待收稳）：回放后最多再等 1 秒，`--json` 增加 `stable`，不稳定时标准错误提示。

- 更新的故事：US-S10；索引与故事数量不变。
- 已运行：`go test -race ./internal/sessioncmd`（新增静默稳定、持续输出不稳定、会话退出稳定三个用例）；`make docs-check`；
  隔离实例中对持续重绘的程序，旧二进制 `screen` 挂到外部 8 秒超时被杀，新二进制约 1 秒返回并提示，空闲 shell 的 `stable` 为 true。
- 限制：`cmd/webmux` 的隧道用例在 WebMux 会话内运行失败，与本改动无关；完整测试交给 CI。

本次局部核对（2026-11-06，起始分支搜索与上游分支选择）：新建工作树“起始分支”与快速开始页“基准分支”支持搜索并选择本地或远程跟踪分支（`origin/*` 等），刷新上游同时拉取远程分支引用。

- 更新的故事：US-J04、US-Q18；索引与故事数量不变。
- 已运行：`go test -race ./internal/workspace ./internal/api`（新增远程跟踪分支刷新与降级用例）；
  `web/test/browser/WorktreeCreateFields.test.ts`、`NewSessionDialog.test.ts`、`QuickStart.test.ts`；`make check`。
- 限制：未在 macOS 上运行；完整 E2E 套件交给 CI。

本次局部核对（2026-11-06，聊天视图端口转发提示）：混合会话在聊天端时，`internal/session` 的端口扫描同时覆盖 ACP sidecar 进程树，前端会话页在聊天视图下连接 `/ws/sessions` 同步监听端口与转发列表并展示顶部端口提示，对应更新 US-R04 与覆盖索引，不新增故事。

- 已运行：`go test -race ./internal/session -run "Test.*ListeningPorts"`（新增 `TestShellSidecarListeningPorts`）；`TerminalHybrid.test.ts` 与 `TerminalNotices.test.ts` Vitest 浏览器测试；`make check`；隔离演示环境下桌面（1280×720）与手机（390×844）明暗主题前后对比截图。
- 限制：完整测试与 E2E 套件交由 CI 运行；未做真机核对。

本次局部核对（2026-11-06，快速开始终端启动支持附件）：快速开始终端启动支持附带上传文件，会话创建后移入上传目录并在首条消息末尾追加附件路径；更新 US-Q18，不新增故事或改变覆盖索引。

- 已运行：
  - `go test -race` 覆盖 `internal/workspace`（含 `TestQuickStartTerminalSide` 终端启动采纳附件并输入路径）、`internal/wire`、`internal/api`（`TestWireSnapshots`）；
  - `make check`（生产构建、`svelte-check`、测试类型检查、Oxlint、`check:arch`、`docs-check`、`persistence-check`、`go vet`、`golangci-lint`）；
  - `web/test/browser/QuickStart.test.ts` Vitest 浏览器用例。
- 限制：完整套件与 E2E 交给 CI；未做真机核对。

本次局部核对（2026-11-06，后台命令未结束时回合以 `end_turn` 结束的缓解 #332）：回合以 `end_turn` 结束但仍有 `in_progress` 工具调用时，服务端记录 `turn.unfinished` 并推迟 `done` 提醒至本轮新进程退出，聊天视图显示“本轮已结束，仍有任务未完成”。

- 更新的故事：US-Q05；索引与故事数量不变。
- 已运行：`go test -race` 覆盖 `internal/hybrid`（`TestWorkRunningOnlyForInProgress`、`TestTurnWithBackgroundCommandIsDoneOnlyAfterItEnds`、`TestTurnWithOpenToolWithoutNewProcessBecomesDoneAfterQuietPolls`、`TestBackgroundWatchStoppedByNewTurnOrSwitch`）、`session`、`wire`、`api`、`archtest`；
  `web/test/wire-contract.test.ts`、`web/test/browser/ChatView.test.ts`（含重连/截断后凭 `turn.unfinished` 保持提示）；`make check`；隔离演示场景在 1280×720 与 390×844、浅色与深色主题下的前后对比截图。
- 限制：使用 fake agent 替身模拟后台命令与无子进程场景，未连接真实 agy 跑完整端到端；非 Linux/macOS 平台 `ProcessTree` 返回空切片（退化为两次静默轮询后记 `done`），未在 macOS 或真机移动浏览器上核对。

本次局部核对（2026-11-06，工作台工作树批量管理）：实现 OpenSpec 变更 `batch-worktree-management`，支持工作树批量预览与删除、未锁定缺失目录 `prunable` 登记清理及侧栏仓库操作菜单。

- 更新的故事：新增 US-J14；更新 US-J03、US-J08、US-C01、US-C15、US-C17 与覆盖索引，同步 `docs/usage/worktrees.md`、`docs/usage/sessions.md`、`docs/protocol/http-workspace.md`。
- 已运行：
  - `go test -race` 覆盖以下包：
    - `internal/workspace`：新增 `batch_remove_test.go`，覆盖未锁定 `prunable` 安全清理与重建冲突拦截、归档提交保留、批量安全/强制删除、确认凭证隔离、确认后外部写入的部分失败隔离、路径并发预留；
    - `internal/api`：新增 `TestWorkspaceBatchRemovalAPIAndAuthBoundaries` 并更新 wire 快照；
    - `internal/wire`、`internal/archtest`；
  - 前端单测 `pnpm --dir web test:unit`（含 `worktree-batch.test.ts`、`wire-contract.test.ts`、`wire-arrays.test.ts`）；
  - 浏览器组件测试：`ManageWorktreesDialog.test.ts`、`WorkbenchTree.test.ts`、`WorktreeDialogs.test.ts`；
  - Playwright E2E：`worktrees.spec.ts`（批量清理空闲与 `prunable` 工作树并跳过受阻项、保留分支）、`app-shell.spec.ts`（工作台侧栏仓库操作菜单）；
  - `make check`（含 `make docs-check`、`format-check`、`golangci-lint` v2.15.0、`pnpm --dir web check:arch`）。
- 限制：未在 macOS 与真机移动浏览器上运行，全量 Playwright 分片与 Storybook 构建交给 CI。

本次局部核对（2026-11-06，测试覆盖率报告）：引入 Go race（`-coverpkg` 跨包插桩）与前端 Vitest（Node 单元 + Chromium 浏览器）覆盖率采集、合并与 CI 摘要，并补齐 `internal/agentprofile/codex`、`internal/tailscale` 及前端 `SplitSessionDialog`、`QuickCommandDialog`、`code-popout-frame`、`upload` 用例。

- 更新的故事：US-L02；覆盖索引同步，故事数量不变。
- 已运行：`make check`（含 `make docs-check`、`make persistence-check`、`format-check`、`golangci-lint` v2.15.0、`pnpm --dir web check:arch`）、`go test -race ./internal/agentprofile/codex ./internal/tailscale`、新增 Vitest 浏览器用例与 CI 全量覆盖率采集。
- 限制：未在 macOS 与真机浏览器上运行。

本次局部核对（2026-11-06，终端启动首条消息命令行参数传递）：混合会话终端端启动时支持按 agent 渐进迁移命令行初始提示参数：

- 已核查并迁移的 agent（Claude Code、Codex CLI、OpenCode、omp、Antigravity CLI）通过首次 TUI 启动参数传入首条消息；
- 未迁移的 agent（Qoder、QoderCN）、已有初始提示的启动配置、边界参数与普通终端命令回退 `SendWhenReady` 注入；
- 更新的故事：US-B01、US-Q18；索引与故事数量不变；
- 已运行：本机实测 `claude 2.1.316`、`codex 0.166.0`、`opencode 2.1.4`、`omp 18.17.0`、`agy 1.5.2` 的 `--`、`-` 前缀、多行与子命令参数行为；`go test -race` 覆盖 `internal/agentprofile/...`、`internal/hybrid/...`、`internal/workspace/...`；`make docs-check`、`make go-lint`；
- 限制：本机未安装 `qoder` 与 `qodercn`，二者保持 `InitialPrompt == nil` 走 `SendWhenReady` 旧路径，待实测后再迁移。

本次局部核对（2026-11-09，软键盘摇杆布局）：软键盘摇杆改为栏内可折叠面板，不再遮挡终端。

- 布局与开关：摇杆从终端内的绝对定位浮层迁入软键栏独立行，由栏体布局自然压缩终端区域，并随软键栏及控制状态显隐；软键栏左侧的键盘与粘贴按钮旁提供“展开摇杆／收起摇杆”开关并保存本机偏好，移除终端菜单旧入口，避免占用右侧固定操作区。
- 样式与交互：移除终端触摸淡化、悬浮穿透逻辑及按键组外层边框、背景与阴影，按键组直接沿用软键栏表面，保留按键映射、方向连发和滑动切向。

- 更新的故事：US-H01；索引与故事数量不变。
- 已运行：`make check`；`TerminalViewport`、`SoftKeys` Vitest 浏览器组件测试（10 项）；Playwright `softkeys.spec.ts`（9 项通过、9 项按平台跳过），覆盖移动端栏内几何关系、无横向溢出、控制切换、按键输入与桌面强制栏显隐联动。
- 视觉核对：真实 Storybook 组件下桌面（1280×900）、平板（768×800）、手机（390×844）明暗主题前后对比，另核对 375×667、320×568、667×375 横屏与 1280×720；摇杆均位于软键栏内、不与终端重叠且无页面横向溢出，开关完整可见；补充手机折叠状态截图及展开、收起交互 GIF。
- 限制：未在 iOS/Android 真机核对；视觉截图通过强制显示软键栏生成，未验证系统 IME 开合；未运行其余未受影响的 E2E、Storybook 与性能套件。

本次局部核对（2026-11-10，agy 与 Kimi 后台启动后的提醒）：识别 `run_in_background: true` 与 `IsDaemon: true`，即使后台启动工具调用已标为 `completed`，仍设置 `turn.unfinished` 并等待本轮新进程退出后再记 `done`，对应更新 US-Q05，不新增故事或改变覆盖索引。

- 真实 ACP 探测：Kimi Code CLI 2.5.2 两次在 prompt 返回后继续监听 20 秒，后台结果文件均已生成，但只收到 `usage_update`；追加一次后续 prompt 仍未收到后台完成通知。
- Antigravity CLI 1.5.4 使用 `Blocking=false`、`WaitMsBeforeAsync=500`、`IsDaemon=false`，在工具仍为 `in_progress`、结果文件尚未生成时正常回复并返回 `end_turn`；结果文件后来生成，但空闲等待 30 秒通知数为 0，下一次 prompt 开始时才补发工具 `completed` 更新与后台完成消息。
- 已运行：`go test -race ./internal/hybrid ./internal/acp ./internal/contract`；新增后台启动用例连续运行 5 次；`make check`（含前端检查、架构守卫、文档守卫、go vet 与 golangci-lint）。
- 真实 WebMux 端到端：以隔离状态目录和动态回环端口启动修改后的二进制，通过 REST 创建 Kimi 聊天会话；状态从 `running` 变为无提醒的 `idle`，结果文件生成后再变为带 `done` 的 `waiting`，随后关闭会话与测试服务。
- 限制：未在真实 WebMux 中重跑 agy 的完整端到端；完成时间来自受管进程树观察，不保证自行 daemonize、脱离进程树或非本地任务；未在 macOS 或真机浏览器上核对。

本次局部核对（2026-11-11，#332 后台状态自动刷新）：后台进程退出且连续两次静默轮询后，仅 agy 自动发送英文 `session/prompt`，要求检查并汇报最新状态；通过真实 ACP 更新刷新工具行与回复，检查回合结束后才记 `done`，并防止旧工具完成事件重复触发检查。对应更新 US-Q05，不新增故事或改变覆盖索引。

- 实现：由 agent 档案声明后台状态检查 prompt，只有 agy 声明；复用正常 prompt 管线，不伪造工具退出码、不读取私有 transcript，其他 agent 不额外发起模型回合。
- 已运行：`go test -race ./internal/hybrid ./internal/acp ./internal/contract`；后台进程、无进程、Kimi 启动调用完成及新回合/切换取消观察的定向回归；`make check`。
- 真实 WebMux 端到端：agy 1.5.4 产生第二个状态检查回合，Kimi 2.5.2 不发起额外回合；两种 agent 均在后台结果文件生成后出现 `done`，自动化脚本验收退出码均为 0。
- 限制：agy 自动检查会产生一次额外模型调用，结果内容由 agent 决定；完成检测仍限受管进程树，不保证自行 daemonize、脱离进程树或非本地任务；未在 macOS 或真机浏览器上核对。

## 2026-11-12：搜索已关闭的路由锚点消息（US-C18）

- 在 `/s/A` 关闭 A 窗格并保留会话与 B 后，旧版选择真实用户消息命中仍只挂载 B，没有高亮，定位 query 留下；不是原生整页导航问题。修复先复用 add 恢复缺失目标，再聚焦、显示聊天和定位。
- 永久消费者回归保留已有 split 定位和旧结果等待验收，并验证关闭路由目标后恢复、仅 A 高亮、URL 消费、B 草稿和原 body/B 身份。精确旧二进制缺少 A 而失败，新版定向 E2E 1 项通过，retries=0。
- 独立非测试 smoke 通过真实 API 创建 A/B 聊天、实际输入和发送、分屏/关闭/搜索 UI 完成；没有 seed 布局、假响应、DOM 改写或测试 runner。
- 修复后 A 与 B 同时保留，A active 且只有其真实用户消息 wm1 高亮；query 在定位后消费，B 的中文草稿与 body/pane/composer 身份保留。两版主文档请求均为零，history 事件 1→2 不是 reload。
- 1280×900、768×800、390×844 浅深色共 12 张前后图；DPR 1.25，另在 320×568、375×667 两主题核对，均无根页面横向溢出。手机显示聚焦 A，B 仍挂载但不同时可见。
- 浅色桌面：[前](../../.github/pr-assets/message-search-closed-pane/before-desktop-light.png) / [后](../../.github/pr-assets/message-search-closed-pane/after-desktop-light.png)。
- 浅色平板：[前](../../.github/pr-assets/message-search-closed-pane/before-tablet-light.png) / [后](../../.github/pr-assets/message-search-closed-pane/after-tablet-light.png)。
- 浅色手机：[前](../../.github/pr-assets/message-search-closed-pane/before-phone-light.png) / [后](../../.github/pr-assets/message-search-closed-pane/after-phone-light.png)。
- 深色桌面：[前](../../.github/pr-assets/message-search-closed-pane/before-desktop-dark.png) / [后](../../.github/pr-assets/message-search-closed-pane/after-desktop-dark.png)。
- 深色平板：[前](../../.github/pr-assets/message-search-closed-pane/before-tablet-dark.png) / [后](../../.github/pr-assets/message-search-closed-pane/after-tablet-dark.png)。
- 深色手机：[前](../../.github/pr-assets/message-search-closed-pane/before-phone-dark.png) / [后](../../.github/pr-assets/message-search-closed-pane/after-phone-dark.png)。
- [真实恢复消息录屏](../../.github/pr-assets/message-search-closed-pane/restore-message.gif)约 3.46 秒，实际浏览器帧按墙钟计时，没有叠加或调速。
- [实际状态清单](../../.github/pr-assets/message-search-closed-pane/proof.json)记录主文档请求、history 事件、目标高亮、query 与身份；未运行满八窗格的独立 smoke，也未验证真机、触控、修饰键或 hover/coarse。
- 私有服务、浏览器、build 与录制中间件均清理；没有安装共享依赖或创建 node_modules 链接，公开资源由提交者后验。

## 2026-11-12：引用已关闭的路由锚点窗格（US-Q05）

- 旧版真实界面先关闭路由锚点窗格并保留会话，再点击引用；实际发生整页导航，保存的布局仍不含目标窗格。修复通过原分屏 add 路径重新打开，不扩大既有 activateSession 的聚焦语义。
- 永久消费者回归覆盖已有窗格激活、关闭后目标身份与数量、当前窗格、其他聊天与 document 身份、未发送草稿保留；定向 E2E 1 项、retries=0，AppShell/TimelineItem browser 20 项通过。
- 生产类型 418 文件、0 错误与警告，lint、架构守卫及 40 项测试、源码格式通过；没有将此记为全仓 E2E、真机或原生修饰键验证。
- 独立 fake provider 服务在桌面 1280×900、平板 768×800、手机 390×844 的浅深色中核对。旧版发生原生导航且未恢复目标，新版六组均无原生导航；窄屏隐藏聊天可正常卸载，切回后草稿仍在。
- 截图准备使用真实保存布局；永久回归通过真实分屏与引用菜单创建场景，没有改 DOM 链接结果。前后为独立私有服务中的等价命名会话，不宣称相同 ID。
- [证据清单](../../.github/pr-assets/session-reference-closed-pane/evidence.json)记录导航、身份、草稿与溢出。
- 浅色桌面：[前](../../.github/pr-assets/session-reference-closed-pane/before-desktop-light.png) / [后](../../.github/pr-assets/session-reference-closed-pane/after-desktop-light.png)。
- 浅色平板：[前](../../.github/pr-assets/session-reference-closed-pane/before-tablet-light.png) / [后](../../.github/pr-assets/session-reference-closed-pane/after-tablet-light.png)。
- 浅色手机：[前](../../.github/pr-assets/session-reference-closed-pane/before-phone-light.png) / [后](../../.github/pr-assets/session-reference-closed-pane/after-phone-light.png)。
- 深色桌面：[前](../../.github/pr-assets/session-reference-closed-pane/before-desktop-dark.png) / [后](../../.github/pr-assets/session-reference-closed-pane/after-desktop-dark.png)。
- 深色平板：[前](../../.github/pr-assets/session-reference-closed-pane/before-tablet-dark.png) / [后](../../.github/pr-assets/session-reference-closed-pane/after-tablet-dark.png)。
- 深色手机：[前](../../.github/pr-assets/session-reference-closed-pane/before-phone-dark.png) / [后](../../.github/pr-assets/session-reference-closed-pane/after-phone-dark.png)。
- [正常速度录屏](../../.github/pr-assets/session-reference-closed-pane/reopen-desktop-light.gif)来自真实 Chromium，1280×900 录制、960×675 输出、10 fps；没有真机、真实触控或原生修饰键注入核对。所有私有进程、浏览器和录屏中间件清理；公开链接由提交者后验。

## 2026-11-12：满布局的引用反馈与重试（US-Q05）

- 精确基线 `47018c05` 先关闭路由锚点、保留会话，再加入其他会话填满 8 个窗格；实际点击引用被消费，目标仍未出现、没有提示，也没有原生导航。没有把此前“原生导航后未重开”的旧故障当作本例。
- 修复在原页面的 danger InlineNotice 报告容量；同一次点击保留八个窗格、原 document/chat/composer 身份和未发送草稿。关闭一个 filler 变为七个后重试，目标恢复为 active，总数仍为八，提示清除且草稿不变，导航始终为零。
- 永久回归用真实分屏菜单、关闭菜单和引用点击完成上述边界；精确旧二进制缺少 alert 而失败，新版容量回归与已有引用回归共 2 项通过，retries=0。
- 独立非测试 smoke 使用真实 API 创建私有 fake-agent 会话，真实输入并发送相对会话链接；加入和关闭窗格均通过 UI。仅在截图前重排持久布局为可读的 chat/七个 PTY 排布，没有改 DOM、会话内容或反馈。
- 桌面 1280×900、平板 768×800、手机 390×844，浅深色共 12 张前后图；DPR 1.25，均无根页面横向溢出，另测 320×568、375×667。手机显示响应式聚焦窗格与标签条，不宣称八个内容同时可见，JSON 保存八个实际 ID。
- 浅色桌面：[前](../../.github/pr-assets/session-reference-capacity/before-desktop-light.png) / [后](../../.github/pr-assets/session-reference-capacity/after-desktop-light.png)。
- 浅色平板：[前](../../.github/pr-assets/session-reference-capacity/before-tablet-light.png) / [后](../../.github/pr-assets/session-reference-capacity/after-tablet-light.png)。
- 浅色手机：[前](../../.github/pr-assets/session-reference-capacity/before-phone-light.png) / [后](../../.github/pr-assets/session-reference-capacity/after-phone-light.png)。
- 深色桌面：[前](../../.github/pr-assets/session-reference-capacity/before-desktop-dark.png) / [后](../../.github/pr-assets/session-reference-capacity/after-desktop-dark.png)。
- 深色平板：[前](../../.github/pr-assets/session-reference-capacity/before-tablet-dark.png) / [后](../../.github/pr-assets/session-reference-capacity/after-tablet-dark.png)。
- 深色手机：[前](../../.github/pr-assets/session-reference-capacity/before-phone-dark.png) / [后](../../.github/pr-assets/session-reference-capacity/after-phone-dark.png)。
- [正常速度容量提示与恢复录屏](../../.github/pr-assets/session-reference-capacity/capacity-retry.gif)约 4.8 秒，来自实际浏览器帧及墙钟时长，没有叠加或调速。
- [实际状态清单](../../.github/pr-assets/session-reference-capacity/proof.json)记录消费事件、身份、导航、草稿和重试。
- 全部私有服务、浏览器与录制中间件清理；没有真机、真实触控、原生修饰键或 hover/coarse 验证。公开链接由提交者后验。

## 2026-11-12：回合 Diff 合并最终交接核心（US-Q05）

- 保留同文件独立片段、共享 Diff 阅读器与增删统计；修改前、修改后及整文件删除的查找来源不变。
- 定向回归：TurnChanges、CodeDiffReader、TerminalRendering 共 15 项浏览器测试，以及 chat-model、code-diff 共 20 项单元测试通过。
- 实际私有应用：在独立 HOME、XDG 配置、状态目录与 FAKEAGENT_STORE 的真实后端运行 3 项 Chromium 桌面端到端场景；仅使用 fake agent，不连接真实供应商。
- 已核对同文件两段 Diff 的展开收起、旧侧与新侧各 1 个匹配、删除文件默认旧侧查找，以及聊天 Diff 内 Cmd+F 与切回终端后的 Cmd+F 焦点归属。
- 此次 macOS 快捷键通过 Chromium 模拟平台与按键核对，未声称在 macOS 真机验证；尾随换行的既有 diffStats 计数行为不在本次范围内。

## 2026-11-12：空文件快照的回合 Diff 净改动（US-Q05）

- 已知修改前快照存在时，将 wire 省略的空 `newText` 归一为空字符串再拼接；同文件先清空再改写保留净增删，清空后恢复保持零增删。
- 未知、仅补丁与不能连续拼接的片段继续独立保留；不改变整文件删除、修改前后查找、快捷键归属或尾随换行统计策略。
- `make check` 通过；turn-changes、chat-model、code-diff 共 28 项单元测试及 TurnChanges、CodeDiffReader、TerminalRendering 共 15 项浏览器测试通过。
- 定向 Chromium E2E 在桌面与 iPhone SE 布局共 9 项通过，覆盖清空后改写/恢复、独立片段、删除文件默认旧侧查找与旧侧/新侧切换；1 项手机 macOS 快捷键场景按原规则跳过，桌面快捷键场景通过。
- 独立实际私有应用 smoke 使用绝对路径 fake agent 与隔离 HOME/XDG/FAKEAGENT_STORE，捕获真实后端省略空 `newText` 的帧，再交给真实 reducer：改写为 1/1，恢复为 0/0，页面均只展开一个净 Diff。
- 私有应用在 1280×720、390×844、375×667、320×568 的浅色/深色媒体设置下无文档横向溢出；未连接真实供应商，未核对真机、真实 macOS、悬停或触屏交互。

## 2026-11-12：新建文件后编辑的净 Diff（US-Q05）

- ACP 新文件的 `oldText` 允许省略或 `null`；Go 使用 nil 指针并在聊天 wire 中省略该字段。前端不增加 nullable JSON 分支，只在有完整快照且没有补丁正文或截断标记时归一不存在的一侧。
- `keep\noriginal` 新建后改成 `keep\nreplacement`，回合摘要保留一个文件、一个最终 Diff，统计为新增 2 行、删除 0 行，而不是累计中间版本的 3/1。
- 未知、仅补丁、截断与不连续片段继续独立；清空再改写的 1/1、清空再恢复的 0/0、尾随换行策略与查找快捷键归属不变。
- 前端生产构建、生产/测试类型检查、Oxlint、架构守卫和源码格式检查通过；turn-changes、chat-model、code-diff 共 31 项单元测试及 TurnChanges、CodeDiffReader、TerminalRendering 共 15 项浏览器测试通过。
- 独立私有生产应用使用绝对路径 fake omp 与隔离 HOME/XDG/FAKEAGENT_STORE；真实 Go WebSocket 新建帧省略 `oldText`，后续编辑帧带完整旧内容。捕获帧交给实际生产 reducer 得到一个 2/0 快照，页面也只展开一个包含 `keep` 与 `replacement` 的 Diff，没有旧版本 `original`。
- 定向 E2E 桌面与 iPhone SE 共 11 项通过，1 项手机 macOS 快捷键场景按既有规则跳过；覆盖新建后编辑、清空后改写/恢复、独立片段、删除文件旧侧查找，以及桌面 Cmd+F 从聊天切回终端的焦点归属。
- 同一真实后端与会话对比 87ea0df 的 reducer 生产 bundle 和修复后 bundle：前者两段 3/1，后者一段 2/0。桌面 1280×900、平板 768×800、手机 390×844 均有浅色/深色前后截图，另核对 375×667、320×568 无文档横向溢出；实际 DPR 为 1.25。
- 证据位于 `.github/pr-assets/turn-diff-review/create-edit-*`：12 张前后截图、4 秒正常速度的展开/收起/再展开 GIF，以及真实 wire、生产 reducer 和各视口 displayedDiff 的 JSON。已检查截图与 GIF 帧序列；公开 PR 内嵌展示由发布者核对。
- 限制：无真实供应商调用，未验证真机、真实 macOS、悬停或触屏；无新 UI 样式或布局修改。故事编号、数量与覆盖索引不变。

## 2026-11-13：迟到后台工具不计入下一轮 Diff（US-Q05）

- 实际复现第一轮 `in_progress` 工具在第二轮开始后发回完成 Diff；修复前第二轮错误显示一个文件的汇总。前端保留工具生命周期的回合归属，迟到补丁只更新原卡片；已结束汇总不追补，同一 ID 明确重新进入 pending/in_progress 时归入新轮。
- 永久回归覆盖迟到完成仍更新原卡片、不污染新轮，以及终态工具 ID 明确重启后正常记录新轮。turn-changes 与 chat-model 共 27 项单元测试通过；原实现的迟到用例先失败，修复后通过。
- `make check` 通过：生产及测试类型检查 419 文件无错误或警告，40 项架构与依赖测试、文档及持久化守卫、Oxlint、go vet 与 golangci-lint 全部通过。定向真实 ACP E2E 桌面与手机共 11 项通过，1 项手机 macOS 快捷键用例按原规则跳过，无重试。
- 两个隔离生产二进制均使用绝对路径、校验 SHA256 的仓库 fake ACP。真实帧在第二轮开始后更新原工具，持久 Activity 仍为第一轮 position 6；修复前页面错误汇总为 1，修复后及真实重连回放均为 0，原工具卡片仍显示 late.ts 与 +1/−1。
- `.github/pr-assets/late-tool-diff/` 保留真实帧、Activity、读屏文本及视口 JSON，桌面 1280×900、平板 768×800、手机 390×844 的明暗主题共 12 张前后截图，以及原工具卡片展开/收起的正常速度 GIF。另核对 375×667、320×568，两种主题均无文档横向溢出。
- 截图实际 DPR 1，最初操作 DPR 1.25；GIF 由实际录屏转码。未声称工具卡片直接展示 Diff 正文，正文来自真实 wire 与 Activity；既有 fake 版本提示未隐藏。
- 限制：仅仓库 fake ACP，无真实供应商调用；未验证真机、真实 macOS 或悬停/触屏。一次录帧脚本未解包 events 信封、一次 limit=100 读取被拒绝，相关不完整录屏不作证据；最终使用真实服务端回放和有效默认分页。故事编号、数量与覆盖索引不变。
