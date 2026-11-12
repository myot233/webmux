## Why

`enrich-repository-details` 之后，仓库面板能显示领先/落后和上次获取时间，但这些数据只在用户去终端执行 `git fetch` 后才会更新。现在唯一的网络读取是新建工作树时对单个起始分支上游的 fetch。

外部删除目录后留下的失效工作树登记，目前只能在批量清理里逐个筛选、勾选，没有一个直接的“清理失效登记”操作。

## What Changes

- 仓库面板标题栏新增“获取远程”，对仓库的每个远程执行带 `--prune` 的 fetch：
  - 结果按远程逐个列出（成功，或认证失败/网络不可达/超时/远端拒绝），错误信息中的凭据已脱敏；
  - 完成后刷新仓库快照，更新上次获取时间和领先/落后；
  - 获取期间面板的其他操作不受阻塞；对同一仓库重复点击时合并为同一次获取。
  - 系统 SHALL NOT 弹出交互式认证，也不修改凭据配置。
- “工作树”分区在存在可安全清理的失效登记（未锁定，且目录已确认不存在的 `prunable` linked worktree）时显示“清理失效登记（N）”：
  - 点击后进入批量清理并预选这些条目，直接进入现有的批量预览；
  - 复用现有的归档记录保留、逐项结果和部分失败重试；
  - 不使用无差别的 `git worktree prune`，被锁定或目录仍存在的条目不受影响。
- CLI 新增 `webmux repo fetch REPO` 和 `webmux worktree prune REPO [--yes]`。
- 依赖 `enrich-repository-details` 提供的仓库面板、`lastFetchedAt` 和 URL 脱敏函数。
- 访问远程的非交互执行、错误分类与脱敏实现为共用的 `remoteGit`，供 `add-branch-sync-actions` 的推送复用。

## Capabilities

### New Capabilities

- `repository-maintenance`：对已注册仓库执行的维护操作：获取远程和清理失效工作树登记，包括触发入口、逐项结果、凭据与认证边界以及并发合并。

### Modified Capabilities

- `shell-workspace-control`：新增 `webmux repo fetch` 与 `webmux worktree prune`。

## Impact

- 后端：
  - `internal/workspace` 新增共用的 `remoteGit`（非交互环境、错误分类、脱敏）和 `FetchRemotes`，按仓库合并并发请求，执行时不持有仓库门闩；
  - `internal/api/workspace.go` 新增 `POST /api/repositories/{id}/fetch`；
  - 契约、wire 与快照随之更新。
  - 清理失效登记不新增服务端接口，直接复用 `batch-removal-preview` / `batch-remove`。
- 前端：
  - `RepositoryPanel` 标题栏增加“获取远程”按钮，并展示逐个远程的结果；
  - `RepositoryWorktrees` 增加“清理失效登记（N）”快捷入口。
- CLI：修改 `internal/clicmd/repo.go`、`worktree.go`。
- 用户故事：新增一个故事（按计划为 US-J17，获取远程并清理失效登记），更新 US-J14。
- 不涉及：定时自动获取。快进、推送与删除本地分支由 `add-branch-sync-actions` 负责。
