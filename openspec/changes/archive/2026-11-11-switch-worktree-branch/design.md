## Context

- 现在只有 `worktree add` 会检出分支，系统里没有 `checkout`/`switch`。
- 删除与释放已经有成熟的两阶段框架，切换直接复用以下部分：
  - `removalStateFromTree` 中的阻止条件模式；
  - `targetSessions(path)`：收集目标路径及其子目录下的会话；
  - `confirm` / `takeConfirmation`：按用途（`purpose`）绑定、5 分钟有效、一次性消费的确认凭证；
  - 仓库门闩 `repositoryState.gate`；
  - 写操作之后用 `context.Background()` 重读状态。
- 会话与工作树按路径关联（`session.Workspace`、`hybrid.WorkspaceRef` 只保存 ID 和路径，不保存分支），工作树 ID 由“仓库 + 路径”生成。原地切换后，关联、主页置顶与归档记录的归位都不受影响。
- 本机 Git 2.47 实测结果：
  - `git switch --no-guess --end-of-options <name>` 能正确拒绝 `-x` 这类名称；
  - 切到已被其他工作树检出的分支时，Git 自身会报错；
  - `--end-of-options` 从 Git 2.24 开始支持，低于当前隐含的最低版本 2.31。
- 依赖 `enrich-repository-details` 提供的 `changes` 计数、`operation`（进行中的 Git 操作）、仓库面板“分支”分区与面板内子视图约定，以及共享的 `WorktreeMenuItems`。
- 代码视图已有外部变化检测：`CodeWorkspace.svelte` 在可见时每 5s 调用一次 `POST …/check`；`internal/codeview/check.go` 按各标签页的来源重新读取文件版本、比对变更列表版本和目录版本，返回 `updated`/`missing`。内容快照以随机 sid 保存且不可变，不按路径缓存。

## Goals / Non-Goals

**Goals:**

- 切换要么完成，要么不改变工作区；不完整的状态只可能来自 Git 自身的失败，并在刷新后如实展示。
- 与删除共用同一套会话归属规则和确认凭证机制，不另起一套约定。

**Non-Goals:**

- 不做 stash 管理、不携带修改切换、不做 merge/rebase、不更新子模块。
- 新建工作树流程不设置上游，保持现有规格。
- 不为“切换”增加浏览器之间的实时推送；其他客户端按现有刷新规则看到结果。

## Decisions

### 1. API 形状：沿用两阶段

- `POST /api/repositories/{id}/worktrees/{wid}/switch-preview`
  - 请求：`{target: {kind: "local"|"remote"|"new"|"detach", branch?, ref?, name?, startPoint?, commit?}}`
  - 返回 `SwitchPreview`，包含：
    - `confirmationId`、`expiresAt`；
    - `worktree`（当前状态）；
    - `target`：`{kind, branch, commit, subject, createsBranch, upstream}`；
    - `sessions`：`[]SessionInfo`；
    - `blockers`：`[{code, message, path?}]`；
    - `warnings`：`[{code, message}]`。
- `POST /api/repositories/{id}/worktrees/{wid}/switch`
  - 请求：`{confirmationId, acknowledgeSessions}`
  - 返回刷新后的 `Worktree`。

有阻止条件时，预览仍然返回 200，但不签发凭证（`confirmationId` 为空），前端据此禁用按钮并展示原因。执行阶段新出现的阻止条件返回 409 `ErrConflict`。

- 备选方案：一步完成的 `POST /switch`，只在有会话时要求预览。这样两条路径的竞态语义不一致，也无法稳定绑定目标提交，被否决。

### 2. argv 与目标解析

预览阶段把目标解析成精确的 OID。远程目标在预览时就算出本地分支名。名称都先经过 `check-ref-format --branch`，并拒绝以 `-` 开头的名称。

| 目标 | argv（`Service.git` 自动加 `-C <path>`） |
|---|---|
| local | `switch --no-guess --end-of-options <branch>` |
| remote | `switch --no-guess --track --create <name> --end-of-options refs/remotes/<ref>` |
| new | `switch --no-guess --no-track --create <name> --end-of-options <startOID>` |
| detach | `switch --detach --end-of-options <commitOID>` |

禁止使用 `--force`、`--discard-changes`、`--merge`、`--orphan`、`--ignore-other-worktrees`。

远程目标使用完整的 `refs/remotes/...`，避免与同名本地分支或标签歧义，并让 `--track` 推导出正确的 `branch.<name>.remote/merge`。

### 3. 阻止条件与检测

- 已跟踪修改：使用 `enrich-repository-details` 的 porcelain v2 计数，`staged + unstaged + conflicted > 0` 即阻止。
- 进行中的操作：直接使用 `enrich-repository-details` 的 `operation` 字段，非空即阻止。它在快照读取时不启动 Git 进程判定，执行阶段重读单个工作树时同样得到最新值。
- 分支占用：`worktree list` 中其他条目的 `branch` 与目标相同即阻止，并在 `blocker.path` 中返回占用者的路径，供前端提供“转到该工作树”。
- 同一目标：本地目标与当前分支相同，或 detach 目标与当前 HEAD 相同且当前已是 detached。
- 警告（不阻止）：
  - 存在未跟踪文件；
  - 工作树处于 locked 状态（锁只限制 prune/move/remove，不限制切换，但锁的原因值得提示）；
  - 存在 `.gitmodules`（子模块不会自动更新）；
  - 有运行会话。
- 被未跟踪文件阻挡：识别 Git 的 `would be overwritten by checkout` 错误输出，取出路径列表（最多 50 条），以 `ErrConflict` 返回。

### 4. 会话确认与竞态

