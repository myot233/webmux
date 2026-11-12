# 核对记录：归档历史与侧栏列表（2026-11-06 起）

本文件记录已归档会话的保留策略、聊天侧栏列表与相关界面故事的局部核对，条目按日期升序。前一份记录见 [verification-log-2026-11](verification-log-2026-11.md)。

本次局部核对（2026-11-06）：聊天模式侧栏的“已归档”分组改为按需挂载与分批渲染，更新 US-Q19，不新增故事或改变覆盖索引。

- 范围：
  - 折叠状态下不再把归档行放进页面，只保留分组标题与数量；
  - 展开后先渲染最近 20 条，底部“显示更多（还有 N 条）”每次再追加 20 条；
  - 收起后丢弃已挂载的行，再次展开从首批开始；
  - 当前打开或正在恢复的归档记录仍会自动展开分组，行为与原来一致。
- 依据：
  - `ChatHistory` 浏览器组件测试新增用例：折叠时列表为空、展开首批 20 条、点击“显示更多”后达到 25 条、再次收起后列表清空；
  - 分页只影响渲染窗口，排序与可访问名沿用 `chatHistorySections`，行内菜单与恢复流程未改动；
  - 原有聊天历史排序、自动展开、释放空间与恢复用例仍通过（`test/browser/ChatHistory.test.ts` 共 6 项）；
  - `svelte-check`、Oxlint、`pnpm check:arch` 与 Oxfmt 格式检查通过。
- 限制：
  - 未运行完整 `make test` 与 E2E 套件；
  - 未启动真实服务或 Storybook 截取明暗主题、桌面与手机截图；
  - 未在真机上核对滚动与触控体验；未重新核验其余故事能力。

本次局部核对（2026-11-06）：混合会话记录达到 200 条时自动淘汰最旧的归档记录，更新 US-Q12，不新增故事或改变覆盖索引。

- 范围：
  - 记录已满时，`records.add` 在同一次原子写入中删除最近未更新的已关闭或已中断记录，再加入新记录；
  - 仍在运行的记录不参与淘汰；全部为运行中时创建仍以 409 `limit` 拒绝，错误说明改为“没有可淘汰的已归档记录”；
  - 淘汰后清理该记录遗留的 actor 与保留最后输出的 shell，清理失败只记日志，不阻塞新会话；
  - 服务端通过 `OnEvicted` 回调一并删除该记录的临时文件夹，并在没有其它记录共享时删除释放空间的私有引用，与“删除记录”流程一致；
  - 新会话启动失败时其记录已被删除，并恢复刚淘汰的记录，避免白丢历史。
- 依据：
  - `internal/hybrid` 新增记录层用例覆盖“淘汰最旧归档”“只保留运行中时拒绝”“优先淘汰更早更新且不淘汰运行中或正在恢复的记录”与“启动失败后恢复”；
  - 服务层用例覆盖满额创建成功、最旧记录消失、新记录存在、总数仍为 200 且写入淘汰日志；
  - 恢复用例覆盖淘汰已中断会话时其 shell 一并结束，以及删除记录后仍能新建；
  - `internal/api` 用例覆盖满额时创建返回 201 并淘汰最小 ID 的旧记录，以及删除记录后再次创建；
  - `go test -race ./internal/hybrid` 与 `go test ./internal/api` 通过。
- 限制：
  - 未运行完整 `make test` 或其它包的 Go 测试；
  - 未在真实多会话、长时间运行的服务上观察淘汰与清理；
  - 未重新核验其余故事能力。

本次局部核对（2026-11-06）：审查各 JSON 存储的容量边界，补齐条数与字节上限，更新 US-Q12、US-B08、US-J01，不新增故事。

- 变更：
  - 混合会话记录按字节淘汰：长命令先于 200 条写满 1 MiB 文件时，淘汰最旧归档记录（可能多条）直到容纳新记录，没有可淘汰的记录时 409 `limit`，不再让存储不可用；
  - agent 选项缓存每个配置最多 32 个选项、约 16 KiB 选项文本，名称与描述截断，保留当前选中项，文件超 1 MiB 时丢弃最旧配置；
  - 延迟合并写入失败写错误日志，I/O 类失败每 30 秒重试；
  - 启动配置总大小超 4 MiB 返回 400 而非存储故障；仓库登记最多 500 个。
