# Portable handoff 核对记录

本页记录 2026-11-11 起交接增强层的实际验证。故事 ID 保持不变。
身份、Activity、历史接口、预览和 inline 的先前依据见[基础层记录](verification-log-2026-11-06.md)；同一对话切换的验证保留在本页。

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

### 2026-11-12：恢复冲突基准与即时投递完成（US-Q20）

- 独立非测试 smoke 单独编译真实应用和 fake provider，隔离 HOME、全部 XDG 目录、状态、上传目录与 provider store；只监听 127.0.0.1，不导入 E2E fixture，也不使用测试 runner。
- 前后对照将 `c0aec11799c3c62b207d88b8cd74ff82275833a9` 的精确源码私有构建为旧前端，并与修复前端连接同一真实后端。记录实际送出的 HTML/JS 路径与 SHA-256；未改 DOM、按钮状态或返回 JSON。
- 旧版实测：revision 1 的本地恢复在第一次刷新后被记成 revision 2；第二次刷新不再显示冲突，自动 PATCH 成 revision 3，覆盖另一设备的任务和历史。这里的「前」是真实故障，不是示意图。
- 修复版实测：同一 revision 1 备份对服务端 revision 2，连续三次刷新及保存关闭后重开仍保持冲突、本地任务和原基准；服务端 revision、任务与历史不变，没有投递。
- 明确读取服务端版本后旧备份清除；后续成功保存到 revision 3，再遇服务端 revision 4 时备份基准为 3，而非旧的 1。保留编辑重新预览清除该基准、保留新任务，仍没有自动发送。
- 即时完成使用真实幂等重放：浏览器明确发送后，私有传输先转发原 POST，等待真实 inline，再以完全相同 payload 和 clientRequestId 请求同一 POST，返回服务端实际完成响应，不制造状态或新命令。
- 旧版完成响应后仍显示原任务和附件；修复版立即清空下一次任务、附件、目标和模型。两版均只新增 1 次 provider prompt；已投递预览和目标 Activity 保留原任务，真实上传文本、目标副本与 provider resource 字节一致。
- 另一路保持实际 delivery GET 响应等待，由另一设备完成同一已审阅请求；等待期间通过可用输入框输入更新任务并真实上传新附件。释放 inline 响应后，新任务和附件仍保留，已投递预览不变，只有 1 次新增 prompt。
- 真实 pending 准备期间保留任务与附件、禁用输入，provider 尚无新增 prompt；完成后仅新增 1 次。fake provider 保存请求后退出产生真实 unknown：任务和附件保留、等待对账禁用，没有明确重试或自动重发，延迟轮询后仍为 unknown。
- 测试子任务最终报告 handoff E2E **52/52 通过**：桌面 26、iPhone SE 26，retries=0、2 workers；测试类型检查 417 文件、0 错误/警告，定向格式检查通过。沿用先前完整 browser 套件依据，不把独立 smoke 计成测试 runner 用例。
- 首次 smoke 误以为归档含生产 assets，实际只有 `.gitkeep`；该轮落到修复 bundle 的截图全部作废并重拍。另修正上传按钮定位和异步读取服务端版本的等待；最终证据来自明确核对旧 bundle 的成功执行。
- 以下 CSS 视口分别为桌面 1280×900、平板 768×800、手机 390×844；实际 DPR **1.25**，PNG 分别为 **1600×1125、960×1000、488×1055**。24 组前后状态均无根页面横向溢出，等待主题转换结束再截图。
- 模拟环境 hover=true、coarse=false；未验证真实触屏、移动键盘、真机或原生 macOS。独立 smoke 实测 inline；injected 共用的完成状态判定不等于本次运行过真实 Codex 原生注入。

