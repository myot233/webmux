## Context

- 可复用的现有机制（`internal/workspace`）：
  - 确认凭证：`confirm` / `takeConfirmation` 按 `purpose` 绑定、5 分钟有效、一次性消费，现有用途为 `remove`、`release`、`batch-remove`；
  - `targetSessions(path)`：收集路径及其子目录下的会话；
  - 仓库门闩 `repositoryState.gate`，以及写操作后用 `context.Background()` 加 `readTimeout` 重读状态的约定；
  - `checkPushed`（`pushcheck.go`）：只读本地远程跟踪引用判断提交是否已推送；
  - 已释放空间的归档记录用 `refs/webmux/released/<id>` 保住提交；`rebuildPlan` 在原分支不存在时退回 `ReasonBranchMissing`，以保存的提交 detached 重建（`rebuild.go`）。
- 依赖 `enrich-repository-details`：`Worktree` 的 `upstream/ahead/behind/upstreamGone/changes/operation`，`Branch` 的上游与领先/落后字段，仓库面板的子视图约定和 `WorktreeMenuItems`。
- 依赖 `add-repository-maintenance`：共用的 `remoteGit`（非交互环境、`RemoteFailure{status, message}` 分类、URL 脱敏）。
- 本机 Git 2.47.3 实测：
  - `push --porcelain <remote> --end-of-options <OID>:refs/heads/<dst>` 可以按提交推送；成功后本地 `refs/remotes/<remote>/<dst>` 随之更新，但不会设置上游；
  - 远端拒绝非快进时，`--porcelain` 输出 `!\t<src>:<dst>\t[rejected] (non-fast-forward)`，退出码 1；
  - `pre-push` 钩子拒绝时，标准输出没有任何引用状态行，只有钩子自己的输出和 `error: failed to push some refs`，退出码 1；
  - `merge --ff-only --end-of-options <OID>` 可用；
  - `config --remove-section branch.<不存在>` 以 128 退出并输出 `no such section`；
  - `rev-list --count <tip> --not --exclude=<name> --branches --remotes --tags --glob='refs/webmux/*'` 能正确数出独有提交；`--exclude` 对 `--branches` 使用不带 `refs/heads/` 前缀的名称；被 `refs/webmux/released/*` 保住的提交不计入独有提交。

## Goals / Non-Goals

**Goals:**

- 三个操作都以分支为对象，共用同一种“预览 → 凭证 → 门闩下复核 → 执行 → 重读”流程；Web 与 CLI 只调用同一组接口。
- 不在持有仓库门闩时访问网络。
- 执行的内容与预览中确认的内容完全一致：推送按提交推送，快进和删除都带旧值校验。

**Non-Goals:**

- 三方工作流（`pushRemote` 与上游不同）的推送目标选择；检测到时只给出警告。
- 删除远端分支、重命名分支、修改上游配置的独立入口。
- 为已删除的分支建立备份引用（由用户决定，结果中只给出 OID 与恢复命令）。

## Decisions

### 1. API：以分支为对象的三组两阶段接口

| 预览 | 执行 | 凭证用途 |
|---|---|---|
| `POST /api/repositories/{id}/branches/fast-forward-preview` `{branch}` | `POST …/branches/fast-forward` `{confirmationId, acknowledgeSessions}` | `purposeFastForward` |
| `POST …/branches/push-preview` `{branch, remote?, target?}` | `POST …/branches/push` `{confirmationId}` | `purposePush` |
| `POST …/branches/delete-preview` `{branches[]}` | `POST …/branches/delete` `{confirmationId, acknowledgeUnmerged}` | `purposeBranchDelete` |

预览有阻止条件时仍返回 200，但 `confirmationId` 为空，与 `switch-worktree-branch` 一致。执行阶段复核失败返回 409 `ErrConflict`，附带“请重新预览”。执行接口都在响应中带上重读后的 `repository`，前端不必再发一次请求。

工作树菜单中的操作也通过这组接口完成，参数是该工作树所在的分支，不另设按工作树的接口。

- 备选方案：快进分成“按工作树”和“按分支”两组接口。两者只在执行方式上不同，分开会让前端和 CLI 各自判断走哪条路径，被否决。

### 2. 快进

预览阶段在门闩下：

1. 用 `rev-list --left-right --count refs/heads/<b>...<upstreamOID>` 精确计算领先/落后，不依赖快照中可能“未计算”的值；
2. 要求有上游且远程跟踪引用存在、落后 > 0、领先 = 0；
3. 用 `worktree list` 查找检出该分支的工作树。找到时，要求它可用、`changes` 中已跟踪修改为 0、`operation` 为空，并收集 `targetSessions`；警告项为未跟踪文件、`.gitmodules` 与运行会话；
4. 用 `log -n 20 --format=%H%x00%s%x00%an%x00%cI%x00 <from>..<to> --` 列出将前进的提交。

凭证绑定分支名、`from`（分支当前提交）、`to`（上游提交）、检出它的工作树 ID 与路径，以及会话 ID 集合。