- 依据：`internal/hybrid`（`bounds_test.go`）、`internal/launch`、`internal/workspace`（`limit_test.go`）新增用例，相关包 `go test -race` 通过。
- 限制：未运行完整 `make test`/`make check`；未在真实磁盘写满环境观察重试。

本次局部核对（2026-11-07）：已归档聊天的删除拆成“删除记录”和“删除记录和工作树”两个菜单项；更新 US-C15、US-C17、US-Q12 与覆盖索引，不新增故事。

- 行为：
  - “删除记录”始终只删除记录与本机草稿，不删除、释放或改动工作树；
  - 关联的 linked worktree 可删除且未释放时，工作台首页已归档区和聊天历史侧栏另提供“删除记录和工作树”；打开删除 worktree 确认流程并列出将删除的聊天，成功删除工作树后再删除该记录，取消则两者都不改变；
  - 已释放、主工作区、直接目录或工作树不可用时只提供“删除记录”。
- 依据：
  - `pnpm --dir web check:test`（0 errors）、`test/archived.test.ts` 单元用例（4 项）通过；
  - `AppShell`、`ArchivedSessions`、`ChatHistory`、`WorktreeDialogs`、`SessionListArchived`、`WorkbenchTree` 浏览器组件测试（65 项）通过；
  - Playwright `quick-start.spec.ts` 桌面（2 通过、1 跳过）与 Pixel 7（3 通过）通过；
  - `make docs-check`、`openspec validate --all --strict --no-interactive`（本次涉及的 spec 与在途变更通过）、`git diff --check` 通过。
- 限制：未运行完整 `make test`/`make check`；未在隔离演示服务上重新截取明暗主题与桌面/手机截图；未重新核验其余故事能力。

本次局部核对（2026-11-07）：新增基于 `STDIN/STDOUT` 子进程的可插拔语音识别与聊天/终端双模式录音按钮，新增 US-G07 并更新覆盖索引。

- 范围：
  - `config.toml` 支持 `[voice]`（`cmd` 参数数组与 `env` 环境变量列表），`GET /api/config` 返回 `voice` 布尔值且不暴露命令与密钥；
  - `GET /ws/voice` 接收浏览器 `16000 Hz` 单声道 `s16le` PCM 二进制帧与 `{"type":"stop"}`，驱动 `voice.cmd` 子进程并解析 `ready`、`partial`、`final`、`error` 事件及纯文本回退；
  - 聊天模式（消息输入框、快速开始页 `/new`）与终端模式（软键快捷栏、桌面顶栏快捷区）提供四阶段切换式语音按钮（空闲、连接中、录音中实时音量、收尾中），识别文本插入当前输入目标且不自动回车。
- 依据：
  - `internal/config`、`internal/voice`、`internal/api` 新增单元测试并更新 `testdata/wire/config.json` 快照；
  - 前端新增 `web/test/voice.test.ts` 与 `web/test/browser/VoiceButton.test.ts`，覆盖重采样、PCM 编码、音量计算及四阶段点击录音插入流程。
- 限制：
  - 自动化测试使用模拟子进程与模拟 WebSocket，未在 CI 中连接外部云端 ASR 服务。

本次局部核对（2026-11-07）：终端与代码区内置 `Maple Mono NF CN` 中文 `unicode-range` 字体切片，更新 US-D08 与覆盖索引，不新增故事。

- 范围：
  - `web/scripts/build-fonts.py` 在去重 Regular/Bold 共享无 hinting 图标（`maple-icons-*.woff2`）的基础上，使用 `cn-font-split` 将 `Maple Mono NF CN`（400/700）的 CJK 与其余字形切分为按需加载的 WOFF2 分片，写入 `web/src/fonts-terminal.css`；
  - `web/src/hooks/terminal/terminal-renderer.ts` 与 `web/src/components/code/CodeWorkspace.svelte` 按需导入 `fonts-terminal.css`，使终端与代码区（含 `/code/<id>` 独立窗口和混合会话代码面板）均使用 `Maple Mono NF CN` 渲染中文并保持 2:1 等宽比；
  - 首页冷启动不预拉取 CJK 分片，UI 正文（`--font-sans`）中文继续使用系统无衬线字体。
