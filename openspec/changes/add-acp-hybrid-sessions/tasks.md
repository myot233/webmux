## 0. 前置

- [x] 0.1 归档在途变更 `add-program-status-signals`：它的任务已全部完成，代码已合入。归档后 `openspec/specs/program-status` 才存在，本变更的 `specs/program-status` 修改才能归档。应单独提 PR，在本变更的第一个实现 PR 之前合入。验证：`openspec validate --all --strict` 通过，`openspec/specs/program-status/spec.md` 存在；对本变更执行 `openspec validate` 时，不再出现“target spec does not exist”的提示。

## 1. 真实 agent 基线（M0）

- [x] 1.1 在隔离 HOME、独立 webmux 实例中，抓取 `omp acp` 与 `opencode acp` 的 `initialize` 响应（协议版本、`loadSession`、`sessionCapabilities`、`_meta.steering`、`agentInfo`），以及 `session/new`、`load`、`resume`、`list`、`close` 的实际行为，写入本目录下的 `baseline.md`。验证：`baseline.md` 中每个档案字段都有对应的实测记录；与 design 决策 9 不一致的地方已回改 design 与 specs。
- [x] 1.2 以实际发行版（而非源码）的 `--help` 输出与实测，确定两个档案的白名单参数、参数个数、会话类参数、ACP 透传参数与“不可勾选”参数（OpenCode 位置目录，omp `--session-dir`/`--no-session`），写入 `baseline.md`。对照记录 OpenCode 2.1.4 与 2.2.0 的差异。验证：每个参数都有一条“保留 / 丢弃 / 透传 / 不可勾选”的实测结论。
- [x] 1.3 验证 bashrc 包装下的 fd 重定向写法（`exec -- "$@" 1>&3 3>&-`）：`.bashrc` 中有 `echo` 时，ACP 握手仍然成功，且 `.bashrc` 提供的环境变量对 agent 可见。验证：结果与命令记录在 `baseline.md` 中。
- [x] 1.4 验证两个 agent 在空闲时以进程组 SIGHUP 退出 TUI 后，会话完整可加载，并测量回合中先按中断键再退出的表现。实测 OpenCode 以 `--standalone` 运行时，回合中结束 TUI 会停止回合（私有服务随 TUI 退出），并且之后 ACP `session/load` 历史完整、没有残留运行中的工具调用。不成立时，按 design“风险”一节退回原方案，并修订相关 specs。另外实测 cwd 为符号链接路径时 `session/list` 的结果，以及 OpenCode 加载后模型回到默认值的判定方式（D25）。验证：结论写入 `baseline.md`，并据此确认 design 决策 9 的“退出方式”与 D25 规则。

## 2. ACP 客户端与假 agent（M1）

- [x] 2.1 新建 `internal/acp`（只依赖标准库），并在 `internal/archtest/layers_test.go` 中归为共享层。实现以下部分：按行成帧（32 MiB 上限）、ID 关联、单写协程、按序分发通知、在独立协程中处理入站请求、挂起调用在进程退出时失败、`session/cancel` 与 `$/cancel_request`。验证：`go test -race ./internal/acp` 覆盖超长行、非 JSON 行、乱序响应、取消、进程退出，以及“日志不含内容”的断言。
- [x] 2.2 实现 v1 子集的类型与方法：`initialize`、`session/new|load|resume|list|close|prompt|cancel|set_config_option`；`session/update` 的各种更新种类；`request_permission`；`elicitation/create`（form）；对 `fs/*`、`terminal/*` 返回方法不存在。验证：表驱动的编解码测试通过，未知更新种类被保留为原始值且不报错。
- [x] 2.3 在 `internal/acp/testdata` 中固定 `schema-v1.27.0` 的 `schema.json` 与 `meta.json`，新增守卫测试，核对所用方法名与字段名都存在于该 schema。验证：故意改错一个字段名时测试失败。
- [x] 2.4 编写测试用假 agent（Go 程序，位于测试数据目录，由测试与 E2E 构建）。它在临时目录中维护会话库，`tui` 模式下发出 OSC 7501 与 Warp 事件、响应中断键、支持 `--resume`/`-s`，`acp` 模式下实现上述方法子集，并能注入延迟、错误、崩溃与权限请求。验证：假 agent 自身的测试覆盖两种模式共享同一会话库。

## 3. 会话外壳、伴随进程与身份（M2）

