## Context

现状（动机见 `proposal.md`）：

- `workspace.Service` 已有单个工作树删除闭环（`PreviewRemoval` → `RemoveWorktree`）：
  - 在仓库门闩 `acquire(ctx, r)` 下读取 Git 清单与状态，检查子模块/嵌套仓库（`unsupportedContents`）、关联会话（`targetSessions`）与已归档混合会话（`targetRecords`）；
  - 通过 `identity` 持有工作树目录、`.git` 文件与 Git 管理目录的打开句柄，存入 5 分钟有效的 `confirmations`（按 `purpose` 隔离）；
  - 执行时用 `s.reserve(ctx, path)` 阻止并发启动会话，强制模式下先调用 `manager.Close` 并等待 `Done()`，最后执行 `git worktree remove [--force] -- <path>`，并通过 `retainRecords` 将已归档混合会话标为已释放空间（建立 `refs/webmux/released/<id>` 私有引用）。
- 当前存在三处限制：
  1. 只有单个工作树预览与删除接口；若前端循环调用 N 次单删预览或删除，会重复获取 N 次仓库门闩并对全仓库所有工作树跑 N 轮 `git status`，也无法在批量执行开始前一次性预留全部目标路径。
  2. `removalState` 把 `!tree.Available || tree.Prunable` 一律视为 `blockUnsupported`，导致外部已删除目录、Git 标为 `prunable` 的失效登记无法在网页端清理；同时 `headCommit` 在 `!tree.Available` 时直接返回空串，无法提取登记中记录的 `HEAD` 提交。
  3. 工作台首页默认只渲染已添加到主页（`homeWorktrees`）或有会话/归档记录的工作树，大量取消主页显示的空闲 linked worktree 只出现在 `HomeWorktreePicker` 中，没有集中筛选与批量删除界面；侧栏仓库节点也只有单一“+”按钮，窄栏宽度下无法并排放两个行尾图标按钮。

## Goals / Non-Goals

**Goals:**

- 提供单仓库独立的“管理工作树”弹窗，从仓库分组标题栏、“主页显示”弹层底部及侧栏仓库行“⋯”菜单均可打开。
- 用一对专用批量接口（`batch-removal-preview`、`batch-remove`）完成单次仓库快照读取、统一确认凭证管理、全路径预留与逐项容错执行。
- 统一单删与批删对未锁定 `prunable` 缺失目录登记的清理支持，保留本地分支，并按相同规则保护已归档混合会话记录。
- 遵循 `webmux-ui` 设计规范（`ResponsiveDialog`、下划线 `SegmentedControl`、明确按钮层级与移动端触控尺寸）。

**Non-Goals:**

- 跨多个已登记仓库的全局批量删除。
- 删除本地或远程 Git 分支（`git branch -d/-D`）。
- 强制解锁 `locked` 工作树或递归删除含子模块/嵌套仓库的目录。
- 新增 `webmux` CLI 批量工作树子命令。

## Decisions

### D1 专用服务端批量接口，而非前端循环调用单删接口

在 `internal/api/workspace.go` 新增两个路由：

| 方法 | 路径 | 说明 |
| --- | --- | --- |
| POST | `/api/repositories/{id}/worktrees/batch-removal-preview` | 请求 `{worktreeIds}`（去重，1..50 项）；返回 `{confirmationId, expiresAt, items}` |
| POST | `/api/repositories/{id}/worktrees/batch-remove` | 请求 `{confirmationId, includeForce, closeSessions}`；返回 `{results}` |

理由与备选方案对比：