- 依据：
  - `web/test/browser/TerminalRenderer.test.ts` 新增 `Maple Mono NF CN` 400/700 字重下 `"中文"` 与 `"WWWW"` 宽度一致（2:1 等宽）的真实浏览器测量断言；
  - `make check`、`pnpm --dir web check:arch` 与 `go test ./web ./internal/api` 静态资源嵌入校验通过。
- 限制：
  - 未重新运行完整 E2E 与性能套件；未重新核验其余故事能力。

本次局部核对（2026-11-07）：窄屏（小于 640px）下快速开始页 `/new` 与聊天视图在未聚焦且无待发内容时收起为单行紧凑输入条，对应更新 US-Q18 与覆盖索引，不新增故事。

- 范围：
  - `web/src/hooks/shared/composer-compact.svelte.ts` 统一跟踪输入框容器焦点及从输入框打开的 `Popover` / `DropdownMenu` 浮层状态；
  - 窄屏空闲（未聚焦且无文字、待发送附件、上传进度与发送中状态）时，`ChatComposer` 与 `QuickStart` 隐藏位置行、初始化指令勾选栏及底部工具栏辅助按钮，输入区收为单行高度并与发送（及回合进行中的停止）按钮同排；
  - 点击或聚焦输入框展开完整卡片，有草稿文字或附件时失焦保持展开，清空后失焦自动恢复紧凑；`/new` 在窄屏或触屏设备上不再挂载时自动聚焦；桌面端（`>= 640px`）布局不变。
- 依据：
  - `ChatView` 与 `QuickStart` 浏览器组件测试（38 项，含窄屏初始紧凑态、点击展开、有草稿失焦保持展开、清空失焦收起）通过；
  - Playwright `quick-start.spec.ts` 桌面与 Pixel 7 E2E 通过；`make check` 与 `pnpm --dir web check:arch` 通过；
  - 隔离演示数据下 390×844 手机与 1280×720 桌面、浅色与深色主题前后对比截图。
- 限制：
  - 未在 iOS/Android 真机软键盘上核对；未重新核验其余故事能力。

本次局部核对（2026-11-10）：新增 Kimi Code CLI 混合会话与进程级文件夹信任，更新 US-B01、US-B08、US-B09、US-Q01、US-Q03、US-Q04、US-Q07、US-Q08，不新增或重编号故事。

- 范围：
  - 新 `kimi` 档案支持 `kimi acp`、TUI 恢复 `kimi --session <id>`、OSC 7501 `app=kimi-code`、原生根进程回退与 `session/list` 候选选择；
  - 新预置使用 `kimi --auto` 且默认混合，已有启动配置不改写；首次从 argv 推导 model/mode，冷恢复只重应用 mode，model/thinking 由 Kimi 保留；
  - `agent_pretrust` 为首次启动注入 `KIMI_CODE_TRUST_WORKSPACE=1`，不写持久信任配置，恢复 TUI 与 ACP 继承同一环境；显式空值、falsy 值和禁用开关优先；
  - 真实兼容性处理：浏览器草稿和 API 向混合或普通 Kimi TUI 提交文本与 Enter 时都在同一回执内分隔 200ms，空正文不等待，延迟期间保持输入锁；Kimi 的 `Already in <mode> mode` 在目标一致时按幂等成功处理，覆盖冷恢复与手动设置，其他错误不吞掉。
- 自动化依据：
  - `internal/agentprofile/kimi` 契约与参数用例、agents 注册/argv 测试、`internal/agenttrust` 直接/包装/显式环境/过宽目录用例通过；
  - `internal/session` 真实启动环境、浏览器草稿与 API 提交延迟、空 Enter 和重试用例通过；`internal/hybrid` 完整 race 套件通过，含普通 TerminalInput 延迟、Kimi 双向往返、候选歧义、崩溃 resume、手动幂等及 PreLaunch 注入到 ACP 的 trust 环境；
  - API/wire 快照重录并通过；Playwright 的 Kimi 预置与 fakeagent 双向往返在桌面和 iPhone SE 通过；
  - `make check`、`make docs-check` 与 `openspec validate add-kimi-hybrid-profile --strict` 通过。
- 真实 Kimi 2.5.2 依据：
  - 独立 ACP 的 initialize/new/set_config_option/prompt/list/resume/load/close/cancel 全部成功，resume 无回放、load 有回放，model/thinking 保留，测试会话已删除；
  - 隔离 WebMux 中 pre-launch trust 无启动阻塞，TUI 首条消息进入 `session/list`，TUI→ACP→TUI→ACP 成功，取消返回 cancelled，恢复 ID 不变，最终 mode=plan、thinking=low、模型保留，Kimi `config.toml` mtime 未变；
  - 隔离 `KIMI_CODE_HOME` 对照中无 trust 变量出现 `blocked/permission`，设置后进入 `idle`。
