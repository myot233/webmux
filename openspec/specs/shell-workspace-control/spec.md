# shell-workspace-control Specification

## Purpose

让脚本和 agent 管理与 Web 端相同的工作区：worktree 的创建、删除与主页显示，仓库登记与仓库设置，启动配置的增删改，以及端口转发。

## Requirements

### Requirement: worktree 命令

系统 SHALL 提供 `webmux worktree` 子命令，并遵守 `shell-session-control` 的输出与退出码约定。仓库参数 `REPO` 可以是仓库 ID、已登记的仓库路径，也可以是 `.`（当前目录所属的已登记仓库）。

- `webmux worktree ls [REPO] [--home|--hidden] [--json]`：不给仓库时列出所有已登记仓库及其 worktree；给了仓库时只列出该仓库。
  - 表格列为仓库、worktree ID、类型（main、linked、bare）、分支或 detached 提交、路径、状态（不可用、锁定、可清理、有改动）、会话数与“主页”。
  - “主页”列的值与 Web 端的规则一致：`pinned`（加入了主页显示）、`sessions`（未加入，但因为有会话或归档记录而显示）、`hidden`。
  - `--home` 只列出在主页显示的 worktree，`--hidden` 只列出不显示的。
  - `--json` 输出 `Repository` 数组，每个 worktree 附加 `home` 字段，取值同上。
- `webmux worktree show REPO WORKTREE [--json]` SHALL 输出单个 worktree 的全部字段、主页状态，以及其中的会话与归档记录。
- `webmux worktree create REPO (--branch NAME [--base REF] | --existing BRANCH) [--path DIR] [--json]`：
  调用创建接口，`--branch` 对应模式 `new`，`--existing` 对应模式 `existing`。
  不给 `--path` 时，SHALL 使用与快速开始相同的默认位置：仓库目录旁的 `<仓库名>.worktrees/<分支叶名>`。
  成功后 SHALL 输出 worktree 路径，`--json` 时输出 `Worktree`。
  与 Web 端一样，新 worktree SHALL 加入主页显示（见“worktree 的主页显示”）；`--no-pin` 时不加入。加入失败只在标准错误警告，不影响退出码。
  `--refresh-base` 时，SHALL 先调用分支基点刷新接口，用取回的上游提交作为 `--base`（见“仓库信息与分支”）。
- `webmux worktree rm REPO WORKTREE [--force] [--yes]`：
  - SHALL 先调用移除预览，把受影响的会话、归档记录与阻止原因写到标准错误；
  - 有阻止原因且没有 `--force` 时，SHALL 以 1 结束；
  - 没有 `--yes` 且标准输入是终端时，SHALL 要求输入 worktree 路径确认；标准输入不是终端又没有 `--yes` 时，SHALL 以 2 结束；
  - 确认后以预览得到的确认 ID 调用移除接口：`--force` 时模式为 `force`，`confirmPath` 为 worktree 路径，`closeSessions` 为 true；
    否则模式为 `safe`。
- 仓库没有登记或参数有歧义时，SHALL 以 2 结束并列出候选。

#### Scenario: 为并行任务准备 worktree

- **WHEN** 编排脚本在仓库目录执行 `webmux worktree create . --branch fix/lint --base main`
- **THEN** 标准输出是新 worktree 的路径，所有浏览器的工作区随即显示它

#### Scenario: 非交互删除需要显式确认

- **WHEN** 脚本以管道运行 `webmux worktree rm . <ID>`，没有给 `--yes`
- **THEN** 不删除任何东西，退出码 2，提示需要 `--yes`

### Requirement: worktree 的主页显示

Web 端主页按服务端设置 `homeWorktrees` 决定显示哪些 worktree（“主页显示”）。CLI SHALL 能读取并修改它，规则与 Web 端完全一致：

- 某个仓库从未设置过时，只把主工作区视为已加入；
- 有会话或归档记录的 worktree 总会显示，与是否加入无关；
- 列表中已不存在的 worktree ID SHALL 在写入时按 Web 端的规则清理。

命令：

- `webmux worktree pin REPO WORKTREE...` 加入主页显示；`webmux worktree unpin REPO WORKTREE...` 取消。
  取消一个仍有会话的 worktree 时，SHALL 在标准错误说明它仍会因为会话而显示。
