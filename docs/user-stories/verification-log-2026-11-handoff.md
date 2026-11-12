# Portable handoff 核对记录

本页记录 2026-11-11 起交接增强层的实际验证。故事 ID 保持不变。
身份基础的先前依据见[基础层记录](verification-log-2026-11-06.md)；Activity、历史接口、预览、inline 与同一对话切换的验证保留在本页。

## 2026-11-11：Handoff 02 完整 Activity 与 replay 对账（US-Q05、US-Q12）

- 范围：UI 截断前的规范化消息、命令输入/退出码/输出、文件修改、计划、压缩、权限结果、错误、回合与切换；附件只存元数据和来源引用。
- 存储：64 KiB 分块追加，快照保留相同前缀；完成后事务发布 position；长项按 UTF-8 字节分页；半条崩溃尾部保持未提交并标 gap。
- 对账：ProviderThread + provider item ID，以及无 ID 的完整项有序前缀；native ID 改变、乱序、缺失或未知无 ID 内容保持 reconcile_required，TUI 缺口不被猜测性删除。
- 验证：存储与 recorder 的针对性 Go race 测试通过，含大于 256 KiB 的消息/工具、重复文本、快照尾部、后台工具跨回合归属、v1 升级和实际 fake-agent 冷 load 去重。
- 基准：Linux amd64 / Go 1.26.10，128 次 4 KiB 追加约 67.9 μs/op、60.31 MB/s、7,279 B/op；这是临时目录中的单次样本，不代表生产磁盘或全套性能验收。
- 限制：当前提供服务层读取，REST 与浏览器历史查询在下一层接入；未在原生 macOS 执行本层测试，未验证真实 provider 的完整回放能力。
- 评审补充：工具标题、命令输入与退出码独立分块保存，metadata-only patch 不改写累计输出；省略初始工具状态按 pending 处理，跨回合完成保持原 Run。新增回归覆盖这两种情况和输出零改写。
- 插话补充：接受插话后切断无 native ID 的消息连续段，显式 failed 丢弃未消费草稿；实际 ACP 调用、关闭和重开数据库的回归测试验证顺序正确且不产生伪崩溃缺口。

## 2026-11-11：Handoff 03 历史投影与分页 API（US-Q05、US-Q12）

- 范围：归档或运行中的 Conversation 分页读取、长项 UTF-8 字节余文、类型化 portable projection；游标使用十进制 int64 字符串，独立于 chat generation/seq。
- 验证：API Go race 测试覆盖认证/恶意 Origin、非法和重复游标、跨 Conversation、单页/聚合上限、并发追加和中文/emoji 拼接；wire snapshots 与 Zod 契约/游标测试共 232 项通过。
- 隐私：只返回规范化可导出内容与必要来源，控制项显式标不可导出；响应 no-store，读取限时 10 秒。
- 集成：`make check` 通过，包含前端架构检查、文档与持久化守卫、vet 和 linter；无可见 UI 改动。
- 限制：尚无浏览器历史选择 UI，本层提供供后续 selector、preview 与 thread_read 复用的读取契约；未在真实 provider 上验证历史完整度。

## 2026-11-11：Handoff 04 选择、预算与持久预览（US-Q20、US-K04）

- 范围：新增 US-Q20，两个索引同步；整项 selector、UTF-8/JSON 成本、provider 用量与估算、目标变化失效、持久草稿、revision 冲突及 stale；配置上限对应 US-K04。
- 验证：`make check`、完整 Go race 测试、238 项历史/预览 Zod 与 wire 测试通过；涵盖 64 KiB 边界、转义与计数位数、重复 ID、超大项、小窗口、附件预算、编辑重启与并发冲突。
- 成本：1 万条候选、500 次 selector 基准约 1.13 ms/op、3.81 MB/op；选择阶段只加载成本元数据，之后读取被选中的完整项。这是本机单次样本。
- 选择语义：按优先级逐项尝试，超大项跳过；预算缩小时总选择成本单调，所选集合可能变化，不承诺严格子集关系。
- 隐私与边界：预览只存配置摘要和附件引用，不持久化 argv/env/blob；手工文本与用户新请求分开，原生注入与 MCP 工具能力保持禁用。
- 限制：没有可见 UI 改动，本层不投递 prompt 或创建 worktree/session；尚未运行真实 provider 或原生 macOS 验收。
- 失效补充：启动配置摘要包含聊天能力开关；目标选项基线与显式覆盖值单独保存，不依赖预算总数判断。禁用聊天后保留编辑、未知窗口下 model 漂移、明确覆盖前后及相同 POST 重试的 Go race 回归通过。

