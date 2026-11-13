## 1. Agent 档案首条提示参数支持

- [x] 1.1 在 `internal/agentprofile` 的 `Profile` 中新增 `InitialPrompt func(prompt string) ([]string, bool)`，在 `Command` 中记录原命令是否已含提示词参数并实现 `InitialTUIArgv(prompt string) ([]string, bool)`
- [x] 1.2 为 `claude`、`codex`、`opencode`、`omp`、`agy` 五个已核查档案实现 `InitialPrompt` 及其边界回退条件，`qoder` 与 `qodercn` 保持未设置以回退到 `SendWhenReady`
- [x] 1.3 在 `internal/agentprofile/agents/agents_test.go` 中添加单元测试 `TestInitialTUIArgv`，覆盖五个已迁移档案、未迁移档案、启动配置自带提示词冲突检测以及前导 `-` / `-h` / `--version` 回退场景

## 2. 混合会话终端启动与工作区编排

- [x] 2.1 在 `internal/hybrid` 的 `CreateInput` 中新增 `InitialPrompt func(id string) (string, error)` 与 `InitialPromptSent *bool`，在 `CreateShell` 分配会话 ID 后展开提示词并仅将 `InitialTUIArgv` 传给首次 `shell.StartRun`，保持 `SessionInfo.Cmd` 与 `Record.Cmd` 不含提示文本
- [x] 2.2 在 `internal/workspace/create_session.go` 中将 `composePrompt` 回调传入 `startSession`，当首条提示已随首次 TUI 启动参数传入时跳过 `sendFirstMessage`（`SendWhenReady`），否则自动回退到 `sendFirstMessage`
- [x] 2.3 扩展 `internal/acp/testdata/fakeagent` 支持 `--`、`--prompt`、`-i`/`--prompt-interactive` 与 `--acp`，并在 `internal/workspace/create_session_test.go` 中验证已迁移档案通过 `argv` 接收完整提示（含初始化指令）且不污染 `SessionInfo.Cmd`，以及未迁移档案回退到 PTY 输入

## 3. 文档与验证

- [x] 3.1 更新 `docs/usage/sessions.md`、用户故事 `US-B01`（`docs/user-stories/access-and-sessions.md`）、`US-Q18`（`docs/user-stories/hybrid-sessions.md`）与验证日志 `docs/user-stories/verification-log-2026-11-shell.md`
- [x] 3.2 运行代码与文档格式化、架构守卫、文档守卫与 Go 竞态测试，确保全部通过