- **备选方案**：前端对勾选的 N 个工作树并发/串行调现有单删接口。缺点是每次调用都会触发整仓 `readRepository`（O(N × M) 次 `git status` 子进程），且第 1 个删除进行时第 N 个路径尚未被 `s.reserve` 预留，存在并发启动新会话的窗口。
- **选定方案**：
  - `PreviewBatchRemoval` 只获取一次仓库门闩、执行一次 `s.current(ctx, r)`，逐项计算 `removalState`、`targetRecords` 与 `checkPushed`，签发一个 `purposeBatchRemove` 类型的 `confirmation`（新增切片字段持有所有目标的 `removalState` 与 `commit`；过期或消费时统一关闭全部 `identity` 文件句柄）。
  - `BatchRemoveWorktrees` 校验凭证（`includeForce && !closeSessions` 返回 `ErrInvalid`），在仓库门闩下先对所有拟执行路径执行 `s.reserve`（若某项路径预留冲突则仅将该项标为 `failed`，其余已预留项继续），执行完毕统一 `defer` 释放预留并刷新一次 `readRepository`。

### D2 批量执行的逐项容错与超时边界

Git 没有跨多个 worktree 的原子事务。批量删除按请求确认时的顺序逐项处理：

1. 若某项在预览时尚有阻塞且请求未开启 `includeForce`，或预览时已为 `!supported`，直接记为 `skipped`。
2. 重新读取当前工作树状态并校验 `sameTarget`、`sameSessions` 及 `headCommit == item.commit`：若确认后状态或提交已变化，记为 `failed`（附冲突说明），不关闭该项任何会话，继续处理下一项。
3. 若该项需要强制删除（`includeForce && closeSessions`），关闭其活动会话并等待 `item.Done()`；等待超时仅使当前项记为 `failed`，不中断后续独立项。
4. 执行 `git worktree remove [--force] -- <path>`：成功则调用 `retainRecords` 保护归档混合会话并记为 `removed`；失败则记为 `failed`。

超时预算：单个 `writeTimeout`（30s）对包含大目录的多工作树偏紧，批量删除采用按项数线性扩展的受限上限 `min(3*time.Minute, max(writeTimeout, time.Duration(len(targets))*15*time.Second))`，同时限制单次批量最多 50 个工作树。

### D3 精准清理选中的未锁定 `prunable` 登记

全局 `git worktree prune` 会清理仓库内**所有**失效登记（包含用户未勾选的条目），而 `git worktree remove -- <path>` 原生支持仅移除指定路径对应的那一条缺失目录登记（前提未 `locked`）。

因此扩展 `removalState` 与身份校验：

- 当 `tree.Kind == "linked" && !tree.Locked && tree.Prunable` 时，使用 `os.Lstat(tree.Path)` 检查路径：
  - 仅当 `errors.Is(err, os.ErrNotExist)` 时，判定为可安全清理的缺失目录登记（`prunableMissing = true`，`supported = true`），跳过目录内 `git status` 与 `unsupportedContents` 检查，仅持有 `common` 与 `$GIT_COMMON_DIR/worktrees` 下对应管理目录的身份句柄；
  - 若路径仍存在（如权限不足或符号链接异常），仍保持 `blockUnsupported`，防止误删不可读目录。
- 执行前再次验证 `os.Lstat(tree.Path)` 仍为 `os.ErrNotExist` 且管理目录身份未变；若期间原路径被外部重新创建，拒绝并报状态变化。
- 关联的活动会话仍通过 `targetSessions(tree.Path)` 检查（若仍有运行于该已删路径的旧会话，同样计入 `blockSessions`）。
- 扩展 `headCommit` 以覆盖目录已缺失的 `prunable` 项：
  - 当 `!tree.Available && tree.Prunable && tree.Head != nil` 时，改在 `r.entry.AnchorPath` 执行 `git rev-parse --verify --end-of-options <*tree.Head>^{commit}`；
  - 若可解析为提交 OID，清理登记后同样通过 `retainRecords` 为原路径下的已归档混合会话建立 `refs/webmux/released/<id>`，保证外部删目录后清理登记仍能保留聊天重建能力。
- 单个 `RemoveWorktree` 共享同一 `removalState`，单删与批删语义完全一致。

### D4 工作台入口与侧栏仓库菜单

