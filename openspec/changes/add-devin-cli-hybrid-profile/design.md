## Context

WebMux 的 `internal/acp` 已实现 Devin 所需的 ACP v1 方法、配置项、图片/嵌入资源提示词与权限回调；`internal/hybrid` 已提供根进程回退识别、`session/list` 候选判定与 TUI/ACP 双向切换，且切换会先 `EndRun` 结束 TUI 再加载会话。

Devin 的 TUI 本身就是 ACP 客户端（会话内由 `devin acp` 子进程服务），因此本设计主要增加适配器数据、trust 记录与少量注册，不扩展通用协议。实测基线见 `baseline.md`。

## Goals / Non-Goals

**Goals:**

- 以现有 `agentprofile.Agent` 与可选接口完成 Devin 接入，不改 hybrid 编排。
- 直接执行与 Bash/Zsh 启动文件包装使用同一 `devin acp`、会话库与凭据。
- 会话 ID（词组 slug）全部经 `session/list` 候选判定获得，不在终端信号中臆造。
- 用 fakeagent 自动化覆盖 Devin 专有线（slug ID、无 `session/resume`、锁），用隔离真实 CLI 做最终验收。

**Non-Goals:**

- 不新增 ACP 方法，不实现 `cognition.ai/*` 私有扩展（mentionTypes、midTurnPrompts、terminalLifecycle、sessionShare、turn_stats 等），不做 Devin steering、`session/delete` 或多根工作区。
- 不支持 `--cloud`（云端会话库）、`--agent-type` 非默认 agent 与 `devin acp --cloud` 中继的混合模式。
- 不自动登录、注销、安装或升级 Devin，不处理凭据；`devin auth` 由用户在 TUI 或 `devin auth login` 完成。
- 不保证外部 devin 进程占用会话时能无损接管；只提供强关恢复路径与清晰错误。
- 不修改 JSON/Zod 结构，不新增连接器。

## Decisions

### D1：新增独立 `devin` 适配器，通用核心保持不变

新建 `internal/agentprofile/devin`，导出 `Agent ap.Agent`。静态档案：

- `Name: "devin"`、`Title: "Devin CLI"`、`Icon: "devin"`、`VerifiedVersion: "3000.12.57"`；
- `ResumeFlag: "--resume"`、`Positional: PositionalDirectory`（`devin [PATH]...` 会打开 Desktop，按目录参数拒绝）；
- `HiddenCommands: ["login", "logout"]`；
- 默认 `ACPArg`（`acp` 子命令）、`Connector` 为空；`InitialPrompt` 返回 `["--", prompt]`；
- `InterruptKeys` 单次 Esc；`ResumesSessions` 返回 `false`（实测 `session/resume` 为 -32601）。

`Identify` 只解释 `ProgramApp == "devin"`，返回 agent 与 `RootCwd`；不解释子记录。不实现 `Steerer`、`LockTracker`、`ProcessRegistry`——`identityFallback()` 因而自动启用 `session/list` 候选判定，无需改 `internal/hybrid/eligibility.go`。

备选方案是把锁与会话 slug 特判写入 hybrid/ACP 核心；违反 agent 隔离架构，不采用。

### D2：参数表以会话库一致性与恢复安全为边界

| 分类 | 参数 |
|---|---|
| Passthrough（保留并透传 `devin acp`） | `--model`（单值） |
| Keep（仅 TUI） | `--permission-mode`、`--verbosity`（单值）、`--sandbox`、`--respect-workspace-trust`（可选值）、`--export`（可选值） |
| Drop | `--prompt-file`（单值）、`--` 之后的位置参数与未识别参数 |
| Session（拒绝混合） | `-c`/`--continue`、`-r`/`--resume`（可选值） |
| Store（拒绝混合） | `--cloud`、`--config`（单值） |
| NonInteractive（拒绝） | `-p`/`--print`（可选值）、`-h`/`--help`、`-V`/`--version` 与全部子命令（清单见下） |
| Directory（拒绝） | 位置 PATH 参数（打开 Desktop） |

NonInteractive 覆盖的子命令为 `acp`、`auth`、`cloud`、`desktop`、`doctor`、`forward`、`help`、`list`、`ls`、`mcp`、`migrate`、`models`、`plugins`、`rm`、`rules`、`sandbox`、`setup`、`skills`、`ssh`、`uninstall`、`update`、`version`。

`--resume`/`--continue` 遵循 Claude Code 先例按 `session_arg` 拒绝而非丢弃：它们明确选择已有会话或打开选择器，静默丢弃会让配置的语义与执行不符。`--config` 可改变 `session_store` 等存储配置且 `devin acp` 无对应参数，两端无法保证一致，按 `session_store` 拒绝。

备选方案是把 `--model` 也仅作 Keep；实测 `devin acp --model` 存在且语义一致，透传可让两端模型一致，因此不采用。

### D3：聊天端配置推导只映射能对齐的项

`ChatSide`：`--model` → `model`；`--permission-mode` 的 `accept-edits`→`accept-edits`、`smart`→`smart`、`dangerous`→`bypass` 映射 `mode`；`auto` 无对应值，连同 `--sandbox`、`--verbosity`、`--respect-workspace-trust`、`--export` 一起列入 `unsupported`，由既有提示告知用户哪些参数不影响聊天端。

