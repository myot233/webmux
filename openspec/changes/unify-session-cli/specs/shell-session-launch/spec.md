## MODIFIED Requirements

### Requirement: run 选择工作区位置

`webmux run` SHALL 提供与 Web 端新建会话对话框中“位置”相同的选择：直接目录、已有 worktree、新建 worktree。

- `--worktree REPO:WORKTREE` 或 `--worktree PATH`（已登记 worktree 的路径）：
  创建请求 SHALL 带 `workspace` 引用，`cwd` 默认为该 worktree 路径；同时给了 `--cwd` 时，`--cwd` SHALL 位于该 worktree 内，否则以 2 结束。
- 自动关联：没有 `--worktree` 且解析后的 `cwd` 位于某个已登记 worktree 内时，CLI SHALL 自动带上该 worktree 的 `workspace` 引用，
  与 Web 端选择该 worktree 的结果一致（服务端本来也按启动目录关联会话，CLI 带上引用是为了让 `--init` 找到仓库的初始化指令）。
- `--new-worktree [--repo REPO] [--branch NAME] [--base REF] [--existing-branch] [--worktree-path DIR] [--refresh-base] [--no-pin]`：
  - `--new-worktree` SHALL 是不带取值的开关；CLI SHALL 只发送一次 `POST /api/sessions`，请求体带 `newWorktree`，由服务端先创建 worktree、再在其中创建会话；
  - 仓库默认取 `cwd` 所属的已登记仓库，找不到时以 2 结束并列出已登记仓库；
  - `--branch NAME` 给出分支名；省略时 SHALL 由服务端按快速开始的规则自动生成不冲突的分支名（形如 `wt/<日期>-<时间>`）；
  - `--existing-branch` 对应模式 `existing`，此时 SHALL 要求 `--branch`，否则以 2 结束；不带时模式为 `new`；
  - `--base REF` 为新分支的起始提交，默认与快速开始页的默认值相同；
  - 不给 `--worktree-path` 时使用与快速开始相同的默认位置；
  - 失败时 SHALL 在标准错误按失败体的 `step` 报告失败步骤与原因，并输出已创建的 worktree 路径与会话 ID，以 1 结束，SHALL NOT 回滚；
  - 新 worktree SHALL 与 Web 端一样加入主页显示，`--no-pin` 时不加入；`--refresh-base` 时先刷新分支基点（规则同 `worktree create`）。
- `--worktree` 与 `--new-worktree` SHALL 互斥。`--new-worktree` 与 `--cwd` SHALL 互斥：会话从新 worktree 的根目录开始，仓库用 `--repo` 选择。
  `--repo`、`--branch`、`--base`、`--existing-branch`、`--worktree-path`、`--refresh-base`、`--no-pin` SHALL 要求 `--new-worktree`，否则以 2 结束。
  `run` 在 `--` 之前出现位置参数时 SHALL 以 2 结束；与 `--new-worktree` 同用时提示用 `--branch` 指定分支名（旧写法 `--new-worktree BRANCH` 因此不会被当作命令执行）。

#### Scenario: 在当前 worktree 中自动关联

- **WHEN** 用户在已登记 worktree `/w/repo.worktrees/fix` 的子目录中执行 `webmux run --detach -- bash`
- **THEN** 会话带有该 worktree 的工作区关联，Web 端把它显示在该 worktree 下

#### Scenario: 新建 worktree 并启动

- **WHEN** 用户在仓库目录执行 `webmux run --new-worktree --branch feat/x --base main --launch-config omp --detach`
- **THEN** CLI 发送一次带 `newWorktree` 的创建请求，服务端先创建分支 `feat/x` 的 worktree，再在其中以 `omp` 配置创建会话，标准输出为会话 ID

#### Scenario: 自动生成分支名

- **WHEN** 用户在仓库目录执行 `webmux run --new-worktree --launch-config omp --detach`，不给 `--branch`
- **THEN** 服务端自动生成不冲突的分支名并创建 worktree，会话在其中启动

#### Scenario: 分支已存在

- **WHEN** `webmux run --new-worktree --branch fix/x --launch-config omp` 指定的分支已经存在
- **THEN** 标准错误报告失败步骤为 worktree、原因为冲突，退出码 1，没有创建会话

