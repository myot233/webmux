# Tasks

## 1. 补测与 Devin 档案

- [x] 1.1 按 `design.md` Open Questions 补测并更新 `baseline.md`：跨进程 `session/load` 的 `mode`/`model` 重置行为（决定 `ReapplyOnLoad`）、`session/prompt` 是否实时回显 `user_message_chunk`、`.bashrc` fd 3 包装下 `devin acp` 握手、权限请求选项形态与 `session/cancel` 的 `stopReason`。
- [x] 1.2 新建 `internal/agentprofile/devin` 并注册：可执行文件 `devin`、实测版本 `3000.12.57`、单次 `Esc` 中断、`ResumesSessions: false`、`ResumeFlag: "--resume"`、隐藏 `login`/`logout` 命令。
  - `PositionalDirectoryPrompt`（实现期细化：`--` 前位置参数判目录、`--` 后为初始提示词）；`InitialPrompt` 返回 `["--", prompt]`。
  - `Identify` 识别 `app=devin` 根记录；按 D2 参数表与 D3 `ChatSide` 推导。
- [x] 1.3 在 `internal/agentprofile/devin/devin_test.go` 与 `internal/agentprofile/agents/agents_test.go` 覆盖 `devin` 的命令分类、`TUIArgv`、`ResumeArgv`、`ACPArgv`（只透传 `--model`）、`Identify` 与版本比较，运行 `go test -race ./internal/agentprofile/...` 验证通过。
- [x] 1.4 按 1.1 实测结果：`session/load` 持久保留 `modes`/`configOptions`，无需 `ReapplyOnLoad`；活动回合不回显 `user_message_chunk`，无需忽略逻辑（实测记录见 `baseline.md`）。
- [x] 1.5 实现 `session_locked` 恢复：`session/load` 返回 `-32015` 且 `lockHolderPid` 进程已死时，先发 `_meta."cognition.ai/force": true` 的 `session/close` 再重试加载；持锁进程存活时按外部占用冲突呈现。在 `internal/hybrid` 测试中覆盖两条路径。

## 2. agenttrust、fakeagent 与混合切换测试

- [x] 2.1 在 `internal/agenttrust` 注册 `devin` 并补测试（`go test -race ./internal/agenttrust`）：
  - 把会话目录追加到 `$XDG_DATA_HOME/devin/cli/trusted_workspaces.json`（默认 `~/.local/share/devin/cli/`）的 `trusted_paths`；
  - 文件缺失时创建最小 JSON、已有项不重复；沿用主目录及祖先拒绝与 `agent_pretrust` 开关。
- [x] 2.2 对照 `baseline.md` 扩展 `internal/acp/testdata/fakeagent`：支持 `--profile devin` 及以可执行文件名 `devin` 自动选定。
  - slug 会话 ID、`app=devin` 根记录无子记录、`session/resume` 返回 Method not found、单次 `Esc` 中断；
  - `session/list` 附 `isLocked`/`lockHolderPid` 且零消息会话不列出；模拟 `session_locked` 与 force `session/close` 释放。
- [x] 2.3 在 `internal/acp/fakeagent_test.go` 与 `internal/hybrid/devin_test.go` 中增加 `devin` 双向切换、候选判定、`session_locked` 恢复与配置推导用例，运行 `go test -race ./internal/acp ./internal/hybrid` 验证通过。

## 3. 启动配置预置、图标与前端适配

- [x] 3.1 在 `internal/launch/presets.go` 新增 `Devin CLI` 预置（`devin --permission-mode dangerous`、图标 `devin`、`Hybrid: true`）。
  - 更新 `internal/launch` 与 `internal/api` 测试并以 `go test ./internal/api -run TestWireSnapshots -update` 重录快照；
  - 运行 `go test -race ./internal/launch ./internal/api ./internal/wire` 验证通过。
- [x] 3.2 添加 `web/src/icons/lobehub/devin.svg`（lobe-icons 固定提交，缺失则仿 `qoder` 自定义），更新 `web/src/components/settings/LaunchConfigIcon.svelte`、`web/src/domains/settings/launch-configs.ts`（图标选项 `Devin CLI`、`devin` 自动匹配）与 `web/src/icons/lobehub/README.md`。
- [x] 3.3 更新 `web/test` 中图标与预设断言（`web/test/e2e/fake-agent-names.txt` 加入 `devin`），运行 `pnpm --dir web check`、`check:test`、`lint`、`check:arch` 与定向 Vitest 用例验证通过。

## 4. 文档、用户故事与验收

- [x] 4.1 更新 `docs/cli-agents.md`、`docs/hybrid-sessions.md`、`docs/architecture.md`、`docs/persistence.md`、相关用户故事（US-B01、US-B09、US-Q01 等）与 `docs/user-stories.md`/`coverage.md` 索引，运行 `make docs-check` 与 `make format-check` 验证通过。
- [x] 4.2 验收：`openspec validate --strict`、`make check`、`make test-fast`（Go race、unit、browser）全部通过。
  - 真实 `devin 3000.12.57` 逐项实测记录于 `baseline.md`（TUI 首条消息、ACP 握手/回放、`session_locked` 与 force `session/close` 恢复、Esc 取消）。
  - 完整 TUI↔聊天切换经 fakeagent 的 `devin` 档案与 `internal/hybrid` 双向切换测试覆盖；真实二进制的端到端界面切换留待维护者验收。