ACP 的 `mode` 另有 `ask`/`plan`，仅通过聊天端配置项修改，不由 argv 推导。

`ReapplyOnLoad` 待实施期按跨进程 `session/load` 实测决定（见 Open Questions）；不预先写死。

### D4：会话 ID 完全复用 `session/list` 候选判定

Devin 会话 ID 是词组 slug（`luminous-acorn`），作为不透明字符串处理，不改写、不假设 UUID。

从聊天恢复的 TUI 用 `resumedSID` 精确回退；全新 TUI 用“规范化 cwd + 本次启动后 updatedAt”候选：零消息会话不出现在 `session/list`（实测），“聊天”按钮保持不可用并提示先发送消息，多候选沿用 `session_ambiguous`。`isLocked` 条目仍可作为候选：锁属正常持锁状态，候选判定不额外过滤。

### D5：会话锁只提供恢复路径，不改变切换编排

切换的 `EndRun` 先于 `session/load`，正常路径不会遇到 `session_locked`。锁只在两类异常出现：外部 devin 进程占用（`lockHolderPid` 存活），或崩溃残留锁。处理策略：

- `session/load` 返回 `-32015 session_locked`（`retryable`）时，若 `lockHolderPid` 对应进程已不存在，先按 `sessionCapabilities.close._meta."cognition.ai/force"` 发一次强制 `session/close` 并重试 `session/load`（已实测可行）；
- 持锁进程仍存活时按现有冲突错误呈现（会话被外部占用），不强行接管。

是否复用现有错误→恢复通道或新增最小钩子由实施按代码现状决定，档案只声明行为要求。

备选方案是把 `isLocked` 会话从候选排除；锁是瞬态信息而候选判定先于加载，排除会误伤正常会话，不采用。

### D6：agenttrust 预写 Devin 信任记录

`agenttrust` 注册 `devin`，把会话目录追加到 `trusted_workspaces.json` 的 `trusted_paths`（位置为 `$XDG_DATA_HOME/devin/cli/`，默认 `~/.local/share/devin/cli/`）；文件缺失时创建最小 JSON，已有记录不重复。

仍沿用 `trustable` 的主目录及祖先拒绝规则；失败仅记日志不阻断启动，Devin 退回自带 `Trust <dir>?` 提示。

备选方案是注入 `--respect-workspace-trust false`：它跳过全部信任检查而非只信任所选目录，范围过大，不采用。

### D7：预置、图标与 fakeagent

- `internal/launch/presets.go` 新增 `Devin CLI`：`command("devin", "--permission-mode", "dangerous")`，`Hybrid: true`，图标 `devin`。权限模式沿用其他预置的显式自动批准惯例；已有 `launch-configs.json` 不迁移。
- 图标：lobe-icons 固定提交中查找 `devin`，缺失则仿 `qoder` 自定义单色 SVG；接入 `LaunchConfigIcon.svelte`、`launch-configs.ts` 选项与 `devin` 可执行文件自动匹配。
- fakeagent 增加 `devin` 档案：可执行名 `devin` 自动选定；发 `app=devin` 根记录（无子记录）；slug 会话 ID；`session/resume` 返回 Method not found；`session/list` 附 `isLocked`/`lockHolderPid` 元数据并可模拟 `session_locked` 与强制 `session/close` 释放；Esc 中断。

## Risks / Trade-offs

- [Devin 后续版本改变 ACP 能力或锁语义] → 档案记录 `3000.12.57` 实测版本；旧版本只提示不阻断；fakeagent 固定当前基线，真实验收复核握手与锁恢复。
- [`agentInfo.version` 为 `0.0.0-dev`，无法用作版本核对] → 版本比较以 `devin --version` 输出为准，在档案注释与文档中注明。
- [同 cwd 多个近期会话误选] → 不猜测，复用 `session_ambiguous` 与用户选择；恢复来的 TUI 优先精确 ID。
- [`--permission-mode dangerous` 自动批准的风险] → 只影响新安装预置，参数显式可见可编辑；已有配置不自动获得。
- [强关持锁会话使原 TUI 进入错误态] → 只对 `lockHolderPid` 已死的残留锁自动强关；存活持有者报错，不静默接管外部会话。
- [trust 预写被误解为工具自动批准] → 只追加 `trusted_paths`，不改变 Devin 权限模式；主目录及祖先仍拒绝。
- [`session/list` 候选依赖 `hasUserMessages`/updatedAt 的实测边界] → 候选规则与 Kimi 一致；未发消息时按钮不可用是既有提示，不新增行为。

## Migration Plan

1. 发布适配器、预置、图标与测试，不执行状态文件迁移。
2. 新安装自然获得 `devin --permission-mode dangerous` 混合预置；已有安装按需手动勾选混合模式。
3. 回滚代码不影响升级前配置；若回滚到无 `devin` 档案的版本，本变更后创建的 Devin 混合配置需先由用户取消混合。

## Open Questions

- 跨进程 `session/load` 后 `mode`/`model` 是否重置（决定 `ReapplyOnLoad` 内容）：实施期用两个 `devin acp` 进程对照实测。
- `session/prompt` 实时回合是否回显 `user_message_chunk`（决定是否需要 agy 式忽略规则）：实施期实测。
- `devin acp` 经 `.bashrc` fd 3 包装、权限请求选项形态与 `session/cancel` 的 `stopReason`：实施期补齐。
