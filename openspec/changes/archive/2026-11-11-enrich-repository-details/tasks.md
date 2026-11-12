## 0. 前置

- [x] 0.1 归档 `batch-worktree-management`（`openspec archive batch-worktree-management`）。完成标准：再次运行 `openspec validate enrich-repository-details --strict` 时，不再出现任何“Archive would refuse this delta”提示。

## 1. 工作树状态模型（`internal/workspace`）

- [x] 1.1 新增 porcelain v2 解析器，产出 `branch.oid/head/upstream/ab` 和四类计数，并保留 `dirty/untracked/ignored` 的原有语义。完成标准：表驱动单测覆盖以下情况并全部通过：
  - 普通、重命名（含额外 NUL 路径）、冲突、未跟踪、忽略、子模块条目；
  - `(initial)`、`(detached)`；
  - 有上游但缺少 `branch.ab`（即上游已删除）；
  - `branch.ab +? -?`（未计算）。
- [x] 1.2 `populateWorktreeStatus` 改为执行 `status --porcelain=v2 -z --branch --untracked-files=normal --ignored=matching`，填充 `Upstream/Ahead/Behind/UpstreamGone/Changes`。完成标准：`go test -race ./internal/workspace` 全部通过，其中删除、释放、批量删除的现有测试不做任何修改。
- [x] 1.3 在 `readRepository` 中为可用工作树批量读取最近提交并回填 `LastCommit`；读取失败时只把该字段置空。完成标准：新增测试断言 detached 工作树和分支工作树都拿到正确的 `hash/subject/author/committedAt`，且整个快照只多出一次 `log` 调用（用 hook 计数）。
- [x] 1.4 实现 `lastFetchedAt`：取 `<common>/FETCH_HEAD` 与 `<common>/worktrees/*/FETCH_HEAD` 中最新的修改时间，没有文件时为 nil。完成标准：单测覆盖只在 linked worktree 中 fetch 的情况，以及从未 fetch 的情况。
- [x] 1.5 扩展 `branches` 的 `for-each-ref` 格式，解析 `upstream:track`（`ahead`、`behind`、两者同时、`gone`、空）和 `committerdate`，`Base` 标注 `kind`。完成标准：`git_test.go` 新增上游领先、落后、已删除、无上游四种用例，并在 500 个分支的夹具上测量，`Branches` 耗时不超过 1s（在测试日志中记录）。
- [x] 1.6 实现 `Remotes` 读取和 URL 脱敏函数。完成标准：单测覆盖 `https://user:token@`、`https://token@`、`ssh://git@`、scp 风格、含 `%40` 的密码、无法解析的 URL，任何输出中都不出现原始凭据。
- [x] 1.7 测量 ahead/behind 开销：构造与上游相差约 10k 提交的分支，记录 `status --branch` 的耗时；超过 2s 时按 design 第 1 节的风险处理，加 `--no-ahead-behind` 并返回“未计算”。完成标准：测试日志中有测量值，代码行为与测量结论一致。
- [x] 1.8 实现 `operation` 判定（design 第 1 节）：不启动 Git 进程取得工作树的 git 目录（`.git` 目录，或 `.git` 文件中的绝对/相对 `gitdir:`），再检查各操作标记文件。完成标准：
  - 单测分别构造 merge、rebase（`rebase-merge` 与 `rebase-apply`）、cherry-pick、revert、bisect 状态，在主工作区与 linked worktree 中各验证一次；
  - `gitdir:` 指向不存在目录时 `operation` 为 null；
  - 用 hook 计数断言快照的 Git 进程数没有因此增加。

## 2. 契约、路由与快照

- [x] 2.1 在 `internal/contract` / `internal/wire` 中新增 `WorktreeChanges`、`Remote`、`Remotes`，并扩展 `Worktree`、`Repository`、`Branch`、`BranchBase`。完成标准：`go build ./...` 通过，`internal/wire` 的测试通过。
- [x] 2.2 在 `internal/api/workspace.go` 中注册 `GET /api/repositories/{id}/remotes`，沿用 `workspaceFailure`、认证与 Origin 校验。完成标准：`internal/api/workspace_test.go` 新增 401、403、404 和成功（脱敏后）四个用例，全部通过。
- [x] 2.3 在 `wire_snapshot_test.go` 中补充 remotes 样例和含新字段的 worktree 样例，然后运行 `go test ./internal/api -run TestWireSnapshots -update`。完成标准：快照 diff 只包含新增字段。
- [x] 2.4 在 `web/src/schemas.ts` 中同步 Zod schema。完成标准：`web/test/wire-contract.test.ts` 通过。

## 3. CLI（`internal/clicmd`）

- [x] 3.1 `worktree ls` 的状态列追加 `+A/-B` 或 `gone`；`worktree show` 和 `repo show` 输出上游、最近提交、修改统计和上次获取时间；`repo branches` 增加上游列与领先/落后列。完成标准：`worktree_test.go` 和 repo 测试的表格与 JSON 断言更新后通过。
- [x] 3.2 新增 `webmux repo remotes REPO [--json]` 并接入帮助文本。完成标准：单测断言输出已脱敏、退出码为 0，未登记仓库时退出码为 2。

## 4. 共享展示组件与工作树详情（`web/src`）