- 视觉依据：基线与当前实现的 Kimi 预置在 1280×900、768×800、390×844 浅色页面均有前后截图，卡片与编辑对话框无横向溢出；未验证真机、触屏常驻与 hover 行为。
- 完整门禁：`make test` 全量通过，覆盖 Go race、Vitest、Storybook、桌面与手机 E2E 及性能测试；性能套件 18 项通过、2 项按既有条件跳过。测试生成的 Playwright 临时报告不纳入交付。

本次局部核对（2026-11-10，快速开始支持选择已有工作树）：快速开始页 `/new` 的分支选择器弹出面板顶部新增“新建 worktree | 已有工作树”分段切换，支持不新建 worktree 而直接在已有工作树中启动会话，对应更新 US-Q18 与覆盖索引，不新增故事。

- 范围：
  - 弹出面板顶部提供 `SegmentedControl`（`新建 worktree` 与 `已有工作树`），选择“已有工作树”时下方展示可搜索的工作树列表（显示工作树名称与路径，不可用的禁用并显示原因），触发器图标与文字切换为 `工作树：<名称>`；
  - 发送时构造 `workspace: { repositoryId, worktreeId }` 而非 `newWorktree`，直接在该工作树中启动；
  - 本设备草稿与上次成功选择（`quickStart`）记住工作树方式及所选工作树，所选工作树被移除或不可用时回退到仓库主工作区。
- 依据：
  - `QuickStart.test.ts` 浏览器测试（21 项，含切换到已有工作树、搜索工作树、直接启动、记住选择及工作树移除后回退到主工作区）通过；
  - Playwright `quick-start.spec.ts` 端到端测试（桌面与手机共 7 项通过、1 项按项目跳过）通过，覆盖选择已有工作树直接启动且不新增 worktree；
  - `make check`、`make docs-check` 与 `pnpm --dir web check:arch` 通过。
- 限制：
  - 未在真机上验证触屏与软键盘交互，无头浏览器未验证 hover 与 `any-pointer: coarse` 触屏常驻；未重新核验其余故事能力。

本次局部核对（2026-11-10）：启动配置新增“完成与介入提醒”设置（提醒范围、四层信号源独立开关与优先级说明、最短回合耗时、完成确认延迟）与 Warp 子代理 `session_id` 完成信号过滤，更新 US-B08、US-C06、US-Q09 与覆盖索引，不新增故事。

- 范围：
  - `internal/launch`、`internal/contract`、`internal/wire` 与 `web/src/contracts/schemas.ts` 新增可选 `attention` 配置，`null` 或省略时跟随服务端全局默认：
    - `mode` 为 `all`/`urgent`/`off`；
    - `programStatus`、`agentProtocol`、`notify`、`bell` 为布尔开关；
    - `minTurnSeconds` 为 `0..300`，`doneDelaySeconds` 为 `0..60`；
  - `internal/session` 按会话应用 `AttentionPolicy`：
    - 支持独立开关四类信号源并在关闭高优先级源时自动回退下一层；
    - 支持 `urgent`（仅保留提问、授权、错误）与 `off`（关闭自动提醒但保留 `webmux notify`），并对非紧急完成信号执行最短回合耗时与完成确认延迟防抖；
    - 按主会话 `session_id` 过滤 Warp 子代理的 `stop`/`idle_prompt`/`stop_failure`/`subagent_start`；
  - `internal/hybrid` 与 `internal/workspace` 将启动配置的提醒策略传入普通终端会话与混合会话（终端端与聊天端同步遵循提醒范围、最短回合耗时与完成确认延迟）；
  - `LaunchConfigDialog.svelte` 新增“完成与介入提醒”分组，提供“跟随全局默认”复选框、提醒范围 `SegmentedControl`、带四层优先级说明的信号源复选框与防误报阈值输入框。
- 依据：
  - `internal/launch`、`internal/session`、`internal/hybrid`、`internal/workspace`、`internal/wire`、`internal/api` 新增单元测试与 wire 快照验证通过；
  - `web/test/launch-configs.test.ts`、`svelte-check`、Oxlint、`pnpm --dir web check:arch` 与 `make docs-check` 通过。
