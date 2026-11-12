## 0. 前置

- [x] 0.1 确认 `enrich-repository-details` 与 `add-repository-maintenance` 已归档：`Worktree.operation`、`Branch` 的上游字段、`remoteGit` 与 URL 脱敏函数已在主干上。完成标准：`openspec validate add-branch-sync-actions --strict` 通过，且不出现“Archive would refuse”提示。

## 1. 快进（`internal/workspace`）

- [x] 1.1 实现 `PreviewFastForward`（design 第 2 节）：
  - 精确计算领先/落后，判定上游与分叉；
  - 找到检出它的工作树，检查已跟踪修改、`operation`、可用性，并收集会话；
  - 列出最多 20 个将前进的提交；没有阻止条件时以 `purposeFastForward` 签发凭证。

  完成标准：单测覆盖可快进、无上游、上游已删除、已同步、已分叉、有修改、正在变基，以及快照中领先/落后“未计算”时预览仍给出精确值。
- [x] 1.2 实现 `FastForward`：
  - 门闩下复核绑定项，校验 `acknowledgeSessions`；
  - 已检出时执行 `merge --ff-only --end-of-options <to>`，使用 `WithoutCancel` 加 3 分钟上限；
  - 未检出时先 `merge-base --is-ancestor`，再执行带旧值的 `update-ref`；
  - 以 `Background` 重读并返回；
  - 未跟踪文件被覆盖时，使用与切换共用的解析函数返回最多 50 条路径（`switch-worktree-branch` 尚未实施时由本任务新增该函数）。

  完成标准：`go test -race ./internal/workspace` 通过，新增测试覆盖：
  - 已检出与未检出两条路径都快进成功，且未检出时任何工作树文件不变；
  - 预览后分支或上游变化导致 409；
  - 有会话但未确认时拒绝，确认后会话仍在运行；
  - 未跟踪文件被覆盖时失败且工作区不变；
  - 请求 ctx 在执行中被取消，快进仍然完成。

## 2. 推送与发布（`internal/workspace`）

- [x] 2.1 实现 `PreviewPush`（design 第 3 节）：解析目标（上游，或发布时的默认远程与目标名），给出上游已删除与 `pushRemote` 警告，列出待推送提交与脱敏 URL，以 `purposePush` 签发凭证。完成标准：单测覆盖以下情况：
  - 有上游；无上游且只有 `origin`；
  - 多个远程且设置了 `remote.pushDefault`；多个远程无法确定；
  - 目标名 `--x` 被拒；已分叉；上游已删除。
- [x] 2.2 实现 `Push`：
  - 门闩下复核后释放门闩，经 `remoteGit` 按提交推送，使用 `WithoutCancel` 加 `pushTimeout`；
  - 发布成功后在门闩下写入上游配置，再重读快照；
  - 按 design 第 3 节解析 `--porcelain` 输出与 `pre-push` 钩子拒绝。

  完成标准：用本地 bare 远程的测试覆盖：
  - 推送成功后领先数为 0；发布后 `branch.<b>.merge` 为 `refs/heads/<target>`；
  - 预览后分支又有新提交导致 409，且远端没有收到新提交；
  - 远端已有新提交时得到 `rejected` 与“先获取远程再快进”的说明，本地配置不变；
  - `pre-push` 钩子（含 `core.hooksPath` 指定的钩子）拒绝时得到“被 pre-push 钩子拒绝”；
  - 推送进行期间同一仓库的快照读取与创建工作树不被阻塞；
  - argv 中不出现 design 第 3 节禁止的选项（用 hook 记录 argv 断言）。

## 3. 删除与批量清理（`internal/workspace`）

- [x] 3.1 在 `Branches` 中增加 `mergedIntoDefault`（一次 `for-each-ref --merged`）。完成标准：单测覆盖已合入、未合入、主工作区尚无提交三种情况；500 个分支夹具上的 `Branches` 耗时仍不超过 1s，并记录在测试日志中。
- [x] 3.2 实现 `PreviewBranchDelete`：
  - 被检出的分支给出阻止原因与占用者路径；
  - 按 design 第 4 节的 `rev-list` 分出“已包含”与“有独有提交”，后者列出最多 20 条提交标题；
  - 单次最多 50 个分支，以 `purposeBranchDelete` 签发凭证。

  完成标准：单测覆盖以下分组：
  - 归入“已包含”：已合入 `main`、只在远程跟踪分支上、只被 `refs/webmux/released/*` 保住、只被其他工作树的 detached HEAD 引用；
  - 归入“有独有提交”：squash merge 后的分支；
  - 被检出的分支不可删除。