- `webmux worktree home REPO [--json]` 输出该仓库加入主页显示的 worktree，并标出是否为默认值（从未设置）。
- `webmux worktree home REPO --set WORKTREE...` 整体替换该仓库的列表；`--reset` 删除该仓库的设置，恢复为只显示主工作区。

写入方式：

- SHALL 读取 `GET /api/settings` 的修订号，只修改 `homeWorktrees` 中这个仓库的条目，再以 `baseRevision` 调用 `PATCH /api/settings`。
- 返回 409 时，SHALL 重新读取并只重放这个仓库的修改，最多重试 3 次，仍冲突时以 1 结束。SHALL NOT 覆盖其他仓库的条目或其他设置项。
- 修改 SHALL 经 `/ws/settings` 同步到所有已打开的浏览器。
- 同样的写入逻辑 SHALL 用于 `worktree create`、`run --new-worktree` 新建后的自动加入。

#### Scenario: 让 agent 新建的 worktree 出现在主页

- **WHEN** 编排者执行 `webmux worktree create . --branch exp/a`，之后又执行 `webmux worktree unpin . <ID>`
- **THEN** 创建后所有浏览器的主页立即显示该 worktree，取消后不再显示，其他仓库的主页设置不变

#### Scenario: 并发修改不丢失

- **WHEN** 浏览器正在修改另一个仓库的主页显示，同时 CLI 执行 `worktree pin`，第一次 PATCH 得到 409
- **THEN** CLI 重新读取后只重放自己的修改，两个修改都保留

#### Scenario: 恢复默认

- **WHEN** 用户执行 `webmux worktree home . --reset`
- **THEN** 该仓库只显示主工作区，以及有会话或归档记录的 worktree

### Requirement: 仓库命令

- `webmux repo ls [--json]` SHALL 列出已登记仓库：ID、名称、主工作区路径、可用性、worktree 数量、主页显示数量、是否设置了初始化指令。
- `webmux repo add [PATH]` SHALL 调用 `POST /api/repositories` 登记仓库，路径默认为当前目录。已登记时 SHALL 输出已有条目，并以 0 退出。
  路径不是 Git 仓库时 SHALL 输出服务端原因，并以 1 结束。
- `webmux repo rm REPO [--yes]` SHALL 取消登记，SHALL NOT 删除磁盘上的任何文件；确认规则与 `worktree rm` 相同。取消登记后，SHALL 一并清理该仓库的主页显示设置。
- 仓库参数 `REPO` 的解析规则与 `worktree` 相同。

#### Scenario: 重复登记

- **WHEN** 脚本在已登记的仓库目录中再次执行 `webmux repo add`
- **THEN** 输出已有条目并以 0 退出，仓库列表中不出现重复项

#### Scenario: 取消登记不删除文件

- **WHEN** 用户执行 `webmux repo rm . --yes`
- **THEN** 仓库从所有浏览器的工作区中消失，`homeWorktrees` 中该仓库的条目被清理，磁盘上的 worktree 与文件保持不变

### Requirement: 仓库设置

`webmux repo settings REPO` SHALL 对应 Web 端的“仓库设置”对话框，读取或修改仓库设置。目前设置项只有初始化指令，以后新增的仓库设置 SHALL 加到这个命令下。

- 不带修改选项时，SHALL 输出全部设置：初始化指令全文、长度与上限，以及其中引用的快捷指令和内置变量；`--json` 时输出 `{initInstruction}`。
- `--init TEXT`、`--init -`（标准输入）或 `--init @FILE` 设置初始化指令，`--clear-init` 清空，对应 `PATCH /api/repositories/{id}`。
  去除首尾空白后为空，等同于清空。超过上限时 SHALL 在本地以 2 拒绝。
- 指令中可以写 `{{webmux}}`、`{{dir}}`、`{{user}}` 与快捷指令引用。引用了不存在的快捷指令时，SHALL 在标准错误给出与设置页相同的提示，但仍然保存。
- `--edit` SHALL 用 `$VISUAL` 或 `$EDITOR` 打开当前指令，保存后写回；没有终端时 SHALL 以 2 结束。

#### Scenario: 为新仓库准备快速开始

- **WHEN** 脚本在新克隆的仓库目录中执行 `webmux repo add && printf '{{webmux}}\n\n{{user}}' | webmux repo settings . --init -`
- **THEN** 仓库出现在所有浏览器的工作区中，之后的 `webmux chat new` 默认带上包含 WebMux 工具说明的初始化指令