| 状态 / 视口         | 主题 | 修改前                                                     | 修改后                                                    |
| ------------------- | ---- | ---------------------------------------------------------- | --------------------------------------------------------- |
| 重复刷新 / 桌面     | 浅色 | [前](../images/handoff/recovery-before-light-desktop.png)  | [后](../images/handoff/recovery-after-light-desktop.png)  |
| 重复刷新 / 平板     | 浅色 | [前](../images/handoff/recovery-before-light-tablet.png)   | [后](../images/handoff/recovery-after-light-tablet.png)   |
| 重复刷新 / 手机     | 浅色 | [前](../images/handoff/recovery-before-light-phone.png)    | [后](../images/handoff/recovery-after-light-phone.png)    |
| 重复刷新 / 桌面     | 深色 | [前](../images/handoff/recovery-before-dark-desktop.png)   | [后](../images/handoff/recovery-after-dark-desktop.png)   |
| 重复刷新 / 平板     | 深色 | [前](../images/handoff/recovery-before-dark-tablet.png)    | [后](../images/handoff/recovery-after-dark-tablet.png)    |
| 重复刷新 / 手机     | 深色 | [前](../images/handoff/recovery-before-dark-phone.png)     | [后](../images/handoff/recovery-after-dark-phone.png)     |
| 即时投递完成 / 桌面 | 浅色 | [前](../images/handoff/completed-before-light-desktop.png) | [后](../images/handoff/completed-after-light-desktop.png) |
| 即时投递完成 / 平板 | 浅色 | [前](../images/handoff/completed-before-light-tablet.png)  | [后](../images/handoff/completed-after-light-tablet.png)  |
| 即时投递完成 / 手机 | 浅色 | [前](../images/handoff/completed-before-light-phone.png)   | [后](../images/handoff/completed-after-light-phone.png)   |
| 即时投递完成 / 桌面 | 深色 | [前](../images/handoff/completed-before-dark-desktop.png)  | [后](../images/handoff/completed-after-dark-desktop.png)  |
| 即时投递完成 / 平板 | 深色 | [前](../images/handoff/completed-before-dark-tablet.png)   | [后](../images/handoff/completed-after-dark-tablet.png)   |
| 即时投递完成 / 手机 | 深色 | [前](../images/handoff/completed-before-dark-phone.png)    | [后](../images/handoff/completed-after-dark-phone.png)    |

- [更新的未发送任务与附件仍保留](../images/handoff/newer-unsent-after-light-desktop.png)是实际等待响应期间的编辑，不通过修改 disabled 控件制造输入。
- 正常速度 GIF：[刷新、冲突与明确选择](../images/handoff/recovery-reload-conflict.gif)（13.08 秒、157 帧）、[明确发送后清空下一次任务](../images/handoff/completed-send-clear.gif)（8.83 秒、106 帧）；均为 1280×900 浅色真实应用录屏，输出 960×675、12 fps，未加速。
- [测量清单](../images/handoff/transition-proof.json)保留新旧状态、实际基准、完成响应、prompt 增量、来源与图片 SHA-256、PNG 尺寸和 GIF 帧数。公开 PR 链接与在线 GIF 播放由提交者检查，本 smoke 不修改 PR。
- 本次复用前文 `preview-*`、`fork-*`、`attachment-only-*` 的未变界面证据；新增前后状态不以这些旧截图代替。独立浏览器和服务均已关闭，私有源码、可执行文件、录屏中间件与临时状态在证据提取后清理；没有真实账户或计费调用。

### 2026-11-12：草稿附件重启、并发删除与 wire 覆盖（US-Q20）

