## ADDED Requirements

### Requirement: Kimi Code CLI 档案

系统 SHALL 内置 Kimi Code CLI 档案，稳定标识为 `kimi`，显示名称为 `Kimi Code CLI`，图标为现有 `kimi`，实测版本为 2.5.2。直接执行 `kimi` 或使用既有 Bash/Zsh 启动文件包装的 `kimi` 配置 SHALL 判定为支持混合模式，不需要第三方 ACP 连接器。

Kimi 的 ACP 启动命令 SHALL 为 `kimi acp`，原配置参数 SHALL NOT 透传给 ACP 子命令。TUI 恢复命令 SHALL 保留白名单参数并追加 `--session <sessionId>`；中断键 SHALL 为一次 Esc（`\x1b`）。版本低于 2.5.2 时 SHALL 按通用规则提示但不阻止切换。

#### Scenario: 预置的 bashrc 包装

- **WHEN** 配置的 argv 为 `["bash","-ic","exec -- \"$@\"","webmux","kimi","--auto"]`
- **THEN** 判定为可用，`hybridSupport.agent` 为 `kimi`，且没有连接器信息

#### Scenario: ACP 不透传启动参数

- **WHEN** 配置的 argv 为 `["kimi","--model","kimi-code-plan-cn/k3.1-pro","--auto"]`，会话从聊天开始
- **THEN** ACP 进程的 argv 为 `["kimi","acp"]`，不包含 `--model` 或 `--auto`

#### Scenario: 恢复到 TUI

- **WHEN** Kimi 混合会话切回 TUI，agent 会话 ID 为 `session_a9599cab-d9fd-4a01-8aeb-e2022d4862aa`
- **THEN** 恢复命令保留白名单参数并追加 `--session session_a9599cab-d9fd-4a01-8aeb-e2022d4862aa`

### Requirement: Kimi 恢复命令保留的参数

Kimi 的恢复命令 SHALL 保留 `-m`/`--model`、可重复的 `--skills-dir` 与 `--add-dir`，以及开关 `--auto`、`--yolo`、`--plan`。这些参数 SHALL 支持 `--flag value` 与 `--flag=value` 形式；每个带值参数只消费一个值。

位置参数、`-c`/`--continue`、`-S`/`--session`（值可选）以及白名单之外的参数 SHALL 按通用规则丢弃。特别是 `--agent` 与 `--agent-file` SHALL NOT 进入带 `--session` 的恢复命令，因为 Kimi 禁止自定义 agent 选择参数与会话恢复参数组合。

#### Scenario: 白名单参数进入恢复命令

- **WHEN** 配置的 argv 为 `["kimi","--model","provider/model","--skills-dir","/srv/skills","--add-dir=/srv/lib","--auto"]`，会话切回 TUI 恢复会话 `session_1`
- **THEN** 恢复命令依次包含这些白名单参数与 `--session session_1`

#### Scenario: 自定义 agent 参数不破坏恢复

- **WHEN** 配置的 argv 为 `["kimi","--agent","reviewer"]` 或 `["kimi","--agent-file","/tmp/reviewer.md"]`，会话切回 TUI 恢复会话 `session_1`
- **THEN** 恢复命令不包含 `--agent` 或 `--agent-file`，只包含 `--session session_1`，丢弃参数按通用规则提示

### Requirement: Kimi 会话选择、目录与会话库参数

`--environment <id>` SHALL 使配置不可用于混合模式，原因为会话库或运行环境不一致；`--environment-cwd <path>` SHALL 使配置不可用于混合模式，原因为命令自行指定工作目录。

`-c`/`--continue` 与 `-S`/`--session` SHALL 作为恢复时丢弃的会话选择参数处理，SHALL NOT 使配置不可用，也 SHALL NOT 进入新的恢复命令或 ACP 命令。

#### Scenario: 远端环境不可用

