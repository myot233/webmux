# Design

## Context

动机见 [proposal](proposal.md)。本设计基于 `2eaf0d66`，区分现有行为与拟议变化，不把设计 PR 当作功能发布。

- `internal/clicmd/turn_stream.go` 和 `internal/mcpcontrol/wait.go` 各自解析同一个聊天 WebSocket，均接受 64 MiB 回放帧。
- CLI 保存回合事件、按消息 ID 拼接回复并在展示时 trim；MCP 只返回最后一条 agent 回复，限制为 64 KiB，保留分块空白。
- CLI `queuedGone` 在看见队列项后检测删除；MCP 只在初始回放后检查队列，后续删除会误等到超时。
- MCP 能利用指定回合的结束事件或当前状态恢复结束判断，并返回 `unfinished`；CLI 目前没有同等完整性和后台工作表达。
- `streams.go` 与 MCP `waitExit` 各自维护会话列表；前者还有多会话、changed、stable、any 等 CLI 策略。
- `mcpstdio.Server.Serve` 同时持有注册表、握手、分派和串行逐行循环；控制工具及 `threadread` 都引用其类型。
- 主规格 `shell-chat-control` 仍有旧 `chat send/switch` 示例；在途 `unify-session-cli` 已描述统一入口。此处不恢复旧命令，指南层按现有命令表修正受影响要求。

## Goals / Non-Goals

**Goals:**

- 一套回合、队列和退出判断服务 CLI 与 MCP，迁移后不保留旧路径或兼容别名。
- 一套 MCP 工具注册和 JSON-RPC 分派支持 stdio 与后续 HTTP，控制和冻结历史注册表始终分开。
- 不增加 MCP 无需返回的事件复制；不为抽象引入后台 goroutine、持久存储或通用事件总线。
- 保留各入口已有输出、资源和超时契约；有意行为修复由规格与回归用例明确标记。

**Non-Goals:**

- 不实现网络监听、认证、指南读取、CLI 新字段或新的控制工具；这些由上层变更负责。
- 不让 CLI 通过 MCP 操作服务，不把 REST 改成 MCP，不追求全部 CLI/MCP 功能对等。
- 不合并有界回放与持久 activity，不用另一数据源悄悄补齐丢失历史。
- 不将客户端规则当作权限，不重做服务端去重，不改变持久会话与进程生命周期。

## Decisions

### 1. 两个共享内核，而不是一个通用框架

```text
CLI adapter             stdio adapter              HTTP adapter (later)
     |                        |                            |
     |                        +-------------+--------------+
     |                                      |
     |                              mcpprotocol
     |                                      |
     |                              mcpcontrol adapter
     |                                      |
     +-------------------+------------------+
                         |
                  sessionobserve
                         |
               localclient REST / WS
                         |
                 authoritative API
                         |
               session / hybrid / workspace

threadread --> separate mcpprotocol registry --> frozen history Reader
```

`sessionobserve` 属客户端层，只依赖 `contract`、`localclient` 和 WebSocket；不导入 CLI、MCP、服务或控制器。
`mcpprotocol` 属共享基础设施层，只依赖标准库与 `contract` 的协议 DTO，不依赖任何工具实现或网络传输。
`mcpstdio` 保留客户端层；未来 `mcphttp` 属控制器层，由 cmd 注入 HTTP handler，不直接构造服务。
新增包登记到架构清单；不为反向依赖添加例外。

不增加名为“公共业务层”的万能 facade：创建、发送、中断仍由同一 REST API 执行，内核只收敛已经重复的观察语义。

### 2. 具体订阅类型与结果，不暴露任意 predicate

`sessionobserve` 提供 `ChatSubscription` 和 `SessionSubscription`，每个对象只有一个读取者，调用方负责 `Close`。
构造函数接受已有 API client 与明确的读取上限；不自行选择 socket、凭据或调用者身份。

聊天订阅分成 `Prime(ctx)` 与 `WaitTurn(ctx, selector, policy)`：

- `Prime` 读取初始 snapshot 与紧随的 replay，保留定位所需事件；后续 `WaitTurn` 先消费保留事件再读实时帧。
- CLI 在发送之前 Prime；选到输入回执后再等待，已经回放的旧回合不替代新提示。
- MCP 在拿到 selector 后附加观察；Prime 中已完成的目标回合可以直接返回。
- Prime 不在读到 snapshot 后就丢弃 replay，不把构造订阅、发送与注册观察拆成有空窗的轮询。