## 2026-11-11：Handoff 05 inline 投递与持久幂等（US-Q20）

- 范围：fork 到新 Conversation，cwd/已有或新 worktree，持久 receipt、资源 checkpoint、actor dispatch gate、独立用户请求与 inline coverage；US-Q20 和覆盖索引同步。
- 验证：`make check`、完整 Go race 测试、242 项 Zod/wire 测试通过；API 覆盖认证/Origin、归档来源、connector 缺失、首次失败结果、显式重试与 payload 冲突。
- 故障注入：receipt 后、worktree 创建后、session 创建后、调用前与调用后分别模拟中断并重开服务，保持资源与命令身份；未进入 actor 的阶段可继续，可能调用过的 unknown 不自动重发。
- 实际协议回归：fake-agent 仅保存用户消息后退出，自动 load 失败时保留 unknown；显式恢复后以精确 native ID 和 payload hash 确认消费，未发送第二个 prompt。正常结果、明确 auth refusal 和后续错误不会混淆消费事实。
- 展示边界：本地 Activity、时间线和标题只使用用户新请求，回放验证后恢复相同显示；历史内容不成为新的用户消息。
- 限制：当前只执行 fork inline；同一 Conversation 切换、原生历史注入、MCP 工具和浏览器交接入口属于后续 stack 层，未宣称真实 provider 验收。

- Handoff 05 评审补充：附件以持久 copy intention/hash 原子发布到目标私有 uploads，明确归目标会话所有；对账 hash 在 capability fallback 后与 dispatch gate 一起提交。新增回归覆盖来源文件消失、忘记来源、服务重启、fallback unknown/load、UI 与 Activity 附件元数据恢复，且未再发送 prompt。
- Handoff 05 删除补充：目标拥有用户新请求、native/hash 与附件引用的回放 receipt，不依赖来源预览。v5 升级、忘记来源后重启/load、不同稳定 item ID 不改写、忘记目标级联删除测试通过；不会保存额外来源历史副本。attention 投影竞态测试改为等待实际 shell idle，连续 20 次 Go race 通过。

## 2026-11-11：Handoff 06 同一对话切换与原生继续（US-Q12、US-Q20）

- 范围：同一 Conversation 的 A→B→A 与 A→B→C、真实 session/resume、已确认 Run/position 覆盖、共享运行绑定与原生身份轮换；位置固定，不经过快速开始。
- 协议回归：实际 fake-agent 验证 A 原生 ID/运行 ID 不变、只选择 B 的未见项、旧 Run 的迟到结果仍计入、保持一个 writer；缺少 resume 时生成 full fresh 草稿并等单独投递。
- 并发与生命周期：前台回合、后台工作及 Codex 外部 writer 锁拒绝切换；持久 owner 阻止其他设备同时投递，重启后继续原 attempt，unknown 不绕过；忘记共享绑定保留其余线程。
- 验证：`make check`、全仓 `go test -race -p 4 ./...`、268 项历史/Zod/wire 测试通过。继承的 CODEX_PATH、XDG 配置和运行时变量在测试中清除，使用隔离临时目录。
- 限制：本层无可见 UI，尚未验证真实 provider 账户；nativeContinue 只标识原生 resume，不表示原生历史注入，后者属于下一层。
- 评审补充：恢复与投递共用 Conversation 的 admission token，阻止旧 provider 在成功切换后通过普通归档恢复重开 writer；unknown 恢复拒绝终端端和原生 ID 替换。实际 fake-agent 回归覆盖拒绝后无新 shell、无新 prompt，原 writer 保持运行。
- 中断补充：原生 TUI 实际退出后，确认 PID 0，再退场 actor/已退出 shell；切换在短时测试期限内完成，原 interrupted 记录保留，不再等执行总超时。
- 来源恢复补充：旧 agent 因活跃回合或暂停队列拒绝退场时，失败目标的 owner 不再占用旧运行；目标明确失败的普通聊天保护与终端恢复语义不变，pending/unknown 仍覆盖来源与目标。
- 实例核验：隔离 HOME/XDG 和 fake-agent 存储，实际私有 WebMux 可执行文件经认证 HTTP 完成忙碌来源的归档恢复、服务重启自动恢复；原生身份不变，普通输入均为 200。实际失败目标恢复后普通输入仍为 409、终端恢复为 200，pending/unknown 来源恢复到两端均为 409。
- 定向验证：conversation、handoff、hybrid、API 未缓存 Go race、固定版本 linter 和文档治理通过；无真实 provider 或账户验收，未重新核验其他故事。

