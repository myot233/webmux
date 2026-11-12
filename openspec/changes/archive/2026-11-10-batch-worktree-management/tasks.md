## 1. 契约与类型（Contract & Schemas）

- [x] 1.1 在 `internal/contract` 与 `internal/wire` 新增批量预览与批量删除的请求/响应类型（`BatchRemovalPreviewRequest`、`BatchRemovalPreview`、`BatchRemoveWorktreesRequest`、`BatchRemoveResult`）及转换函数
- [x] 1.2 在 `web/src/schemas.ts` 新增对应的 Zod schema 并通过 `z.infer` 导出 TypeScript 类型

## 2. 服务端 `prunable` 支持与批量删除（`internal/workspace`）

- [x] 2.1 扩展 `removalState` 与身份校验：对 `tree.Kind == "linked" && !tree.Locked && tree.Prunable` 且 `os.Lstat(tree.Path)` 为 `os.ErrNotExist` 的缺失目录登记标为可安全清理，持有 `common` 与对应管理目录句柄并在执行前复核目录仍不存在
- [x] 2.2 扩展 `headCommit`：当目录已缺失但 `tree.Prunable && tree.Head != nil` 时，回退在 `AnchorPath` 校验 `<*tree.Head>^{commit}`，使清理失效登记后仍能通过 `retainRecords` 保护关联归档混合会话的提交
- [x] 2.3 扩展 `confirmation` 支持 `purposeBatchRemove` 及多项状态/句柄生命周期管理（超时与消费时统一关闭全部 `identity` 句柄）
- [x] 2.4 实现 `PreviewBatchRemoval`：校验 `worktreeIds`（非空、去重、上限 50），在单次仓库门闩与 `s.current` 快照下生成各项预览并签发批量确认凭证
- [x] 2.5 实现 `BatchRemoveWorktrees`：校验 `includeForce` 与 `closeSessions` 授权，预先 `s.reserve` 所有拟删路径，按序复核并执行安全/强制 `git worktree remove` 与 `retainRecords`，单项失败记录原因继续执行其余项，结束时刷新仓库状态并返回逐项结果

## 3. HTTP API 与 Wire 快照（`internal/api`）

- [x] 3.1 在 `internal/api/workspace.go` 注册 `POST /api/repositories/{id}/worktrees/batch-removal-preview` 与 `POST /api/repositories/{id}/worktrees/batch-remove`，沿用认证、Origin 检查与 `workspaceFailure` 状态映射
- [x] 3.2 在 `internal/api/wire_snapshot_test.go` 补充批量预览与批量删除样例，运行 `go test ./internal/api -run TestWireSnapshots -update` 更新快照

## 4. 前端批量管理弹窗与入口（`web/src`）

- [x] 4.1 新增纯逻辑辅助模块（工作树本地分类 `safe` / `blocked` / `protected`、搜索过滤与统计），供弹窗与单元测试复用
- [x] 4.2 新增 `ManageWorktreesDialog.svelte`：基于 `ResponsiveDialog`、`SegmentedControl`（`size="form" wrap`）与 `Checkbox` 实现“筛选多选 → 实时预览与可选强制授权 → 部分失败重试”三段流程，遵循 `webmux-ui` 规范
- [x] 4.3 在 `shell.svelte.ts` 与 `AppShell.svelte` 注册 `manageWorktrees(repository)` 及 `LazyDialog` 挂载，删除成功后刷新仓库与归档并同步清理草稿和 `homeWorktrees` 固定项
- [x] 4.4 在 `RepositoryGroup.svelte` 标题栏增加“管理工作树”按钮，在 `HomeWorktreePicker.svelte` 底部增加“批量管理…”入口，并在 `SessionList.svelte` 处理当前收窄查看的工作树被删后的仓库视图回退
- [x] 4.5 新增 `SidebarRepositoryMenu.svelte` 并在 `WorkbenchTree.svelte` 替换仓库节点的单一“+”按钮为“⋯”菜单（含“新建工作树…”与“管理工作树…”）

## 5. 测试、用户故事与验证

- [x] 5.1 Go 单元测试（`internal/workspace`、`internal/api`）：覆盖未锁定 `prunable` 清理与归档提交保护、目录重新出现时的冲突拦截、批量仅删安全项跳过受阻项、显式强制删除关闭会话、确认后单项外部写入导致的部分失败与路径并发预留保护
- [x] 5.2 前端 Vitest 单测与浏览器组件测试：覆盖工作树分类筛选、`ManageWorktreesDialog` 预览/强制授权/部分失败重试、`WorkbenchTree` 仓库菜单及 `RepositoryGroup` / `HomeWorktreePicker` 入口
- [x] 5.3 Playwright E2E 测试：覆盖从工作台打开“管理工作树”批量删除空闲与 `prunable` 工作树、保留分支及归档记录转为已释放空间
- [x] 5.4 更新用户故事（`docs/user-stories/code-and-worktrees.md` 新增 `US-J14` 并更新 `US-J03`、`US-J08`；同步工作台侧栏相关故事与索引计数）
- [x] 5.5 运行 `make format`、`make check`、`pnpm --dir web check:arch` 与测试门禁，在隔离演示数据上采集桌面/窄屏、浅色/深色主题的 UI 截图证据
