## Context

WebMux 的 `internal/acp` 已实现 Kimi 所需的 ACP v1 方法、配置项、图片/资源提示词、权限与表单回调；`internal/hybrid` 已提供根进程回退识别、`session/list` 候选判定、冷恢复配置补齐和 TUI/ACP 双向切换。Kimi 也已有预置与图标，因此本设计只增加适配器数据与少量注册，不扩展通用协议。

本机实测使用原生 Linux ELF `kimi 2.5.2`，结果如下：

- TUI 发出 OSC 7501 查询并报告 `app=kimi-code`。
- ACP 握手声明 `loadSession`、`list`、`resume`、`close`、图片与嵌入资源。
- `kimi acp` 只接受登录相关参数，普通 TUI 参数不能透传。
- 两轮 ACP 测试确认：模型与思考等级跨 load/resume 保留，模式在两种恢复后都回到 `default`。

## Goals / Non-Goals

**Goals:**

- 以现有 `agentprofile.Agent` 与可选接口完成 Kimi 接入，不改 hybrid 编排。
- 保证直接执行和 Bash/Zsh 启动文件包装使用同一 `kimi acp`、会话库与环境。
- 精确延续模型和模式，并只补齐实测会丢失的配置。
- 用 fakeagent 自动化覆盖 Kimi 专有线，用隔离真实 CLI 做最终验收。

**Non-Goals:**

- 不新增 ACP 方法、Kimi 私有扩展、steering、MCP 转发或额外目录传递。
- 不支持用户自定义 agent 档案，也不保证 `--agent`/`--agent-file` 的自定义主 Agent 在混合切换中保留。
- 不自动登录、注销、安装或升级 Kimi，不处理认证令牌。
- 不修改前端组件、图标、JSON/Zod 结构，不为其他安装方式臆造 node 启动器规则。
- 不持久化 Kimi trust，不修改全局 `require_trust`，也不把目录信任扩展为工具自动批准。

## Decisions

### D1：新增独立 Kimi 适配器，通用核心保持不变

新建 `internal/agentprofile/kimi`，导出 `Agent ap.Agent`。静态档案设置：

- `Name: "kimi"`、`Title: "Kimi Code CLI"`、`Icon: "kimi"`、`VerifiedVersion: "2.5.2"`；
- `ResumeFlag: "--session"`、`Positional: PositionalPrompt`；
- `HiddenCommands: ["login", "logout"]`；
- 默认 `ACPArg` 空值，即使用内置 `acp` 子命令；`Connector` 为空，原参数不透传；
- `InitialPrompt` 为空，初始用户指令沿用安静后键入的回退路径，因为 `-p` 是非交互模式；`Manager` 按 argv 保存档案声明的 200ms 文本/Enter 分隔，让浏览器草稿、REST 输入以及混合与普通会话行为一致。空正文只发 Enter 时不等待，延迟期间继续持有输入锁。

`Identify` 只解释 `ProgramApp == "kimi-code"`，返回 agent 与 `RootCwd`，不解释子记录 ID。`InterruptKeys` 返回一次 Esc；`ResumesSessions` 返回 true；`ReapplyOnLoad` 只返回 `[]string{"mode"}`。不实现 `Steerer`、`LockTracker` 或 `ProcessRegistry`。

备选方案是把 Kimi 特判写入 hybrid 或 ACP 核心；这会违反现有 agent 隔离架构，因此不采用。

### D2：参数表以恢复安全和 ACP 可执行性为边界

参数分类如下：

| 分类 | 参数 |
|---|---|
| Keep（恢复保留） | `-m`/`--model`（单值）、`--skills-dir`（单值、可重复）、`--add-dir`（单值、可重复）、`--auto`、`--yolo`、`--plan` |
| Drop | `-c`/`--continue`、`-S`/`--session`（可选值）、位置提示词及未白名单参数 |
| Store | `--environment`（单值） |
| Directory | `--environment-cwd`（单值） |
| NonInteractive | `-p`/`--prompt`、`--output-format`（单值）、`-h`/`--help`、`-V`/`--version` |

非交互子命令表覆盖当前 CLI 的 `export`、`fork`、`provider`、`session`、`acp`、`web`、`server`、`rc`/`remote`、`login`、`doctor`、`vis`、`install-desktop`/`install-app`、`app`、`migrate`、`import-sessions`、`upgrade`/`update` 与 `help`。

`--agent`、`--agent-file` 不进入 Keep：Kimi 明确禁止它们与 `--session` 组合。它们按通用未知/丢弃参数处理，避免恢复命令启动失败。`KIMI_CODE_HOME` 是环境变量而非 argv，沿用“两端环境一致”即可保证同一会话库。

备选方案是透传 `--model` 或 `--auto` 到 ACP；实测 `kimi acp --auto` 直接以未知参数退出，因此不采用。

### D3：argv 只推导模型与模式，恢复只重应用模式

`ChatSide` 将 model 映射到 `model`，将三种模式参数映射到 ACP 的 `default/plan/auto/yolo` 集合中的同名值。`--plan` 优先于自动模式，未采用的自动模式参数进入 unsupported；`--skills-dir`、`--add-dir` 也进入 unsupported。`--auto` 与 `--yolo` 本身的互斥不在 WebMux 重复实现。

`ReapplyOnLoad` 不包含 `model` 或 `thinking`。这是与 Codex、Qoder 等档案不同的实测行为，必须保持精确，避免无意义的配置写入和潜在覆盖。

