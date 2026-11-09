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
  - `internal/sessioncmd` 单元测试：主页状态与 `web/src/home-worktrees.ts` 规则对照、设置冲突后的重放与放弃。
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
