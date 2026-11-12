## ADDED Requirements

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
