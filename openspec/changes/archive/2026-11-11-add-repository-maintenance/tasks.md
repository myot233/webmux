## 0. 前置

- [x] 0.1 确认 `enrich-repository-details` 已归档，并且 URL 脱敏函数与 `lastFetchedAt` 已在主干上。完成标准：`openspec validate add-repository-maintenance --strict` 通过。

## 1. 获取服务（`internal/workspace`）

- [x] 1.1 新增共用的 `Service.remoteGit`（design 第 3 节），在 `Service.git` 之上为单次调用追加环境变量，不影响其他调用：
  - 设置 `SSH_ASKPASS_REQUIRE=never`；
  - 只有 `GIT_SSH_COMMAND`、`core.sshCommand` 与 `GIT_SSH` 都未设置时，才设置 `GIT_SSH_COMMAND="ssh -o BatchMode=yes"`；
  - 失败转成 `RemoteFailure{status, message}`。

  完成标准：单测分别断言三者之一已设置时不注入、都未设置时注入，以及本地 bare 远程上调用的返回值。
- [x] 1.2 实现 `FetchRemotes`：
  - 读取远程清单，按名称排序后逐个执行 `fetch --prune --recurse-submodules=no --end-of-options <remote>`；
  - 每个远程有独立时限，整体上限 3 分钟，执行使用 `WithoutCancel`；
  - 全部结束后以 `Background` 重读快照，与逐远程结果一起返回。

  完成标准：用本地 bare 仓库作为远程的测试覆盖以下情况：
  - 成功后远程已删除的分支被清除，`lastFetchedAt` 前进；
  - 一个远程不可达时，另一个远程仍然成功；
  - 请求 ctx 取消后获取仍然完成。
- [x] 1.3 实现按仓库的 `fetchFlight` 合并。完成标准：并发两次调用的测试断言 git fetch 只执行一次（用 hook 计数），两个调用方收到相同结果；同时进行的 `CreateWorktree` 不需要等待获取完成。
- [x] 1.4 在 `remoteGit` 中实现错误分类与消息脱敏（`auth`/`network`/`timeout`/`rejected`/`error`，消息最多 200 字符）。完成标准：表驱动单测覆盖每种关键字（含 `Host key verification failed` 归为 `auth`），以及包含 `https://u:tok@` 的输出，所有输出中都不含原始令牌；日志断言不含 URL。

## 2. 契约、路由与快照

- [x] 2.1 在 `internal/contract` / `internal/wire` 中新增 `FetchResult{remote, status, message}` 与 `FetchRemotesResponse{results, repository}`，并在 `web/src/schemas.ts` 中同步。完成标准：`web/test/wire-contract.test.ts` 通过。
- [x] 2.2 在 `internal/api/workspace.go` 中注册 `POST /api/repositories/{id}/fetch`，沿用认证、Origin 校验和 `workspaceFailure`；没有远程时返回 409 并说明原因。完成标准：`workspace_test.go` 覆盖 401、403、404、409 和 200。
- [x] 2.3 在 `wire_snapshot_test.go` 中补充获取结果样例（含一个成功和一个认证失败的远程），然后运行 `go test ./internal/api -run TestWireSnapshots -update`。完成标准：快照只新增 `repository-fetch.json`。

## 3. CLI

- [x] 3.1 新增 `webmux repo fetch REPO [--json]`，退出码为 0/1。完成标准：单测覆盖全部成功、部分失败和没有远程三种情况。
- [x] 3.2 新增 `webmux worktree prune REPO [--yes] [--json]`：在本地筛选失效登记，再走批量预览和执行，确认规则与 `worktree rm` 相同。完成标准：单测覆盖没有可清理项（退出码 0）、非交互且没有 `--yes`（退出码 2）、带 `--yes` 删除成功，以及部分失败（退出码 1）。

## 4. 前端（`web/src`）

- [x] 4.1 在 `RepositoryPanel` 标题栏增加“获取远程”按钮：
  - 使用 tabler `cloud-download`，窄屏只显示图标并保留 `aria-label`/`title`；
  - 进行中显示进度，用 `InlineNotice` 逐个列出远程结果，并用响应中的 `repository` 更新仓库状态；
  - 没有远程时禁用并说明原因。

  完成标准：浏览器测试覆盖成功、部分失败、没有远程，以及进行中再次点击不重复发请求。
- [x] 4.2 在 `RepositoryWorktrees` 中增加“清理失效登记（N）”快捷入口：只预选未锁定的 `prunable` linked worktree，直接进入批量预览；N 为 0 时不显示。完成标准：浏览器测试覆盖 N 的计数排除 locked、点击后预选集合正确，以及 N 为 0 时入口隐藏。
- [x] 4.3 新增 E2E `repository-maintenance.spec.ts`：先用本地 bare 远程推进上游，在面板中获取后看到落后数；再从外部删除两个工作树目录，执行“清理失效登记”，确认后分支保留。完成标准：`pnpm --dir web exec playwright test test/e2e/repository-maintenance.spec.ts` 通过。
- [x] 4.4 更新 `RepositoryPanel` 的 Storybook story，加入获取进行中与部分失败的场景，并加入 `overflow.spec.ts`。完成标准：在 390×844 和 320×568 视口下没有横向溢出。

## 5. 用户故事与集成验证

- [x] 5.1 新增用户故事（按计划为 US-J17：获取远程并清理失效登记），更新 US-J14（清理失效登记的快捷入口），并把 `docs/user-stories.md` 的区间更新到当时的最大编号。`switch-worktree-branch` 与本变更可并行实施，编号以实施时下一个空闲编号为准，不预留空号。完成标准：`make docs-check` 通过。
- [x] 5.2 集成验证：依次运行 `make format`、`make check`、`pnpm --dir web check:arch`、`make test-fast`，再运行 4.3 中的 E2E。完成标准：全部通过。另外采集三种视口的截图供 PR 使用。

## Workflow follow-up

- 经评审合并后归档本变更；之后再开始 `add-branch-sync-actions`，它复用这里的 `remoteGit`。
