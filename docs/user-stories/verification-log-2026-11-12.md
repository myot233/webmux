# 核对记录：2026-11-12 起

本文记录本机客户端响应处理与测试环境隔离的局部核对。前一份综合记录见 [verification-log-2026-11-06](verification-log-2026-11-06.md)，后续综合记录从本页追加。

## 行为与故事范围

- US-M08：普通 JSON 请求与 multipart 上传共用响应处理；调用方忽略响应内容时，响应截断或读取失败仍返回错误，不再误报成功。
- `Get` 与其他请求统一保留非成功状态及最多 64 KiB 原始错误体，支持读取结构化失败信息；不改变服务端协议或状态码。
- US-L02：Codex 混合会话夹具显式清空宿主 `CODEX_PATH`，从夹具自己的 `PATH` 查找假 agent；往返测试设置无效宿主路径覆盖此回归。
- 更新现有故事的验收要点与源码依据，不新增故事或改变两个功能索引的范围及计数。

## 复现与回归证据

- HTTP 修复前，普通请求和上传的截断响应测试返回 nil；`Get` 的结构化、纯文本与超限错误体均为空。
- 新增 22 个真实 HTTP 传输场景，覆盖截断、连接复用、JSON 解码，以及各请求方法的结构化、纯文本、恰好 64 KiB 与超限错误体。
- 未修改的主分支上，四个 Codex 混合会话用例在继承宿主 `CODEX_PATH` 时失败；仅清除该变量后全部通过，确认测试未隔离宿主路径。

## 执行与限制

- 已运行 `go test -race ./internal/localclient ./internal/docguard ./internal/archtest`，固定版本 `make go-lint` 零告警。
- 原工作区依赖版本与锁文件不一致；隔离工作树重新按 frozen lockfile 安装后，`make check` 全部通过。
- 测试使用隔离连接，不连接现有开发服务；未在原生 Windows 或 macOS 运行。
- 完整 Go race 与 PR CI 的最终执行结果记录在 PR 验证说明中，不以局部验证代表全量通过。

## 2026-11-13：MCP 发现入口（US-S20）

- `webmux mcp` 复用 CLI 连接规则，提供能力发现、筛选分页和会话详情；交接历史工具仍保持独立。
- 已运行 MCP 控制/协议/历史、clicmd、架构与文档守卫的 Go race 测试；全部通过。
- 已按 frozen lockfile 安装前端依赖并完成构建，Svelte 检查为零错误、零警告。
- 测试使用隔离传输；未连接用户运行中的 WebMux，也未在真实第三方 MCP 客户端或原生 macOS/Windows 验证。

## 2026-11-13：MCP 会话操作（US-S21）

- 新增创建、输入、中断工具，复用请求 ID 与现有本机 API；创建失败保留已创建资源的结构化文本。
- 已运行 MCP 控制、本机客户端、clicmd、文档与架构守卫的 race 测试，全部通过。
- 测试覆盖重试不改请求、排队回执、非法身份及创建部分失败；未在第三方 MCP 客户端执行真实 agent 任务。

## 2026-11-13：MCP 增量读取（US-S22）

- Activity 读取过滤非导出项且保留原始游标；终端回放分页返回原始字节及重置/淘汰信息。
- 真实原始输出 API 与服务测试覆盖淘汰后的后缀、分页、过期游标重置、超过 4 MiB 的历史读取一字节页，以及不附着或取得控制。
- 活动分页与非法游标测试使用隔离传输，不读取已有开发会话。
- 已运行 `go test -race ./internal/mcpcontrol ./internal/clicmd ./internal/localclient ./internal/archtest ./internal/docguard`，全部通过。

## 2026-11-13：MCP 回合等待（US-S23）

- 指定回合、排队项或首回合等待，超时只结束观察，待回答请求返回给调用方；终端仅以进程退出作为退出等待的完成条件。
- 已运行 MCP 控制、clicmd、本机客户端、架构和文档守卫的 race 测试，全部通过。
- 测试覆盖旧回合排除、队列到回合关联、缺失历史、UTF-8 回复上限、待回答请求和超时；不连接已有服务。

- 机器人审查回归：turn.end 自带结束时的后台工作标记，不读取尚未更新的旧 state；丢失回合起点后的后续消息保持回复不完整标记。
- 真实独立 WebMux 二进制与 fake agent 的 MCP stdio 验收通过：八工具发现、创建去重、终端分页与退出码 7、首条/追加聊天回合及六项持久历史。测试实例已关闭。
- 真实 OS 管道取消回归通过；CLI 子进程在 stdin 打开时响应 SIGINT/SIGTERM，Windows 客户端交叉编译通过。
- 空白提示和空初始化指令不返回虚构等待选择器；创建 API 的 initialInput 只携带实际提交回执，相关服务和 wire 快照已验证。
- 五 MiB 无关聊天回放仍可等待目标短回复；传输上限与 64 KiB 回复上限分别约束。