- [x] 4.1 新增 `WorktreeSyncSummary.svelte`（`compact`/`full` 两种形态），按 webmux-ui 规范实现领先/落后指示、“无上游”“上游已删除”“未计算”“未知”几种状态，需要时在 `ui/badge` 中加变体并登记到架构守卫。完成标准：浏览器组件测试覆盖这五种上游状态和可访问名称，`pnpm --dir web check:arch` 通过。
- [x] 4.2 把 `WorktreeDetailsDialog` 的内容拆成 `WorktreeDetails.svelte`，`WorktreeDetailsDialog` 只保留弹窗外壳；内容增加“分支与上游”“最近提交”“修改统计”三节，以及进行中操作的提示。完成标准：`WorktreeDetailsDialog.test.ts` 新增对三节内容和“正在变基”的断言，原有的容量测量测试仍然通过。
- [x] 4.3 首页工作树标题（`WorktreeBadges` / `WorkspaceGroup`）接入 `compact` 指示。完成标准：`RepositoryGroup.test.ts` 断言领先指示的可访问名称“领先上游 3 个提交”。

## 5. 仓库面板与入口迁移（`web/src`）

- [x] 5.1 把 `RepositoryDetailsDialog` 重构为 `RepositoryPanel` 外壳加 `RepositoryOverview` 分区：
  - 支持 `initialTab`；
  - 外壳内维护“分区 → 子视图”的视图栈，子视图带“返回”，`busy` 时阻止返回、切换分区与关闭。

  完成标准：浏览器测试断言概览各行、不可用仓库只显示“概览”，以及子视图返回后分区的筛选和滚动位置保持不变；e2e helper `expectHomeWorktrees` 依赖的“主页 n / 共 m”文案保持不变。
- [x] 5.2 新增 `RepositoryWorktrees` 分区：
  - 迁入 `ManageWorktreesDialog` 的列表、筛选、选择模式和三段流程；
  - 浏览状态下点击行，在面板内打开 `WorktreeDetails` 子视图；每行提供 `WorktreeMenuItems` 菜单（本变更只有“工作树详情…”），`busy` 时禁用 Tabs；
  - 删除 `ManageWorktreesDialog.svelte`，并把 `ManageWorktreesDialog.test.ts` 迁移为分区测试。

  完成标准：迁移后的测试覆盖原有全部用例，另加“浏览状态点击行进入子视图”“行菜单没有删除项”和“执行期间切换分区”三个用例。
- [x] 5.3 新增 `RepositoryBranches` 分区：本地和远程两组、搜索与匹配计数，占用路径可点击并在面板内打开工作树详情子视图。完成标准：浏览器测试覆盖搜索、占用跳转、无上游、上游已删除和未计算几种情况。
- [x] 5.4 新增 `RepositoryRemotes` 分区：脱敏 URL、复制、空状态。完成标准：浏览器测试覆盖 fetch 与 push 相同和不同两种情况，以及空状态。
- [x] 5.5 `shell.svelte.ts` / `AppShell.svelte`：把面板入口合并为 `repositoryDetails(repository, tab?)`，`manageWorktrees` 和“主页显示”中的“批量管理…”都改为定位到“工作树”分区，删除 `managingWorktrees` 状态。完成标准：`HomeWorktreeDialog` 的相关测试更新后通过，`pnpm --dir web check` 通过。
- [x] 5.6 侧栏工作树行用 `SidebarRowMenu` 加 `WorktreeMenuItems`（新增 `ondetails`）替换原来的删除按钮，可访问名称为“工作树操作：{名称}”。不可用仓库的菜单只保留“仓库详情…”。完成标准：`WorkbenchTree.test.ts` 改为通过菜单定位删除操作，并新增“工作树详情…”和“主工作区没有删除项”两个用例。
- [x] 5.7 迁移 e2e：`repository-menu.ts`、`worktrees.spec.ts`、`repository-scope.spec.ts`、`workspace-discovery.spec.ts` 中依赖“管理工作树”弹窗或侧栏删除按钮的步骤。完成标准：以下命令全部通过：

  ```
  pnpm --dir web exec playwright test test/e2e/worktrees.spec.ts \
    test/e2e/repository-scope.spec.ts test/e2e/workspace-discovery.spec.ts
  ```
- [x] 5.8 为 `RepositoryPanel`（四个分区）和 `WorktreeSyncSummary` 新增 Storybook story，并把相关场景加入 `overflow.spec.ts`。完成标准：在 1280×720、390×844、320×568 三个视口下 `scrollWidth <= innerWidth`，Storybook 测试通过。

## 6. 用户故事与集成验证

- [x] 6.1 更新用户故事，完成标准：`make docs-check` 通过。
  - 在 `docs/user-stories/code-and-worktrees.md` 中新增 US-J15（查看仓库与工作树的同步详情）；实施时 US-J15 已被占用则取下一个空闲编号；
  - 更新 US-J02、US-J03、US-J14 以及工作台导航相关故事中的入口描述；
  - 把 `docs/user-stories.md` 的区间更新到最大编号。
- [x] 6.2 集成验证：依次运行 `make format`、`make check`、`pnpm --dir web check:arch`、`make test-fast`，再运行 5.7 中的 E2E。完成标准：全部通过。在隔离演示数据上采集桌面、平板、手机三种视口和浅色/深色主题的前后对比截图，供 PR 使用。

## Workflow follow-up

- 经评审合并后归档本变更；之后再开始 `switch-worktree-branch` 与 `add-repository-maintenance`，`add-branch-sync-actions` 在 `add-repository-maintenance` 之后开始。
