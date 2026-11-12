## 1. Kimi 适配器

- [x] 1.1 新建 `internal/agentprofile/kimi/kimi.go`，实现档案、`kimi-code` 识别、一次 Esc、`session/resume` 与仅 `mode` 的 `ReapplyOnLoad`；新增 `kimi_test.go` 运行 agenttest 契约并断言版本、图标、隐藏认证命令和无 steering/锁/注册表接口，`go test -race ./internal/agentprofile/kimi` 通过。
- [x] 1.2 按 design D2 实现 Kimi 参数与子命令分类，覆盖直接/Bash/Zsh 写法、单值与 inline 参数、会话选择丢弃、远端环境/目录拒绝、非交互参数和自定义 Agent 不进入恢复命令；在 `internal/agentprofile/agents/agents_test.go` 增加边界用例并运行 `go test -race ./internal/agentprofile/...`。
- [x] 1.3 实现 `ChatSide` 的 model/mode 种子、plan 优先级和 skills/add-dir unsupported 提示；测试 `kimi acp` 始终无原参数、`kimi --session <id>` 恢复 argv、仅 `mode` 重应用，`go test -race ./internal/agentprofile/...` 通过。
- [x] 1.4 按真实 TUI 实测为 Kimi 增加 200ms 文本/Enter 分隔，保持同一回执、输入临界区和其他 agent 零延迟；覆盖延迟与重试，`go test -race ./internal/agentprofile/kimi ./internal/session ./internal/hybrid` 通过。
- [x] 1.5 按真实 ACP 复现处理 `Already in <mode> mode` 幂等错误，目标不匹配与其他错误仍失败；fakeagent 同步模拟，运行 `go test -race ./internal/agentprofile/kimi ./internal/hybrid -run 'TestKimi|TestContract'` 通过。

## 2. 注册与预置

- [x] 2.1 在 `internal/agentprofile/agents/agents.go` 注册 `kimi.Agent`，并在 `internal/archtest/layers_test.go` 将新包分类为 shared；运行 `go test -race ./internal/agentprofile/... ./internal/archtest`，确认八个档案、支持列表和架构守卫通过。
- [x] 2.2 将 `internal/launch/presets.go` 的 Kimi 预置改为 `kimi --auto` 且 `Hybrid: true`，保持总数、名称和图标不变；更新 `internal/launch` 预置/恢复测试，验证新预置混合可用、已有文件不被改写，`go test -race ./internal/launch` 通过。
- [x] 2.3 按 design D7 让 pre-launch 返回补充环境并交给 `startOptions`；以 manager 测试证明变量进入真实启动环境、其他 agent 环境不变，`go test -race ./internal/session ./cmd/internal/assemble` 通过。
- [x] 2.4 在 `internal/agenttrust` 注册 Kimi，仅注入目录级 `KIMI_CODE_TRUST_WORKSPACE=1`；覆盖直接/包装命令、显式 falsy、禁用开关和过宽目录，`go test -race ./internal/agenttrust` 通过。

## 3. 集成与契约测试

- [x] 3.1 扩展 `internal/acp/testdata/fakeagent` 支持可执行文件名 `kimi`：TUI 报告 `app=kimi-code` 且无会话 ID，ACP 提供 model/thinking/mode，load/resume 只重置 mode；构建 fakeagent 并运行 `go test -race ./internal/acp`，确认既有协议测试无回归。
- [x] 3.2 在 `internal/hybrid` 增加 Kimi 用例，覆盖 OSC/根进程识别、首条消息前不可切换、唯一/多个候选、恢复 ID 优先、TUI→ACP→TUI 双向切换、崩溃 resume、mode 补齐以及 model/thinking 不重应用；运行 `go test -race ./internal/hybrid` 通过。
- [x] 3.3 更新启动配置与 hybrid 相关 API/wire 快照，重录 `internal/api/testdata/wire` 与必要的 `internal/wire` 快照；运行 `go test -race ./internal/api ./internal/wire`，确认 `hybridSupport.agent=kimi`、预置参数和无连接器字段符合契约。
- [x] 3.4 在现有 `web/test/e2e/launch-configs.spec.ts` 或 `hybrid.spec.ts` 增加 Kimi 预置混合开关和 fakeagent 双向切换用例；按项目 Playwright 配置运行受影响 spec，确认桌面与移动视口均通过且不连接现有开发者服务。

## 4. 文档与用户故事

- [x] 4.1 更新 `docs/cli-agents.md`、`docs/hybrid-sessions.md`、`docs/architecture.md` 与 `docs/persistence.md` 的支持列表、Kimi ACP、`--auto` 预置、目录级 trust、候选识别、配置恢复和自定义 Agent/额外目录限制；运行 `make docs-check`。
- [x] 4.2 更新 US-B01、US-B09、US-Q01 及实际受影响的其他故事与索引计数，记录真实验证结果且不新增或重编号故事 ID；运行 `make docs-check`，交付说明列出受影响故事 ID。

## 5. 真实验收与完整门禁

- [x] 5.1 使用隔离 cwd 对真实 `kimi` 重跑 ACP 基线：initialize/new/set_config_option/prompt/list/resume/load/close/cancel；断言 resume 无回放、load 有回放、model/thinking 保留、mode 重置后由 WebMux 补齐，并只删除本次创建的测试会话；在交付说明记录 Kimi 版本与结果。
- [x] 5.2 使用隔离 WebMux 服务和真实 `kimi` 验证 pre-launch 注入后无启动 trust 阻塞，再完成 TUI 首条消息及 TUI→ACP→TUI→ACP；以 `session/list` 中出现本次新会话和两端可见历史为成功依据，验证取消、恢复 ID 与配置文件未写入全局 trust，不连接现有开发者会话、不删除非测试会话。
- [x] 5.3 运行 `npx --yes @fission-ai/openspec@1.13.0 validate add-kimi-hybrid-profile --strict`、`make check` 与 `make test`；确认全部通过，检查 `git status` 无测试临时产物，交付说明列出命令、真实验收证据和任何未验证边界。