## 2026-11-13：P2P 诊断集成验收（US-S24）

- 合并 main 时保留 MCP 命令与既有 US-S20–US-S23，P2P 诊断使用未占用的 US-S24；主索引、覆盖索引与原验证记录同步。
- `make check` 通过，前端类型零错误/警告、架构守卫与固定版本 Go lint 零问题；完整 Go race 与 866 项前端单元测试通过。
- `nop2p` 的 P2P、wire、clicmd、API 与主命令包 race 通过；普通和 `nop2p` 独立二进制均通过真实服务、会话内 CLI 及本地 UDP4/UDP6 STUN 验收。
- 覆盖实际 ICE UDP 绑定、有效配置、空 peer、会话内只读状态、原控制接口 403、保留 MCP 命令、离线显式探测及服务不可用/整体超时退出码。
- 首次浏览器套件的既有图形历史测试超时：图片移出视口不代表全部异步输出已解析。夹具改为等最终输出标记再搜索，保留图片恢复、清除与资源释放断言及原超时上限。
- 修正同步点后，完整浏览器套件 731 项通过，Storybook 75 项通过，E2E 472 项通过/48 项跳过，独立性能套件 18 项通过/2 项跳过；复用此前通过的 Go race 与单元结果，不将首次失败的 `make test` 记为一次全绿。
- 验收使用隔离 HOME/XDG、状态目录和本地 STUN，不访问或重启现有服务；未验证外网 NAT 穿透、物理触屏或 macOS/Windows 实机。

## 2026-11-13：P2P 探测超时退出码竞态（US-S24）

- #562 合并后的 main CI 中，`TestP2PProbeCancellationAndTimeout` 收到退出码 1 而非 124；本地及 PR 的先前成功不覆盖该次失败。
- UDP socket 截止时间可能先于 context 计时器发布 `Err()`。CLI 改为直接比较整体绝对截止时间，避免将整体超时误报为普通探测失败；取消、单目标失败与既有时限不变。
- 原超时/取消测试在 race 与跨包 atomic coverage 下连续 100 次通过；clicmd 完整 race、`nop2p` race、vet 与固定版本 Go lint 通过（0 issues）。
- 实际编译的 CLI 对隔离静默 UDP 端口连续 10 次以 30ms 超时运行，均输出失败 JSON 并返回 124；未访问运行中的服务，临时二进制已清理。

## 2026-11-13：工具展开动效用例同步（US-L02）

- 修复后的 main CI 中，Storybook 工具展开动效用例首次失败、重试通过；fail-on-flaky 门禁保持失败，未据此发版。
- 用例在切换手机视口与减少动态效果后立即执行物理点击；补齐 ResizeObserver 后续帧与媒体变更处理的渲染同步，并确认点击前仍为展开状态。
- 保留原动画中间帧、反向展开、320px 溢出、物理点击、减少动态效果与子节点清理断言；不修改产品 UI、超时、重试配置或 fail-on-flaky 门禁。
- 本地六 worker、零重试：该场景 20 次通过，完整 Storybook 75 项通过；Storybook 类型检查零错误/警告。纯测试同步修正，不改变产品故事契约或索引计数。

## 2026-11-13：ACP 默认注入控制 MCP（US-S20）

- ACP 的 new/load/resume 默认携带当前实例的 `webmux` stdio 配置；历史保存关闭不影响控制工具，交接时保留独立的冻结历史服务。
- `make check` 通过；主命令包、`internal/hybrid` 与 `internal/acp` 的全量 race 测试通过。主命令包首次受宿主 XDG 状态影响的隧道用例失败，清除宿主环境后全量重跑通过。
- 默认配置、探测隔离与交接集成的定向 race 测试通过：独立 fake agent 在 new/load/resume 发现全部八个控制工具并成功调用 capabilities；关闭会话后控制及历史 helper 均退出。
- 配置测试覆盖缺失可执行文件、无执行权限、缺失或未持有的 socket 和非 socket 文件；未修改 agent 全局配置或调用真实 provider。
- CI 首次未通过源码文件规模守卫（main.go 为 509 行）；按重启恢复职责拆到独立文件后，完整架构与文档 race 守卫通过，Go lint 零问题。
- 发布集成同步 main 的 #575 时保留两侧验收记录；清除宿主 CODEX/XDG/WebMux 定位环境后，组合源码的主命令包、hybrid 与 acp 完整 race 再次通过。

