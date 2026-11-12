# Proposal

## Why

当前 WebMux 的完成与介入提醒策略（`program_status`、`agent_protocol`、`bell_attention`）仅支持在 `config.toml` 中按服务端全局维度开关，且会话在收到任意层级的 `done`、`notify`、`bell` 信号时均会立即触发提醒（会话徽章、应用内弹窗、浏览器与 Web Push 通知）。在实际使用中，这会导致三类高频误报与打扰：

1. **终端与工具噪音（场景 A）**：普通 Shell 预设、日志监控脚本或构建工具发出的 `BEL` 响铃与 `OSC 9`/`OSC 777` 桌面通知会被当作需要用户处理的提醒；若在全局关闭，又会导致其他依赖响铃或通知的旧版 CLI 工具失去提醒能力。
2. **子代理、后台任务与短回合提前报“已完成”（场景 B）**：
   - 如 Issue `#189` 所述，`omp` 等支持子代理的 Agent 在主任务尚未结束时，子代理退出会触发 `OSC 7501` 根状态 `done` 或携带子代理 `session_id` 的 Warp `cli-agent` `stop` 事件，导致提前误报完成；
   - 如 Issue `#332` 及部分 Agent 的分步执行场景，Agent 在启动初始化、工具切换间隙或仅耗时 1～2 秒的微不足道交互中短暂发出 `done`/`idle_prompt`，随即继续工作或用户仍在当前屏幕前，却已弹出完成通知。
3. **仅关注阻塞介入、不希望被“已完成”打扰（场景 C）**：用户在后台挂机运行长任务或多开 Agent 时，往往只关心必须人工介入的阻塞事件（`question` 提问、`permission` 授权、`error` 出错停止），而不希望每次回合正常结束（`done`）都弹出通知打断当前心流。

在启动配置（运行预设）中引入按预设定制的“完成与介入提醒”设置，配合信号源优先级说明、Warp 主会话 `session_id` 校验以及最短耗时与延迟确认防抖机制，可以让用户针对不同 Agent 与 Shell 预设精确消除误报，同时保持简洁易用的配置体验。

## What Changes

- **启动配置新增可选的提醒策略（`attention`）**：
  - 在 `launch.Config`、`launch.Summary`、`launch.Input` 及前后端契约中新增可选字段 `attention`（默认为 `null`，表示“跟随全局默认”）。
  - 自定义 `attention` 时支持三个维度的配置：
    1. **提醒范围（`mode`）**：
       - `all`：全部提醒（需要回答、等待授权、出错停止、本轮完成、通知与响铃）；
       - `urgent`：仅需介入时提醒（只保留 `question`、`permission`、`error`，忽略 `done`、`notify`、`bell`）；
       - `off`：关闭自动提醒（仅保留 `webmux notify` 显式命令行提醒）。
    2. **信号来源开关**：提供 `programStatus`（`OSC 7501` 状态协议，L1）、`agentProtocol`（Warp `cli-agent` 事件，L2）、`notify`（`OSC 9`/`OSC 777` 桌面通知，L3）与 `bell`（`BEL` 响铃，L4）四项独立开关，可按预设覆盖全局配置。
    3. **完成防误报阈值**：
       - `minTurnSeconds`：最短回合耗时（`0`～`300` 秒），耗时低于该值的回合不触发 `done`、`notify`、`bell`；
       - `doneDelaySeconds`：完成确认延迟（`0`～`60` 秒），收到 `done`、`notify`、`bell` 后等待该窗口，若期间恢复 `working`、产生新工具调用或用户输入则自动取消提醒。
- **Warp `cli-agent` 事件主会话 `session_id` 过滤**：
  - 在 `internal/session` 处理 Warp `cli-agent` 事件时记录当前主会话的 `session_id`（由 `prompt_submit` 或首个 `session_start` 确立）；
  - 当收到 `session_id` 不匹配的子代理 `stop`、`idle_prompt`、`stop_failure`、`subagent_start` 事件时忽略，避免子代理结束误报主会话完成；子代理触发的 `question_asked` 与 `permission_request` 仍正常提醒。
- **混合会话（ACP 聊天端）与终端端统一遵守预设提醒策略**：
  - 通过启动配置创建终端或混合会话时，将解析后的会话级提醒策略传入 `session.Manager` 与 `hybrid.Service`，使终端信号与 ACP 聊天回合结束（`end_turn`）统一执行 `mode`、`minTurnSeconds` 和 `doneDelaySeconds` 规则。
- **启动配置弹窗 UI 与信号优先级说明**：
  - 在 `LaunchConfigDialog.svelte` 中参照“输出预览”的设计模式新增“完成与介入提醒”分组：默认勾选“跟随全局默认”，取消勾选后使用分段控件（`SegmentedControl`）选择提醒范围，按需展示信号来源开关与防误报阈值；
  - 在界面提示与用户文档（`docs/usage/attention.md`、`docs/usage/sessions.md`）中明确说明四层信号源优先级（`OSC 7501` > Warp Agent 事件 > 桌面通知 > 终端响铃）。

## Capabilities

### New Capabilities

（无新增顶层 capability，全部作为现有 capability 的增量扩展。）

### Modified Capabilities

- `launch-configs`：为启动配置的数据模型、校验规则、REST 接口与编辑弹窗新增可选的 `attention`（完成与介入提醒）配置及信号源优先级提示。
- `agent-attention`：支持会话级提醒策略覆盖（`mode`、四层信号源开关、`minTurnSeconds` 最短回合耗时、`doneDelaySeconds` 延迟确认防抖），并新增 Warp `cli-agent` 主会话 `session_id` 校验以过滤子代理 `stop` 误报。
- `program-status`：支持按会话覆盖 `programStatus` 开关；当启动配置禁用 `programStatus` 时，该会话忽略 `OSC 7501` 状态上报与查询。
- `chat-view`：ACP 聊天侧在回合以 `end_turn` 结束准备触发 `done` 提醒时，同样遵守会话绑定的提醒范围（`mode`）、最短回合耗时（`minTurnSeconds`）与完成确认延迟（`doneDelaySeconds`）。

## Impact

- **后端**：`internal/launch`、`internal/contract`、`internal/wire`、`internal/session`、`internal/hybrid`、`internal/workspace`、`internal/api`（含 `testdata/wire` 快照）。
- **前端**：`web/src/contracts/schemas.ts`、`web/src/domains/settings/launch-configs.ts`、`web/src/components/settings/LaunchConfigDialog.svelte` 及相关组件/单元测试。
- **文档与用户故事**：`docs/usage/attention.md`、`docs/usage/sessions.md` 及受影响的 `docs/user-stories/` 条目。
