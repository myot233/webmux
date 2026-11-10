## Why

Kimi Code CLI 已有普通终端预置与品牌图标，但启动配置不能启用混合模式，用户无法在同一 WebMux 会话中切换原生 TUI 与 ACP 聊天视图。本机 `kimi 2.5.2` 已实测具备 WebMux 所需的 ACP v1 与 OSC 7501 能力，现在可以补齐档案而无需新增连接器或协议栈。

## What Changes

- 新增内置 `kimi` agent 档案：识别可执行文件 `kimi` 与 OSC 7501 `app=kimi-code`，以 `kimi acp` 启动聊天端、以 `kimi --session <id>` 恢复 TUI，一次 Esc 中断，实测版本为 2.5.2。
- Kimi 不通过终端信号提供当前会话 ID 时，复用现有 `session/list` 候选判定：从聊天恢复来的 TUI 使用本次恢复 ID，新 TUI 则按规范化 cwd 与启动后的更新时间选择会话。
- ACP 进程不透传原启动参数；实测 `kimi acp --auto` 以未知参数失败。TUI 的 `-m`/`--model`、`--auto`、`--yolo` 与 `--plan` 分别映射到聊天端 `model` 与 `mode`，其余保留参数明确提示是否在聊天端生效。
- Kimi TUI 的文本与 Enter 在同一回执内分隔 200ms，避免粘贴爆发吞掉提交；其他 agent 保持零延迟。
- 实测 `session/load` 与 `session/resume` 保留 `model`、`thinking`，但把 `mode` 重置为 `default`；冷恢复后只重应用记录的 `mode`，避免覆盖 Kimi 已恢复的模型与思考等级。
- 新安装的 Kimi Code CLI 预置改为 `kimi --auto` 且默认 `hybrid: true`；已有 `launch-configs.json` 及其中的 Kimi 配置不改写，现有 `kimi` 图标与前端组件复用。
- `agent_pretrust` 增加 Kimi 支持：启动前为当前会话补充官方 `KIMI_CODE_TRUST_WORKSPACE=1`，只信任本次选择的工作区且不改写持久配置。首次 TUI、恢复 TUI 与 ACP 继承同一环境；显式变量设置与禁用开关优先。
- 复用现有 ACP 图片、嵌入资源、权限请求、表单、取消、历史回放与崩溃重连能力；不实现 Kimi 私有 steering，不新增 ACP 方法、第三方依赖或认证流程。

实测基线：

- ACP 的 initialize、新建、配置、提示词、列出、加载、恢复、关闭与取消均成功；`resume` 无历史回放，`load` 有回放，取消返回 `stopReason=cancelled`。
- 真实 TUI 已观察到原生进程 `kimi`、OSC 7501 查询及 `app=kimi-code` 状态记录。
- 隔离 `KIMI_CODE_HOME` 对照实测：无 trust 变量时出现 `blocked/permission`，设置后直接进入 `idle`。
- 自动化 TUI 首条消息与完整双向切换留作实施验收，不预先宣称通过。

## Capabilities

### New Capabilities

无。本次复用现有混合会话能力，不引入新的能力域。

### Modified Capabilities

- `hybrid-agent-profiles`：内置档案增加 Kimi Code CLI，声明其命令识别、参数分类、ACP 启动、TUI 恢复、中断与版本行为。
- `agent-session-identity`：声明 `kimi-code` 的终端身份识别及无终端会话 ID 时的候选判定。
- `hybrid-agent-specifics`：声明 Kimi 的目录级文件夹信任、聊天端配置推导、恢复后配置补齐、认证与回合能力边界。
- `launch-configs`：新安装的 Kimi 预置默认混合并使用 `--auto`，已有配置保持不变。

## Impact

- Go：新增 `internal/agentprofile/kimi/`，修改 `internal/agentprofile/agents/agents.go`、`internal/agenttrust`、`internal/session` 的 pre-launch 环境返回、`internal/launch/presets.go` 与 `internal/archtest/layers_test.go`。
- 测试：新增 Kimi 适配器契约与参数测试，扩展 `internal/agentprofile/agents`、agenttrust、launch 预置、fakeagent、hybrid 双向切换与 wire 快照；以隔离目录执行真实 `kimi` ACP/TUI/trust 验收。
- API/前端：`hybridSupport.agent` 新增稳定值 `kimi`，预置摘要发生变化；不改 JSON 结构、Zod schema、图标目录或前端组件。
- 文档与用户故事：更新 `docs/cli-agents.md`、`docs/hybrid-sessions.md`、`docs/architecture.md`、`docs/persistence.md` 及受影响的 US-B01、US-B09、US-Q01 等相关故事。
- 部署与认证：聊天端直接运行同一服务用户的 `kimi acp`，复用其现有登录状态与 `KIMI_CODE_HOME`；无需安装连接器，不自动登录或注销。