#### Scenario: 查看设置

- **WHEN** 用户执行 `webmux repo settings .`
- **THEN** 输出初始化指令全文，并列出其中引用的 `{{webmux}}` 与快捷指令名称

### Requirement: 仓库信息与分支

- `webmux repo show REPO [--json]` SHALL 输出仓库的全部信息：ID、名称、Git 公共目录、主工作区路径、可用性与错误、检查时间、
  初始化指令摘要，以及各 worktree 与主页状态。
- `webmux repo refresh REPO` SHALL 调用 `GET /api/repositories/{id}/worktrees`，让服务端重新检查仓库并返回最新的 worktree 列表，与 Web 端刷新按钮的效果相同。
- `webmux repo branches REPO [--json]` SHALL 列出本地分支、所在提交，以及已检出该分支的 worktree 路径。
- `webmux repo commits REPO [--from REF]... [--json]` SHALL 列出最近的提交，供选择 `--base`。
- `webmux repo base REPO BRANCH [--json]` SHALL 调用分支基点刷新接口：取回上游并报告基点、来源，以及取回失败时的警告；
  警告写到标准错误，不影响退出码。

#### Scenario: 从最新的远端主干开分支

- **WHEN** 脚本执行 `webmux worktree create . --branch fix/x --base main --refresh-base`
- **THEN** CLI 先取回 `main` 的上游，再以取回的提交为基点创建 worktree；取回失败时警告并使用本地 `main`

### Requirement: 启动配置命令

- `webmux launch-config ls [--json]` SHALL 列出启动配置摘要：名称、图标、命令、是否混合模式，以及环境变量名（不含值）。
- `webmux launch-config show NAME [--json] [--reveal-env]` SHALL 按区分大小写的精确名称输出一条配置。
  默认 SHALL NOT 输出环境变量的值，`--reveal-env` 时才输出；`--json` 的结构与 `GET /api/launch-configs/{id}` 相同，值按 `--reveal-env` 决定是否替换为 `"<hidden>"`。
- `webmux launch-config add NAME [--icon ICON] [--env NAME=VALUE]... [--hybrid] [--preview-lines N] [--preview-skip N] -- <argv...>`
  SHALL 调用 `POST /api/launch-configs`。`argv` 原样作为命令，SHALL NOT 经过 shell 展开。
  `--hybrid` 时 SHALL 先调用混合模式支持检查，不支持时输出原因并以 1 结束。
- `webmux launch-config edit NAME [--rename NEW] [--icon ICON] [--env NAME=VALUE]... [--unset-env NAME]... [--hybrid|--no-hybrid] [--preview-lines N] [--preview-skip N] [-- <argv...>]`
  SHALL 读取现有配置，只修改给出的字段，再用 `PUT /api/launch-configs/{id}` 整体保存。未给出的字段 SHALL 保持原值，包括环境变量的值。
- `webmux launch-config rm NAME [--yes]` SHALL 删除配置，确认规则与 `worktree rm` 相同。
- `webmux launch-config presets [--json]` SHALL 列出内置预设；`webmux launch-config restore [PRESET]` SHALL 恢复全部或指定的预设。
- 名称不存在时 SHALL 以 2 结束并列出已保存的名称；名称冲突或校验失败时 SHALL 输出服务端原因并以 1 结束。
- 环境变量值 SHALL 只在请求体中传输，SHALL NOT 出现在命令的标准错误或诊断输出中。

#### Scenario: 用脚本添加 agent 配置

- **WHEN** 用户执行 `webmux launch-config add "Claude (sandbox)" --icon claude --hybrid --env CLAUDE_CONFIG_DIR=/w/.claude -- claude --dangerously-skip-permissions`
- **THEN** 配置出现在 Web 端新建会话的命令选择器中，且已开启混合模式

#### Scenario: 只改一个字段

- **WHEN** 用户执行 `webmux launch-config edit "Claude (sandbox)" --unset-env CLAUDE_CONFIG_DIR`
- **THEN** 只删除这个变量，命令、图标、混合模式与其他环境变量的值保持不变

#### Scenario: 默认隐藏变量值

- **WHEN** 用户执行 `webmux launch-config show "Claude (sandbox)" --json`
- **THEN** `env` 中每个值都是 `"<hidden>"`

### Requirement: 端口转发命令