## Handoff 07：Codex 原生历史（US-Q12、US-Q20）

- 范围：可选 HistoryInjector、独立 stdio app-server 客户端、目标持有的注入 receipt、历史与用户 prompt 分别记账；预览不启动 agent 探测能力。
- 协议：核对 codex-acp 2.5.0/2.6.0 与 Codex 0.166/0.167 的固定源码；原生帧省略版本字段，角色保持 user/input_text 与 assistant/output_text。
- 实际 stdio：fake Codex 共享 ACP/native 原生存储，核对 coverage、每项角色、顺序和完整正文；用户新请求独立发送，load/restart 不重复 Activity 或 prompt。
- 故障：注入保存后退出保持 unknown，无 inline 回退；显式 load 的完整有序证明只确认历史，原用户命令单独继续，不重复注入。
- 边界：旧版本与明确 MethodNotFound 回退 inline；64 KiB 用户请求可独立于原生历史，inline 回退超限时拒绝且不截短。
- 存储：claim 后崩溃、错误原生 ID、部分证明、新命令绕过、后续用户失败和来源删除的 Go race 测试通过。
- 传输：无版本字段、紧随 ACK 的 EOF、阻塞写取消、超长帧拒绝、错误正文不泄漏的 Go race 测试通过。
- 集成：`make check`、全仓 `go test -race -p 4 ./...`（隔离 XDG/CODEX 环境）、268 项 wire 前端测试通过。
- same-thread 对账：load 使来源过期时保留历史事实和原用户草稿，旧命令拒绝发送；重新预览由用户决定。

## Handoff 08：只读历史 MCP（US-Q20）

- 范围：固定 argv scope 的 `mcp-serve`，经本机 HTTP API 读取冻结来源；数据库仍由服务端独占生命周期。
- 协议：采用 MCP initialize 时代协议，支持 2024-11-05 至 2025-11-25 的四个固定版本；现代 discovery 明确返回 MethodNotFound 以便客户端选择 legacy。
- 实际链路：活 SQLite → Unix HTTP API → CLI stdio MCP → 独立 fake provider，取回 45 项历史，分页无重复；长中文/emoji 按字节 offset 完整重建。
- 活跃复用：普通目标首次接入 MCP，以及连续两次 handoff 刷新冻结 source point；保留原 runtime/native ID，经真实 stdio 读到当前 scope。
- scope 与生命周期：非法 cursor/limit、跨 Conversation、未来项拒绝；new/load/resume 配置与关闭后的 helper 清理通过。
- 门禁：`make check`、隔离环境全仓 Go race、237 项 wire-contract 前端测试通过。补齐集成用例 Manager 关闭后，MCP+既有 CLI 联合重复 2 次通过；既有 CLI 单独重复 5 次通过。

## Handoff 09：浏览器交接与位置（US-Q20、US-Q05）

