## Why

目前 WebMux 只能在新建工作树时选择分支。已有工作树（包括主工作区）要换到另一个分支，只能回到终端执行 `git switch`，期间看不到修改、运行会话、分支占用这些风险。

另一种做法是“在新目录打开这个分支”，但它在仓库面板的分支列表里也没有直接入口，每次都要从“新建工作树…”重新选择分支。

## What Changes

- 新增已有工作树的原地分支切换。服务端采用两阶段流程：先预览得到确认凭证，再执行。支持四种目标：
  - 未被其他工作树检出的本地分支；
  - 远程跟踪分支：新建同名本地分支，并以它为上游；
  - 从指定起点新建分支；
  - 分离到指定提交。
- 切换的保护规则：
  - 存在已暂存、未暂存或冲突的已跟踪修改时拒绝切换，不自动 stash，也不丢弃修改；
  - 存在进行中的 merge、rebase、cherry-pick、revert 或 bisect 时拒绝切换；
  - 目标分支已被其他工作树占用时拒绝，并提供“转到该工作树”；
  - 系统永远不使用 `--force`、`--discard-changes` 或 `--merge`。
- 工作树下有运行中会话时，预览列出这些会话。用户必须明确确认“会话继续运行，并将看到切换后的文件”才能执行；系统不关闭这些会话。确认后会话集合发生变化时，要求重新预览。
- 新增“在新工作树打开”：
  - 仓库面板“分支”分区中，未被占用的本地分支和远程分支提供该行操作，打开预填好的新建工作树弹窗；
  - 切换弹窗因修改或会话受阻时，提供同名的更安全替代按钮。
- 工作树“⋯”菜单（首页、侧栏、仓库面板的工作树行共用）新增“切换分支…”。从仓库面板发起时，切换流程作为面板内子视图展示，不叠弹窗。
- CLI 新增 `webmux worktree switch`，语义与 Web 端一致。
- 依赖 `enrich-repository-details` 提供的修改统计、进行中操作（`operation`）、仓库面板与面板内子视图约定，以及工作树菜单。

## Capabilities

### New Capabilities

- `worktree-branch-switch`：已有工作树原地切换分支的目标类型、阻止条件、会话确认、竞态处理，以及“在新工作树打开”分支的入口。

### Modified Capabilities

- `shell-workspace-control`：新增 `webmux worktree switch` 子命令。

## Impact

- 后端：
  - `internal/workspace` 新增 `PreviewSwitch` / `SwitchWorktree`，以及确认用途 `purposeSwitch`；
  - 复用 `targetSessions`、`confirm`/`takeConfirmation` 和仓库门闩；
  - 进行中操作的阻止直接复用 `enrich-repository-details` 的 `operation`，不另做检测；
  - `internal/api/workspace.go` 新增 `POST .../worktrees/{wid}/switch-preview` 与 `POST .../worktrees/{wid}/switch`；
  - 契约、wire 与快照随之更新。
- 前端：
  - 新增切换流程组件 `SwitchBranch.svelte`，由独立弹窗 `SwitchBranchDialog.svelte` 或仓库面板子视图承载；
  - `WorktreeMenuItems` 增加入口；
  - `RepositoryBranches` 增加行操作；
  - `NewSessionDialog` / `WorktreeCreateFields` 支持预填分支与起点；
  - 切换成功后刷新仓库状态。
- CLI：`internal/clicmd/worktree.go`。
- 用户故事：新增一个故事（按计划为 US-J16，切换已有工作树的分支），更新 US-J05。
- 不涉及：stash 管理、合并/变基、子模块更新、为新建工作树设置上游。