执行阶段在门闩下复核绑定项与阻止条件，再按是否检出分两条路径：

| 情况 | argv |
|---|---|
| 已检出 | `-C <工作树> merge --ff-only --end-of-options <to>` |
| 未检出 | 先 `merge-base --is-ancestor <from> <to>`，再 `update-ref -m "webmux: fast-forward" refs/heads/<b> <to> <from>` |

已检出的路径与切换一样会改写工作区，因此使用 `context.WithoutCancel` 加 3 分钟上限，失败或超时后以实际 Git 状态为准，不做隐式回滚。未跟踪文件被覆盖的错误输出（`would be overwritten by merge`）与切换共用同一个解析函数，取出最多 50 条路径；哪个变更先实施，就由它新增这个函数。

- 备选方案：执行时先 `fetch` 再快进。这样预览中的目标提交在执行时可能变化，凭证无法绑定确定的提交，被否决；需要最新上游时，先用“获取远程”。

### 3. 推送与发布

目标解析（预览阶段）：

- 有上游（`branch.<b>.remote` 与 `branch.<b>.merge`）且上游引用存在：目标是上游的远程与分支，请求中给出的 `remote`/`target` 被拒绝（400）；要求领先 > 0、落后 = 0；
- 没有上游，或上游已删除：按“发布”处理。远程取请求中的值；缺省时依次取 `remote.pushDefault`、`origin`、唯一的远程，仍无法确定时返回阻止条件。目标分支名默认等于本地分支名，经 `check-ref-format --branch` 校验，并拒绝以 `-` 开头的名称。上游已删除时加警告“会在远端重新创建该分支”；
- 配置了 `branch.<b>.pushRemote` 或 `remote.pushDefault`，且与上游远程不同：警告“WebMux 推送到上游 X；如需推送到 Y，请在终端操作”。

预览列出待推送的提交：有上游时为 `<upstreamOID>..<OID>`；发布时为 `<OID> --not --remotes`，最多 20 条，并给出总数；另给出脱敏后的 push URL。凭证绑定分支名、`OID`、远程名、目标分支、预览时的上游提交（若有）以及是否为发布。

执行：

1. 在门闩下复核分支仍指向 `OID`、上游未变化，然后**释放门闩**；
2. 通过 `remoteGit` 执行 `push --porcelain <remote> --end-of-options <OID>:refs/heads/<target>`；
3. 使用 `context.WithoutCancel` 加 `pushTimeout = 3min`；
4. 发布成功后，再次取得门闩，在 `branch.<b>.remote` 仍未设置且分支仍存在时，写入 `branch.<b>.remote` 与 `branch.<b>.merge = refs/heads/<target>`；
5. 以 `Background` 重读快照，与结果一起返回。

argv 禁止 `--force`、`--force-with-lease`、`--force-if-includes`、`--delete`、`--mirror`、`--all`、`--tags`、`--follow-tags`、`--no-verify`，refspec 不允许 `+` 前缀。

按提交推送而不是按分支名推送：执行时分支即使已被 agent 再次提交，推送的仍是预览中确认的那个提交；新的提交会使第 1 步复核失败，要求重新预览。

结果分类沿用 `remoteGit`，并补充推送特有的两种：

- 有 `!` 状态行：`rejected`，`non-fast-forward` 与 `fetch first` 的说明为“远端有新的提交，请先获取远程再快进”，`remote rejected` 附带远端给出的原因；
- 没有任何引用状态行、输出中也没有认证或网络关键字，且 `rev-parse --git-path hooks/pre-push` 指向的钩子存在：`rejected`，说明为“被 pre-push 钩子拒绝”，附带脱敏后的输出首行。该命令会遵循 `core.hooksPath`（实测），返回相对路径时按工作树路径解析。

- 备选方案：`push --set-upstream <remote> <b>:<target>`。这样推送的是执行时的分支尖端，与预览不一致，被否决。上游改为成功后显式写入配置。

### 4. 删除与批量清理

**清理候选。** `GET /branches` 为每个本地分支增加 `mergedIntoDefault`：用一次 `for-each-ref --merged=<主工作区 HEAD> --format=%(refname) refs/heads/` 得到，主工作区尚无提交或是 bare 仓库时都为 false。清理候选是未被检出，且 `upstreamGone` 或 `mergedIntoDefault` 为真的分支。

**预览分组。** 对每个所选分支执行

```
rev-list --count <OID> --not --exclude=<b> --branches --remotes --tags --glob=refs/webmux/* <其他工作树的 HEAD...>
```

结果为 0 归入“已包含”；大于 0 归入“有独有提交”，再用同一组参数执行 `log -n 20 --format=%h%x00%s` 列出标题。并发受 `worktreeGate` 限制，单次最多 50 个分支。被检出的分支直接给出阻止原因与占用者路径。凭证绑定每个分支的名称、`OID` 与分组。

**执行。** 在门闩下逐个处理：

