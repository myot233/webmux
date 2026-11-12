# Design

## Context

参见 `proposal.md` 中对三类完成通知误报与打扰场景的分析。当前架构中：

- `internal/launch` 持有启动配置（`Config`、`Summary`、`Input`），其中 `Preview *bool` 已提供了“可选指针字段表示跟随全局默认 vs 按预设显式覆盖”的成熟模式；
- `internal/session.Manager` 仅在启动时从 `session.Options` 读取全局 `ProgramStatus`、`AgentProtocol`、`BellAttention` 布尔值，所有会话共享同一套开关，且收到 `done`、`notify`、`bell` 时在 `raiseAttentionLocked` 中立即发布；
- `internal/hybrid` 在聊天回合结束（`stopReason == "end_turn"`）且无后台终端任务时直接调用 `publishAttention(..., KindDone, ...)`，尚无按会话过滤 `done`、校验最短回合耗时或延迟防抖的机制；
- `internal/archtest` 强制分层规则：控制器层（`api`、`wire`）与服务层（`launch`、`session`、`hybrid`、`workspace`）各自独立，前后端 JSON 契约集中在 `internal/contract` 与 `web/src/contracts/schemas.ts`。

## Goals / Non-Goals

**Goals:**

- 在不破坏现有 `launch-configs.json` 兼容性与全局 `config.toml` 配置的前提下，支持在每个启动配置上可选定制提醒策略（`attention`）。
- 在终端会话（`internal/session`）与混合聊天会话（`internal/hybrid`）中统一落实提醒范围（`mode`）、四层信号来源开关、最短回合耗时（`minTurnSeconds`）与完成确认延迟（`doneDelaySeconds`）。
- 在 `internal/session` 中通过主会话 `session_id` 校验消除 `omp` 等多代理工具的子代理 `stop` 误报（Issue `#189`）。
- 提供符合 `webmux-ui` 规范、清晰易懂的启动配置 UI，并在 UI 提示与用户文档中明确传达四层信号源优先级。

**Non-Goals:**

- 不新增独立的服务端全局 TOML 字段（全局默认继续由现有的 `program_status`、`agent_protocol`、`bell_attention` 控制）。
- 不改变 `webmux notify` 显式命令行通知的行为（即使预设将自动提醒设为 `off`，用户在脚本中主动调用的 `webmux notify` 依然生效）。
- 不修改各第三方 CLI Agent 自身的插件实现，仅在 WebMux 会话协议解析与状态机层做防误报收敛。

## Decisions

### 1. 启动配置 `AttentionSettings` 数据结构与“跟随全局默认”语义

- **方案**：在 `internal/launch` 中定义值类型结构体 `AttentionSettings`，并在 `Config`、`Summary`、`Input` 中使用指针 `*AttentionSettings`（JSON 字段 `attention,omitempty`）：
  ```go
  type AttentionMode string
  const (
      AttentionModeAll    AttentionMode = "all"
      AttentionModeUrgent AttentionMode = "urgent"
      AttentionModeOff    AttentionMode = "off"
  )

  type AttentionSettings struct {
      Mode             AttentionMode `json:"mode"`
      ProgramStatus    bool          `json:"programStatus"`
      AgentProtocol    bool          `json:"agentProtocol"`
      Notify           bool          `json:"notify"`
      Bell             bool          `json:"bell"`
      MinTurnSeconds   int           `json:"minTurnSeconds"`
      DoneDelaySeconds int           `json:"doneDelaySeconds"`
  }
  ```
- **理由**：与现有的 `Preview *bool` 保持一致：`nil` 代表“跟随全局默认”，非 `nil` 代表按预设覆盖。在 `internal/contract` 与 `web/src/contracts/schemas.ts` 中镜像相同结构，并在 `launch.ValidateInput` 中统一校验枚举和数值范围（`minTurnSeconds` `0..300`，`doneDelaySeconds` `0..60`）。
- **备选方案**：将 7 个字段平铺在 `launch.Config` 顶层并使用多个 `*bool`/`*int` 指针。放弃原因：字段过多且强相关，聚合为 `attention` 子对象更清晰，也便于前端“跟随全局默认”一键折叠。

### 2. 会话级 `AttentionPolicy` 解析与注入路径

