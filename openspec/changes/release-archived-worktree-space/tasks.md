## 1. 记录与契约

- [x] 1.1 `internal/hybrid`：新增 `Release` 类型和 `Record.Released` 可选字段，实现 `MarkReleased`（立即持久化）与 `ClearReleased(releaseID)`；记录已释放时 `resume` 返回 `ErrConflict`
- [x] 1.2 `internal/contract` 与 `internal/wire`：`HybridRecord` 增加 `released`；新增 `ReleasePreview`、`ReleaseResult`、`RebuildPreview`、`RebuildRequest` 契约类型及转换
- [x] 1.3 `web/src/contracts/chat-schemas.ts` 与 `web/src/contracts/schemas.ts`：同步新增 Zod schema，并用 `z.infer` 导出类型

## 2. 释放（internal/workspace）

- [x] 2.1 把 `removalState` 的阻止项拆成带类型的 blocker，保证删除预览的线上文案不变；`confirmation` 增加 `purpose` 字段，确认标识不得跨用途使用
- [x] 2.2 实现忽略项清单与大小估算：`git status --ignored` 取顶层条目，用 `lstat` 遍历，不跟随符号链接，总时限 2 秒、条目上限 200,000，清单最多 50 项
- [x] 2.3 实现 `PreviewRelease`：在删除规则的基础上去掉忽略文件阻止，增加“HEAD 必须可解析为提交”和“至少有一条已归档记录”的校验
- [x] 2.4 实现 `ReleaseWorktree`：在仓库锁与路径预留下重新校验 → `update-ref refs/webmux/released/<id>` → `MarkReleased` → 非强制 `git worktree remove`；失败时 `ClearReleased` 并删除引用
- [x] 2.5 实现 `DropRelease` 和按仓库的孤立 `refs/webmux/released/*` 清扫，清扫只在记录加载成功后进行

## 3. 重建（internal/workspace）

- [x] 3.1 实现 `PreviewRebuild`：判定 `reuse` / `branch` / `detached`，给出原因（`branchMissing` / `branchMoved` / `branchCheckedOut`）及阻止项（路径被占用、仓库未登记或不可用）
- [x] 3.2 实现 `RebuildWorktree`：重新计算 mode 并与请求比较，不一致时返回 `ErrConflict`；执行 `worktree add <branch>` 或内部的 `addDetached`；成功后 `ClearReleased` 并删除引用

## 4. API

- [x] 4.1 `internal/api` 注册 `release-preview`、`release`、`rebuild-preview`、`rebuild` 四个路由，保持鉴权与 Origin 检查，在 `errors.go` 中映射错误
- [x] 4.2 `DELETE /api/hybrid-sessions/{id}` 删除已释放的记录后调用 `DropRelease`
- [x] 4.3 在 `wire_snapshot_test.go` 中加入新接口与带 `released` 的记录，执行 `go test ./internal/api -run TestWireSnapshots -update` 重新录制

## 5. 前端

- [x] 5.1 `recoverable.ts` 新增 `previewRelease`、`releaseWorktree`、`previewRebuild`、`rebuildWorktree`；`archived.ts` 新增 `releasableRecordWorktree`，`ArchiveActions` 增加 `release`
- [x] 5.2 新增 `ReleaseWorktreeDialog.svelte`（展示阻止原因、记录、忽略项与大小、不可找回提示）和 `RebuildWorktreeDialog.svelte`（展示路径、短 OID、重建方式与原因），遵循 `webmux-ui` 技能
- [x] 5.3 `SidebarSessionMenu`、`ChatHistory`、`AppSidebar`、`ArchivedSessions` 接入“释放空间”菜单项与“已释放空间”标记
- [x] 5.4 侧栏点击与“恢复聊天 / 恢复到聊天 / 恢复到终端”遇到已释放的记录时，先走重建确认，再调用 `resumeHybrid`；已释放记录的“删除记录”只删除记录

## 6. 测试与验证

- [x] 6.1 Go 测试：
  - 只有忽略文件时释放成功，提交与分支保留；
  - 未提交修改、未跟踪文件或运行中会话时拒绝；
  - 确认后目标变化时拒绝；
  - 删除失败时回滚标记与引用；
  - 未重建时恢复返回 409；
  - 重建的 `branch`、`detached`（分支移动或删除）、`reuse` 三种模式，以及路径被占用时阻止；
  - 删除最后一条记录后引用被清理
- [x] 6.2 Vitest 测试：覆盖 `releasableRecordWorktree` 的判定，以及 `ArchivedSessions` 和 `ChatHistory` 的菜单项、已释放标记、恢复前的确认流程
- [x] 6.3 Playwright E2E（`hybrid.spec.ts`）：释放 → 已释放标记 → 恢复聊天时确认重建 → 进入聊天页
- [x] 6.4 更新用户故事（新增 US-J13，更新 US-Q12、US-Q19、US-C15）和覆盖索引，在 `verification-log` 中只记录实际运行过的验证
- [x] 6.5 运行 `make check`、`pnpm --dir web check:arch` 和 `make test-fast`（按用户要求在 PR CI 上运行完整门禁）；在隔离演示服务上截取侧栏与首页的桌面、手机、明暗主题截图