- 精确旧版真实 HTTP 上传并生成预览后重启同一状态目录，草稿 GET 仍为 200，但附件字节已被暂存清理删除，重新预览返回 400；没有 provider 请求。
- 修复版在持久预览前将受限暂存文件流式复制到已验证来源 runtime 的私有 uploads，不夺走原暂存文件；已有来源上传不重复复制。沿用来源保留、清扫与忘记，不新增持久命名空间或放宽普通上传目录的活会话要求。
- 真实进程重启后原 staging 照常删除，重新预览为 201；审阅副本与独立目标副本均保留 41 字节，SHA-256 为 `84f82dc1ad9e21015de7198f53f22c7e3bf257473119d5878571bd4b36d880e0`。
- 预览后为 0 次 prompt，明确发送后仅 1 次，实际 fake provider 请求引用目标副本；认证 401、恶意 Origin 403、任意或符号链接附件 400、no-store 均核对。临时目录保留不等于抵御操作系统清理。
- 永久回归覆盖离线来源、重启与重新预览、不夺走其他表单暂存、来源/目标隔离、来源忘记与保留清扫，以及任意、符号链接和外来路径拒绝。handoff/artifacts race、架构与文档守卫及固定 Go linter 通过，linter 为 0 issues。
- 并发删除 smoke 仅在私有旧/新二进制的 `LatestPreview` 与 `Handoffs.Get` 之间加调度门，未改查询结果或响应；另一条真实 HTTP DELETE 忘记 fork 目标并触发外键级联，再释放原 GET。
- 来源仍可读时，旧版返回 404，修复版返回 200 及真实来源、空 preview/delivery；真正忘记来源后仍为 404。两版均 no-store，未认证 401、恶意 Origin DELETE 403；生产代码没有调度门，未为此增加专用测试钩子。
- 该 smoke 首轮把 source 的真实 `id` 误写为 `conversationId`，仅修正脚本断言后完成；没有为断言修改生产合约。调度门、私有工作树、脚本、二进制与服务均清理。
- 三个实际 HTTP wire fixture 覆盖来源加空草稿、已保存手工 revision 2 加空投递、预览加关联 failed receipt；共享 `HandoffDraftSchema` 注册和 schema-path 规范化不改旧 golden 或正文。
- wire 重录、API wire/normalizer race 与共享 Zod 合约 248 项通过；本轮 wire 覆盖没有宣称成功原生 provider、附件或非空 repository/worktree 字段。最终组合门禁另记，不把分支定向结果冒充整套。
- [实际新旧后端响应与字节清单](../images/handoff/backend-draft-proof.json)保留重启、并发时序、状态码与限制；均使用隔离状态和 fake provider，没有真实账户或计费请求。

### 2026-11-12：在途自动保存与冲突投递解锁（US-Q20）

- 将本机草稿记忆和自动保存的 effect 合并，复用其中实际读取的 task/context 依赖；没有额外编辑快照数组或并行保存约定。冲突只暂停预览读取与采纳，不停止独立 delivery 查询。
- 永久回归在真实 PATCH 已保存后延迟其浏览器响应，期间单独修改任务、再单独修改历史；旧版服务端在 10 秒后仍为第一次文字，新版保存后续值且保持另一字段。另以真实延迟 provider 产生 pending，再证明原冲突弹窗可解锁。
- 独立非测试浏览器与真实嵌入式 Go 应用，旧版为 `6165aeb7b4df2a10488fb90512c38251bc91da33`；仅转发并等待实际 HTTP 响应，不伪造成功、pending、正文或 DOM 状态。
- 旧版两项独立编辑在释放响应后 10.5 秒仍只在本机保留最新值；新版均保存后续值，新的 incognito context 读取同一最新文字，其他字段保留，没有自动发送。
- 真实 fork 目标设置 `FAKEAGENT_DELAY=session/new=8s`，两版开始时均为真实 pending 与 locked；真实 inline 后，旧版无 delivery GET 且持续锁定，新版 9 次 GET 后解除投递锁。
- 两版均保留本地未发送任务、历史和原恢复基准，冲突期间 0 次自动 PATCH/POST；每次 handoff 恰好 1 次 provider prompt，已审阅 payload 不被本地冲突文字替换。
- 24 张截图覆盖桌面 1280×900、平板 768×800、手机 390×844 和浅深色；DPR 1.25，PNG 为 1600×1125、960×1000、488×1055，均无根页面横向溢出。另在 320×568、375×667 两主题核对稳定状态。
- 所有主题截图均等待有限动画结束；以下「前」来自真实故障状态，不用正常旧图或修改 disabled 控件制造差异。