- **方案**：
  - 在 `internal/session` 中定义 `AttentionPolicy`（包含 `Mode`、`ProgramStatus`、`AgentProtocol`、`Notify`、`Bell`、`MinTurn time.Duration`、`DoneDelay time.Duration`），放入 `session.CreateRequest.Attention *AttentionPolicy`。
  - 当 `CreateRequest.Attention == nil` 时，`session.Manager` 使用 `session.Options` 中的全局默认值构造会话的有效策略（`Mode: "all"`，`ProgramStatus: opts.ProgramStatus`，`AgentProtocol: opts.AgentProtocol`，`Notify: true`，`Bell: opts.BellAttention`，阈值均为 `0`）。
  - 在 `internal/workspace` 的 `CreateSession` 中，当根据 `configId` 加载到 `launch.Config` 且 `cfg.Attention != nil` 时，将其转换为 `session.AttentionPolicy` 传给 `session.CreateRequest`（以及混合会话的 `hybrid.StartOption` / 会话策略查询）。
- **理由**：保持 `workspace` 编排 `launch`、`session`、`hybrid` 的单向依赖，符合 `internal/archtest` 的服务层边界，不让 `session` 反向依赖 `launch`。

### 3. 四层信号源优先级控制与 `omp` 子代理 `session_id` 过滤

- **信号源优先级与开关作用点**：
  - **L1 (`programStatus`)**：在 `session.applyOSC7501` 中检查当前会话策略 `s.attentionPolicy.ProgramStatus`。若为 `false`，直接忽略所有 `OSC 7501` 序列（包括不对 `OSC 7501;?` 写回应答），从而不会向 `statusSlots` 写入根记录，自动让位给 L2 Warp `cli-agent` 事件。
  - **L2 (`agentProtocol`)**：在 `buildChildEnv` 中仅当 `s.attentionPolicy.AgentProtocol` 为 `true` 时注入 `WARP_CLI_AGENT_PROTOCOL_VERSION=1`；在 `applyOSC` 中仅当该开关启用时解析 `warp://cli-agent` JSON 载荷。
  - **L3 (`notify`)**：在 `applyOSC` 中遇到普通 `OSC 9` / `OSC 777;notify` 时，若 `s.attentionPolicy.Notify` 为 `false`，忽略该通知提醒。
  - **L4 (`bell`)**：在 `raiseBellAttention` 中检查 `s.attentionPolicy.Bell`。
- **Warp `session_id` 主会话过滤（解决 Issue `#189`）**：
  - 在 `Session` 结构体上新增 `warpRootSessionID string`（受 `s.mu` 保护）。
  - 在 `agentEventLocked(ev oscAgentEvent)` 中按以下规则维护与校验主会话标识：
    - 当 `ev.Event == "prompt_submit"` 且 `ev.SessionID != ""` 时，更新 `s.warpRootSessionID = ev.SessionID`；
    - 当 `ev.Event == "session_start"` 且 `ev.SessionID != ""` 且 `s.warpRootSessionID == ""` 时，记录初始 `s.warpRootSessionID = ev.SessionID`；
    - 当 `s.warpRootSessionID != ""` 且 `ev.SessionID != ""` 且 `ev.SessionID != s.warpRootSessionID` 时，对 `stop`、`idle_prompt`、`stop_failure`、`subagent_start` 直接忽略（不调用 `setAgentWorkingLocked(false)`，也不触发 `done`/`error` 提醒）；
    - 对子代理发出的 `question_asked` 与 `permission_request` 仍正常触发提醒，因为子代理阻塞等待授权或回答同样需要用户介入。
  - 当会话针对 `omp` 预设关闭 `programStatus`（或未来 `omp` 修复 `OSC 7501` 的 `id=` 参数）时，L2 的 `session_id` 过滤即可彻底拦截子代理 `stop` 引起的提前“已完成”误报。

### 4. 最短回合耗时（`minTurnSeconds`）与完成确认延迟（`doneDelaySeconds`）状态机

- **回合起点记录（`turnStartedAt`）**：
  - 在终端 `Session` 中维护 `turnStartedAt time.Time`（受 `s.mu` 保护）：会话初始化时设为 `now`；每当用户在终端输入（`clearAttentionOnInput`）、收到 `prompt_submit`、或者状态从非 `working` 进入 `working`（`OSC 7501` `state=working` 或 `setAgentWorkingLocked(true)`）时刷新 `turnStartedAt = now`。
  - 在混合会话 `hybrid.actor` 中，每个聊天回合已有明确的回合开始时间（`runPrompt` 启动时刻）。