- 同一对话换 Agent 弹窗与新聊天页面共用预览、独立任务和服务端 revision；不读取 ChatModel 或拼接 quickStartMemory。
- API：新增按来源/模式恢复草稿，任务单独 PATCH 不改变 native 资格；冻结来源工作位置与目标位置分开。
- 桌面和 iPhone SE：20 项 E2E 通过，覆盖关闭/刷新/重启、A→B→A、三种位置、并发冲突、超预算、分片、unknown 与明确重试。
- 审查回归：恢复新工作树草稿保留已保存分支和精确基准，仓库 HEAD 移动后仍从原提交创建；发送前发现未审阅 revision 则不发送，进入冲突流程。桌面/手机 4 项针对性复验通过。
- 轮询回归：延迟旧 GET 不覆盖成功 autosave；另一设备的投递结果不清空本设备更新的用户任务，桌面/手机 4 项针对性 E2E 通过。
- 最终 UI 复验：38 项桌面/手机 handoff E2E 通过，发送与保存关闭先锁定编辑再等待 PATCH；目标记录删除、来源查询暂时失败不清空本设备未发送内容。
- 初始化竞争：分支列表加载前禁用新分支输入，避免建议名与输入拼接。延迟列表的桌面/手机 E2E 和 59 项工作树/新会话浏览器测试通过。
- 输入框/键盘缩小聊天视口时，位置未变的排队 scroll 事件不取消底部跟随；用户主动上滚仍暂停跟随。新增确定性事件顺序在修改前失败，修改后聊天/视口 34 项与完整 712 项 browser 通过，`make check` 通过。
- 聊天滚动补充浅深色及桌面/平板/手机前后截图；正常速度[手机多行输入与收起](../images/handoff/composer-follow.gif)保持最新回复可见。截图为正常交互，事件竞争由上述回归测试验证。
- 滚动修复后，桌面/手机的聊天↔终端往返、长历史重放与文档滚动 E2E 19 项通过，3 项按既有布局条件跳过。
- 关闭后焦点回到会话菜单，桌面/手机复验通过。补充桌面/手机 4 项 E2E 通过：新回合只标 stale，删除来源后清除本设备恢复缓存。
- Go race 覆盖全仓；随机来源路径引发的 wire 快照差异修复后 API 包复验通过，conversation/handoff/threadread/archtest 再验通过。
- 前端 786 项单元、712 项真实 Chromium 浏览器测试通过；`make check`、前端架构守卫和测试类型检查通过。
- 真实应用检查浅/深色及 1280×900、768×800、390×844、375×667、320×568、667×375；根页面无横向溢出，打开的菜单、Select 和弹窗均在视口内。
- [交接弹窗](../images/handoff/preview-light-desktop.png)、[手机新聊天](../images/handoff/fork-dark-phone.png)和前后截图使用隔离 fake agent 数据；版本提示反映 fake agent 的版本。
- 正常速度 GIF：[编辑、保存关闭与切换](../images/handoff/switch-flow.gif)，[位置选择、历史分片与新工作树投递](../images/handoff/fork-flow.gif)。
- Chromium 模拟未代替真实手机键盘/触屏；没有真机或原生 macOS 运行验证。鼠标 hover 媒体条件为 true、coarse 为 false，未核对真实触屏常驻与硬件悬停。

### 2026-11-12：准入取消与写者所有权（US-Q12）

- 准入结果不随调用者取消而丢弃；取消发生在接管后时仍等待旧 ACP 写者退出，清除打开中状态，未开始的原生调用保持 pending。
- 定向 Go race 回归通过：准入阻塞时取消不会提前返回；实际 fake Codex 在 `session/close` 期间取消后原 PID 已退出，会话可正常关闭并保留中断记录。
- `internal/handoff` 的定向原生注入 Go race 套件通过，覆盖独立用户请求、来源删除后回放、未知结果、重启与版本回退。
- 首次生命周期测试错误地预期关闭会将 interrupted 改成 closed；按既有中断记录语义修正断言后，`internal/hybrid -run TestHistory` 通过。

## Handoff 10：迁移、恢复与数据边界（US-Q05、US-Q12、US-Q20、US-K04）