- [x] 3.1 在 `internal/session` 中把外壳与运行分开，普通会话保持一对一。验证：`go test -race ./...` 全部通过，`go test ./internal/api -run TestWireSnapshots` 无差异，`BenchmarkAttachOutput` 与基线相比没有回退。
- [x] 3.2 增加 `CreateShell`、`StartRun`/`EndRun`、驱动接口与 `SetIDReserver`；切换导致运行结束时，以 4409 关闭终端连接，并以新代际启动下一次运行。验证：新增测试覆盖“运行结束不发布退出帧”“新运行的续接收到 reset”“`Manager.Close` 交给驱动”“保留 ID 不会被随机分配”。
- [x] 3.3 增加伴随进程：与 PTY 运行相同的环境、新进程组、经 `startChild` 登记、stdin/stdout 管道、有界 stderr 环，以及 bashrc 包装的 fd 3 写法；清理复用 `cleanupProcesses`。验证：Linux 测试证明 subreaper 不会抢走伴随进程的退出状态，关机时伴随进程被清理，`.bashrc` 中的 `echo` 不进入 stdout。
- [x] 3.4 在扫描器中提取身份字段（Warp 的 `agent`、`plugin_version`、`session_id`、`cwd`，7501 的 `app` 与子记录 `id` 首段），加长度和字符集校验，只存于内存，并提供 `Manager.Identity(id)`。验证：`osc_test.go` 与 `attention_test.go` 原有用例不变且通过；新用例证明提取不改变关注与状态，普通会话的 `Info` JSON 不含这些字段。
- [x] 3.5 在进程表中增加根进程可执行文件名与 cwd（Linux `/proc/<pid>/cwd`，macOS `PROC_PIDVNODEPATHINFO`）。验证：Linux 与 macOS CI 上，对在子目录中启动的进程断言 cwd 正确。
- [x] 3.6 在 `internal/contract`、`internal/wire`、`internal/hub` 中加入关闭码 4409 与可选的 `SessionInfo.hybrid`。验证：普通会话的快照不变，新增 `ws-terminal-moved` 快照；`docs/protocol/websocket.md` 的关闭码表增加 4409。

## 4. 档案与启动配置（M3）

- [x] 4.1 新建 `internal/agentprofile`（共享层），实现 OpenCode 与 omp 档案、命令写法识别、恢复 argv、ACP argv、身份映射与版本比较。验证：表驱动测试覆盖 specs `hybrid-agent-profiles` 中的全部场景，以及两种参数写法与未知参数。
- [x] 4.2 在 `launch.Config` 中增加 `hybrid`（`omitempty`），保存时校验，并计算 `hybridSupport`；新增 `POST /api/launch-configs/hybrid-support`。同步 `contract`、`wire`、`schemas.ts` 与快照。验证：`internal/launch` 与 `internal/api` 测试覆盖 400、旧文件读取不改写、摘要字段；快照已重新录制并评审。
- [x] 4.3 在 `internal/statefile` 中登记 `hybrid` 描述符（`hybrid-sessions.json`，故障策略 `unavailable`），并更新 `persistence_inventory.json` 与 `docs/persistence.md` 的清单表。验证：`make persistence-check` 通过。

## 5. 混合会话服务：聊天端（M4）

- [x] 5.1 新建 `internal/hybrid`（Service），实现每会话 actor、记录存储（上限 200、1 秒去抖写盘、启动时检查残留并改为 interrupted）、经 `workspace` 的配置创建路径与 `startSide`。验证：`go test -race ./internal/hybrid` 覆盖创建、记录内容不含环境变量值、存储损坏时 503、普通会话不受影响。
- [x] 5.2 实现从聊天开始：伴随进程启动、`initialize`、`session/new`、规范化事件转换（design 决策 4）、按字节计量的事件日志、代际与序号。验证：用假 agent 驱动的测试断言事件序列、`toolCallId` upsert、超长字段截断、日志截断后 `from` 前移。
- [x] 5.3 实现挂在 `/api/sessions/{id}/chat/` 下的 REST 命令与 `/ws/s/{id}/chat`：发送、队列操作、取消、请求答复、配置项；`clientRequestId` 去重；错误映射写入 `api/errors.go`。验证：`internal/api` 测试覆盖认证、Origin、409 `answered`/`switching`、续接只补缺失部分、慢客户端 4408；新增 `hybrid` 快照场景。
- [x] 5.4 实现队列语义（自动发送、暂停与继续、编辑、上移、立即发送）和插话能力开关。验证：测试覆盖 specs `chat-prompt-queue` 的全部场景，其中 OpenCode 档案不暴露插话。
- [x] 5.5 把聊天端回合状态映射到 `Info.Status`、`Info.Attention`、`Info.Preview`，并在切换时清除关注。验证：`push` 的现有测试不变且通过；新增测试断言出现权限请求时会产生 `permission` 推送。

## 6. 切换、资格与异常（M5）

