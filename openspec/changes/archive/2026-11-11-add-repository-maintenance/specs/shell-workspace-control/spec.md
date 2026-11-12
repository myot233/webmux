## ADDED Requirements

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
