# shell-session-launch Specification

## Purpose
让 shell 启动会话的能力与 Web 端一致：选择或新建 worktree、混合会话的起始端与首条消息、初始化指令、标签，以及等待、跟随和预演，并提供一步完成快速开始的 `chat new`。

## Requirements

### Requirement: run 选择工作区位置

`webmux run` SHALL 提供与 Web 端新建会话对话框中“位置”相同的选择：直接目录、已有 worktree、新建 worktree。

- `--worktree REPO:WORKTREE` 或 `--worktree PATH`（已登记 worktree 的路径）：
  创建请求 SHALL 带 `workspace` 引用，`cwd` 默认为该 worktree 路径；同时给了 `--cwd` 时，`--cwd` SHALL 位于该 worktree 内，否则以 2 结束。
- 自动关联：没有 `--worktree` 且解析后的 `cwd` 位于某个已登记 worktree 内时，CLI SHALL 自动带上该 worktree 的 `workspace` 引用，
  与 Web 端选择该 worktree 的结果一致（服务端本来也按启动目录关联会话，CLI 带上引用是为了让 `--init` 找到仓库的初始化指令）。
- `--new-worktree BRANCH [--base REF] [--existing-branch] [--worktree-path DIR] [--repo REPO]`：
  - SHALL 先创建 worktree，成功后再在其中创建会话；
  - 仓库默认取 `cwd` 所属的已登记仓库；
  - `--existing-branch` 对应模式 `existing`，否则为 `new`；
  - 不给 `--worktree-path` 时使用与快速开始相同的默认位置；
  - worktree 已创建而会话创建失败时，SHALL 保留 worktree，在标准错误输出其路径并以 1 结束。
  - 新 worktree SHALL 与 Web 端一样加入主页显示，`--no-pin` 时不加入；`--refresh-base` 时先刷新分支基点（规则同 `worktree create`）。
- `--worktree` 与 `--new-worktree` SHALL 互斥。`--new-worktree` 与 `--cwd` SHALL 互斥：会话从新 worktree 的根目录开始，仓库用 `--repo` 选择。

#### Scenario: 在当前 worktree 中自动关联

- **WHEN** 用户在已登记 worktree `/w/repo.worktrees/fix` 的子目录中执行 `webmux run --detach -- bash`
- **THEN** 会话带有该 worktree 的工作区关联，Web 端把它显示在该 worktree 下

#### Scenario: 新建 worktree 并启动

- **WHEN** 用户在仓库目录执行 `webmux run --new-worktree feat/x --base main --launch-config omp --detach`
- **THEN** 先创建分支 `feat/x` 的 worktree，再在其中以 `omp` 配置创建会话，标准输出为会话 ID

### Requirement: run 设置起始端与首条消息

- `--start-side terminal|chat` SHALL 映射为创建请求的 `startSide`。`chat` SHALL 要求所用启动配置开启了混合模式，否则在本地以 2 结束；聊天端没有终端可附着，因此 `chat` SHALL 隐含 `--detach`，与 `--attach` 一起使用时以 2 结束。
- `--prompt TEXT`（`-` 表示标准输入）SHALL 映射为 `prompt`：聊天端作为首条提示，终端端作为输入并回车。
- `--init` SHALL 映射为 `useInitInstruction`，要求会话带有工作区关联，否则在本地以 2 结束。
- 这些选项 SHALL 可以与 `--launch-config` 同用；与自定义命令同用时，`--start-side chat` 与 `--init` SHALL 以 2 结束。

#### Scenario: 以聊天端启动并下达任务

- **WHEN** 用户执行 `webmux run --worktree . --launch-config "Claude Code" --start-side chat --init --prompt "修复 lint" --detach`
- **THEN** 会话以聊天端启动，首条提示是展开后的初始化指令加“修复 lint”

### Requirement: run 的输出与等待

- `--json` SHALL 让 `--detach` 输出一个 JSON 对象：`session`（`SessionInfo`），代替单独一行会话 ID；不再包含 `link`。
- `--wait STATE`（取值同 `webmux wait --until`）SHALL 在创建后等待会话第一次进入该状态再退出；它隐含 `--detach`，与 `--attach`、`--follow` 互斥。
  超时（`--timeout`）以 124 退出。
- `--dry-run` SHALL 只解析参数、启动配置与工作区，并把将要发送的请求写到标准输出，SHALL NOT 创建 worktree 或会话。
  请求中的 `env` SHALL 只输出变量名，用 `"<inherited>"` 或 `"<explicit>"` 代替值。
- `--label KEY=VALUE`（可重复）SHALL 映射为会话标签（见 `session-metadata`）。

#### Scenario: 等 agent 就绪

- **WHEN** 脚本执行 `webmux run --launch-config omp --detach --wait idle --timeout 60s --json`
- **THEN** omp 启动并报告空闲后，CLI 输出含 `session` 的 JSON（不含 `link`）并以 0 退出

#### Scenario: 预览请求不泄露环境值

- **WHEN** 已导出 `ANTHROPIC_API_KEY` 的用户执行 `webmux run --dry-run -- claude`
- **THEN** 输出的请求中 `env.ANTHROPIC_API_KEY` 为 `"<inherited>"`，没有创建任何会话