## 2026-11-13：CLI/MCP 共享观察内核（US-S23）

- `sessionobserve` 统一聊天/会话订阅、指定回合、队列移除、请求解决、历史完整性与退出事实；CLI 的 US-S09/S14 状态策略与结果外壳保持不变。
- 已运行 `go test -race ./internal/sessionobserve ./internal/clicmd ./internal/mcpcontrol ./internal/archtest`，全部通过。
- 独立二进制、隔离配置/状态与 fake agent：`run --reply --json` 返回首回合，CLI 10ms 超时返回 124 后回合仍运行；stdio MCP 随后按 t2 取回对应完成回复。
- MCP 的真实 q1 关联到 t4 而非正在运行的 t3；CLI `run --follow --strip -- sh -c 'printf observer-exit; exit 7'` 输出正文并返回 7。
- 另一个客户端删除 CLI/MCP 已观察到的队列项：两者及时报 queued input removed，CLI 返回 1；原会话仍运行，不中断执行。
- 评审发现正常 `forwards` 广播被误拒绝：新增回归先失败，再将其作为会话事实的 no-op；观察、CLI 与 MCP 的完整 race 通过。
- 真实实例在 CLI/MCP 同时等待退出时添加转发：两端均继续等到目标退出并保留 exitCode=7，不再被无关端口转发变更打断。

## 2026-11-13：共享 MCP 协议与 stdio 迁移（故事行为不变）

- 控制 MCP 与冻结历史改用共享 Registry/Session，stdio 只保留逐行 framing；移除旧 Tool/Server 所有权，不保留别名。
- `mcpprotocol`、`mcpstdio`、`mcpcontrol`、`threadread`、`clicmd` 与 `archtest` 的完整 race 测试通过；阻塞工具调用不持握手状态锁，stdio 仍顺序执行。
- 冻结历史跨来源、冻结点与 provider reopen 的真实子进程集成回归 `TestHandoffMCPReadsOmittedHistoryAndSurvivesProviderReopen` race 通过。
- 独立构建二进制与隔离 fake agent：控制 stdio 以 2024-11-05 初始化，列出八个控制工具并成功调用 capabilities；未知工具返回错误后 ping 仍成功。
- 同实例创建真实冻结预览，再启动 `mcp-serve` 以 2025-11-25 初始化，只发现 thread_read，实际分页取回冻结位置 26 前的消息与回复；控制入口没有 thread_read，历史入口没有 session_launch。
- US-S20、US-S23 与冻结历史故事的权限、超时和可见字段保持不变。

## 2026-11-13：共享操作指南与 unfinished（US-S05/S06/S08/S09/S11/S14/S20–S23）

- `agentguide` 共享六项操作语义，控制 registry 提供 instructions/capabilities/guide_read 的同一工具快照；精选七主题，严格参数与完整编码 64 KiB 边界，冻结历史工具集不变。
- 观察、指南、MCP、CLI 与架构的 race 测试通过；主命令包首次因长 TMPDIR 超过 Unix socket 路径限制失败，改用短的真实临时目录后完整 race 通过。符号链接不足以修复该夹具的 canonical path。
- 独立二进制与隔离 fake agent：end_turn+unfinished=true 的 JSON 保留原对象/events、退出 0 且 stderr 告警；正常回合为 false，既有待请求/队列移除/超时 124/进程退出 7 已实际覆盖。
- 真实 stdio 发现九工具，guide_read 的七主题均可读；null topic、未知主题、任意路径参数隔离失败，后续 ping 成功。TCP llms 实际页面没有实例 Token、会话 ID 或路径。
- 重复创建使用同一逻辑 ID 得到同一 session；重复终端输入 ID 只输出一次标记。另经真实 WebSocket query 读取 retained submission，状态为 written。
- 非 Codex 的 omp 先发现 model 的允许值，再实际应用 smart；权限 ask 模式读取当前请求，以 optionId=allow 回答 kind=allow_once，不用 kind 替代，不扩大权限模式。
- 普通 kill 保留标记、rm 清理；混合 archive/resume 保留同一路径，forget 清理；归档标记经真实服务重启仍保留，随后 forget 清理。记录淘汰与目录 sweep 的定向 race 回归通过。
- 实际 CLI 拒绝配置 ID 而接受名称：示例、帮助和设计已据此统一为 CLI 名称/MCP API 配置 ID，不改变现有命令契约。
- Codex 审查发现 launch 主题仍遗漏配置 ID 示例，现改为真实名称并明确 MCP/API 才用 ID；隔离注册仓库与 fake omp 的 `launch-config options "Opaque Guide Name" --json` 实际返回 mode/model 及 fast/smart 选项，渲染后的 launch 指南同步生效。
- 栈顶 `make test-fast` 与完整 `make test` 通过；该指南 PR 的独立 CI 亦通过，包括 Go race、四个 E2E 分片、构建及打包检查。