凭证用途为 `purposeSwitch`，绑定以下内容：

- 工作树 ID 与路径；
- 预览时的 `branch` 和 `head`；
- 目标解析出的 OID；本地目标同时记录分支尖端；
- `targetSessions` 中未退出会话的 ID 集合（排序后保存）。

执行步骤：

1. 获取仓库门闩；
2. 重新读取单个工作树（`currentTree`）及其状态；
3. 逐项比对绑定内容和阻止条件，有任何变化即返回 409，并附带“请重新预览”；
4. 会话集合非空时，要求 `acknowledgeSessions == true`；
5. 执行 `git switch`。

切换不调用 `reserve`，因为切换不会删除目录，期间启动新会话不会落到已删除的路径。新会话的出现由第 3 步的集合比对发现。

### 5. 执行时限与取消

`git switch` 被中途杀掉会留下不一致的索引和工作区，因此执行使用 `context.WithoutCancel(ctx)`，另设独立上限 `switchTimeout = 3min`，客户端断开不会中止 Git。超时后报告 504，并用 `context.Background()` 重读状态返回实际情况。这和创建失败的规则一致：以实际 Git 状态为准，不做隐式回滚。

- 备选方案：沿用 30s 的 `writeTimeout`。大仓库切换时更可能被截断，留下残缺状态，被否决。

### 6. 前端

切换流程的内容是 `SwitchBranch.svelte`，按 `enrich-repository-details` 的约定由两种外壳承载：
- 从首页或侧栏发起时，由 `SwitchBranchDialog.svelte`（基于 `ResponsiveDialog`）承载；
- 从仓库面板的工作树行或工作树详情子视图发起时，作为面板子视图承载，带“返回”，不叠第二层弹窗。

内容结构如下：

- 顶部用 `SegmentedControl`（`size="form" wrap`）选择三种模式：已有分支 / 新建分支 / 分离到提交；
- 已有分支模式复用 `PickerField` 加 `BranchCommandList`，分“本地”“远程”两组，当前分支和已被占用的分支标记后禁用；
- 新建分支模式复用 `branch-name.ts` 的校验和默认名生成，起点选择沿用 `WorktreeCreateFields` 的起始分支选择；
- 分离模式使用 `/commits` 列表。

选择变化后自动请求预览，用 `LatestRequest` 丢弃过期的响应。预览区依次展示“当前 → 目标”、阻止原因、警告和会话列表。有会话时显示确认复选框，与批量强制删除授权的模式一致。

按钮的安排：

- 主按钮“切换”使用 `default` 样式；
- 受阻（修改或会话）时，右下角提供 `success` 样式的“改为在新工作树打开”；
- 分支被占用时，提供“转到该工作树”：在面板中切换到该工作树的详情子视图，在独立弹窗中则关闭弹窗并打开该工作树的详情。

“在新工作树打开”通过 `shell.start({kind: "new-worktree", repositoryId, preset})` 打开现有弹窗；从面板发起时先关闭面板，再打开新建弹窗，屏幕上仍只有一层。`NewSessionDialog` / `WorktreeCreateFields` 新增 `preset = {mode, branch?, startBranch?, name?}`，只用于设置初始值，后续交互与现有流程相同。

切换成功后调用 `repositories.refresh(true)`：独立弹窗关闭，面板子视图回到工作树详情。当前首页正收窄在该工作树时保持收窄（ID 不变），标题随新分支更新。已打开的代码视图由现有的 5s 变化检测把受影响的标签页标为已更新或已删除，不需要额外通知。

### 7. CLI

在 `worktree.go` 中新增 `switch` 子命令，参数按第 1 节映射成 `target`。确认流程复用 `worktree rm` 的终端判定与 `--yes` 约定。

## Risks / Trade-offs

- [风险] agent 正在写文件时切换分支，会让它的后续写入落在新分支上 → 预览中对“工作中”状态的会话单独高亮并要求确认。系统不替用户做决定，也不中断会话，这与“会话关闭不会自行触发 worktree 删除”的分离原则一致。
- [结论] 代码视图会不会在切换后显示旧内容：不会长期停留。见 Context 中的变化检测，它就是 agent 在终端改文件时使用的同一机制，代码视图可见时切换后最多约 5s 标出变化，页面隐藏后重新可见时立即检查。本变更只在 E2E 中断言这一点，不新增刷新通道。
- [风险] 切换会执行 smudge/process 过滤器（如 Git LFS）和 `post-checkout` 钩子，它们可能访问网络、需要凭据或长时间运行：
  - LFS 下载失败时，Git 可能已更新部分文件；
  - `post-checkout` 失败时检出已经完成，但 `git switch` 仍以非零退出。

  → 执行沿用 3 分钟上限；失败或超时后以 `Background` 重读实际状态返回，错误信息附带脱敏后的 Git 输出首行，提示在终端完成。新建工作树（`worktree add`）已有同样的暴露面。本变更不禁用过滤器或钩子，因为那会检出错误的文件内容或跳过用户的仓库约定。
- [风险] `--track` 依赖 `remote.<name>.fetch` 映射；映射不规范时可能推导不出上游 → 执行后读取 `branch.<name>.merge`，未设置时在结果中给出警告，不回滚。
- [取舍] 不支持携带修改切换，而这恰好是 Git 默认允许的行为 → 这是保守选择。用户可以在终端自行切换，WebMux 刷新后如实展示结果；需要并行处理时可改用“在新工作树打开”。

## Migration Plan

先归档 `enrich-repository-details`，再实现本变更。没有持久化结构变化；回滚只需还原代码，未使用的 `purposeSwitch` 凭证会随服务重启清空。