- `webmux forward ls [--json]` SHALL 列出转发：端口、标签、状态与来源会话。
- `webmux forward add PORT [--label TEXT] [--session SESSION] [--no-compat]` SHALL 调用新增接口。
  `--session` 接受 `self`，`--no-compat` 关闭全部开发服务器兼容选项。成功后 SHALL 输出转发端口。
- `webmux forward rm PORT` SHALL 移除转发。
- 端口非法、指向 WebMux 自己的端口或已经转发时，SHALL 输出服务端原因并以 1 结束。

#### Scenario: agent 启动开发服务器后开转发

- **WHEN** 会话内的 agent 启动 Vite 后执行 `webmux forward add 5173 --session self --label web`
- **THEN** 所有浏览器的转发列表出现 5173，CLI 输出端口 5173

### Requirement: CLI 输出上游同步状态

CLI SHALL 输出与 Web 端仓库面板相同的 Git 同步详情，规则与 `repository-details` 一致：

- `webmux worktree ls` 的状态列 SHALL 追加领先/落后（如 `+2/-1`）或 `gone`，无上游时不追加；
- `webmux worktree show` 与 `webmux repo show` SHALL 输出上游、领先/落后、最近提交与修改统计，`repo show` 另输出上次获取远程时间；
- `webmux repo branches` SHALL 增加上游与领先/落后列；
- 所有 `--json` 输出 SHALL 与对应 HTTP 响应字段一致。

#### Scenario: 脚本检查是否需要推送

- **WHEN** 脚本执行 `webmux worktree show . <ID> --json`
- **THEN** 输出中包含上游、领先与落后数，脚本可据此判断是否有未推送提交，无需另行调用 Git

#### Scenario: 无上游的分支

- **WHEN** `webmux repo branches .` 列出一个没有配置上游的本地分支
- **THEN** 该行上游列为空、领先/落后列为 `-`，不显示 `0/0`

### Requirement: 远程清单命令

`webmux repo remotes REPO [--json]` SHALL 调用 `GET /api/repositories/{id}/remotes`，列出远程名称、fetch URL 与 push URL。URL 与 Web 端一样经过凭据脱敏。仓库参数解析规则与 `worktree` 相同。

#### Scenario: 列出远程

- **WHEN** 用户执行 `webmux repo remotes .`
- **THEN** 标准输出逐行列出远程名称与 URL，含令牌的 URL 显示为 `***`，退出码 0

### Requirement: 仓库维护命令

- `webmux repo fetch REPO [--json]` SHALL 调用 `POST /api/repositories/{id}/fetch`，逐行输出每个远程的结果（与 Web 端相同的分类与脱敏）。全部成功时以 0 结束，任一远程失败时以 1 结束，仓库没有远程时以 1 结束并说明原因。
- `webmux worktree prune REPO [--yes] [--json]` SHALL 选出该仓库全部可安全清理的失效登记（规则同 `repository-maintenance`），调用批量删除预览并把预览写到标准错误：
  - 没有可清理项时以 0 结束并说明；
  - 确认规则与 `worktree rm` 相同；
  - 确认后调用批量删除，逐项输出 `removed`、`skipped`、`failed`，有 `failed` 时以 1 结束。
- 仓库参数解析规则与 `worktree` 相同。

#### Scenario: 脚本获取后检查落后

- **WHEN** 脚本先执行 `webmux repo fetch .`，再执行 `webmux worktree show . <ID> --json`
- **THEN** 第一条命令以 0 结束，第二条输出的落后数反映远端最新提交

#### Scenario: 非交互清理失效登记

- **WHEN** 脚本以管道运行 `webmux worktree prune .`，没有给 `--yes`，且存在 2 条失效登记
- **THEN** 不删除任何登记，标准错误列出这 2 条并提示需要 `--yes`，退出码 2

### Requirement: worktree switch 命令

`webmux worktree switch REPO WORKTREE (BRANCH | --track REMOTE/BRANCH | --create NAME [--from REF] | --detach REF) [--yes] [--json]` SHALL 先调用切换预览，再以确认凭证执行切换，规则与 Web 端 `worktree-branch-switch` 一致。

- 预览的阻止原因、警告与受影响会话 SHALL 写到标准错误；
- 有阻止原因时 SHALL 以 1 结束；
- 有运行会话且没有 `--yes` 时：标准输入是终端则要求输入 `yes` 确认，否则以 2 结束；
- 成功后标准输出 SHALL 为新分支名或 detached 提交，`--json` 时输出 `Worktree`。