选择器是封闭的四类：`TurnID`、`PromptID`、`QueueItemID`、`FirstTurn`，每次只能选一类。
`steer` 在 CLI 已确认的当前 turnId 上等待；不增加能跨回合匹配的任意回调。
策略只保留实际差异：提交前/提交后附加、遇请求停止/继续、回复字节上限、是否收集事件。

内核结果不带协议 JSON tag：

```text
TurnObservation
  TurnID, QueueItemID
  Phase: pending | queued | running | waiting_for_user | ended
  StopReason, Error
  Reply, ReplyTruncated, HistoryIncomplete
  Unfinished
  Requests[]
  Events[] (only when requested)

Wait return: (latest observation, error)
  errors: deadline / cancelled / disconnected / invalid frame /
          queue removed / selected history unavailable
```

deadline 时保留最后观察结果，不能合成 `ended`、stopReason 或 exitCode。
CLI 映射为 124，MCP 映射为部分结果加 `waitTimedOut=true`；取消与断线维持错误，不转成等待超时。
MCP 的 `timeoutMs=0` 是策略：最多十秒接收初始回放，随后立即返回，并非零 deadline。

### 3. 回放、队列和请求的确定性顺序

```text
before-submit: unseen --> present --> started --> running --> ended
                           |
                           +--> absent --> queue-removed error

after-submit: prime replay --> matched turn / present queue
                          \-> neither --> history-unavailable error
```

每帧先应用目标回合事件，再判断队列消失，避免同帧 turn.start 与移除队列项被误判为删除。
提交前初始 absent 不代表删除；提交后初始 absent 且没有匹配回合不能证明删除，也不能继续等任意新回合。
一旦 matched turn.start，队列消失属于正常发送，不再执行 queue-removed 判断。

- 明确 turnId 的结束事件可单独确认结束；没有 start 时 `ReplyTruncated` 为真。
- 使用匹配的当前状态恢复正在运行或已结束回合时，保持历史不完整标记；不得因后续完整 chunk 清除它。
- `FirstTurn` 遇到 truncated 或 from>1 明确失败，不能把后来的 turn 当作首回合。
- 目标 turn.end 的 stopReason、error、unfinished 优先于较旧 snapshot；回合结束不是后台工作全部完成。
- replay 内已 open 又 resolved 的请求不触发“等待用户”；读完整帧后再决定是否仍有未解决请求。
- requests 仅归于已匹配回合；等待期间出现其他回合不会改变目标。

### 4. 回复累积和事件收集保持有界选择

内核按 agent message ID 合并文本，保留原始 chunk 空白，不 trim，不导出 thought；CLI 在现有输出边界执行 trim。
MCP 设置 64 KiB 回复上限，UTF-8 字符只保留完整前缀；`ReplyTruncated` 对字节上限和历史缺失都保持单调。
CLI 保留现有回合事件收集语义，MCP 关闭 `CollectEvents`，不会先构建全量事件再删掉。
内核不积累无关回合的正文；每个订阅的回放/输出上限单独生效，不把传输上限错当成模型回复上限。

### 5. 会话列表只共享事实应用与退出判断

`SessionSubscription` 统一 snapshot、created、updated、removed 的应用，以及目标不存在、已移除、已退出的判断。
CLI 的多会话条件、changed 基线、stable 计时、any 聚合留在现有 CLI 策略代码，改用共享订阅读取。
MCP exit 模式只用单会话退出判断；返回进程 exitCode，而不是把 wait 调用本身的成功当作进程成功。
CLI 保留 64 MiB 列表帧上限，MCP 保留 4 MiB；不因共享而扩大任何一端资源边界。

### 6. MCP 协议机制与工具注册分离

`mcpprotocol` 定义 `Tool`、不可变 `Registry`、每连接的 `Session` 和 `Dispatch(ctx, message)`。
Registry 在构造时拒绝重复名称或无 handler 的工具；`Session` 只持有协商版本与 initialized 状态。
握手状态变更短锁保护，工具调用在锁外执行；不在共享内核实现全局队列或串行工具锁。