Kimi 还可能让底层 mode 与 load/resume 报告值不一致：底层已经是目标 mode，却报告 `default`，再次设置返回 `Already in <mode> mode`。适配器通过 `ConfigErrorHandler` 只接受完全匹配的幂等 mode 错误，冷恢复与手动设置都由 hybrid 将目标值作为最终实际值；其他错误仍按失败提示。

备选方案是把所有配置都重应用，实现虽更简单但违背实测，并可能覆盖 Kimi 自己恢复的状态，因此不采用。

### D4：会话识别完全复用现有候选机制

Kimi 没有适配器自有 ID 来源时，`identityFallback()` 已会启用 `session/list` 候选判定；根进程 `kimi` 与 `app=kimi-code` 都可完成 agent 识别。无需修改 `internal/hybrid/eligibility.go`。

从聊天切到 TUI 时，`--session <id>` 由 `resumedSID` 提供精确回退；从全新 TUI 切到聊天时，唯一近期候选自动采用，多候选沿用 `session_ambiguous` 选择。`session_` 前缀只作为不透明字符串处理。

### D5：只更新新安装预置，不做状态迁移

`internal/launch/presets.go` 中 Kimi 项改为 `command("kimi", "--auto")` 并设 `Hybrid: true`。总数不变，图标白名单与前端不变。已有 `launch-configs.json` 不调用 Seed 覆盖，旧 Kimi 配置只有用户手动启用时才按新档案校验。

### D6：fakeagent 模拟差异，真实 CLI 验证契约

fakeagent 增加 `kimi` 档案选择：TUI 报告 `app=kimi-code` 且不报告会话 ID；ACP 提供 `model`、`thinking`、`mode`，load/resume 保留前两项并重置 mode。自动化测试覆盖 argv、候选选择、双向切换、崩溃 resume、模式补齐以及模型/思考等级不被重应用。

真实 CLI 不进入 CI：实施验收使用隔离 cwd 和仅由测试创建的会话，完成 TUI 首条消息、TUI→ACP、ACP→TUI、再次 ACP、取消与清理。不得连接现有开发者会话或删除非测试会话。

### D7：pre-launch 返回会话环境，Kimi 使用官方单进程 trust

将 `session.Options.PreLaunch` 改为返回补充后的环境 map。`resolveCreate` 把该结果交回 `Create`，再由 `startOptions` 使用；hook 仍接收 argv/env 副本，其他 agent 的行为和返回环境不变。这样补充变量进入会话自身环境，而不是只存在于 hook 的临时副本。

`agenttrust` 注册 `kimi`。通过现有 `trustable` 校验后，Kimi 不写配置，只在会话和服务环境都未显式设置 `KIMI_CODE_TRUST_WORKSPACE` 时补充 `1`。显式空值或 falsy 值也视为用户决定；`agent_pretrust=false` 不安装 hook。由于 ACP 与恢复 TUI 都沿用会话环境，无需给 Profile 增加另一套 TUI trust 环境字段。

官方文档说明该变量等价于对当前工作区选择“Trust this folder”，只作用于当前进程且不写持久记录。隔离对照实测中，未设置时出现 `blocked/permission`，设置后出现 `idle`。因此不采用以下方案：

- `[workspace] require_trust=false`：会全局关闭其他工作区的信任门。
- 修改 `config.toml` 或推测持久信任记录：没有必要，且会扩大变更与回滚范围。
- 只把变量写入新预置：自定义 Kimi 命令和 chat-first 会话无法覆盖。

## Risks / Trade-offs

- [Kimi 后续版本改变参数或恢复语义] → 档案记录 2.5.2 实测版本；旧版本只提示不阻断，真实验收复核握手与配置恢复，fakeagent 固定当前基线。
- [同 cwd 多个近期会话导致误选] → 不猜测，复用 `session_ambiguous` 与用户选择；从聊天恢复的 TUI 优先使用精确 ID。
- [`--auto` 自动执行工具的风险] → 只改变新安装预置，保持参数显式可见且可编辑；已有配置不自动获得该权限。
- [目录 trust 被误解为工具自动批准] → trust 只跳过工作区确认，不改变 Kimi 权限模式；pre-launch 继续拒绝主目录及其祖先。
- [pre-launch 签名变化影响其他会话] → 保持其他 agent 返回环境不变，并以 manager 测试证明补充环境真正进入启动进程。
- [自定义 Agent、skills 或额外目录在聊天端不一致] → 不透传、不假装生效；能保留到 TUI 的参数列入 unsupported，自定义 Agent 在恢复时按丢弃参数提示。
- [TUI 启动权限页使自动化停顿] → 真实验收在隔离空目录显式处理启动权限，不以进程存活或 OSC idle 代替首条消息成功。
- [Kimi 外部客户端双写无法仅靠 WebMux 全部发现] → 保持现有内部占用检测与错误回滚，不宣称完整的外部占用保证。

## Migration Plan

1. 发布适配器、预置与测试，不执行状态文件迁移。
2. 新安装自然获得 `kimi --auto` 的混合预置；已有安装按需手动勾选。
3. 回滚代码不会影响升级前配置。若回滚到不支持 Kimi 的版本，新安装在本变更后生成的 Kimi 混合配置需先由用户取消混合，避免旧版本无法校验该档案。