- 全局 `history_capture=false` 保留正常聊天和身份/未决投递保护，拒绝新正文捕获与历史/交接/MCP；重新启用不回填关闭区间的原生 replay。
- 关闭期间的新建、绑定与原生身份替换在同一事务写入关闭标记；重新启用后才观察到的替换也延续该标记。事务失败不发布新绑定，重新开启后的新实时正文仍可捕获。
- 身份替换的关闭标记回归与更新后的全仓 Go race、lint、vet、完整 arch/doc 守卫再次通过。
- 来源 forget 后，目标保留最小 dispatch 凭据与占用成本；unknown 继续阻止新命令，精确目标 replay 可对账，不恢复来源操作。
- JSON 与 SQLite 的发布/删除意图标记区分资源创建窗口和已删除运行记录；资源准备期间 forget 在关闭 writer 之前拒绝竞争。
- release/rebuild 标记与配置删除保留历史/草稿；forget、MaxRecords 淘汰与未开始 fork 草稿清理通过，原生目标事实按目标生命周期保留。
- 子进程在成功状态提交前退出后仍为 unknown；Activity 提交失败、磁盘满/只读模拟、重复 replay、旧 Run 后台 compaction 与 full 策略去重通过。
- 修复 fake provider 在 UTF-8 字符中间切 chunk；实际 stdio 测试用中文/emoji 核对正常聊天，关闭捕获也不损坏回复。
- 隔离环境全仓 Go race 通过，包含完整 `internal/archtest`；786 项 Vitest unit、712 项 Chromium browser、75 项 Storybook 通过。
- 完整桌面/手机 E2E 439 项通过，47 项按既有平台/布局条件跳过；`make check` 与新增文档后的独立 docs/arch 守卫通过。
- 独立 4× CPU / CLS 性能门禁 18 项通过，2 项按既有平台条件跳过。
- v9 来源已删除的迁移补充五种状态：确认成功、prompt unknown、native unknown、仅历史确认和未发送；真实 stdio 恢复成功 fork 后原任务调用仍为 1，后续独立任务正常发送，不恢复来源操作。
- 组合输入视口 fixture 排除原生聚焦滚动和状态换行引起的 scroll anchoring，保留精确 50px 与结束后 0px 的断言；修正后 81 个 browser 文件全部通过。
- 1 万 Activity、500 次 selector、1 MiB 分块流、btrfs/tmpfs 对照、纯 Go Linux/Darwin 构建的真实数据见[故障矩阵与性能报告](../protocol/handoff-validation.md)。

## 2026-11-12：合并修复复验（US-Q05、US-Q12、US-Q20）

- 迁移依次为 selection 计数/索引 v9、草稿索引 v10、dispatch 凭据 v11；来源已删除的 fork 升级 fixture 精确还原 v10，而非保留 v10 索引却标成 v9。
- 修复后 conversation、handoff、hybrid、api、threadread 五个包的 Go race 通过；此前的 `make check`、MCP 命令回归和 251 项前端 schema/wire 回归通过。未把这些定向复验记为全仓测试。
- 4 万项 HTTP 回归的真实测量：创建预览 3.86 秒，完整选择元数据分页 1.34 秒；首、中、尾正文及单项读取通过，分页不再逐页全量计数或重排。
- 非测试 CLI smoke 通过真实 Unix HTTP API 与 `mcp-serve`：10,014 UTF-8 字节正文分 26 段精确恢复，冻结点之外的 cursor 拒绝；无认证的 selection 访问返回 401。
- 新建 v11 数据库的真实 fork、重复命令、两次重启及来源 forget 均保留目标凭据和已发布 runtime；provider 调用数为 2→2→2→3，重启不重发原任务，随后明确的新任务正常发送。
- 真实 HTTP A→B→A：未消费的被拒请求保留在 delta，后续已消费请求不重放；交付使游标 0→18，下一普通请求推进至 23，后续 delta 不重复旧请求。
- 独立可执行程序取消已分离的 ACP 写者：等待约 2 秒完成退出，原 PID 消失，native 调用数为 0、CallStarted=false，正常关闭完成；确定性 race 回归另行覆盖 admission/reply 竞态。
- 真实浏览器经 110 次普通输入生成 320 项省略历史，全部浏览到末项并跨越首 100 项边界，Back/Next 保留正文，余文 offset 0→4000→0；编辑、保存关闭、刷新、同对话切换与新目录 fork 通过。
- 桌面/平板/手机浅深色证据与正常速度 GIF：[分页](../../.github/pr-assets/handoff-ui-repair/reader-page-100-to-102.gif)、[草稿与切换](../../.github/pr-assets/handoff-ui-repair/draft-save-reopen-refresh-switch.gif)。历史原图使用不同样本，不宣称为新录制的受控前后对照。

