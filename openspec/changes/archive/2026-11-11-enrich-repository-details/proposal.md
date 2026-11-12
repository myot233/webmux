## Why

现在“仓库详情…”只显示路径、主页计数、会话数和 Git 读取时间。工作树详情只有分支、8 位提交、是否干净和占用空间。用户看不到某个工作树相对上游领先或落后多少、最近一次提交、具体改了多少文件，也看不到仓库里有哪些分支、分支被哪个工作树占用、有哪些远程。

同一类信息分散在三处：首页分组的“详情”、“仓库详情”和“管理工作树”的列表行。三处展示的字段和措辞都不一样。后续的分支切换（`switch-worktree-branch`）、仓库维护（`add-repository-maintenance`）和分支同步操作（`add-branch-sync-actions`）都依赖一个信息完整、只有一份的仓库面板，所以要先做这一步。

## What Changes

- 服务端为每个工作树补充 Git 状态，全部在本机计算，不访问网络：
  - 上游分支，以及领先/落后上游的提交数；
  - 上游是否已删除（gone）；
  - 最近提交：完整哈希、标题、作者、提交时间；
  - 修改统计：已暂存、未暂存、未跟踪、冲突的条目数；
  - 进行中的 merge、rebase、cherry-pick、revert 或 bisect，读取文件状态得出，不增加 Git 进程。
  现有的 `dirty`、`hasUntracked`、`hasIgnored` 字段的语义保持不变。领先/落后被跳过计算时显示“未计算”，与“无上游”区分。
- 仓库快照新增“上次获取远程时间”（近似值），供界面标注 ahead/behind 数据的新鲜度。
- 分支接口为每个本地分支补充上游、领先/落后、上游已删除标记、最近提交标题与时间，以及占用它的工作树。远程跟踪分支单独列出。
- 新增只读的远程清单接口，返回远程名称和 fetch/push URL。URL 中的密码或令牌一律脱敏。
- “仓库详情”改为带下划线 Tabs 的仓库面板，共四个分区：
  - 概览：路径、Git 公共目录、默认分支、远程数、上次获取时间、会话统计、读取时间，以及移除仓库的入口；
  - 工作树：全部工作树，每行带上游同步状态、修改统计和最近提交，点击打开工作树详情；
  - 分支：本地分支和远程分支，可搜索；
  - 远程：远程清单。
- **BREAKING（界面结构）**：“管理工作树”弹窗并入仓库面板的“工作树”分区，批量清理成为该分区的选择模式，原有的预览、强制授权和逐项结果流程不变。
  仓库菜单的“管理工作树…”保留，改为打开面板并定位到该分区。“主页显示”底部的“批量管理…”入口同样定位到该分区。`ManageWorktreesDialog` 作为独立弹窗移除。
- 工作树详情补充“分支与上游”“最近提交”“修改统计”三节，作为单个工作树详情的唯一内容。首页分组和侧栏以独立弹窗打开它；在仓库面板内则作为子视图打开，带“返回”，不在面板上再叠弹窗。
- 侧栏工作树行把单一的删除按钮换成“⋯”菜单，包含“工作树详情…”和“删除 worktree…”。仓库面板的工作树行使用同一菜单（删除在面板中通过批量清理完成）。后续变更的分支操作入口加入同一菜单。
- 首页工作树标题和侧栏工作树行用带文字说明的紧凑指示显示领先/落后提交数和“上游已删除”。
- CLI 同步：`webmux worktree ls/show` 和 `webmux repo show/branches` 输出上游同步状态、最近提交和修改统计。新增 `webmux repo remotes`。
- 前置条件：先归档已完成的 `batch-worktree-management`，本变更的增量规格基于归档后的主规格。
- 规格纠正：`batch-worktree-management` 规格中“仓库分组标题栏的‘管理工作树’按钮”在代码中并不存在，入口只在仓库菜单。本变更的增量规格去掉这句，不删除任何现有功能。
- “仓库设置”（初始化指令）保持独立弹窗，不并入仓库面板，原因见 design 的 Non-Goals。

## Capabilities

### New Capabilities

- `repository-details`：仓库面板的分区结构、工作树 Git 同步详情、分支清单、远程清单与凭据脱敏，以及这些数据的新鲜度表达。

### Modified Capabilities

- `workspace-overview`：工作树标题在现有状态之外显示上游领先/落后和“上游已删除”。
- `workbench-navigation`：仓库节点菜单的“管理工作树…”改为打开仓库面板的“工作树”分区。新增工作树节点的“⋯”菜单。
- `worktree-batch-management`：批量清理的承载位置由独立弹窗改为仓库面板的“工作树”分区，流程与保护规则不变。
- `shell-workspace-control`：CLI 输出新增上游同步状态、最近提交、修改统计和远程清单。

## Impact

- 后端：
  - `internal/workspace`：
    - 状态读取由 `status --porcelain=v1` 改为 `--porcelain=v2 --branch`；
    - 每次快照用一次批量 `log --no-walk` 读取各工作树的最近提交；
    - `branches` 的 `for-each-ref` 格式增加上游字段；
    - 新增远程读取和 URL 脱敏；
    - 读取 `FETCH_HEAD` 的修改时间。
  - `internal/api/workspace.go` 新增 `GET /api/repositories/{id}/remotes`。
  - `internal/contract`、`internal/wire` 和 `internal/api/testdata/wire` 快照随之更新。
- 前端：
  - 修改 `web/src/schemas.ts`；
  - `RepositoryDetailsDialog` 重构为仓库面板；
  - `ManageWorktreesDialog` 的列表和流程迁入面板分区；
  - `WorktreeDetailsDialog` 的内容拆为 `WorktreeDetails`，供独立弹窗与面板子视图共用；
  - 修改 `WorktreeBadges`、`WorkbenchTree`、`RepositoryMenuItems`、`WorktreeMenuItems`、`HomeWorktreeDialog` 和 `AppShell` / `shell.svelte.ts` 的入口；
  - 迁移 e2e helper `repository-menu.ts` 与相关 spec。
- CLI：`internal/clicmd/repo.go`、`worktree.go`。
- 用户故事：
  - 新增 US-J15（查看仓库与工作树的同步详情）；
  - 更新 US-J02、US-J03、US-J14 以及工作台导航相关故事。
- 不涉及：任何会改变 Git 状态的操作（切换分支、fetch、prune 由后续变更负责）。
