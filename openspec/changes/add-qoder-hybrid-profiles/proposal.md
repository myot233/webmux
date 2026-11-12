## Why

混合模式已支持 OpenCode、omp、Codex CLI、Claude Code 与 Antigravity CLI（`agy`），
但预置的 Qoder 与 QoderCN 启动配置只能作为普通终端会话使用。

Qoder（国际版，`qoder`/`qodercli`）与 QoderCN（国内版，`qodercn`/`qoderclicn`/`qoder-cn`）
均为支持 ACP v1 的 CLI agent：`qoder --acp` / `qodercn --acp`
在 stdin/stdout 上运行 JSON-RPC，具备以下能力：

- `session/new`、`session/load`、`session/resume`、`session/list`
- `session/close`、`session/fork`、`session/delete`
- 配置项、附件、权限、插话（`_meta.qoder.promptQueueing`）

两者 CLI 参数与 Claude Code 高度一致：
`--resume <uuid>`、`--continue`、`--model`、`--permission-mode`、
`--dangerously-skip-permissions` 等。

本变更让 Qoder 与 QoderCN 在同一 WebMux 会话中
双向切换原生 TUI 与 ACP 聊天视图，
复用现有混合会话、聊天队列与恢复能力。

## What Changes

- 档案数据模型扩展：新增 `ACPArg` 字段，
  允许档案声明 ACP 启动参数（默认 `"acp"` 子命令；
  Qoder/QoderCN 为 `"--acp"` 标志）。
  `ACPArgv()` 使用该字段替代硬编码的 `"acp"`。
- 新增 Qoder 适配器包 `internal/agentprofile/qoder`：
  `qoder.New(Variant)` 按版本（名称、标题、可执行文件、OSC 7501 程序 id）
  构造适配器。Qoder 档案（`qoder`，标题 "Qoder"）识别 `qoder` 与 `qodercli`
  可执行文件及其版本化名称（如 `qodercli-1.1.81`），内置 ACP（`qoder --acp`，
  不透传参数），`--resume <uuid>` 恢复，参数分类表与 Claude Code 对齐，
  一次 Esc 中断，支持 `session/resume`、配置重应用与插话，实测版本 1.1.81。
- 新增 QoderCN 档案（`qodercn`，标题 "QoderCN"）：
  复用 `qoder.New`，识别 `qodercn`、`qoderclicn` 与 `qoder-cn` 可执行文件
  及其版本化名称，OSC 7501 程序 id 为 `qodercn`、`qoder-cli-cn` 或 `qoderclicn`，
  其余行为与 Qoder 档案相同。
- 会话识别：两者均不通过 OSC 7501 子记录传递会话 ID。
  终端信号未识别出 agent 时，系统回退按 PTY 根进程可执行文件匹配当前配置的档案；
  对既不跟踪写锁也不读取进程注册记录的档案，在 `session/list` 中取同一 cwd 下、
  更新时间晚于本次 TUI 启动的会话作为候选：唯一时自动选择，多个时返回
  `session_ambiguous` 由用户选择，没有候选时保持不可切换。
- 从 argv 推导首次聊天端的 `model`、`effort` 与 `mode`
  （`-m`/`--model`、`--reasoning-effort`、`--permission-mode`、
  `--dangerously-skip-permissions`/`--yolo`）；
  冷 load/resume 后重应用 model/effort。
- 隐藏聊天菜单中的 `login` 与 `logout` 命令。
- 新安装的 Qoder 与 QoderCN 预置配置改为 `hybrid: true`，
  默认带 `--dangerously-skip-permissions`；
  已有 `launch-configs.json` 不改写。
- 新增 `qoder` 图标：服务端白名单、前端下拉与自动检测、Svelte 组件
  与自定义 SVG 资源（`web/src/icons/lobehub/qoder.svg`）同步注册。QoderCN 复用同一图标。

## Capabilities

### New Capabilities

无。本次复用现有内置 ACP、混合会话、聊天队列与恢复能力。

### Modified Capabilities

- `hybrid-agent-profiles`：内置档案增至 7 个
  （OpenCode、omp、Codex、Claude Code、Qoder、QoderCN、`agy`）；
  ACP 启动命令支持 `--acp` 标志式启动；
  声明 Qoder 与 QoderCN 的参数、恢复、配置推导、版本与聊天能力。
- `agent-session-identity`：新增根进程可执行文件回退识别，
  以及无自有会话 ID 来源档案的 `session/list` 候选判定。
- `launch-configs`：新安装预置增至 13 条，Qoder 与 QoderCN 预置默认混合，
  已有配置保持不变；新增 `qoder` 图标。

## Impact

- Go：
  - 新增 `internal/agentprofile/qoder/qoder.go`（含 `Variant` 与 `New`）
  - 新增 `internal/agentprofile/qodercn/qodercn.go`（复用 `qoder.New`）
  - `internal/agentprofile/profile.go` 新增 `ACPArg` 字段
  - `internal/agentprofile/command.go` 的 `ACPArgv()` 使用 `ACPArg`
  - `internal/agentprofile/agenttest/agenttest.go` 契约测试支持 `ACPArg`
  - `internal/agentprofile/agents/agents.go` 注册两个新档案
  - `internal/hybrid/eligibility.go` 新增根进程可执行文件回退识别与
    `identityFallback()` 候选探测条件
  - `internal/launch/presets.go` 更新预置配置
  - `internal/launch/types.go` 图标白名单增加 `qoder`
- Web：
  - `web/src/domains/settings/launch-configs.ts` 增加 `qoder` 图标选项与自动检测
  - `web/src/components/settings/LaunchConfigIcon.svelte` 导入并注册图标组件
  - `web/src/icons/lobehub/qoder.svg` 新增自定义 Qoder 图标
- 测试：扩展 agentprofile 契约测试与 agents 分类测试、launch 预置测试、
  wire 快照；fakeagent 的 Qoder/QoderCN 替身、hybrid 切换测试与
  E2E 启动配置用例待补（见 tasks 6.3、6.4）。
- 部署：聊天端使用内置 `--acp`，无需额外安装连接器；
  TUI 端不依赖连接器。
- 用户故事与文档：更新混合会话相关用户故事、
  `docs/cli-agents.md` 与 `docs/hybrid-sessions.md`。