### 2026-11-12：历史 reader 刷新取消（US-Q20）

- 准入使用保留 execution context 的不可取消结果等待；排队时取消不接管 writer，等待旧进程退出期间取消仍完成清理和 actor 转换，不恢复 provider。
- Go race 定向验证 `internal/hybrid` 的 reader 排队取消、关闭期取消、连接丢失，以及 `cmd/webmux` 的真实 MCP 集成通过；受影响 hybrid 包 lint 为 0 issues。
- 独立编译程序经公开 service API 正常刷新 MCP，再在旧 writer 的延迟 `session/close` 中取消并请求正常关闭；真实 `webmux mcp-serve` 通过 Unix HTTP/SQLite 完成读取。
- 独立 smoke 确认旧 writer 与 MCP 子进程均已退出，取消至 join 为 2007 ms，记录为 interrupted/PID 0；只有首次正常刷新发生一次 resume，无用户 prompt，服务关闭在本次测量中为 0 ms。
- 精确排队竞争由确定性回归覆盖；独立 smoke 不使用 `go test`，但 provider 仍为隔离 fake Codex，不代表真实账户或原生 macOS 验收。

### 2026-11-12：长历史草稿读取成本（US-Q20）

- CPU/分配 profile 显示，草稿读取曾在 wire 截取前反复解码完整省略 ID 集合；40,000 项用例中 `readPreview` 累计分配约 33.31 MiB。
- 读取层现保留最多 100 个省略 ID 样本，并读取独立持久计数；原生投递仍保留全部选入 ID，手动上下文覆盖说明使用完整计数，选择分页仍恢复全部 40,000 项。
- 精确 race、atomic coverage 和跨包 `coverpkg` 定向回归通过，本机总用时 14.677 → 12.477 秒；POST 的 profile CPU 为 3.10 → 2.84 秒，草稿读取分配降至采样阈值以下。
- 两个独立编译的服务、隔离 HOME/XDG/状态目录和绝对路径 fake 命令进行真实 HTTP 对照：POST 153 → 124 ms，PATCH 100 → 4.24 ms，GET 30.4 → 0.82 ms。
- 两次 HTTP 对照均恢复 400 页、40,000 个不重复省略 ID，最大选择页 11,449 字节；分页总计 168 → 172 ms。样本均为 100 项，完整省略计数均为 40,000。
- 以上为本机单次对照，不代表 CI 或生产时延保证；生产 10 秒请求 deadline 未改变。受影响 Go 包的固定版本 lint 为 0 issues。
- 首次全仓 coverage/race 仅失败于 `agenttrust/TestPrepareWritesThroughSymlinkedConfig` 的文件模式夹具；API、Conversation、handoff、hybrid、thread_read 均通过。
  - 原因：继承 `umask 0077`，初始模式实际为 `0600`。
  - 随后组合核心 `853f55bd` 显式建立夹具模式，`make check` 与全仓相同 race/atomic coverage 检查通过。
- CI 的完整 instrumentation 仍使 40,000 项快照构建超过 REST 期限；恢复回归不再把 instrumentation 墙钟时间当成功能契约。
  - service 先构建大夹具，真实 HTTP GET/PATCH、selection 与 MCP 继续核对完整数据和响应边界。
  - 数据规模、断言和生产 10 秒期限不变；大数据 HTTP POST 的生产路径由上面的独立二进制测量验证。

### 2026-11-12：禁用历史后的选择分页（US-Q20、US-K04）

- selection 路由复用 `HistoryEnabled()` 门禁；禁用捕获仍保留的身份服务不再使历史读取入口部分开放，未修改认证、Origin 检查或派发保护。
- 既有禁用历史路由回归纳入 selection；该回归、40,000 项选择恢复和历史认证/游标验证的 Go race 通过（12.646 秒），受影响 API 包固定版本 lint 为 0 issues。
- 独立编译的实际服务加载一项已持久预览与 Activity，经 enabled → disabled → enabled 两次重启，认证 selection 返回 200 → 503 → 200，末次响应与首次逐字段一致；三阶段未认证均为 401、外部 Origin 均为 403，selection 保持 no-store。
- fixture 离线经 service 写入，保留为未发布 reservation（`published=0`），避免不存在遗留 JSON 清单的记录被正常启动清理。最初未保留的 fixture 得到 404，不能作为历史禁用成功的依据。
- 禁用响应不包含来源路径或预览 ID。HOME/XDG/状态目录独立，无 provider 进程或计费请求；三次实际服务均已正常关闭，未验收真实账户或原生 macOS。

