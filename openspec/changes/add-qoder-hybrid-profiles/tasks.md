## 1. 档案数据模型扩展

- [x] 1.1 `internal/agentprofile/profile.go`：
      `Profile` 新增 `ACPArg string` 字段，
      文档说明默认空值等同于 `"acp"` 子命令，
      设置后替代该默认值
- [x] 1.2 `internal/agentprofile/command.go`：
      `ACPArgv()` 使用 `c.Profile.ACPArg`（非空时）
      替代硬编码的 `"acp"`
- [x] 1.3 `ACPArg` 为 `"--acp"` 时的 argv 生成测试：
      `internal/agentprofile/agents/agents_test.go` 的 `TestACPArgv`
      覆盖 qoder/qodercn 直接执行与 bashrc 包装（fd 3）两种写法；
      `internal/agentprofile/agenttest/agenttest.go` 的契约测试按 `ACPArg`
      （空值回落 `"acp"`）校验每个档案的 ACP argv

## 2. Qoder 档案

- [x] 2.1 创建 `internal/agentprofile/qoder/qoder.go`：
      以 `Variant{Name, Title, Executables, ProgramApps}` 与 `New(Variant)`
      构造适配器，导出 `Agent = New(Variant{Name: "qoder", Title: "Qoder",
  Executables: ["qoder", "qodercli"], ProgramApps: ["qoder-cli"]})`；
      档案 `Icon = "qoder"`、`VerifiedVersion = "1.1.81"`、
      `ACPArg = "--acp"`、`ResumeFlag = "--resume"`、
      `Positional = PositionalPrompt`、`HiddenCommands = ["login", "logout"]`；
      `ProcessExecutable` 同时接受版本化二进制名（如 `qodercli-1.1.81`）
- [x] 2.2 定义参数分类表 `qoderOptions()`：
      Keep: `-m`/`--model`, `--reasoning-effort`, `--thinking`,
      `--thinking-budget`, `--context-window`, `--permission-mode`,
      `--fallback-model`, `--system-prompt`, `--append-system-prompt`,
      `--output-style`, `--max-output-tokens`, `--max-model-request-retries`,
      `--agents`, `--agent`, `--plugin-dir`, `--settings`, `--setting-sources`,
      `--mcp-config`, `--allowed-mcp-server-names`, `--allowed-tools`,
      `--disallowed-tools`, `--tools`, `--add-dir`,
      开关 `--dangerously-skip-permissions`/`--yolo`, `--strict-mcp-config`,
      `--bare`, `-d`/`--debug`；
      Drop: `-c`/`--continue`, `-r`/`--resume`（可选值），
      `--attachment`, `-i`/`--prompt-interactive`, `-n`/`--name`；
      Session: `--fork-session`, `--session-id`, `--from-pr`,
      `--remote-session`, `--remote-control`, `--remote`；
      Directory: `-w`/`--cwd`, `--worktree`（可选值）；
      NonInteractive: `-p`/`--print`, `-h`/`--help`, `-v`/`--version`,
      `--list-models`, `--list-sessions`, `--no-session-persistence`,
      `-o`/`--output-format`, `--input-format`, `--delete-session`；
      Store: `--config-dir`
- [x] 2.3 定义子命令表：`mcp`, `hooks`, `hook`, `rollback`,
      `update`, `config`, `plugins`, `plugin`, `commit`,
      `agents`, `agent`, `skills`, `skill`, `feedback`,
      `security`, `login`, `status`, `remote-control`,
      `sites`, `wiki`；会话子命令 `SessionSubcommands = ["fork-session"]`
- [x] 2.4 实现 `Identify()`：
      匹配 `s.ProgramApp == variant.Name` 或 `variant.ProgramApps` 之一
      （Qoder 为 `qoder`/`qoder-cli`），返回 `{Agent, Cwd: RootCwd}`；
      不发 OSC 7501 时由 `internal/hybrid/eligibility.go` 的 `identify()`
      回退按 PTY 根进程可执行文件（`Profile.RunsProcess`）识别，
      `identityFallback()` 对既无 LockTracker 也无 ProcessRegistry 的档案
      开启 `session/list` 候选探测
- [x] 2.5 实现 `InterruptKeys()`：
      返回 `[][]byte{[]byte("\x1b")}`（一次 Esc）
- [x] 2.6 实现可选接口：
      `ResumesSessions() bool` 返回 `true`；
      `ReapplyOnLoad() []string` 返回 `["model", "effort"]`；
      `SessionSteering` 嵌入（`IdlePrompt: true`）
