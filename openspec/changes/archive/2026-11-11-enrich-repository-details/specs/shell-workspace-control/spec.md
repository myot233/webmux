## ADDED Requirements

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