#### Scenario: 会话创建失败保留 worktree

- **WHEN** `run --new-worktree` 的 worktree 已创建，但会话创建失败
- **THEN** worktree 保留，标准错误报告失败步骤为 session 并给出 worktree 路径，退出码 1

### Requirement: run 设置起始端与首条消息

- `--view terminal|chat` SHALL 映射为创建请求的 `view`，缺省为 `terminal`。`chat` SHALL 要求所用启动配置开启了聊天视图，否则在本地以 2 结束；聊天视图没有终端可附着，因此 `chat` SHALL 隐含 `--detach`，与 `--attach` 一起使用时以 2 结束。
- `--prompt TEXT`（`-` 表示标准输入）SHALL 映射为 `prompt`：聊天视图作为首条提示，终端视图作为输入并回车。
- `--init` SHALL 映射为 `useInitInstruction`，要求会话带有工作区关联（含 `--new-worktree`），否则在本地以 2 结束。
- 以下选项 SHALL 只在 `--view chat` 时有效，否则在本地以 2 结束：
  - `--set ID=VALUE`（可重复）：映射为 `agentConfig`，`ID` 与可选值来自 `webmux launch-config options`；
  - `--permission ask|auto`：映射为 `permissionMode`；
  - `--attach-file PATH`（可重复）：先上传到 `POST /api/uploads`，再作为 `attachments` 随首条提示发送；SHALL 要求 `--prompt`，任一上传失败时 SHALL 不创建会话并以 1 结束；
  - `--reply`：见“run 的输出与等待”。
- 这些选项 SHALL 可以与 `--launch-config` 同用；与自定义命令同用时，`--view chat` 与 `--init` SHALL 以 2 结束。

#### Scenario: 以聊天端启动并下达任务

- **WHEN** 用户执行 `webmux run --worktree . --launch-config "Claude Code" --view chat --init --prompt "修复 lint" --detach`
- **THEN** 会话以聊天视图启动，首条提示是展开后的初始化指令加“修复 lint”

#### Scenario: 编排者分派任务并取回结果

- **WHEN** 会话内的 agent 在仓库目录执行 `webmux run --new-worktree --view chat --launch-config Codex --set model=gpt-5 --label task=lint --prompt "修复所有 lint 并汇报" --reply`
- **THEN** 新 worktree 中以聊天视图启动 Codex 并设置模型，回合结束后标准输出为 Codex 的最终回复，新会话的 `parentId` 为调用者所在会话

#### Scenario: 终端视图拒绝聊天专有选项

- **WHEN** 用户执行 `webmux run --launch-config omp --permission auto --detach`
- **THEN** 退出码 2，提示该选项需要 `--view chat`，不发送任何请求

### Requirement: run 的输出与等待

- `--json` SHALL 让 `--detach` 输出一个 JSON 对象：`session`（`SessionInfo`），本次新建了 worktree 时另含 `worktree`，并含 `configWarnings`；不再包含 `link`，代替单独一行会话 ID。
- 创建响应中的 `configWarnings` SHALL 写到标准错误。
- `--wait STATE`（取值同 `webmux wait --until`）SHALL 在创建后等待会话第一次进入该状态再退出；它隐含 `--detach`，与 `--attach`、`--follow` 互斥。
  超时（`--timeout`）以 124 退出。
- `--reply` SHALL 要求 `--view chat` 与 `--prompt`，与 `--wait`、`--follow` 互斥：创建后跟踪首条提示开启的回合，按 `webmux send --reply` 的规则输出最终回复并决定退出码。
- `--dry-run` SHALL 只解析参数、启动配置与工作区，并把将要发送的请求写到标准输出，SHALL NOT 创建 worktree 或会话，也 SHALL NOT 上传附件。
  请求中的 `env` SHALL 只输出变量名，用 `"<inherited>"` 或 `"<explicit>"` 代替值。
- `--label KEY=VALUE`（可重复）SHALL 映射为会话标签（见 `session-metadata`）。

#### Scenario: 等 agent 就绪

- **WHEN** 脚本执行 `webmux run --launch-config omp --detach --wait idle --timeout 60s --json`
- **THEN** omp 启动并报告空闲后，CLI 输出含 `session` 的 JSON（不含 `link`）并以 0 退出

