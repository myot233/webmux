## ADDED Requirements

### Requirement: Kimi 的 ACP 环境与认证

Kimi 的 ACP 进程与资格探测 SHALL 复用会话环境、服务用户、`HOME` 与 `KIMI_CODE_HOME`，直接运行同一配置的 `kimi acp`。两端 SHALL 复用 Kimi 已有登录状态，系统 SHALL NOT 为聊天端要求独立凭据、自动执行登录或注销，也 SHALL NOT 在记录、负载或日志中保存凭据值。

聊天命令菜单 SHALL 隐藏 `login` 与 `logout`。Kimi 返回认证错误时 SHALL 按通用 agent 错误呈现，用户须在 Kimi TUI 或终端完成登录。

#### Scenario: 复用现有登录

- **WHEN** 服务用户已在同一 `KIMI_CODE_HOME` 登录 Kimi Code CLI，未设置额外 ACP 凭据
- **THEN** 聊天端以该身份创建和恢复会话，不启动新的登录流程

#### Scenario: 认证命令不出现在菜单

- **WHEN** 用户在 Kimi 混合会话的聊天视图中输入 `/`
- **THEN** 命令菜单中没有 `/login` 或 `/logout`

### Requirement: Kimi 的目录级文件夹信任

`agent_pretrust` 开启且启动命令识别为 `kimi` 时，系统 SHALL 在首次启动前为会话补充 `KIMI_CODE_TRUST_WORKSPACE=1`，使当前工作区在本次 TUI、恢复 TUI 与 ACP 进程中获得 Kimi 官方的目录级信任。该变量 SHALL 随会话环境传递，SHALL NOT 写入 `config.toml`、设置全局 `require_trust=false` 或信任其他目录。

系统 SHALL 沿用现有文件夹信任边界：主目录、其祖先与文件系统根 SHALL NOT 被自动信任。会话或服务环境已显式设置 `KIMI_CODE_TRUST_WORKSPACE`（包括 falsy 值）时 SHALL 保留用户设置；`agent_pretrust=false` 时 SHALL NOT 补充该变量或写入任何信任记录。

#### Scenario: 新 Kimi 会话跳过启动信任页

- **WHEN** `agent_pretrust` 开启，用户在普通项目目录新建 Kimi 会话，未显式设置 trust 变量
- **THEN** 会话环境包含 `KIMI_CODE_TRUST_WORKSPACE=1`，TUI 不停留在启动信任页，Kimi 配置文件中不产生全局信任设置

#### Scenario: ACP 继承同一目录信任

- **WHEN** Kimi 混合会话从 TUI 切到聊天或从聊天恢复到 TUI
- **THEN** 两端都继承首次启动时确定的 trust 变量，工作区与首次选择一致

#### Scenario: 用户显式设置优先

- **WHEN** 会话环境包含 `KIMI_CODE_TRUST_WORKSPACE=0`，或服务配置 `agent_pretrust=false`
- **THEN** WebMux 不把变量改写为 `1`，Kimi 按自身规则决定是否显示信任页

#### Scenario: 拒绝过宽目录

- **WHEN** 用户试图在主目录、主目录祖先或文件系统根自动预信任 Kimi
- **THEN** WebMux 不补充 trust 变量，记录既有警告并允许启动流程按原规则继续

### Requirement: Kimi 的聊天端配置推导

首次在聊天端打开 Kimi 混合会话时，系统 SHALL 从白名单 argv 推导配置种子：

- `-m`/`--model <model>` → `model=<model>`；
- `--auto` → `mode=auto`；
- `--yolo` → `mode=yolo`；
- `--plan` → `mode=plan`。

`--skills-dir`、`--add-dir` 以及其他未推导的保留参数 SHALL 列入“以下参数在聊天端不生效”，SHALL NOT 透传给 `kimi acp`。多个模式参数同时出现时，`--plan` SHALL 优先于 `--auto` 与 `--yolo`，未采用的模式参数 SHALL 列为聊天端不生效；`--auto` 与 `--yolo` 的冲突仍由 Kimi 自身校验。

#### Scenario: 预置的自动模式

- **WHEN** 新安装的 Kimi 预置以 `kimi --auto` 从聊天开始
- **THEN** 首次聊天端配置种子包含 `mode=auto`，ACP 进程仍只执行 `kimi acp`

#### Scenario: 模型与计划模式

- **WHEN** 配置 argv 为 `["kimi","--model","provider/model","--plan"]`
- **THEN** 配置种子包含 `model=provider/model` 与 `mode=plan`

#### Scenario: 额外目录不静默生效

- **WHEN** 配置 argv 为 `["kimi","--add-dir","/srv/lib","--skills-dir","/srv/skills"]`
- **THEN** 聊天视图列出 `--add-dir /srv/lib` 与 `--skills-dir /srv/skills` 在聊天端不生效

