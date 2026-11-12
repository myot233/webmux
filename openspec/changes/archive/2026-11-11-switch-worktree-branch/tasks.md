## 0. 前置

- [x] 0.1 确认 `enrich-repository-details` 已归档：`openspec/specs/repository-details/spec.md` 存在，且 `Worktree.changes` 与 `Worktree.operation` 已在主干上。完成标准：`openspec validate switch-worktree-branch --strict` 通过，且不出现“Archive would refuse”提示。

## 1. 切换服务（`internal/workspace`）

- [x] 1.1 新增目标解析：`local` / `remote` / `new` / `detach` 四种目标解析为精确 OID，`remote` 推导出本地分支名，名称经 `check-ref-format --branch` 校验并拒绝以 `-` 开头的名称。完成标准：单测覆盖四种目标、远程同名本地分支已存在、名称 `--orphan`，以及不可解析的起点。
- [x] 1.2 新增阻止与警告检测：
  - 阻止：已跟踪修改、进行中操作（直接使用 `Worktree.operation`）、分支占用（附带占用者路径）、同一目标；
  - 警告：未跟踪文件、locked、`.gitmodules` 和运行会话。

  完成标准：单测覆盖每种阻止与警告；进行中操作的判定本身由 `enrich-repository-details` 的测试覆盖，这里只断言 `operation` 非空时预览不签发凭证。
- [x] 1.3 实现 `PreviewSwitch`：只读阶段在仓库门闩下执行，没有阻止条件时以 `purposeSwitch` 签发凭证，绑定工作树身份、`branch/head`、目标 OID 和会话 ID 集合。完成标准：有阻止条件时不签发凭证的测试，以及凭证不能用于删除或释放接口的测试。
- [x] 1.4 实现 `SwitchWorktree`，按以下顺序执行：
  1. 取得门闩，重新读取，比对绑定内容，校验 `acknowledgeSessions`；
  2. 按 design 第 2 节的 argv 执行，使用 `WithoutCancel` 加 3 分钟上限；
  3. 以 `Background` 重读并返回 `Worktree`；
  4. 把 Git 的 `would be overwritten` 错误输出解析为最多 50 条路径的 `ErrConflict`。

  完成标准：`go test -race ./internal/workspace` 通过，新增测试覆盖以下情况：
  - 确认后新会话出现导致拒绝；
  - 目标分支前进导致拒绝；
  - 未确认会话导致拒绝；
  - 未跟踪文件被覆盖时失败且工作区不变；
  - 请求 ctx 在执行中被取消，切换仍然完成；
  - `remote` 目标设置了 `branch.<name>.merge`；
  - `post-checkout` 钩子以非零退出时，返回的 `Worktree` 反映已完成的检出，错误中不含未脱敏的 URL。

## 2. 契约、路由与快照

- [x] 2.1 在 `internal/contract` / `internal/wire` 中新增 `SwitchTarget`、`SwitchPreviewRequest`、`SwitchPreview`、`SwitchRequest`，并在 `web/src/schemas.ts` 中同步。完成标准：`web/test/wire-contract.test.ts` 通过。
- [x] 2.2 在 `internal/api/workspace.go` 中注册 `switch-preview` 与 `switch` 两个路由，沿用认证、Origin 校验和 `workspaceFailure`。完成标准：`workspace_test.go` 覆盖 401、403、400（非法名称）、409（状态变化）和 200，全部通过。
- [x] 2.3 补充 `wire_snapshot_test.go` 中的切换预览样例（含阻止条件与会话），然后运行 `go test ./internal/api -run TestWireSnapshots -update`。完成标准：快照只新增 `worktree-switch-preview.json`。

## 3. CLI

- [x] 3.1 新增 `webmux worktree switch`：参数映射、标准错误中的预览输出、`--yes` 与终端确认、退出码 0/1/2，并接入帮助文本。完成标准：`worktree_test.go` 覆盖干净切换、非交互且有会话（退出码 2）、有修改（退出码 1）和 `--json` 输出。

## 4. 前端（`web/src`）

- [x] 4.1 `NewSessionDialog` / `WorktreeCreateFields` 支持 `preset = {mode, branch?, startBranch?, name?}`，只用于设置初始值。完成标准：`WorktreeCreateFields.test.ts` 新增“已有分支预填”和“远程起点加默认名预填”两个用例，现有用例不变。
- [x] 4.2 新增切换流程内容组件 `SwitchBranch.svelte` 和独立弹窗外壳 `SwitchBranchDialog.svelte`，实现遵循 webmux-ui 规范，包含：
  - 三种模式的分段控件，分组且可搜索的分支列表（当前分支与被占用分支禁用）；
  - 自动预览（`LatestRequest`），阻止原因、警告、会话列表与确认复选框；
  - “改为在新工作树打开”和“转到该工作树”两个替代操作。

  完成标准：浏览器组件测试覆盖：
  - 干净切换成功；
  - 有修改时主按钮禁用并出现替代按钮；
  - 分支被占用；
  - 有会话时未勾选确认则不能提交；
  - 409 后自动重新预览；
  - 过期的预览响应被丢弃。
- [x] 4.3 接入“切换分支…”入口：
  - `WorktreeMenuItems` 新增该项，bare、不可用、待清理的工作树不显示；首页、侧栏与仓库面板工作树行同时出现；
  - 在 `shell.svelte.ts` 与 `AppShell.svelte` 中注册 `switchBranch(worktree)` 和对应的 `LazyDialog`；在 `RepositoryPanel` 中注册同一流程的子视图；
  - 切换成功后执行 `repositories.refresh(true)`。

  完成标准：`WorkbenchTree.test.ts`、`RepositoryGroup.test.ts` 和仓库面板工作树分区测试断言菜单项的显隐；面板测试断言从行菜单发起时只有一层对话框。
- [x] 4.4 在 `RepositoryBranches` 中为未被占用的本地分支和远程分支增加“在新工作树打开…”行操作，打开 `shell.start({kind: "new-worktree", preset})`。完成标准：浏览器测试断言两种分支预填的模式与默认名。
- [x] 4.5 在 4.6 的 E2E 中打开该工作树的代码视图，切换后断言已打开的标签页在变化检测后显示新分支的内容（design 第 6 节与 Risks 中的结论），不新增刷新通道。
- [x] 4.6 新增 E2E `worktree-switch.spec.ts`，覆盖以下流程：从侧栏切换干净工作树、有修改时改为在新工作树打开、有运行 Shell 会话时确认后切换且会话仍在运行。完成标准：`pnpm --dir web exec playwright test test/e2e/worktree-switch.spec.ts` 通过，菜单操作复用 `repository-menu.ts` 中的 helper。
- [x] 4.7 新增 `SwitchBranchDialog` 的 Storybook story（含阻止、会话、长分支名三种场景），并加入 `overflow.spec.ts`。完成标准：在 390×844 和 320×568 视口下没有横向溢出。

## 5. 用户故事与集成验证

- [x] 5.1 新增用户故事（按计划为 US-J16：切换已有工作树的分支），更新 US-J05（在新工作树打开已有分支的新入口），并把 `docs/user-stories.md` 的区间更新到当时的最大编号。`add-repository-maintenance` 与本变更可并行实施，编号以实施时下一个空闲编号为准，不预留空号。完成标准：`make docs-check` 通过。
- [x] 5.2 集成验证：依次运行 `make format`、`make check`、`pnpm --dir web check:arch`、`make test-fast`，再运行 4.6 中的 E2E。完成标准：全部通过。另外采集三种视口的截图，以及切换流程的 GIF，供 PR 使用。

## Workflow follow-up

- 经评审合并后归档本变更。