- stdio adapter 按原顺序扫描与编码，保持 64 KiB 帧上限、stdout 仅协议及原有串行执行行为。
- 协商版本继续接受 `2024-11-05`、`2025-03-26`、`2025-06-18`、`2025-11-25`；未知版本返回现有基线，现代 discover 仍为 MethodNotFound。
- 分派保持 tools/list、tools/call、ping、JSON-RPC 错误与 `isError`/`structuredContent` 结果；工具 schema 仍由 mcpcontrol 构造。
- 调用 deadline 由 registry/server 配置：控制工具 185 秒，冻结历史 10 秒；HTTP 自己选择允许协商的版本子集。
- `mcpcontrol` 和 `threadread` 同一提交迁移为新协议类型，删除 `mcpstdio.Tool/Server` 的旧所有权；不保留 re-export 或 type alias。
- frozen Reader、工具名、来源冻结点、可读取 conversation 均不改变；控制 registry 不出现 `thread_read`。

协议请求、响应、初始化结果与工具结果外壳的 wire DTO 放在 `internal/contract`，保持当前字段与空值表示。
Registry 持有 handler，不把 handler/context/锁放入 wire DTO；未来 HTTP 控制器不以匿名 JSON 或架构豁免绕过线上类型规则。

### 7. 入口差异是明确契约

| 边界       | CLI                         | MCP                        |
| ---------- | --------------------------- | -------------------------- |
| 等待时长   | 默认无限，用户可设 duration | 默认 30 秒，最大 60 秒     |
| 超时结果   | 退出 124                    | 部分结果，waitTimedOut     |
| 回复展示   | 原有 trim 与文本/事件 JSON  | 原始空白，最大 64 KiB      |
| 待回答请求 | 默认停止，可 auto-wait      | 返回请求，不批准           |
| 历史读取   | chat log 有界 replay        | activity 持久已提交历史    |
| 身份来源   | 本机连接规则                | 本机 stdio；远程由上层认证 |

共享的判断只证明指定回合/进程的状态；不替用户判断任务验收标准。

## Risks / Trade-offs

- [提交前后附加时序不同] → 显式 AttachPhase 策略，并覆盖 initial absent、present 后删除、start 后消失三条路径。
- [状态 fallback 误取其他回合] → 只接受精确 selector，缺失明确失败，不调用持久 activity 猜测目标。
- [共享抽象增加内存] → 不收集无关正文，MCP 关闭事件收集，回复上限在追加时执行，不在完整复制后裁剪。
- [提取协议扩大冻结历史权限] → 每个 server 单独构造不可变 Registry，保留 Reader 自身范围检查和跨 conversation 拒绝。
- [旧规格与当前命令漂移] → 不以旧示例复活命令；指南变更仅修正与本次语义相关的要求，保留未涉及的规格历史。

## Migration Plan

设计 stack 为“核心 → 指南 → 网络”，每层各自带 proposal、design、specs、tasks，均只提交规划文件。
后续实现 stack 的合并顺序如下；每层迁移其全部消费者、带回归与真实 smoke，不提交不可用骨架：

1. **observe**：引入观察层并同时迁移 CLI/MCP；修复队列删除和缺失回放，更新 US-S23。整层完成后旧等待判断消失。
2. **protocol**：提取协议内核并同时迁移 stdio 控制与冻结历史；已有 stdio 客户端可继续使用，不提供网络端点。
3. **guide**：共用核心规则、指南工具、CLI unfinished 输出及六项修正；沿用上两层接口。
4. **network**：认证 TCP API client 与完整 HTTP MCP 生命周期同层交付，入口默认关闭；不先注册未实现端点。

observe 与 protocol 没有技术上的相互依赖；为保持线性 stack，按此顺序审阅，不声称二者存在业务依赖。
回滚 observe/protocol 只需回退对应提交并重建，不迁移磁盘格式；上层代码必须随依赖一起回退。
网络层关闭配置即可停止新连接，不删除现有 WebMux 会话。

## Verification Plan

共享状态机用确定性的 frame 序列覆盖 selector、同帧顺序、回放缺失、请求解决、后台状态和队列删除，断言可见结果而不是内部布局。
CLI/MCP 适配层只验证不同的输出/错误映射；已有行为测试迁移，不保留两份同路径状态机测试。

实现时用隔离实例和 fake agent 实际运行 CLI 与 stdio MCP：排队后从另一客户端删除、指定 turn 结束、后台工作未结束、进程退出码 7。
观察两个入口均及时结束并指向同一目标，超时/断线后会话仍存在；不连接开发者已有服务。
协议 smoke 完成 initialize → initialized → tools/list → tools/call，并分别验证控制 registry 无 thread_read、冻结 registry 无控制工具。
执行相关包 race 测试、archtest、docguard 与 CLI 帮助引用检查，记录实际执行范围，不把设计检查当运行验证。