- 限制：
  - 未在 iOS/Android 真机上核对触控体验，无头浏览器未验证 hover 与 `any-pointer: coarse`；未重新核验其余故事能力。

本次局部核对（2026-11-11）：新增全仓生产源码文件 `< 500` 行架构守卫并完成存量超限文件按职责拆分，更新 US-L15 与覆盖索引，不新增故事。

- 范围：
  - `internal/archtest/filesize_test.go` 新增 `TestSourceFileSize` 与 `TestSourceFileSizeViolationMessage`，限制 `cmd/` 与 `internal/` 下所有非测试生产 `.go` 文件少于 500 行（`<= 499` 行），超限时输出按职责拆分 `<family>_<part>.go` 的重构指令；
  - `web/test/architecture.test.ts` 新增 `production source files stay under 500 lines`，限制 `web/src/` 下非测试、非 stories 且非 `src/lib/components/ui/` 生成组件的 `.ts` 与 `.svelte` 生产文件少于 500 行，超限时输出按领域模块、子组件与分层边界拆分的指引；
  - 全仓 25 个 Go 生产文件与 32 个前端生产文件按职责边界完成拆分，保持原有对外 API、组件行为与持久化清单一致；
  - 合入 #497、#499 与 handoff 身份存储后，`internal/hybrid` 的 `service.go`、`recovery.go`、`link.go` 再次超限，按职责拆出 `create.go`（会话创建与准入）、`record_removal.go`（记录删除与 actor 退役）与 `config_reapply.go`（配置重应用）。
- 依据：
  - `make check`（含 `svelte-check`、`oxfmt --check`、`make docs-check`、`make persistence-check`、`oxlint`、`pnpm --dir web check:arch`、`go vet`、`golangci-lint`）全部通过；
  - `go test -race ./...` 与前端 Vitest 单元及浏览器组件测试通过。
  - 合并后在本地重跑 `make check` 与 `go test -race ./...` 通过。
- 限制：
  - 本次为纯工程守卫与内部模块拆分，不改变用户可见行为；未重新运行完整 Playwright E2E 与性能测试套件。

本次局部核对（2026-11-11，v0.14.5 发布前）：在 Linux/WSL2 本机以隔离 WebMux 实例运行真实 Kimi 2.5.2 与 agy 1.5.4，覆盖 #497、#499、#503，涉及 US-Q04、US-Q05、US-Q12 与 US-L15。

- Kimi：ACP 启动后台 Bash 后切到 TUI，当前会话绑定变为 `local`，原有其他事件保持不变且首次备份存在；TUI 真实执行 Bash 并生成、读取 `KIMI_TUI_REAL_OK`。
- 归档恢复：关闭会话、重启隔离服务后恢复到终端，再切回聊天，WebMux 与 Kimi 会话 ID 均保持不变，首次绑定备份未被覆盖。
- agy：真实后台命令退出后自动补发一次状态检查，旧工具更新为 `completed`，输出文件内容正确，清除 `unfinished` 并产生 `done`；没有用户补发消息，也没有重复刷新回合。
- 发现并修复：Kimi 后台退出只产生 `done`，未清除 `turn.unfinished`。直接完成路径现在同步清除此标记并发布状态；在已有 Kimi 后台测试中补充回归断言。
- 修复后真实复验：Kimi 与 agy 均先出现 `unfinished`，实际输出文件分别为 `KIMI_FIXED_OK` 与 `AGY_FIXED_OK`，最终页面显示“已完成”；Kimi 只有用户回合，agy 只有用户回合与一次自动检查。
- 依据：
  - `make build VERSION=0.14.5`、`go test ./internal/archtest -run TestSourceFileSize`、`pnpm --dir web check:arch` 通过；
  - Go race 定向覆盖后台启动、进行中的工具、无新进程与新回合/切换取消观察四条路径，通过；
  - Chromium 查看本地真实实例；Kimi 完成状态覆盖桌面、平板、手机视口，手机 390×844 无页面横向溢出。
- 限制：仅验证当前 Linux/WSL2 与已安装 agent 版本；未验证 macOS、真实移动设备、外部推送投递或脱离受管进程树的 daemon。