### 2026-11-12：首次预览前的跨设备草稿（US-Q20）

- 修复设备 A 尚未预览时的本地任务、目标、模型、上传附件和新聊天位置，被设备 B 新建的服务端预览无条件覆盖的问题。
- 沿用 choice 备份，不增加缓存格式或伪造 recovery ID/revision；仅来源默认位置不会误触发冲突。另一设备的历史只读，直到用户明确读取服务端版本或保留编辑重新预览。
- 双设备 E2E 使用两个独立浏览器 context、实际文件上传和不同目标；核对关闭与重复重开后的完整 choice 备份、两种明确选择、保留本地附件和位置、重新预览不投递及单独确认发送。
- 最终 handoff E2E 桌面与 iPhone SE 共 46 项通过，无重试；真实 Chromium browser 全套 731 项通过。`check`、`check:test`、`lint`、`check:arch`（含 40 项架构测试）、源码格式检查和生产构建通过。
- 初次新增回归暴露夹具问题：所选模型不在 fake provider 的选项中，同名上传也会规范化文件名。改用真实支持的 `smart` 模型和来源唯一文件名后，完整 E2E 复验通过；未修改生产附件或模型语义。
- 独立非测试 smoke：单独编译的真实应用服务、隔离 HOME/XDG/状态目录和绝对路径 fake 命令，以两个独立 context 实际上传文件并建立 A 未预览 → B 手工预览 → A 重开的转换，未导入 E2E fixture 或运行测试 runner。
- smoke 直接核对 choice 完整不变、无虚构 recovery、附件文件字节与 fork 位置；保留编辑后 B 的手工正文不被冒用或改写，来源历史重新生成。
- smoke 在重新预览后仍只有来源会话、仅 1 次原始 provider prompt；明确发送后才出现第 2 次 prompt，目标 Activity 独立保存 A 任务和复制后的附件字节。另一组双 context smoke 核对读取服务端版本后刷新不再冲突，且不产生投递。
- 浅/深色及 1280×900、768×800、390×844、375×667、320×568、667×375 共 12 组无页面横向溢出；检查模拟鼠标 hover 与键盘焦点，没有真实触屏或移动键盘验证。
- 最终冲突状态截图（不是修改前后对照）：[桌面浅色](../images/handoff/unpreviewed-conflict-light-desktop.png)、[平板浅色](../images/handoff/unpreviewed-conflict-light-tablet.png)、[手机浅色](../images/handoff/unpreviewed-conflict-light-phone.png)。
- 深色截图：[桌面](../images/handoff/unpreviewed-conflict-dark-desktop.png)、[平板](../images/handoff/unpreviewed-conflict-dark-tablet.png)、[手机](../images/handoff/unpreviewed-conflict-dark-phone.png)。
- [正常速度冲突处理与明确发送](../images/handoff/unpreviewed-conflict-flow.gif)为独立 smoke 的 1280×900 浅色真实录屏片段，保留原始速度，输出 960×675、12 秒、144 帧；不是生产账号或真实模型验收。

### 2026-11-12：空任务的附件交接（US-Q20）