#### Scenario: 预览请求不泄露环境值

- **WHEN** 已导出 `ANTHROPIC_API_KEY` 的用户执行 `webmux run --dry-run -- claude`
- **THEN** 输出的请求中 `env.ANTHROPIC_API_KEY` 为 `"<inherited>"`，没有创建任何会话

#### Scenario: --reply 与 --wait 互斥

- **WHEN** 用户执行 `webmux run --launch-config Codex --view chat --prompt hi --reply --wait idle`
- **THEN** 退出码 2，不创建会话

### Requirement: 非交互地跟随运行

`webmux run --follow [其他 run 选项] -- <argv...>` SHALL 创建持久会话，并像普通命令一样在当前进程中跟随它，但不需要终端：

- 会话输出 SHALL 以原始字节流到标准输出；`--strip` SHALL 按 `logs --strip` 的规则去除控制序列。会话 ID SHALL 写到标准错误。
- CLI SHALL 以观察者身份连接，SHALL NOT 转发标准输入，SHALL NOT 夺取控制或改变尺寸。
  会话保持服务端默认尺寸（80×24），直到某个浏览器取得控制并调整尺寸；`run --follow` 不新增尺寸字段。
- 程序结束时，CLI SHALL 以程序的退出码退出。程序被信号终止时，SHALL 按 shell 惯例以 128 加信号编号退出。
- CLI 收到 SIGINT 或 SIGTERM 时，SHALL 只断开连接并以 130 或 143 退出，SHALL NOT 结束会话进程；标准错误 SHALL 提示会话继续运行，可以用 `webmux attach` 重新查看。
  `--kill-on-interrupt` 时，SHALL 先结束会话进程，再按程序的结果退出。
- 连接意外中断时 SHALL 以 1 退出，并提示会话可能仍在运行、可以用 `webmux attach` 查看；本地套接字上很少断线，本变更不做自动续接。
- `--follow` 与 `--detach`、`--attach`、`--view chat`、`--reply` SHALL 互斥。

#### Scenario: 在持久会话里跑测试并拿到退出码

- **WHEN** 脚本执行 `webmux run --follow --strip -- go test ./... > test.log; echo $?`
- **THEN** `test.log` 是测试输出，`$?` 等于 `go test` 的退出码，Web 端在测试期间可以实时查看这个会话

#### Scenario: 中断只分离

- **WHEN** 用户在 `run --follow -- make build` 运行中按 Ctrl+C
- **THEN** CLI 以 130 退出并提示会话继续运行，`make build` 在 WebMux 中继续执行

### Requirement: 查询 agent 配置项

`webmux launch-config options NAME [--repo REPO] [--json]` SHALL 调用 `POST /api/launch-configs/{id}/options`，请求体为 `{repositoryId}`，
列出该启动配置的 agent 提供的配置项：ID、名称、类型、当前值，以及可选值。仓库默认取当前目录所属的已登记仓库。
配置未开启聊天视图时（服务端返回 409 `no_chat_view`）SHALL 以 1 结束。

#### Scenario: 选择模型

- **WHEN** 用户执行 `webmux launch-config options "Claude Code" --json | jq '.[] | select(.id=="model")'`
- **THEN** 输出模型配置项及其可选值，可以直接用于 `run --view chat --set model=<值>`

#### Scenario: 终端启动配置

- **WHEN** 用户对未开启聊天视图的启动配置执行 `webmux launch-config options Shell`
- **THEN** 退出码 1，标准错误说明该配置没有聊天视图

## REMOVED Requirements

### Requirement: 一条命令完成快速开始

**Reason**: 快速开始并入 `POST /api/sessions`，`webmux run` 吸收了 `chat new` 的全部选项，不再保留第二个创建命令。

**Migration**: `webmux chat new --agent NAME [--branch B] [--base REF] TEXT [--wait]` 改为 `webmux run --new-worktree --view chat --launch-config NAME [--branch B] [--base REF] --prompt TEXT [--reply]`；
`--terminal` 改为 `--view terminal`（或省略 `--view`）；`--attach PATH` 改为 `--attach-file PATH`；`--set`、`--permission`、`--label`、`--repo` 同名保留；`--init` 需要显式给出。