本次局部核对（2026-11-11）：归档恢复沿用最后的聊天或终端模式，不受导航侧栏模式影响；更新 US-Q10、US-Q12、US-Q19 与 US-D09，不新增故事。

- 范围：首页列表与会话恢复页合并为一个“恢复”入口；侧栏不再强制恢复到聊天；工作树重建与缺失配置等确认仍保留原模式。
- 浏览器组件验证：ArchivedSessions、AppShell、ChatHistory、WorkbenchTree 共 57 项通过；前端类型检查、测试类型检查、Oxlint、架构守卫与源码格式检查通过。
- 隔离实例 E2E：14 项定向场景通过，覆盖切换模式后归档、聊天/工作台导航、两种恢复视图及释放后重建；2 项工作台树手机场景沿用桌面专用跳过。
- 视觉验证：Storybook 在 1280×720、768×1024、390×844、375×667、320×568 的明暗主题下无页面横向溢出，长标题保留截断，恢复后键盘 Tab 可进入更多操作。
- 交互证据：隔离实例运行 fake agent，聊天侧栏直接恢复终端模式并保留对话，录制实际操作；未使用开发者的现有服务。
- 文档治理：原混合故事文件已超过体积上限，把 US-Q18 原文迁至 `hybrid-sessions/quick-start.md`，原入口保留链接及共用实现依据，故事 ID 与计数不变。
- 限制：未验证真实移动设备、悬停显示或真机触屏常驻行为；未重新核对其他故事。

本次局部核对（2026-11-11）：新增机会性 WebRTC DataChannel P2P 直连升级、本地路由器端口映射与设置页双路径回显测速，新增 US-H12 并更新 US-A04、US-K03、US-M08、US-L10 与覆盖索引。

- 范围：
  - `internal/config` 与 `internal/p2psettings` 新增 `[p2p]` 配置段（默认启用直连升级、自动端口映射与 Cloudflare/Google 公共 STUN）及 `<state_dir>/p2p-settings.json` 持久化覆盖，接入配置诊断视图与持久化清单；
  - `internal/p2p` 基于 `pion/webrtc/v4` 实现双栈单端口 ICE UDP 复用、UPnP IGD / NAT-PMP / PCP 网关端口映射、32 KiB 流分片与背压控制及 `/api/*` 与文件上传 RPC 承载：
    - 支持 `-tags nop2p` 裁剪构建（Linux amd64 `CGO_ENABLED=0 -trimpath -ldflags "-s -w"` 默认 `60,620,960 B`，`-tags nop2p` 为 `55,607,456 B`，体积差 `4.78 MiB`）；
  - `internal/api`、`internal/hub` 与 `cmd/webmux` 接入 `/api/p2p*` 路由、终端与实时流桥接、本机状态摘要及会话内进程 403 边界；
  - `web/src/p2p*.ts`、`ws.ts` 与 `terminal-socket.svelte.ts` 实现本机/低延迟自动跳过、候选类型校验（拒绝 `relay`）、滞回切换、终端流先建后拆 `append` 重叠裁剪续接及断连无感回落；
  - `P2PSettings.svelte`、`NetworkSettings.svelte` 与 `SidebarFooter.svelte` 新增设置页网络分区控制面板、一键双路径回显测速与侧栏主机状态传输路径显示。
- 依据：
  - `openspec validate "add-p2p-direct-upgrade" --strict`、`make check`、`go test -race ./...`、`go test -tags nop2p ./internal/p2p ./cmd/webmux` 通过；
  - 前端单元与浏览器组件测试（`p2p.test.ts`、`p2p-handoff.test.ts`、`P2PSettings.test.ts`、`TerminalReplay.test.ts`、`SettingsNavigation.test.ts`）通过；
  - 隔离实例 Playwright E2E（`p2p.spec.ts`，桌面与手机视口）覆盖本机自动跳过、真实 Chromium ↔ Pion WebRTC 直连升级、终端流与无 HTTP XHR 的分块文件上传、双路径回显测速及在线关闭无损回退，桌面（1280×900）、平板（768×800）与手机（390×844）无页面横向溢出。
- 限制：
  - 仅在 Linux 本地隔离实例与无头 Chromium 中核验；无头浏览器未验证 `hover` 与 `any-pointer: coarse` 媒体查询，未在 macOS、iOS/Android 真机或跨运营商对称 NAT / 移动蜂窝公网链路上实测打洞成功率。
