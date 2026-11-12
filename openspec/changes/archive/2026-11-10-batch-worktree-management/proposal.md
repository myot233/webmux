## Why

日常并行使用多个 CLI Agent 时，单个仓库下会快速堆积大量已完成任务的空闲 linked worktree 与外部删目录后残留的 `prunable` 登记，其中许多空闲项已取消“主页显示”而不在首页分组中展示。

目前用户只能在工作台逐个点开工作树菜单预览并删除，且目录已缺失的 `prunable` 登记在网页端被直接拦截，缺少按单仓库集中盘点、筛选并批量清理工作树的能力。

## What Changes

- 新增单仓库独立的“管理工作树”响应式弹窗（桌面端 Dialog、窄屏底部 Drawer），列出该仓库下全部 worktree（含主工作区、未添加到主页的空闲项、受阻项与 `prunable` 待清理项），支持按分支/提交/路径搜索、按状态分段筛选（全部、可安全清理、有阻塞、受保护）以及快捷多选。
- 在工作台提供三处打开该仓库“管理工作树”弹窗的入口：
  - 工作台首页仓库分组标题栏新增“管理工作树”按钮；
  - 仓库“主页显示”弹层底部说明区新增“批量管理…”快捷入口；
  - 工作台侧栏导航树的仓库节点将原有的单一“+”按钮改为“⋯”操作菜单，包含“新建工作树…”与“管理工作树…”。
- 采用“选择 → 服务端实时批量预览 → 确认执行 → 结果反馈”流程：
  - 默认只安全删除无未提交修改、无未跟踪文件、无活动会话的 linked worktree 及未锁定的 `prunable` 缺失目录登记；
  - 预览中将存在未提交修改、未跟踪文件或活动会话的可强制项单独分组，默认跳过，仅当用户显式勾选强制删除授权（同意关闭关联活动会话并永久丢弃目录内容）时一并强制删除；
  - 始终保留全部本地分支；被删目录下的已归档混合会话记录（含强制删除刚关闭的混合会话）按既有机制转为“已释放空间”并由私有引用保护提交；
  - 执行按逐项结果返回（`removed`、`skipped`、`failed`），单项状态变化或失败不阻塞同批其余独立工作树，部分失败时在弹窗内展示原因并支持对失败项重新预览重试。
- 统一单删与批删对 `prunable` 缺失目录登记的处理：未锁定且目录确认已不存在（`os.ErrNotExist`）的 linked worktree 允许在页面安全清理其 Git 登记，并回退通过 Git 登记中的 `HEAD` 保护关联的已归档混合会话记录。
- 后端新增单仓库批量删除预览与批量删除 API 端点，同步更新 `internal/contract`、`internal/wire`、`web/src/schemas.ts` 及 wire 快照。

## Capabilities

### New Capabilities

- `worktree-batch-management`: 单仓库工作树批量管理弹窗的筛选多选、批量删除实时预览、可选强制删除授权、逐项执行结果反馈与重试。

### Modified Capabilities

- `worktree-lifecycle`: 允许在网页端安全清理未锁定且目录已不存在的 `prunable` linked worktree 登记，并在目录缺失时仍为关联的已归档混合会话保护 `HEAD` 提交。
- `workspace-overview`: 在仓库分组标题栏与“主页显示”弹层提供单仓库“管理工作树”入口，并在当前选中的工作树被批量删除后回退到仓库总览。
- `workbench-navigation`: 工作台侧栏仓库节点的行内操作由单一“+”按钮调整为“⋯”菜单，提供“新建工作树…”与“管理工作树…”。

## Impact

- 后端：
  - `internal/workspace`：扩展 `removalState` 与 `headCommit` 支持未锁定的缺失目录 `prunable` 登记；新增 `PreviewBatchRemoval` 与 `BatchRemoveWorktrees`，复用确认凭证、文件身份校验、路径预留与归档记录保留逻辑；
  - `internal/api`：在 `workspace.go` 注册 `batch-removal-preview` 与 `batch-remove` 路由，保持鉴权、Origin 校验与错误状态映射；
  - `internal/contract`、`internal/wire` 及 `internal/api/testdata/wire` 快照。
- 前端：
  - 新增 `ManageWorktreesDialog.svelte`，挂载于 `AppShell.svelte` 并通过 `Shell` 上下文暴露；
  - 更新 `RepositoryGroup.svelte`、`HomeWorktreePicker.svelte`、`WorkbenchTree.svelte` 接入入口；
  - 更新 `web/src/schemas.ts` 增加批量预览与批量删除的 Zod 契约。
- 用户故事：新增 `US-J14`（批量管理与清理工作树），同步更新 `US-J08`、`US-J03` 及工作台导航相关故事。
- 不涉及 CLI 批量子命令与本地分支删除。