### Requirement: 非交互地跟随运行

`webmux run --follow [其他 run 选项] -- <argv...>` SHALL 创建持久会话，并像普通命令一样在当前进程中跟随它，但不需要终端：

- 会话输出 SHALL 以原始字节流到标准输出；`--strip` SHALL 按 `logs --strip` 的规则去除控制序列。会话 ID SHALL 写到标准错误。
- CLI SHALL 以观察者身份连接，SHALL NOT 转发标准输入，SHALL NOT 夺取控制或改变尺寸。
  会话保持服务端默认尺寸（80×24），直到某个浏览器取得控制并调整尺寸；`run --follow` 不新增尺寸字段。
- 程序结束时，CLI SHALL 以程序的退出码退出。程序被信号终止时，SHALL 按 shell 惯例以 128 加信号编号退出。
- CLI 收到 SIGINT 或 SIGTERM 时，SHALL 只断开连接并以 130 或 143 退出，SHALL NOT 结束会话进程；标准错误 SHALL 提示会话继续运行，可以用 `webmux attach` 重新查看。
  `--kill-on-interrupt` 时，SHALL 先结束会话进程，再按程序的结果退出。
- 连接意外中断时 SHALL 以 1 退出，并提示会话可能仍在运行、可以用 `webmux attach` 查看；本地套接字上很少断线，本变更不做自动续接。
- `--follow` 与 `--detach`、`--attach`、`--start-side chat` SHALL 互斥。

#### Scenario: 在持久会话里跑测试并拿到退出码

- **WHEN** 脚本执行 `webmux run --follow --strip -- go test ./... > test.log; echo $?`
- **THEN** `test.log` 是测试输出，`$?` 等于 `go test` 的退出码，Web 端在测试期间可以实时查看这个会话

#### Scenario: 中断只分离

- **WHEN** 用户在 `run --follow -- make build` 运行中按 Ctrl+C
- **THEN** CLI 以 130 退出并提示会话继续运行，`make build` 在 WebMux 中继续执行

### Requirement: 一条命令完成快速开始

`webmux chat new TEXT|-` SHALL 对应 Web 端“新聊天”（快速开始），调用 `POST /api/quick-starts`，
一次完成新建 worktree、启动 agent、应用配置和发送首条消息。消息 SHALL 非空（与快速开始页一致），缺少时以 2 结束。

- `--repo REPO`：默认取当前目录所属的已登记仓库，找不到时以 2 结束并列出已登记仓库。
- `--agent NAME`：启动配置名称，SHALL 是开启混合模式的配置。只有一条混合配置时可以省略，否则省略时以 2 结束并列出候选。
- `--base REF`：起始提交，默认与快速开始页的默认值相同。
- `--branch NAME`：新分支名，默认与快速开始页一样自动生成。
- `--terminal`：映射为 `startSide: "tui"`，首条消息作为输入回车，且 SHALL 拒绝 `--set`、`--permission`、`--attach` 与 `--wait`（终端端没有可等待的聊天回合）。
- `--set ID=VALUE`（可重复）：映射为 `config`，`ID` 与可选值来自 `webmux launch-config options`。
- `--permission ask|auto`：映射为 `permissionMode`。
- `--init` 与 `--no-init`：映射为 `useInitInstruction`；仓库设置了初始化指令时默认为 `--init`，与快速开始页一致。
- `--attach PATH`（可重复）：先上传到 `POST /api/quick-starts/uploads`，再作为附件引用。
- `--label KEY=VALUE`（可重复）：会话创建后设置标签。

结果处理：

- 成功时，SHALL 输出会话 ID 与 worktree 路径（`--json` 时输出 `QuickStartResult`，不含 `link`）。
  `configWarnings` SHALL 写到标准错误。
- 失败时，SHALL 在标准错误按 `step` 报告失败步骤与原因，并输出已创建的 worktree 与会话，以 1 结束，SHALL NOT 回滚。
- `--wait` SHALL 等首条提示的回合结束，再按 `chat send --wait` 的规则输出最终回复与退出码。

#### Scenario: 编排者分派任务并取回结果

- **WHEN** 会话内的 agent 在仓库目录执行 `webmux chat new --agent Codex --label task=lint --wait "修复所有 lint 并汇报"`
- **THEN** 新 worktree 中启动 Codex，回合结束后标准输出为 Codex 的最终回复，新会话的 `parentId` 为调用者所在会话

#### Scenario: 分支已存在

- **WHEN** `--branch fix/x` 指定的分支已经存在
- **THEN** 标准错误报告失败步骤为 worktree、原因为冲突，退出码 1，没有创建会话

### Requirement: 查询 agent 配置项

`webmux launch-config options NAME [--repo REPO] [--json]` SHALL 调用 `POST /api/quick-starts/options`，
列出该启动配置的 agent 提供的配置项：ID、名称、类型、当前值，以及可选值。仓库默认取当前目录所属的已登记仓库。
配置未开启混合模式时 SHALL 以 1 结束。

#### Scenario: 选择模型

- **WHEN** 用户执行 `webmux launch-config options "Claude Code" --json | jq '.[] | select(.id=="model")'`
- **THEN** 输出模型配置项及其可选值，可以直接用于 `chat new --set model=<值>`
