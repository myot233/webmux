## Why

`enrich-repository-details` 之后，仓库面板会显示“落后 N”“领先 N”“无上游”“上游已删除”，但看到这些状态后，用户仍要回到终端执行 `pull`、`push` 或 `branch -D`。删除工作树时分支被保留，分支会越积越多，WebMux 里却没有任何清理入口。

本变更补上三个不丢内容的写操作，让面板上的同步状态都有对应的处理方式；分叉、变基、合并冲突这类需要判断的情况仍交给终端或 agent。

## What Changes

- **快进到上游**：分支有上游、落后且不领先时，把本地分支快进到已获取的上游提交。
  - 分支已被某个工作树检出时，在该工作树中执行只允许快进的合并；已跟踪修改或进行中的 Git 操作会阻止快进，运行中的会话需要明确确认；
  - 分支未被检出时，直接更新分支引用，并校验它仍是旧值；
  - 只使用本地已有的远程跟踪引用，不访问网络；要拿最新上游，先用 `add-repository-maintenance` 的“获取远程”。
- **推送 / 发布分支**：
  - 有上游、领先且不落后时，把预览中确认的那个提交推送到上游分支；
  - 没有上游时“发布”：推送到所选远程的同名（可改名）分支，成功后把它设为上游；
  - 系统永远不使用强制推送、`--force-with-lease`、删除远端分支、`--mirror`、`--all` 或推送标签；远端拒绝非快进更新时，提示先获取再快进；
  - 预览列出将推送的提交和脱敏后的目标 URL，确认后才执行。
- **删除本地分支**（单个或批量）：
  - 被任何工作树检出的分支不能删除；
  - 提交都已包含在其他分支、远程跟踪分支、标签或 WebMux 保护引用中的分支，确认后即可删除；
  - 有独有提交的分支（常见于 squash merge 之后的“上游已删除”分支）单独列出提交数与标题，需要再单独勾选确认；
  - 结果中返回被删分支的提交 OID 和恢复命令 `git branch <名称> <OID>`，不另建备份引用；
  - 仓库面板“分支”分区提供“清理分支（N）”：候选是未被检出、且上游已删除或已合入默认分支的分支；预览只预选提交已全部包含在其他引用中的那些，有独有提交的默认不选。分支清单为此新增“已合入默认分支”标记。
- 入口：工作树“⋯”菜单（首页、侧栏、仓库面板工作树行）新增“快进到上游”“推送…”或“发布分支…”，按状态显示；工作树详情“分支与上游”一节提供同样的操作；“分支”分区的每行提供快进、推送/发布、删除。从仓库面板发起的流程作为面板内子视图展示。
- CLI 新增 `webmux repo ff`、`webmux repo push` 与 `webmux repo branch-rm`。
- 依赖：`enrich-repository-details`（同步字段、`operation`、仓库面板与子视图约定、工作树菜单），以及 `add-repository-maintenance` 的共用远程调用 `remoteGit`（非交互、错误分类、脱敏）。

## Capabilities

### New Capabilities

- `branch-operations`：已注册仓库中本地分支的写操作，包括快进到上游、推送与发布、删除与批量清理，以及各自的前提条件、预览确认、会话确认、远程认证边界和恢复信息。

### Modified Capabilities

- `shell-workspace-control`：新增 `webmux repo ff`、`webmux repo push` 与 `webmux repo branch-rm`。

## Impact

- 后端：
  - `internal/workspace` 新增快进、推送、删除分支三组“预览 + 执行”，确认用途 `purposeFastForward`、`purposePush`、`purposeBranchDelete`；复用 `targetSessions`、`confirm`/`takeConfirmation`、仓库门闩与 `remoteGit`；
  - `internal/api/workspace.go` 新增 `POST /api/repositories/{id}/branches/{fast-forward,push,delete}-preview` 与对应的执行路由；
  - `internal/contract`、`internal/wire` 与 `internal/api/testdata/wire` 快照随之更新。
- 前端：
  - 新增 `FastForwardBranch.svelte`、`PushBranch.svelte`、`DeleteBranches.svelte` 三个流程组件，由独立弹窗或仓库面板子视图承载；
  - `WorktreeMenuItems`、`WorktreeDetails`、`RepositoryBranches` 增加入口；`web/src/schemas.ts` 同步。
- CLI：`internal/clicmd/repo.go`。
- 用户故事：新增一个故事（按计划为 US-J18，在 WebMux 中快进、推送和清理分支），更新 US-J14（删除工作树后清理遗留分支的入口）。
- 不涉及：远端分支删除、强制推送、pull 的合并或变基、stash、标签管理、提交与暂存。