- 发送准入与普通聊天一致：任务或已上传附件至少一项非空；不改变上传中、冲突、预算、过期、pending、unknown 与锁定保护，不补虚构文字，也不扩大后端范围。
- 图片和文件回归经真实上传、空任务预览与明确发送，核对 Activity 空任务、目标副本字节及 fake provider 原生 image/resource 内容；无文字无附件仍不可发送。
- 最终 handoff E2E 桌面与 iPhone SE 共 50 项通过、禁用重试，包含既有上传竞争与首次预览前双设备恢复。
- 初次新回归误将 worker 历史 provider store 当作空目录，并误用 Activity 的 path 字段；改为按本例 cwd 计数、读取公开 reference 后复验通过，生产逻辑未因此扩大。
- 定向 Chromium browser（历史 reader 与聊天附件）14 项通过；类型、测试类型、lint、40 项架构测试、源码格式、文档守卫与生产构建通过。本次沿用 `dcd9950` 的完整 731 项 browser 依据，没有重跑全套。
- 独立非测试 smoke 单独编译服务，隔离 HOME/XDG/状态与 provider store，使用绝对路径 fake agent；桌面 1280×900 上传 33 字节文本文件、手机尺寸 390×844 上传 68 字节 PNG，均保持空任务且实际副本和 provider 内容逐字节一致。
- 两组 smoke 在上传和预览后各仅有 1 次来源 prompt、没有 delivery；点击「发送交接」后各新增且仅新增 1 次 prompt，结果为 inline。未连接真实 provider 或付费账户。
- 前后截图使用同一真实后端、同一已上传文件和保存的空任务预览，对比保留的修改前生产 bundle 与本次 bundle；没有修改 DOM 按钮或编辑器数据。浅深色均测得修改前 disabled、修改后 enabled，无页面横向溢出。
- 桌面 1280×900、平板 768×800、手机 390×844；实际 DPR 为 1.25，hover=true、coarse=false；未验证真实触屏、移动键盘或真机。
- 浅色前后：[桌面前](../images/handoff/attachment-only-before-light-desktop.png) / [后](../images/handoff/attachment-only-after-light-desktop.png)，[平板前](../images/handoff/attachment-only-before-light-tablet.png) / [后](../images/handoff/attachment-only-after-light-tablet.png)。
- 浅色手机：[前](../images/handoff/attachment-only-before-light-phone.png) / [后](../images/handoff/attachment-only-after-light-phone.png)；深色桌面：[前](../images/handoff/attachment-only-before-dark-desktop.png) / [后](../images/handoff/attachment-only-after-dark-desktop.png)。
- 深色前后：[平板前](../images/handoff/attachment-only-before-dark-tablet.png) / [后](../images/handoff/attachment-only-after-dark-tablet.png)，[手机前](../images/handoff/attachment-only-before-dark-phone.png) / [后](../images/handoff/attachment-only-after-dark-phone.png)。
- [正常速度明确发送](../images/handoff/attachment-only-send.gif)：独立 smoke 的 1280×900 深色录屏，输出 960×675、12 fps；保留点击前附件与空任务、投递中及完成状态，没有加速。

## 核对范围与限制

07/08 层无可见 UI；09 层的界面证据和局限记录在上方。
测试使用隔离临时状态与 fake provider，未连接真实 provider 账号、未发出计费模型请求。
版本矩阵是上游协议核实；未宣称在原生 macOS 或真实移动设备上运行本层测试。
集成门禁和后续增强的实际结果在完成后追加，不把未运行项目记为通过。

## 2026-11-12：续接建议与最终 native handoff 核心集成（US-Q07）

- 合并 selection v9、draft v10、dispatch v11 及最终聊天滚动核心；保留原生 writer、unknown/pending 准入与冻结位置规则，不改动核心焦点或跟随控制。US-Q07 的建议验收迁入「模式与时间线」子页。
- 验证：生产前端构建一次后复用嵌入产物，格式、文档、持久化、测试类型、前端 lint/架构、Go vet 与固定版本 linter 全部通过；原核心日志有一处 Oxfmt 差异，格式化该文件后继续门禁。3 项建议浏览器测试、253 项契约测试、桌面/手机 2 项建议 E2E，以及 Hybrid/API/Config/Conversation/Handoff 五包 race 通过。
- 隔离真实应用：配置假生成命令后，新回合建议请求数为 1；Tab 离开空消息框，忽略后刷新保留同一回合标识且请求数为 0。断开聊天连接至服务端结束回合，重连后无历史建议、请求数为 0；下一新回合请求数为 1，点按仅填入草稿。
- 本次无功能界面重设计，沿用已发布的截图/GIF；未连接真实 provider 或重测物理触屏，不把定向回归当作全套测试。