- **适用信号范围**：
  - 仅对非紧急完成类信号（`KindDone`、终端自动产生的 `KindNotify`、`KindBell`）检查 `mode`、`minTurnSeconds` 与 `doneDelaySeconds`；
  - 紧急介入信号（`KindQuestion`、`KindPermission`、`KindError`）在 `mode != "off"` 时始终立即发布，且立即取消任何挂起的延迟完成定时器；
  - 命令行显式通知（`SourceCLI`，即 `webmux notify`）不受 `mode`、`minTurnSeconds`、`doneDelaySeconds` 限制，始终立即发布。
- **延迟确认定时器（`doneDelayTimer`）与取消条件**：
  - 当非紧急完成信号通过 `mode == "all"` 与 `minTurnSeconds` 检查，且 `doneDelaySeconds > 0` 时：不立即写入 `s.attention` 并广播，而是存入 `s.pendingAttention` 并启动/重置 `time.AfterFunc(doneDelay, ...)`。
  - 在定时器触发前，若发生以下任一事件，立即取消定时器并清空 `pendingAttention`：
    1. 会话进入 `working` 状态（`OSC 7501` `state=working`、Warp `prompt_submit`/`tool_call`/`subagent_start`）；
    2. 用户向终端写入输入或清除提醒（`WriteInput`、`ClearAttention`）；
    3. 收到更高优先级的紧急提醒（`question`、`permission`、`error`）；
    4. 会话退出或关闭。
  - 定时器到期且状态仍为空闲时，获取会话锁正式写入 `s.attention` 并调用 `notifySubscribers` 广播给前端与 `push.Service`。`hybrid.actor` 中采用相同的定时器与取消机制（在新 `Prompt`、切换到终端、后台任务启动或 actor 关闭时取消）。

### 5. 启动配置弹窗 UI 设计与信号优先级透出

- **交互结构（遵循 `webmux-ui` 规范）**：
  - 在 `LaunchConfigDialog.svelte` 中，紧随“输出预览”fieldset 之后新增“完成与介入提醒”fieldset：
    - 顶行：标题 `完成与介入提醒` + `可选`，配复选框 `跟随全局默认`。
    - 当取消勾选“跟随全局默认”时展开配置面板：
      1. **提醒范围**：使用设计系统的 `SegmentedControl`（`size="form"`，`wrap`），提供三个清晰选项：`全部提醒`（`all`）、`仅需介入`（`urgent`）、`关闭`（`off`），并在下方配一行简短说明（例如选中“仅需介入”时显示“仅在需要回答、等待授权或出错停止时提醒，忽略回合完成与响铃”）。
      2. **信号来源**（当 `mode !== "off"` 时显示）：采用 `sm:grid-cols-2` 紧凑网格展示四个复选框：
         - `状态协议 (OSC 7501)`
         - `Agent 事件 (Warp 协议)`
         - `桌面通知 (OSC 9 / 777)`
         - `终端响铃 (BEL)`
           下方附带常驻辅助说明文案：
         > 优先级：状态协议 (OSC 7501) > Agent 事件 > 桌面通知 > 终端响铃。高优先级信号生效时自动忽略低优先级信号；若某工具的 OSC 7501 存在子任务误报，可关闭该项以回退到 Agent 事件。
      3. **防误报过滤**（仅当 `mode === "all"` 时显示）：采用 `sm:grid-cols-2` 展示两个数字输入框：
         - `最短回合耗时 (秒)`（`0` 表示不限制）
         - `完成确认延迟 (秒)`（`0` 表示立即提醒，等待指定秒数确认未继续工作后再提醒）
- **列表摘要展示**：
  - 在 `web/src/domains/settings/launch-configs.ts` 的 `launchConfigSummary` 中，若 `config.attention` 非空，追加简短标识（如 `仅介入提醒`、`提醒已关闭` 或 `自定义提醒`），让用户在预设列表中一目了然。

## Risks / Trade-offs

- **[Risk] 设置了 `doneDelaySeconds` 后完成通知比终端实际空闲晚几秒到达** → 默认值为 `0`（零延迟，保持现有即时行为），仅在用户针对特定容易误报的预设主动开启时才延迟；且一旦触发紧急提醒（`question`/`permission`/`error`）会绕过延迟立即通知。
- **[Risk] 用户不理解为什么要关闭 `OSC 7501` 才能解决部分 Agent 的状态协议误报** → 在 UI 的“信号来源”区域直接写出四层优先级关系及回退说明，并在 `docs/usage/attention.md` 中补充针对子代理误报的配置示例。
- **[Risk] 延迟定时器在会话销毁或并发状态变更时引发竞态** → 定时器回调统一通过 `Session` 的互斥锁（或 `hybrid.actor` 的事件循环）二次校验会话存活状态与当前工作状态，会话关闭时显式 `Stop()` 定时器。