| 状态 / 视口     | 主题 | 修改前                                                             | 修改后                                                            |
| --------------- | ---- | ------------------------------------------------------------------ | ----------------------------------------------------------------- |
| 自动保存 / 桌面 | 浅色 | [前](../images/handoff/autosave-before-light-desktop.png)          | [后](../images/handoff/autosave-after-light-desktop.png)          |
| 自动保存 / 平板 | 浅色 | [前](../images/handoff/autosave-before-light-tablet.png)           | [后](../images/handoff/autosave-after-light-tablet.png)           |
| 自动保存 / 手机 | 浅色 | [前](../images/handoff/autosave-before-light-phone.png)            | [后](../images/handoff/autosave-after-light-phone.png)            |
| 自动保存 / 桌面 | 深色 | [前](../images/handoff/autosave-before-dark-desktop.png)           | [后](../images/handoff/autosave-after-dark-desktop.png)           |
| 自动保存 / 平板 | 深色 | [前](../images/handoff/autosave-before-dark-tablet.png)            | [后](../images/handoff/autosave-after-dark-tablet.png)            |
| 自动保存 / 手机 | 深色 | [前](../images/handoff/autosave-before-dark-phone.png)             | [后](../images/handoff/autosave-after-dark-phone.png)             |
| 冲突完成 / 桌面 | 浅色 | [前](../images/handoff/conflict-delivery-before-light-desktop.png) | [后](../images/handoff/conflict-delivery-after-light-desktop.png) |
| 冲突完成 / 平板 | 浅色 | [前](../images/handoff/conflict-delivery-before-light-tablet.png)  | [后](../images/handoff/conflict-delivery-after-light-tablet.png)  |
| 冲突完成 / 手机 | 浅色 | [前](../images/handoff/conflict-delivery-before-light-phone.png)   | [后](../images/handoff/conflict-delivery-after-light-phone.png)   |
| 冲突完成 / 桌面 | 深色 | [前](../images/handoff/conflict-delivery-before-dark-desktop.png)  | [后](../images/handoff/conflict-delivery-after-dark-desktop.png)  |
| 冲突完成 / 平板 | 深色 | [前](../images/handoff/conflict-delivery-before-dark-tablet.png)   | [后](../images/handoff/conflict-delivery-after-dark-tablet.png)   |
| 冲突完成 / 手机 | 深色 | [前](../images/handoff/conflict-delivery-before-dark-phone.png)    | [后](../images/handoff/conflict-delivery-after-dark-phone.png)    |

- 正常速度 GIF：[后续编辑继续保存](../images/handoff/autosave-follow-up.gif)为平板深色 9.28 秒，[冲突中跟进真实完成](../images/handoff/conflict-delivery-unlock.gif)为桌面深色 12.32 秒；均为真实录屏，未加速。
- [实际响应与测量](../images/handoff/save-conflict-proof.json)记录请求、DOM、全新 context、prompt、二进制与 bundle hash；本次没有真实账户、真机、原生 injected、unknown 实际状态或认证拒绝验证，不能替代此前各自的证据。
- 最终共享组合 handoff E2E 56/56 通过（桌面 28、iPhone SE 28、retries=0、2 workers）；handoff、artifacts 与完整 API Go race 分别通过，最终 API 为 82.350 秒。
- 首轮附件持久化集成使两个旧路径恒等断言失败；删除暂存路径不变这一实现细节断言，保留草稿选择验收，并核对实际目标附件的名称与完整字节，随后 56 项全部通过。
- 最终 `make check` 通过：生产/测试类型 0 错误警告、生产构建、格式、文档与持久化、lint、架构守卫及 40 项测试、Go vet 与固定版本 linter 0 issues。控制器 497 行，源码 hash 与独立 smoke 完全一致；未扩大任何守卫基线或性能门限。
- 所有独立浏览器、服务、私有工作目录与录屏中间件清理；公开静态资源和 PR 内嵌由提交者后验。

## 核对范围与限制

07/08 层无可见 UI；09 层的界面证据和局限记录在上方。
测试使用隔离临时状态与 fake provider，未连接真实 provider 账号、未发出计费模型请求。
版本矩阵是上游协议核实；未宣称在原生 macOS 或真实移动设备上运行本层测试。
集成门禁和后续增强的实际结果在完成后追加，不把未运行项目记为通过。