- [x] 6.1 实现 TUI→ACP 切换：中断键、等待空闲、10 秒超时后的选择、进程组退出、`session/load`、按 D25 补齐配置。实现 ACP→TUI 切换：取消、退回队列、`session/close`、恢复 argv。验证：用假 agent 驱动的测试覆盖两种场景的往返、回合中切换，以及进程观察器断言任何时刻至多一个写者。
- [x] 6.2 实现资格判定（五个条件、去抖的 `session/list` 探测、并发上限 2）、ID 变化提示与跨会话占用检测。验证：测试覆盖 specs `agent-session-identity` 的全部场景。
- [x] 6.3 实现回滚、TUI 自行退出转为 interrupted、ACP 崩溃后重连一次、`auth_required` 引导、空闲休眠与唤醒（`settings.json` 新增 `chatIdleSleepMinutes`，`hybrid` 经 `cmd/` 注入并订阅设置变更，设置不可用时取默认值 30），以及关机顺序。验证：故障注入测试覆盖 `load` 失败、回滚失败、SIGKILL、休眠条件、唤醒时 `resume`/`load` 的选择，以及设置变更立即生效。
- [x] 6.4 实现 `POST /api/sessions/{id}/resume` 与 `GET/DELETE /api/hybrid-sessions[/{id}]`：`session/list` 核对、候选列表、`config_missing` 确认、沿用原 ID、移除已退出的外壳。验证：测试覆盖 specs `hybrid-session-recovery` 的全部场景，包括重启后恢复到两端。
- [x] 6.5 在 `internal/attach` 与 `internal/sessioncmd` 中处理 4409：被切走时恢复终端、输出提示并以退出码 0 结束（被动分离）；聊天端拒绝附着，退出码 1；`webmux ls` 显示所在端；`webmux run --launch-config` 创建混合会话。验证：`internal/attach` 与 `internal/sessioncmd` 测试覆盖 specs `terminal-session-cli` 的全部场景。

## 7. 前端（M6）

- [x] 7.1 新增聊天 schema（由 `schemas.ts` 统一导出）、`chat-model.ts` 纯 reducer、`hooks/chat-socket.svelte.ts` 与 `hooks/hybrid.svelte.ts`，复用 `ws.ts` 的重连约定；`ws.ts` 新增终止状态 `moved`，收到 4409 不重连。验证：Vitest 单元测试覆盖 reducer 的 upsert、未知 `toolCallId` 的占位、重置与截断，以及 4409 不触发重连；`pnpm --dir web check` 通过。
- [x] 7.1a 修订草稿清理：可恢复的混合会话（来自 `GET /api/hybrid-sessions`）视为存在，4409 不视为会话不存在，移除记录时删除草稿。验证：浏览器测试覆盖 specs `terminal-draft-persistence` 中修订后的三个场景。
- [x] 7.2 把 PR #186 的原型组件提升到 `web/src/components/chat/`，接入 `pages/Terminal.svelte`，实现按所在端渲染、切换控件、进度、资格原因、D21 选择框、退回草稿与 ID 变化提示；原型 stories 改为引用正式组件。验证：浏览器测试与 Storybook 布局矩阵（6 种视口 × 明暗主题）通过，`pnpm --dir web check:arch` 无新增违规。
- [x] 7.3 实现首页混合会话卡片与“可恢复”分组、新建会话的起始端选择（按浏览器记住）、设置页的混合模式勾选框与 `hybrid-support` 去抖查询，以及聊天端空闲休眠设置。验证：浏览器测试覆盖 specs `workspace-overview`、`launch-configs` 的新增场景。
- [x] 7.4 新增 Playwright E2E（桌面与 Pixel 7），使用假 agent：从两端开始的往返、回合中切换、权限答复的多设备一致、排队与停止、休眠唤醒、重启后恢复、`webmux attach` 被 4409 断开。验证：`pnpm --dir web exec playwright test test/e2e/hybrid.spec.ts` 通过，CI 的 Ubuntu 与 macOS 分片通过。

## 8. 文档、用户故事与验收（M7）

- [x] 8.1 更新 `docs/architecture.md`（非目标、会话外壳/运行、混合会话生命周期）、`docs/protocol/`（`/ws/s/{id}/chat`、`/api/sessions/{id}/chat|switch|resume`、`/api/hybrid-sessions`、4409、被切走时退出 0）、`docs/usage.md`（混合模式、占用风险、资格探测会短暂启动 agent、OpenCode 以 `--standalone` 运行、版本提示、回退前取消混合勾选）与 `docs/deployment.md`。验证：`make docs-check` 与 `make format-check` 通过。
- [x] 8.2 在 `docs/user-stories/` 新增混合会话分组的故事（创建、切换、恢复、本地接入、聊天、队列），修订 B、C、D09/D10、M01/M03、L10 及两处索引与计数。验证：故事 ID 唯一，索引引用可解析，来源路径存在。
- [x] 8.3 用真实 OpenCode 与 omp，在隔离 HOME 中按 issue #181 的验收要点逐条验收，结果追加到 `baseline.md`；为 PR 截取桌面与手机、亮色与暗色、权限、排队、压缩、切换中的真实截图。验证：`baseline.md` 中逐条记录通过或未通过；`make test` 全部通过。