#### Scenario: 脚本切换空闲工作树

- **WHEN** 脚本执行 `webmux worktree switch . <ID> feat/a`，工作树干净且没有运行会话
- **THEN** 工作树切换到 `feat/a`，标准输出为 `feat/a`，退出码 0

#### Scenario: 非交互且有运行会话

- **WHEN** 脚本以管道运行切换命令，工作树下有运行会话，且没有给 `--yes`
- **THEN** 不切换，标准错误列出会话并提示需要 `--yes`，退出码 2

#### Scenario: 有未提交修改

- **WHEN** 目标工作树有未暂存修改
- **THEN** 不切换，标准错误说明原因，退出码 1

### Requirement: repo ff 命令

`webmux repo ff REPO BRANCH [--yes] [--json]` SHALL 按 `branch-operations` 的规则快进分支，仓库参数解析规则与 `worktree` 相同：

- SHALL 先调用快进预览，把将前进的提交数、受影响的工作树与会话、阻止原因写到标准错误；
- 有阻止原因时 SHALL 以 1 结束；
- 有运行会话且没有 `--yes` 时：标准输入是终端则要求输入 `yes` 确认，否则以 2 结束；没有会话时直接执行；
- 成功时 `--json` 输出对应 HTTP 响应。

#### Scenario: 非交互快进有会话的工作树

- **WHEN** 脚本以管道运行 `webmux repo ff . feat/a`，检出 `feat/a` 的工作树下有运行会话，且没有给 `--yes`
- **THEN** 不快进，标准错误列出会话并提示需要 `--yes`，退出码 2

### Requirement: repo push 命令

`webmux repo push REPO BRANCH [--remote REMOTE] [--to NAME] [--yes] [--json]` SHALL 按 `branch-operations` 的规则推送或发布分支：

- SHALL 先调用推送预览，把目标远程、目标分支、脱敏后的 URL 与待推送提交写到标准错误；
- 没有 `--yes` 时：标准输入是终端则要求输入 `yes` 确认，否则以 2 结束；
- 远端拒绝、认证失败、网络错误或超时 SHALL 以 1 结束，并输出与 Web 端相同分类的脱敏信息；
- 成功时 `--json` 输出对应 HTTP 响应。

`--remote` 与 `--to` 只用于没有上游的“发布”：

- 对有上游的分支给出时，SHALL 以 2 结束并说明原因；
- 发布时没有 `--remote`，依次使用 `remote.pushDefault`、`origin`、唯一的远程；仍无法确定时 SHALL 以 2 结束并列出远程。

#### Scenario: 脚本发布 agent 的分支

- **WHEN** 脚本执行 `webmux repo push . feat/c --yes`，`feat/c` 没有上游，仓库只有 `origin`
- **THEN** 分支被推送到 `origin/feat/c` 并设为上游，退出码 0

#### Scenario: 非交互推送没有确认

- **WHEN** 脚本以管道运行 `webmux repo push . feat/a`，没有给 `--yes`
- **THEN** 不推送，标准错误列出待推送的提交并提示需要 `--yes`，退出码 2

### Requirement: repo branch-rm 命令

`webmux repo branch-rm REPO (BRANCH... | --cleanup) [--unmerged] [--yes] [--json]` SHALL 按 `branch-operations` 的规则删除本地分支：

- SHALL 先调用删除预览，按“已包含”“有独有提交”两组写到标准错误；
- `--cleanup` 选中全部清理候选，规则同 Web 端“清理分支”；
- 有独有提交的分支只有给出 `--unmerged` 时才删除，否则跳过并说明；
- 确认规则与 `worktree rm` 相同；
- 执行后逐项输出 `deleted`、`skipped`、`failed`，`deleted` 行附带提交 OID 与恢复命令；有 `failed` 时以 1 结束；
- 成功时 `--json` 输出对应 HTTP 响应。

#### Scenario: 清理时跳过有独有提交的分支

- **WHEN** 用户执行 `webmux repo branch-rm . --cleanup --yes`，候选中有 1 个分支有独有提交
- **THEN** 其余候选被删除并输出各自的 OID 与恢复命令，该分支输出 `skipped` 并提示需要 `--unmerged`，退出码 0