- [x] 2.7 实现 `ChatSide` 函数 `qoderChatSide`：
      `-m`/`--model` → `model`、`--reasoning-effort` → `effort`；
      `--dangerously-skip-permissions`/`--yolo` → `mode=bypass_permissions`（优先）；
      否则 `--permission-mode` 取值属于 `default`、`accept_edits`/`acceptEdits`、
      `bypass_permissions`/`bypassPermissions`/`yolo`、`dont_ask`/`dontAsk`、
      `auto`、`plan` 时归一为 snake_case 写入 `mode`，未知取值列为不生效；
      其余保留参数列为聊天端不生效
- [x] 2.8 `internal/agentprofile/qoder/qoder_test.go`：
      `TestContract`（agenttest 契约：识别、TUI/恢复/ACP argv、steering、ReapplyOnLoad）、
      `TestProcessExecutable`、`TestACPArgv`、`TestChatConfig`、
      `TestSteeringUsesPromptRequired`；参数分类与恢复命令由
      `internal/agentprofile/agents/agents_test.go` 覆盖

## 3. QoderCN 档案

- [x] 3.1 创建 `internal/agentprofile/qodercn/qodercn.go`：
      复用 `qoder.New`，`Agent = qoder.New(qoder.Variant{Name: "qodercn",
  Title: "QoderCN", Executables: ["qodercn", "qoderclicn", "qoder-cn"],
  ProgramApps: ["qoder-cli-cn", "qoderclicn"]})`，
      参数表、ACP、恢复与聊天端推导与 Qoder 相同
- [x] 3.2 `Identify()` 匹配
      `s.ProgramApp` 为 `"qodercn"`、`"qoder-cli-cn"` 或 `"qoderclicn"`
- [x] 3.3 `internal/agentprofile/qodercn/qodercn_test.go`：
      `TestContract`（含恢复 argv 与 ReapplyOnLoad）、`TestProcessExecutable`
      （接受 `qoderclicn-1.1.81`，拒绝 `qoder`/`qodercli`）、`TestACPArgv`；
      参数分类由共用的 `qoderOptions()` 与 agents_test 覆盖

## 4. 注册与预置

- [x] 4.1 `internal/agentprofile/agents/agents.go`：
      在 `defaultRegistry` 中添加 `qoder.Agent` 和 `qodercn.Agent`
- [x] 4.2 `internal/launch/presets.go`：
      新增两个预置配置：
  - `{Name: "Qoder", Icon: "qoder",
     Cmd: command("qoder", "--dangerously-skip-permissions"),
     Hybrid: true}`
  - `{Name: "QoderCN", Icon: "qoder",
     Cmd: command("qodercn", "--dangerously-skip-permissions"),
     Hybrid: true}`
- [x] 4.3 `internal/launch/types.go`：
      `normalize()` 图标白名单增加 `"qoder"`

## 5. 前端图标

- [x] 5.1 创建 `web/src/icons/lobehub/qoder.svg`：
      自定义 Qoder 品牌图标（LobeHub 上游无此图标），
      并在 `web/src/icons/lobehub/README.md` 中注明
- [x] 5.2 `web/src/components/settings/LaunchConfigIcon.svelte`：
      以 `~icons/lobe/qoder` 导入并注册 `qoder` 图标
- [x] 5.3 `web/src/domains/settings/launch-configs.ts`：
      `LAUNCH_ICON_OPTIONS` 增加
      `{value: "qoder", label: "Qoder"}`；
      `launchConfigIcon()` 自动检测
      `qoder`/`qodercn`/`qoderclicn`/`qoder-cn` → `"qoder"`

## 6. 集成测试

- [x] 6.1 `internal/agentprofile/agents/agents_test.go`：
      增加 qoder/qodercn 的命令分类、ACP argv 与识别用例；
      `internal/launch/store_test.go` 预置名称、图标与混合开关更新为 13 条；
      `internal/archtest/layers_test.go` 登记两个新包
- [x] 6.2 `internal/api/testdata/wire/` 与
      `internal/wire/testdata/response-arrays.json`：更新 wire 快照
- [ ] 6.3 `web/test/e2e/launch-configs.spec.ts`：
      验证 Qoder/QoderCN 预置配置的混合模式开关
- [ ] 6.4 `internal/acp/testdata/fakeagent` 扩展：
      增加 Qoder/QoderCN 档案的测试替身（`--acp` 标志启动、
      按可执行文件名选定档案、不发 OSC 7501 会话 ID），
      并在 `internal/hybrid` 测试中覆盖根进程可执行文件回退识别
      与 `session/list` 候选判定下的双向切换

## 7. 文档

- [x] 7.1 `docs/cli-agents.md`：新增 Qoder 与 QoderCN 段落
- [x] 7.2 `docs/hybrid-sessions.md`：更新支持的 agent 列表
- [x] 7.3 相关用户故事更新
      （`docs/user-stories/access-and-sessions.md`、`docs/user-stories/hybrid-sessions.md`）