- [x] 3.3 实现 `DeleteBranches`：
  - 门闩下逐项复核，“有独有提交”组只在 `acknowledgeUnmerged` 时处理；
  - 执行带旧值的 `update-ref -d`，再执行 `config --remove-section`；
  - 逐项返回 `deleted`/`skipped`/`failed` 与 OID。

  完成标准：单测覆盖：
  - 未确认独有提交时只删除“已包含”组；
  - 预览后分支前进被跳过；
  - 分支配置被一并删除，没有分支配置的分支也能删除；
  - 删除后 `PreviewRebuild` 对引用该分支的归档记录返回 `ReasonBranchMissing`，且仍可重建。

## 4. 契约、路由与快照

- [x] 4.1 在 `internal/contract` / `internal/wire` 中新增 design 第 5 节的类型，扩展 `Branch.mergedIntoDefault`，并在 `web/src/schemas.ts` 中同步。完成标准：`web/test/wire-contract.test.ts` 通过。
- [x] 4.2 在 `internal/api/workspace.go` 中注册三组预览与执行路由，沿用认证、Origin 校验与 `workspaceFailure`；凭证用途不匹配时返回 409。完成标准：`workspace_test.go` 为每组覆盖 401、403、400（非法参数）、409（状态变化或用途不匹配）和 200。
- [x] 4.3 在 `wire_snapshot_test.go` 中补充样例：快进预览（含会话）、推送预览（发布）、推送结果（被拒）、删除预览（两组）与删除结果；然后运行 `go test ./internal/api -run TestWireSnapshots -update`。完成标准：快照只新增这些文件，以及 `branches` 样例中的 `mergedIntoDefault` 字段。

## 5. CLI（`internal/clicmd`）

- [x] 5.1 新增 `webmux repo ff`、`webmux repo push`、`webmux repo branch-rm`（含 `--cleanup`、`--unmerged`）并接入帮助文本，退出码按 `shell-workspace-control` 的增量规格。完成标准：单测覆盖：
  - 每个命令的成功、阻止（1）、非交互且没有 `--yes`（2）和 `--json` 输出；
  - `push` 对有上游的分支给出 `--remote`（2）；
  - `branch-rm --cleanup` 跳过有独有提交的分支。

## 6. 前端（`web/src`）

- [x] 6.1 新增纯函数 `branchActions`，按 design 第 6 节的表格决定可用操作。完成标准：Vitest 单测覆盖表中每一行，以及 detached、bare、不可用、待清理的工作树没有任何分支操作。
- [x] 6.2 新增 `FastForwardBranch.svelte`、`PushBranch.svelte`、`DeleteBranches.svelte` 三个流程组件及各自的独立弹窗外壳，遵循 webmux-ui 规范：
  - 预览展示提交列表、阻止原因、警告与会话确认；
  - 推送展示脱敏 URL，发布时提供远程选择与目标名；
  - 删除分两组展示，“有独有提交”组需单独勾选，结果行可复制恢复命令。

  完成标准：浏览器组件测试覆盖每个流程的成功、阻止、409 后自动重新预览、过期预览响应被丢弃，以及删除时未勾选独有提交确认只删除“已包含”组。
- [x] 6.3 接入入口，完成后刷新仓库状态：
  - `WorktreeMenuItems`（首页、侧栏、仓库面板工作树行）；
  - `WorktreeDetails` 的“分支与上游”一节；
  - `RepositoryBranches` 的行操作与“清理分支（N）”；
  - 从仓库面板发起时作为子视图承载。

  完成标准：`WorkbenchTree.test.ts`、`RepositoryGroup.test.ts` 和仓库面板分支分区测试断言各状态下菜单项的显隐与“清理分支（N）”的计数；面板测试断言流程只占一层对话框。
- [x] 6.4 新增 E2E `branch-operations.spec.ts`，使用本地 bare 远程覆盖：
  - 在面板中获取远程后快进落后的工作树；
  - 发布一个没有上游的分支，并看到领先数归零；
  - 删除一个工作树后用“清理分支”删除遗留分支，并验证 squash merge 分支默认未选中。

  完成标准：`pnpm --dir web exec playwright test test/e2e/branch-operations.spec.ts` 通过，菜单操作复用 `repository-menu.ts` 中的 helper。
- [x] 6.5 为三个流程组件新增 Storybook story（含阻止、会话、长分支名、长 URL、两组删除），并加入 `overflow.spec.ts`。完成标准：在 390×844 和 320×568 视口下没有横向溢出，Storybook 测试通过。

## 7. 用户故事与集成验证

- [x] 7.1 新增用户故事（按计划为 US-J18：在 WebMux 中快进、推送和清理分支），更新 US-J14（删除工作树后清理遗留分支的入口），并把 `docs/user-stories.md` 的区间更新到当时的最大编号；编号以实施时下一个空闲编号为准，不预留空号。完成标准：`make docs-check` 通过。
- [x] 7.2 集成验证：依次运行 `make format`、`make check`、`pnpm --dir web check:arch`、`make test-fast`，再运行 6.4 中的 E2E。完成标准：全部通过。另外采集三种视口的截图，以及推送与清理流程的 GIF，供 PR 使用。

## Workflow follow-up

- 经评审合并后归档本变更。