- **WHEN** 用户对 argv 为 `["kimi","--environment","remote"]` 的配置勾选混合模式
- **THEN** 保存返回 400，原因为聊天视图无法找到同一会话库

#### Scenario: 指定远端工作目录不可用

- **WHEN** 用户对 argv 为 `["kimi","--environment-cwd","/srv/project"]` 的配置勾选混合模式
- **THEN** 保存返回 400，原因提示在新建会话时选择目录

#### Scenario: 旧会话选择参数被丢弃

- **WHEN** 配置的 argv 为 `["kimi","--session","session_old"]`
- **THEN** 配置可用于混合模式，后续恢复命令不包含 `--session session_old`，只追加当前会话 ID

### Requirement: Kimi 不启动交互界面的参数

`-p`/`--prompt`、`--output-format`、`-h`/`--help`、`-V`/`--version` SHALL 使配置不可用于混合模式，原因为“该参数不启动交互界面”。

首个位置参数为 `export`、`fork`、`provider`、`session`、`acp`、`web`、`server`、`rc`、`remote`、`login`、`doctor`、`vis`、`install-desktop`、`install-app`、`app`、`migrate`、`import-sessions`、`upgrade`、`update` 或 `help` 时，SHALL 以相同原因拒绝混合模式。

#### Scenario: 非交互提示词

- **WHEN** 用户对 argv 为 `["kimi","-p","总结改动"]` 的配置勾选混合模式
- **THEN** 保存返回 400，原因为该参数不启动交互界面

#### Scenario: 会话管理子命令

- **WHEN** 用户对 argv 为 `["kimi","session","list"]` 或 `["kimi","fork"]` 的配置勾选混合模式
- **THEN** 保存返回 400，原因为该参数不启动交互界面

## MODIFIED Requirements

### Requirement: 内置档案与可识别的命令写法

系统 SHALL 内置 OpenCode、omp、Codex、Claude Code、Antigravity CLI（`agy`）、Qoder、QoderCN 与 Kimi Code CLI 八个档案，
SHALL NOT 开放用户自定义档案。启动配置的 argv 只有在以下写法之一中，且可执行文件的基本名与某个档案匹配时，
才 SHALL 视为可用于混合模式：

1. 直接执行：`[<可执行文件>, 参数…]`；
2. webmux 的启动文件包装：`["bash", "-ic", "exec -- \"$@\"", "webmux", <可执行文件>, 参数…]`
   或 `["zsh", "-ic", "exec -- \"$@\"", "webmux", <可执行文件>, 参数…]`。

其他任何写法（例如其他 shell 包装、`env` 前缀）SHALL 视为不可用，并给出原因“命令写法不受支持”。
未匹配任何档案时，服务端 SHALL 在 `hybridSupport.supportedAgents` 中返回当前注册表支持的
agent 显示名称列表。

#### Scenario: 预置的 bashrc 包装

- **WHEN** 配置的 argv 为 `["bash","-ic","exec -- \"$@\"","webmux","omp"]`
- **THEN** 判定为可用，档案为 omp

#### Scenario: Codex 预置配置

- **WHEN** 配置的 argv 为 `["bash","-ic","exec -- \"$@\"","webmux","codex","--dangerously-bypass-approvals-and-sandbox"]`
- **THEN** 判定为可用，档案为 codex

#### Scenario: agy 的 bashrc 包装

- **WHEN** 配置的 argv 为 `["bash","-ic","exec -- \"$@\"","webmux","agy","--dangerously-skip-permissions"]`
- **THEN** 判定为可用，档案为 `agy`

#### Scenario: Kimi 预置配置

- **WHEN** 配置的 argv 为 `["bash","-ic","exec -- \"$@\"","webmux","kimi","--auto"]`
- **THEN** 判定为可用，档案为 `kimi`

#### Scenario: 自定义包装

- **WHEN** 配置的 argv 为 `["sh","-c","omp"]`
- **THEN** 判定为不可用，原因为命令写法不受支持