## 2026-11-13：认证网络 MCP 与跨入口验收（US-S25）

- 默认关闭的 `/mcp` 只挂到既有 TCP listener；配置 owner Token，保留 REST/WS 的 Host、Origin 与认证边界，不开放 Unix/P2P、OAuth 或冻结历史工具。
- 官方 MCP Inspector 2.15.0 Web 仅配置实例 URL、Authorization 和 legacy 代际，以 Streamable HTTP / 2025-11-25 连接；客户端配置没有 WebMux command 或 socket。
- 实际 UI 完成 initialize、九工具发现、capabilities、guide_read、创建、发送、活动/终端分页、指定 turn/queue 等待和显式 interrupt；q1 最终关联 t2，不误取忙碌 t1。
- Inspector 重试原创建/发送 ID 返回原 session/turn；终端等待保留 exitCode=7，活动 UTF-8 分页与 terminal generation/offset 续读通过。
- 同一 MCP 会话长等待期间列表在 1ms 返回；11 秒观察超时正常响应，未被十秒 body 读取期限打断，随后原 selector 仍获得正常 end_turn。
- 实际 notifications/cancelled 返回 request_cancelled，任务继续；丢弃观察 TCP 连接后在途 ID 仍保留到任务完成，失败投递后释放。DELETE 等待观察清理、使 MCP ID 返回404，不中断业务 turn。
- 创建/发送已提交后故意丢弃 HTTP JSON 正文，再以新 RPC ID、原业务 ID/负载重试：只创建一个带专有标记的会话，终端消费者只收到一次输入。
- 服务进程实际运行在另一个 WebMux PTY 内：HTTP capabilities/guide self 为 null，远程创建无隐式 parent；本机 stdio 子进程仍由真实 Unix peer 归属 parent，未知指南 self 保持 null，不新增 PID 发现。
- 真实重启后旧 MCP session404；重新初始化后原 conversation 与活动 cursor 从2续到11，读到原回复。旧 turn 明确报 retained history unavailable，不将传输重建冒充 turn 恢复。
- 对照真实 stdio/HTTP：九工具名称与完整 schema 相同，session_get 结果相同，capabilities 仅 transport/locality/instructions 不同；冻结历史的独立 stdio 与跨来源拒绝沿用已通过的集成回归。
- IPv6-only `[::1]:0` 实例实际完成 MCP 初始化、经本实例 TCP REST 创建/读取、经 WS 等待 exitCode=7；有效配置只暴露 mcp.enabled=true/source=toml，不含 Token。
- 无/错/重复 Bearer、query/Cookie/仅 MCP ID 均401；非法 Origin/Host 为403，Unix 与关闭/尾斜杠入口404。真实请求另覆盖415/406、batch/null ID/重复键400、超限413及 no-store，之后 ping 仍成功。
- loopback/wildcard 的 enabled+空 Token 启动均退出1，TOML 未改、未生成 Token 或 settings JSON；既有配置加载器仅创建空 lock 文件。
- 全部 Go race、866 单元、731 浏览器、75 Storybook、472 E2E（48跳过）、18性能（2跳过）随完整 make test 通过；make test-fast、格式、docguard、persistence、架构与 vet 通过。
- 固定 Go lint 2.15.0 最初因宿主二进制用 Go1.26 构建而不能加载 Go1.27.4；以项目 toolchain 重建同版本，修正两项静态提示后0 issues，传输层 race 再通过。
- 本地前端类型构建首次有五项 implicit-any 错误，未改无关前端源码；四个实现 PR 的 CI Format/check/build 均通过，Vite 和上述运行套件也通过。未验证 OAuth-only hosted clients、互联网代理部署或其他操作系统实机。
- #586→#587→#588→#589 的首轮 CI 全部通过；#588 的配置名称审查已按真实 CLI 发现结果修正。Pullfrog 工作流390651953为 disabled_manually，未运行、未获其 approval，未擅自更改仓库工作流启用状态。