### Requirement: Kimi 冷恢复后的配置补齐

Kimi 2.5.2 在新 ACP 进程执行 `session/load` 或 `session/resume` 后保留 `model` 与 `thinking`，但将 `mode` 返回为 `default`。因此 Kimi 档案 SHALL 只声明 `mode` 为冷恢复后需要重应用的配置项，SHALL NOT 重应用 `model` 或 `thinking`。

重应用 SHALL 使用 WebMux 已记录的配置值；首次从带模式参数的启动配置打开时，使用推导种子。Kimi 返回的实际配置 SHALL 作为界面最终状态，设置失败按通用规则提示，不得显示请求值已生效。

Kimi 在底层模式已经等于目标值时可能返回 `Already in <mode> mode` 错误，即使 load/resume 的配置字段仍显示 `default`。适配器 SHALL 仅对目标 mode 完全匹配的该错误按幂等成功处理，并将界面值设为已生效的目标 mode；其他配置错误 SHALL NOT 被吞掉。

#### Scenario: 冷恢复保留模式

- **WHEN** 用户已将 Kimi 聊天端 `mode` 改为 `plan`，随后切到 TUI 再切回聊天，新的 ACP 进程在 load/resume 后返回 `mode=default`
- **THEN** 系统自动调用 `session/set_config_option` 恢复 `mode=plan`

#### Scenario: 不覆盖已恢复的模型与思考等级

- **WHEN** 新的 ACP 进程在 load/resume 后返回此前的 `model=provider/model` 与 `thinking=low`
- **THEN** 系统不对这两项调用 `session/set_config_option`，只处理需要重应用的 `mode`

#### Scenario: 目标模式已生效的幂等错误

- **WHEN** 重应用 `mode=plan` 时 Kimi 返回 `Already in plan mode`，但配置字段仍为 `default`
- **THEN** 系统按成功处理并在聊天视图显示 `mode=plan`，不提示恢复失败

#### Scenario: 手动选择已生效的模式

- **WHEN** 用户手动设置 `mode=plan` 时 Kimi 返回 `Already in plan mode`
- **THEN** 系统同样按幂等成功处理并显示 `mode=plan`，不提示设置失败

#### Scenario: 不吞掉其他配置错误

- **WHEN** Kimi 对 mode 返回与目标不一致的错误或其他配置项返回错误
- **THEN** 系统按通用规则提示恢复失败，不伪造成功

### Requirement: Kimi 的聊天端回合能力

Kimi 档案 SHALL 声明 `session/resume` 可用于崩溃重连，SHALL 复用握手中的图片与嵌入资源能力以及现有权限、表单、消息队列和取消管线。音频输入 SHALL 按 Kimi 未声明支持处理。

Kimi 2.5.2 未声明 WebMux 可识别的私有 steering 能力，因此系统 SHALL NOT 对 Kimi 调用私有 steering 扩展；插话 SHALL 使用通用排队规则。`session/resume` SHALL 无历史回放，`session/load` SHALL 按通用规则开启新的日志生成并接收回放。

通过浏览器草稿或 API 向混合或普通 Kimi TUI 提交文本与 Enter 时，系统 SHALL 在同一确认回执和输入临界区内分隔两次写入，间隔 200 毫秒，避免 Enter 被当作粘贴内容。

- 延迟期间系统 SHALL 继续持有输入锁，其他输入不得交错；
- 空正文只发 Enter 时 SHALL NOT 等待；
- 其他 agent 的提交间隔 SHALL 保持为 0；
- 重试同一请求 SHALL NOT 因分隔而重复写入。

#### Scenario: 崩溃后无回放重连

- **WHEN** Kimi ACP 进程异常退出且 Kimi 仍声明 `session/resume`
- **THEN** WebMux 以 `session/resume` 重连，不重复回放整段历史

#### Scenario: 取消进行中的回合

- **WHEN** Kimi 回合正在输出，用户点击停止
- **THEN** WebMux 发送 `session/cancel`，最终 `session/prompt` 返回 `stopReason=cancelled`，界面按通用规则结束该回合

#### Scenario: 文本与 Enter 分隔提交

- **WHEN** 用户通过浏览器草稿或 API 向混合或普通 Kimi TUI 提交一条文本消息
- **THEN** 文本写入 200ms 后才写入 Enter，整次提交只有一个确认回执，重试不重复发送

#### Scenario: 空正文只发送 Enter

- **WHEN** 用户向 Kimi TUI 提交空正文和 Enter
- **THEN** 系统立即写入 Enter，不额外等待 200ms

#### Scenario: 其他 agent 不增加延迟

- **WHEN** 用户向非 Kimi 混合会话提交文本
- **THEN** 文本与 Enter 的提交间隔保持为 0，行为与接入 Kimi 前一致
