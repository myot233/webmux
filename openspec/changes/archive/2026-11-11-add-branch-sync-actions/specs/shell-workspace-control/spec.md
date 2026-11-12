## ADDED Requirements

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