1. 复核分支未被检出且仍指向 `OID`，否则记为 `skipped`，说明“分支已变化，请重新预览”；
2. “有独有提交”组只有在 `acknowledgeUnmerged` 为 true 时才处理，否则记为 `skipped`；
3. 执行 `update-ref -d refs/heads/<b> <OID>`（同时删除该分支的 reflog），再执行 `config --remove-section branch.<b>`，后者的 `no such section` 视为成功；
4. 每个分支的结果为 `{branch, status, commit, message}`，`deleted` 时附带恢复命令 `git branch <b> <OID>`。

恢复依赖对象仍在仓库中：没有其他引用指向的提交，会在 Git 垃圾回收清除不可达对象之前一直可恢复（默认 `gc.pruneExpire` 为两周）。结果中的提示如实写明这一点，不承诺永久可恢复。

- 备选方案：`branch -d` / `branch -D`。`-d` 的“已合并”判定依据当前 HEAD 或上游，在多工作树下不直观；`-D` 不带旧值校验，预览后分支变化时会删掉未确认的提交，被否决。
- 备选方案：删除前写备份引用 `refs/webmux/deleted/...`。需要额外的过期清理，用户已选择不做。

### 5. 契约

- `FastForwardPreview{confirmationId, expiresAt, branch, from, to, upstream, commits{total, items}, worktree?, sessions, blockers, warnings}`；
- `PushPreview{confirmationId, expiresAt, branch, commit, remote, pushUrl, target, publish, commits{total, items}, blockers, warnings}`；
- `BranchDeletePreview{confirmationId, expiresAt, items[{branch, commit, group: "contained"|"unmerged", unmerged{total, items}, blockers}]}`；
- 执行响应：`FastForwardResult{repository}`、`PushResult{status, message, repository}`、`BranchDeleteResult{results[], repository}`；
- `Branch` 增加 `mergedIntoDefault`。

`blockers`/`warnings` 使用与切换相同的 `{code, message, path?}`。所有字段只增不改，`wire` 快照新增三组样例。

### 6. 前端

三个流程组件 `FastForwardBranch.svelte`、`PushBranch.svelte`、`DeleteBranches.svelte`，按 `enrich-repository-details` 的约定由独立弹窗或仓库面板子视图承载。入口规则集中在一个纯函数 `branchActions(branch | worktree)` 中，菜单、工作树详情和“分支”分区都用它决定显示哪些操作，避免三处判断不一致：

| 状态 | 操作 |
|---|---|
| 有上游、落后 > 0、领先 = 0 | 快进到上游 |
| 有上游、领先 > 0、落后 = 0 | 推送… |
| 没有上游，或上游已删除 | 发布分支… |
| 领先与落后都 > 0 | 不提供，提示“已分叉” |
| 领先/落后未计算 | 两项都显示，由预览给出精确结果 |
| 未被检出（仅“分支”分区） | 删除分支… |

推送预览中，目标 URL 以等宽文本显示；发布时提供远程选择与目标名称输入。删除结果逐项展示，`deleted` 行提供复制恢复命令的按钮。“清理分支（N）”放在“分支”分区的标题行。

### 7. CLI

`internal/clicmd/repo.go` 新增 `ff`、`push`、`branch-rm` 三个子命令，分别调用上述接口；终端判定、`--yes` 与退出码约定沿用 `worktree rm`。`branch-rm --cleanup` 先读取 `/branches`，按清理候选规则在本地选出分支，再走同一个预览。

## Risks / Trade-offs

- [风险] `pre-push` 钩子运行测试，耗时超过 3 分钟 → 记为 `timeout`，结果说明“远端状态未知，请获取远程后确认”。不跳过钩子，因为跳过会绕开用户的仓库约定。
- [风险] 推送在传输中途因超时被结束 → 远端对每个引用的更新是原子的，只是本地不知道结果；处理同上。
- [风险] 快进已检出分支时，过滤器（LFS）或 `post-merge` 钩子失败 → 与切换相同：以重读后的实际状态返回，错误附带脱敏后的输出首行。
- [风险] 分支很多时 `mergedIntoDefault` 拖慢 `GET /branches` → 只多一次 `for-each-ref --merged`；沿用 `enrich-repository-details` 在 500 个分支夹具上的 1s 测量，一并计入。
- [取舍] 有独有提交的分支允许删除 → 单独分组、默认不选、需要单独确认，结果给出 OID 与恢复命令；不建备份引用，恢复窗口受垃圾回收限制，文案如实说明。
- [取舍] 不支持三方工作流的推送目标 → 检测到时给出警告，推送到上游；需要时在终端操作。

## Migration Plan

先归档 `enrich-repository-details` 与 `add-repository-maintenance`，再实现本变更；它与 `switch-worktree-branch` 互不依赖，唯一共用的是“未跟踪文件会被覆盖”的解析函数，由先实施的一方新增。没有持久化结构变化，回滚只需还原代码；未使用的确认凭证随服务重启清空。