- **状态提升**：在 `AppShell.svelte` 维护 `managingWorktrees = $state<Repository | null>(null)`，通过 `LazyDialog` 按需加载 `ManageWorktreesDialog.svelte`，并在 `Shell` 接口新增 `manageWorktrees(repository: Repository): void`。
- **仓库标题栏与主页选择器**：
  - `RepositoryGroup.svelte` 标题栏在 `HomeWorktreePicker` 与“新建工作树”之间新增 `outline` 按钮“管理工作树”（同排统一 `size="sm"` + `min-h-9 sm:min-h-8`）；
  - `HomeWorktreePicker.svelte` 底部提示栏右侧增加轻量按钮“批量管理…”，点击后关闭 Popover 并调用 `onmanage()`。
- **侧栏仓库节点菜单**：
  - `WorkbenchTree.svelte` 仓库行右侧原本只有一个 `file-tree-action` 的“+”按钮；窄侧栏并排放两个悬停按钮会挤压仓库名与等待计数。
  - 新增 `SidebarRepositoryMenu.svelte`（与 `SidebarSessionMenu.svelte` 同构，复用 `ListButton variant="file-tree-action"` 与惰性挂载的 `DropdownMenu`），可访问名称为 `仓库操作：{repo.name}`，菜单项依次为“新建工作树…”与“管理工作树…”。
- **选中工作树回退**：在 `SessionList.svelte`（或 `repositories` 选择状态同步处）检测：当 `shell.repositories.choice === repo.id` 且 `shell.repositories.worktree` 不再存在于刷新后的 `repo.worktrees` 时，自动调用 `shell.repositories.select(repo.id)` 回退到该仓库总览。

### D5 `ManageWorktreesDialog.svelte` 界面分层

组件按单一状态机驱动三个视图阶段（`select` → `preview` → `result`，全部成功时直接关闭）：

- **`select` 阶段**：
  - 搜索框 + `SegmentedControl`（`size="form" wrap`，四档：`all` / `safe` / `blocked` / `protected`）；
  - 初次分类基于当前 `Repository.Worktrees` 快照、`shell.sessions.sessions` 与 `shell.archived.records` 本地派生（`$derived`），零额外请求即可流畅筛选；
  - 勾选集合存为 `SvelteSet<string>`（仅存参与渲染的选择 ID）。
- **`preview` 阶段**：
  - 提交选中 ID 到 `batch-removal-preview`，展示服务端权威分类（可安全清理项、受阻可强制项、不可删除跳过项）、归档保留提示与未推送提交警告；
  - 受阻项存在时渲染强制授权 `Checkbox`（绑定 `includeForce`），未勾选时只提交安全项，勾选后切换 `destructive` 主按钮。
- **`result` 阶段（仅当存在 `failed` 项时进入）**：
  - 展示成功/跳过/失败统计与失败原因列表，将勾选集合收敛为失败项 ID，提供“重新预览重试”回到 `preview` 阶段。

## Risks / Trade-offs

- [列表本地预分类与服务端实时预览存在短暂时间差] → 本地分类仅用于第一步筛选体验，真正执行前必须经过第二步服务端实时预览与确认凭证校验；若期间状态变化，预览会准确归入受阻或不可删除组。
- [批量强制删除可能一次关闭多个会话并丢弃多处未提交代码] → 默认不勾选强制授权，且预览中逐项列出受阻原因与将被停止的会话标题；服务端校验 `includeForce && closeSessions` 双重标志。
- [大体积忽略目录导致批量 `git worktree remove` 耗时较长] → 服务端限制单批最多 50 项并按项数扩展超时上限；前端在 `busy` 期间锁定弹窗关闭路径并显示进行中提示。
- [侧栏仓库新建工作树从一键点击变为点开“⋯”菜单再选一项] → 多一次点击，但换取了侧栏行宽不拥挤且在会话页也能直接打开工作树批量管理的能力；仓库主页标题栏仍保留直接可见的“新建工作树”按钮。

## Migration Plan

- 纯新增 REST 路由与前端弹窗，不涉及持久化文件格式变更，无需数据迁移。
- 回滚时直接还原代码提交即可；期间通过批量删除保留的归档记录使用的是既有的 `hybrid.Record.Released` 与 `refs/webmux/released/<id>` 格式，与旧版完全兼容。
