## Why

Devin CLI 已实测具备 WebMux 混合会话所需的 ACP v1 与 OSC 7501 能力（`devin 3000.12.57`，`devin acp` 内嵌 ACP 服务，无需第三方连接器），但目前没有内置档案，启动配置无法启用混合模式。

本机实测已覆盖 initialize、`session/new`、`session/list`、`session/load`、强制 `session/close`、TUI 的 `app=devin` 状态记录与工作区信任提示，可以补齐档案、预置、图标与文档。

## What Changes

- 新增内置 `devin` agent 档案：识别可执行文件 `devin` 与 OSC 7501 `app=devin`，以 `devin acp` 启动聊天端、以 `devin --resume <id>` 恢复 TUI，单次 `Esc` 中断，实测版本为 `3000.12.57`。
- Devin 会话 ID 为词组 slug（如 `luminous-acorn`），TUI 的 OSC 7501 根记录不携带会话 ID；复用现有 `session/list` 候选判定（规范化 cwd + 本次启动后的更新时间），恢复来的 TUI 优先使用恢复 ID。
- `session/resume` 实测不存在（-32601 Method not found）：`ResumesSessions()` 为 `false`，崩溃重连走 `session/load` 回放。
- Devin 会话按进程持锁：`session/list` 的 `_meta` 暴露 `isLocked`/`lockHolderPid`，持锁会话的 `session/load` 返回 `-32015 session_locked`（可重试）。切换流程先结束 TUI 再加载，正常路径不冲突；残留锁（崩溃或外部占用）由带 `_meta."cognition.ai/force"` 的 `session/close` 释放后重试。
- 参数分类：
  - 透传 `--model` 到 `devin acp`；`--permission-mode`/`--sandbox`/`--verbosity`/`--respect-workspace-trust`/`--export` 仅保留在 TUI；
  - `-c`/`--continue`、`-r`/`--resume` 为会话选择参数拒绝混合；`--cloud`、`--config` 因会话库/配置不一致拒绝；
  - `-p`/`--print`、`-h`/`-V` 与全部子命令按非交互拒绝；`--prompt-file` 丢弃；
  - `--` 之后为初始提示词；PATH 位置参数（会打开 Desktop）按目录参数拒绝。
- 聊天端配置推导：`--model` 映射 `model`；`--permission-mode` 的 `accept-edits`/`smart`/`dangerous` 映射 `mode` 的同名或 `bypass`，`auto` 与其他仅 TUI 参数列入 `unsupported`。
- `agenttrust` 增加 Devin 支持：启动前把会话目录追加进 `~/.local/share/devin/cli/trusted_workspaces.json` 的 `trusted_paths`（遵循 `XDG_DATA_HOME`），跳过工作区信任提示；home 目录及其祖先仍拒绝。
- 新安装的 `Devin CLI` 预置：`devin --permission-mode dangerous`，图标 `devin`，`hybrid: true`；已有 `launch-configs.json` 不改写。新增 `web/src/icons/lobehub/devin.svg`（lobe-icons 固定提交，缺失时仿 `qoder` 自定义）并接入图标目录与可执行文件自动匹配。
- 复用现有 ACP 图片/嵌入资源、权限请求、取消、历史回放与崩溃重连能力；不实现 Devin 私有 `cognition.ai/*` 扩展、steering 或 `session/delete`。

实测基线（详见 baseline.md）：

- `initialize` 返回 `loadSession: true`、`promptCapabilities.image/embeddedContext`、`sessionCapabilities.{list,delete,additionalDirectories,close(force)}` 与 `authMethods: [devin-browser]`；`agentInfo` 为 `affogato`/`Devin Agent`/`0.0.0-dev`。
- `session/new` 返回 slug `sessionId`、`modes`（accept-edits/smart/ask/plan/bypass）与 `configOptions`（`mode`、`model` 两个 select）；`session/list` 仅列出已有消息的会话。
- `session/load` 回放 `user_message_chunk`/`agent_thought_chunk`/`agent_message_chunk`/`session_info_update`/`available_commands_update`/`usage_update`；`session/close` 返回 `{}`；`session/resume` 不存在。
- 真实 TUI 已观察到 `state=idle|working|done:app=devin`（`msg` 为 base64，ST 结尾）与 OSC 133 提示符标记，未观察到 7501 查询或子记录；强关持锁会话后 TUI 发 `state=error` 且进程存活。
- `session/close` 带 force `_meta` 可释放 TUI 持有的会话锁，随后 `session/load` 成功；`session/load` 的 force `_meta` 不能抢锁。
- 工作区信任记录为 `~/.local/share/devin/cli/trusted_workspaces.json` 的 `trusted_paths` 数组；信任覆盖子目录。
- 跨进程 `session/load` 是否重置 `mode`/`model`、TUI 持锁中断键以外的细节与完整双向切换留作实施验收，不预先宣称通过。

## Capabilities

### New Capabilities

无。本次复用现有混合会话能力，不引入新的能力域。

### Modified Capabilities

- `hybrid-agent-profiles`：内置档案增加 Devin CLI，声明其命令识别、参数分类、ACP 启动、TUI 恢复、中断与版本行为。
- `agent-session-identity`：声明 `devin` 的终端身份识别（`app=devin`、根进程回退）及无终端会话 ID 时的 `session/list` 候选判定。
- `hybrid-agent-specifics`：声明 Devin 的目录级工作区信任、聊天端配置推导、会话锁与强制释放、认证与回合能力边界、`cognition.ai/*` 扩展的不支持范围。
- `launch-configs`：新安装的 Devin CLI 预置默认混合并使用 `--permission-mode dangerous`，已有配置保持不变。

## Impact

- Go：新增 `internal/agentprofile/devin/`，修改 `internal/agentprofile/agents/agents.go`、`internal/agenttrust`、`internal/launch/presets.go` 与 `internal/archtest/layers_test.go`；按实测结果决定是否扩展 `internal/hybrid` 的 `session_locked` 恢复路径。
- 测试：新增 Devin 适配器契约与参数测试，扩展 `internal/agentprofile/agents`、agenttrust、launch 预置、`internal/acp/testdata/fakeagent`（`devin` 档案：slug 会话 ID、无 `session/resume`、锁模拟）、`internal/hybrid` 双向切换与 wire 快照；以隔离目录执行真实 `devin` ACP/TUI/trust 验收。
- API/前端：`hybridSupport.agent` 新增稳定值 `devin`，预置摘要变化；新增 `devin` 图标（`web/src/icons/lobehub/devin.svg`、`LaunchConfigIcon.svelte`、`launch-configs.ts`）；不改 JSON 结构或 Zod schema。
- 文档与用户故事：更新 `docs/cli-agents.md`、`docs/hybrid-sessions.md`、`docs/architecture.md`、`docs/persistence.md` 及受影响的 US-B01、US-B09、US-Q01 等相关故事。
- 部署与认证：聊天端直接运行同一服务用户的 `devin acp`，复用 `~/.local/share/devin/credentials.toml` 的现有登录状态；无需安装连接器，不自动登录或注销。
