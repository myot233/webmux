# HTTP API：仓库与代码工作区

## 仓库与 worktree

| 方法       | 路径                                                            | 说明                                                                                                                                                                                |
| ---------- | --------------------------------------------------------------- | ----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| GET / POST | `/api/repositories`                                             | 列出仓库快照；POST `{path}` 登记仓库（新增 201，已存在 200 且保留原指令）                                                                                                           |
| PATCH      | `/api/repositories/{id}`                                        | `{initInstruction}` 修改或清空（空字符串）初始化指令，200 返回仓库；超过 4000 个字符 400                                                                                            |
| DELETE     | `/api/repositories/{id}`                                        | 仅移除登记，204                                                                                                                                                                     |
| GET        | `/api/repositories/{id}/worktrees`                              | 刷新仓库快照（含 `checkedAt` 与 `worktrees`）                                                                                                                                       |
| GET        | `/api/repositories/{id}/branches`                               | `{branches, bases, defaultBase, defaultBaseSubject, defaultBranch, defaultParent}`，不 fetch                                                                                        |
| GET        | `/api/repositories/{id}/commits?from=<oid>[&from=<oid>]`        | `{commits: [{hash, subject, author, committedAt}]}`：自 1–2 个完整提交 ID（本地与上游尖端）合并最近 50 条，新到旧，超出上限的起始提交仍会列出；非法 ID 400                          |
| POST       | `/api/repositories/{id}/branches/refresh`                       | `{branch}`；fetch 已配置上游并返回远程基线或本地回退；`upstreamHead` 为 fetch 到的上游尖端（本地分叉时也返回），未成功时为空                                                        |
| POST       | `/api/repositories/{id}/branches/fast-forward-preview`          | `{branch}`；快进预览 `{confirmationId, expiresAt, branch, from, to, upstream, commits, worktree, sessions, blockers, warnings}`，只读本地远程跟踪引用                               |
| POST       | `/api/repositories/{id}/branches/fast-forward`                  | `{confirmationId, acknowledgeSessions}`；返回 `{repository}`                                                                                                                        |
| POST       | `/api/repositories/{id}/branches/push-preview`                  | `{branch, remote?, target?}`（`remote`/`target` 只用于发布）；推送预览 `{confirmationId, expiresAt, branch, commit, remote, pushUrl, target, publish, commits, blockers, warnings}` |
| POST       | `/api/repositories/{id}/branches/push`                          | `{confirmationId}`；返回 `{status, message, repository}`，远端拒绝也是 200                                                                                                          |
| POST       | `/api/repositories/{id}/branches/delete-preview`                | `{branches}`（1–50 个本地分支）；`{confirmationId, expiresAt, items: [{branch, commit, group, unmerged, blockers}]}`                                                                |
| POST       | `/api/repositories/{id}/branches/delete`                        | `{confirmationId, acknowledgeUnmerged}`；返回 `{results: [{branch, status, commit, message}], repository}`                                                                          |
| GET        | `/api/repositories/{id}/remotes`                                | `{remotes: [{name, fetchUrl, pushUrl}]}`，按名称排序；URL 已去除凭据（见下），不联网                                                                                                |
| POST       | `/api/repositories/{id}/fetch`                                  | 空 body `{}`；获取全部远程，返回 `{results: [{remote, status, message}], repository}`（见下）；没有远程 409                                                                         |
| POST       | `/api/repositories/{id}/worktrees`                              | `{path, mode: "new", branch, base}` 或 `{path, mode: "existing", branch}`，201                                                                                                      |
| POST       | `/api/repositories/{id}/worktrees/{worktreeId}/removal-preview` | 删除预览：`{confirmationId, expiresAt, worktree, sessions, archived, commit, push, blockers, warnings, forceAllowed}`                                                               |
| POST       | `/api/repositories/{id}/worktrees/{worktreeId}/remove`          | `{confirmationId, mode: "safe"}`；强制删除另需 `confirmPath` 与 `closeSessions: true`，204                                                                                          |
| POST       | `/api/repositories/{id}/worktrees/batch-removal-preview`        | 批量删除预览：`{worktreeIds}`（1–50 项去重），返回 `{confirmationId, expiresAt, items}`                                                                                             |
| POST       | `/api/repositories/{id}/worktrees/batch-remove`                 | `{confirmationId, includeForce?, closeSessions?}`（`includeForce` 为 true 时 `closeSessions` 必须为 true），返回 `{results: [{worktreeId, path, branch, status, error}]}`           |
| POST       | `/api/repositories/{id}/worktrees/{worktreeId}/release-preview` | 释放空间预览：`{confirmationId, expiresAt, worktree, commit, push, archived, ignored, ignoredMore, ignoredBytes?, sizeComplete, blockers}`                                          |
| POST       | `/api/repositories/{id}/worktrees/{worktreeId}/usage`           | 计算工作树目录占用：空 body，返回 `{bytes, entries, complete}`；只在请求时遍历，不随仓库快照返回                                                                                    |
| POST       | `/api/repositories/{id}/worktrees/{worktreeId}/release`         | `{confirmationId}`；删除目录、保留分支与已归档记录并标为已释放，`{releaseId, recordIds}`                                                                                            |
| POST       | `/api/repositories/{id}/worktrees/{worktreeId}/switch-preview`  | 切换预览：`{target}`，返回 `{confirmationId, expiresAt, worktree, target, sessions, blockers, warnings}`（见下）                                                                    |
| POST       | `/api/repositories/{id}/worktrees/{worktreeId}/switch`          | `{confirmationId, acknowledgeSessions}`；原地切换，200 返回刷新后的 `Worktree`                                                                                                      |

删除预览的 `sessions` 是该 worktree 中运行中的会话，`archived` 是其中已归档（`closed` 或 `interrupted`）的会话，二者都是 `SessionInfo`。

删除成功后，`archived` 的记录（含强制删除时刚关闭的会话留下的）保留，并与释放空间一样标为已释放。

- `commit` 是确认时解析的 HEAD 完整 OID，以私有引用 `refs/webmux/released/<id>` 保住提交；记录的 `released` 保存原路径、提交与分支。
- 之后可经 `POST /api/hybrid-sessions/{id}/rebuild` 重建并恢复。
- DELETE 响应没有记录 ID，204 不变；客户端重读 `GET /api/sessions?state=archived`。
- `commit` 为空（HEAD 无法解析）时无法保存释放点：删除不被阻止，记录保持原样、不能重建。
- 执行时 HEAD 与确认不同则 409。
- 未锁定且目录已缺失的 linked `prunable` 工作树允许安全清理（`git worktree remove -- <path>`），并回退在主仓库校验已记录的 HEAD 提交以保留关联归档记录。
- 批量删除确认凭证与单项删除、释放空间隔离；`status` 为 `removed`、`skipped` 或 `failed`，单项失败不中断同批其余项。

切换分支（`switch-preview` / `switch`）沿用同样的两阶段确认，凭证只能用于切换：

- `target` 为 `{kind: "local", branch}`、`{kind: "remote", ref, name?}`（`ref` 如 `origin/fix/b`，新建本地分支 `name`，默认去掉远程名，并以 `ref` 为上游）、`{kind: "new", name, startPoint}`（不设置上游）或 `{kind: "detach", commit}`；名称不合法或以 `-` 开头时 400。
- 响应的 `target` 为 `{kind, branch, commit, subject, createsBranch, upstream}`：`branch` 是切换后所在分支（detach 为 `null`），`commit` 是解析出的完整 OID；`sessions` 是工作树及其子目录下运行中的会话，切换不会关闭它们。
- `blockers` 与 `warnings` 每项为 `{code, message, path?}`；有阻止原因时仍返回 200，`confirmationId` 为空、`expiresAt` 为 `null`。
  - 阻止：`changes`（已跟踪修改）、`operation`（进行中的合并、变基等）、`occupied`（`path` 为占用该分支的工作树）、`same`、`exists`（同名本地分支已存在）、`unverifiable`、`unsupported`（bare、不可用或待清理）；
  - 警告：`untracked`、`locked`、`submodules`、`sessions`。
- 执行时在仓库门闩下重新检查：工作树、分支或 HEAD、目标提交或会话集合变化时 409 并要求重新预览；有会话却未带 `acknowledgeSessions: true` 时同样拒绝。未跟踪文件会被覆盖时 409，消息列出至多 50 个路径。
- Git 执行不随客户端断开而中止，上限 3 分钟（超时 504）；钩子或过滤器失败时返回错误，消息带脱敏后的 Git 输出首行，工作树以刷新后的实际状态为准。

`push` 是 `commit` 的推送检查：`{state, branches, more}`。

- `state`：
  - `pushed`：至少一个远程跟踪引用包含该提交；`branches` 列出至多 5 个（如 `origin/main`），`more` 为其余数量；
  - `unpushed`：已配置远程，但没有任何远程跟踪引用包含它；
  - `noRemote`：未配置远程；
  - `unknown`：没有提交，或检查失败、超过 5 秒。
- 检查只读本地引用（`git for-each-ref --contains`），不联网、不 fetch，可能落后于远程。
- 它只用于警告，不阻止删除或释放。

`bases` 每项为 `{name, head, subject}`，`subject` 是引用指向的提交标题。浏览器用标题展示候选项，创建请求的 `base` 仍传引用或 OID。

- `defaultBase` 是主工作区的精确提交 OID，`defaultBaseSubject` 是对应标题；无默认提交时分别为 `null` 和空字符串。
- `defaultBranch` 是主工作区当前本地分支。detached HEAD、无提交或 bare 仓库为 `null`，不会根据相同 OID 猜测。

分支刷新仅接受已存在的本地分支及其已配置上游，不回退到同名 remote。

- 请求：30 秒非交互 `git fetch --no-tags`，不会切换或重置本地分支。
- 成功：`{branch, base, baseSubject, baseSource: "remote", upstream, fetchWarning: ""}`；仅当本地尖端是上游提交的祖先时，`base` 是本次上游跟踪引用的精确 OID。本地领先或分叉时保留本地尖端并说明。
- 降级：无上游、上游为本机仓库、fetch 失败或超时仍返回 200。`baseSource` 为 `"local"`，`base` 回退到所选本地分支，`fetchWarning` 给出非阻断说明。
- 安全：响应不包含 remote URL、命令行或原始 stderr；普通 GET 分支清单始终无网络行为。

### 同步详情字段

仓库快照与分支清单的同步字段全部在本机计算，不访问网络：

- 仓库：`lastFetchedAt` 为 `<common>/FETCH_HEAD` 与 `<common>/worktrees/*/FETCH_HEAD` 中最新的修改时间（近似值），从未获取时为 `null`。
- 工作树新增以下字段，`dirty`、`hasUntracked`、`hasIgnored` 的含义不变：
  - `upstream`：如 `origin/feat/x`，无上游或 detached 时为 `null`；`ahead`/`behind`：领先/落后提交数；`upstreamGone`：上游引用已不存在；
  - `lastCommit`：`{hash, subject, author, committedAt}`；`changes`：`{staged, unstaged, untracked, conflicted}`，状态读取失败时为 `null`；
  - `operation`：`merge`、`rebase`、`cherry-pick`、`revert`、`bisect` 或 `null`。
- 上游状态的判读：`upstream` 为 `null` 表示无上游；`upstreamGone` 为 true 时 `ahead`/`behind` 为 `null`；`upstream` 非空、未删除但 `ahead` 为 `null` 表示领先/落后未计算。
- `branches` 每项另有 `subject`、`committedAt`、`upstream`、`ahead`、`behind`、`upstreamGone`，以及 `mergedIntoDefault`（主工作区 HEAD 已包含该分支；bare 或主工作区尚无提交时为 false）；`bases` 每项另有 `kind`（`local` / `remote`）与 `committedAt`。

`remotes` 的 URL 在返回前脱敏，服务日志不记录原始 URL：带密码的 URL 只把密码换成 `***`；只有用户名的 `http`/`https` URL 整个用户名换成 `***`；`ssh://` 与 scp 风格地址原样返回；无法解析的 URL 丢弃 `@` 之前的部分。

### 分支操作

快进、推送与删除都先预览、后执行，凭证 5 分钟有效、只能使用一次，且按操作隔离（用另一操作的凭证执行返回 409）：

- 预览有阻止条件时仍返回 200，`confirmationId` 为空字符串、`expiresAt` 为 `null`；`blockers`、`warnings` 每项为 `{code, message, path?}`，`path` 是相关工作树（如占用分支的工作树）。
- `commits`、`unmerged` 为 `{total, items}`：`items` 最多 20 条 `{hash, subject, author, committedAt}`，`total` 是整个范围的提交数。
- 执行时复核预览绑定的分支与上游提交、推送目标和会话集合，任一变化返回 409 并要求重新预览；执行响应带重读后的 `repository`。
- 快进只用本地远程跟踪引用：分支被检出时在该工作树中 `merge --ff-only`，`sessions` 非空时需要 `acknowledgeSessions: true`，会话不会被关闭；未被检出时只移动分支引用。
- 推送按预览中的 `commit` 推送到 `remote` 的 `target`，不强制推送、不推送标签；`publish` 为 true 时成功后设为上游。`pushUrl` 已脱敏；`status` 与获取远程相同（`ok`、`auth`、`network`、`timeout`、`rejected`、`error`）。
- 删除的 `group` 为 `contained`（提交都能从其他引用到达）或 `unmerged`（`unmerged` 列出独有提交），后者只在 `acknowledgeUnmerged` 为 true 时删除，否则记为 `skipped`；结果 `status` 为 `deleted`、`skipped` 或 `failed`，`deleted` 的 `message` 带恢复命令 `git branch <名称> <OID>`。

错误码：400 输入错误，404 不存在，409 冲突或确认过期，503 Git / 清单不可用，504 超时，500 存储或非预期错误。JSON 结构见 `internal/contract/workspace.go`。

仓库快照带 `initInstruction`（未设置时为空字符串），保存在 `repositories.json` 的登记记录中；旧清单缺字段视为没有指令。它是一段模板：`{{名称}}` 在创建会话带 `useInitInstruction` 时展开为同名快捷指令的正文（见 [创建会话](http-session-create.md#首条提示与初始化指令)），其余文字原样发送，不会被执行。

### 获取远程

`POST /api/repositories/{id}/fetch` 按名称逐个执行 `git fetch --prune --recurse-submodules=no --end-of-options <remote>`（不加 `--no-tags`），不持有仓库门闩，不阻塞创建、删除、切换与快照读取：

- 时限：每个远程 60 秒，整体最多 3 分钟；客户端断开不会中断获取。同一仓库同时只获取一次，重复请求等待并得到同一结果。
- `results` 按远程名排序，每项 `status` 为 `ok`、`auth`、`network`、`timeout`、`rejected` 或 `error`；`ok` 时 `message` 为空，否则为中文说明加脱敏后的 Git 输出首行，最多 200 字符。
- 非交互：设置 `GIT_TERMINAL_PROMPT=0` 与 `SSH_ASKPASS_REQUIRE=never`；`GIT_SSH_COMMAND`、`core.sshCommand` 与 `GIT_SSH` 都未设置时才注入 `GIT_SSH_COMMAND="ssh -o BatchMode=yes"`。不修改 Git 或凭据配置。
- `repository` 是全部远程结束后重新读取的仓库快照（`lastFetchedAt` 与领先/落后随之更新）。服务日志只记录仓库 ID、远程名与分类，不记录 URL 与 Git 输出原文。

清理失效登记没有独立接口：Web 端与 `webmux worktree prune` 选出未锁定、目录已缺失的 `prunable` linked worktree，走上表的批量删除预览与执行，从不执行 `git worktree prune`。

## 从快速开始创建会话

快速开始没有独立接口：新建 worktree 或选择已有工作树、以聊天视图启动 agent、应用配置与权限模式并发送首条提示，都由一次 `POST /api/sessions` 完成。
新建 worktree 时请求体带 `newWorktree`；选择已有工作树时带 `workspace`（另有 `view`、`prompt` 等字段）。
请求与失败步骤、初始化指令展开、`POST /api/uploads` 与 `POST /api/launch-configs/{id}/options` 见 [创建会话](http-session-create.md)。

## 代码工作区

位于 `/api/sessions/{id}/code`，响应带 `Cache-Control: no-store`。

| 路由                                                       | 说明                                                                                              |
| ---------------------------------------------------------- | ------------------------------------------------------------------------------------------------- |
| `GET /context`                                             | 根目录与能力：`contextId`、`rootPath`、`browseAvailable`、`diffAvailable`、`writableAvailable` 等 |
| `GET /tree?contextId&nodeId&includeIgnored&cursor`         | 分页目录列表                                                                                      |
| `GET /files?contextId&q&includeIgnored`                    | 有界路径搜索                                                                                      |
| `GET /locate?contextId&path`                               | 解析用户输入的路径，返回根内 `nodeId`、`path`、`kind`（file / directory）                         |
| `GET /file?contextId&nodeId&source`                        | 文件快照，`source` 为 worktree / index / head                                                     |
| `GET /changes?contextId&cursor&q&nodeId&group`             | 分页 Git 修改列表                                                                                 |
| `GET /diff?contextId&changeId`                             | 冻结的新旧双侧原文                                                                                |
| `GET /snapshots/{snapshotId}/file?side=old\|new`           | 读取已捕获的比较侧全文                                                                            |
| `GET /snapshots/{snapshotId}/raw?side=old\|new&download=1` | 按快照返回原始字节（≤100 MiB）；Diff 快照必须指定 `side`；也接受 HEAD                             |
| `POST /check`                                              | 批量变化检测，返回 unchanged / updated / missing / expired / unknown                              |

`/locate` 的相对路径以本机 OSC 7 报告目录为基点，没有报告时用启动目录；按实际文件系统解析符号链接后，必须位于代码根内（403），远端报告返回 409。结果只是节点标识，读取仍走根目录的逐段 `O_NOFOLLOW` 检查。

文档（`document` / `old` / `new`）带 `mediaType`（未识别为空串）、`size`（字节数）与 `downloadable`；`kind` 为 text / binary / unsupported / too-large / symlink / image / pdf。

扩展名属于 png、jpg、jpeg、gif、webp、avif、bmp、ico 且文件头签名匹配时识别为图片，`mediaType` 取自签名；不超过 20 MiB 为 `image`，更大为 `too-large` 但仍可下载。`.svg` 为合法 UTF-8 且首个 XML 元素是 `svg` 时保持 `text` 并带 `image/svg+xml`。

扩展名为 `.pdf` 且前 1024 字节内出现 `%PDF-` 时为 `pdf`，`mediaType` 为 `application/pdf`；超过 20 MiB 为 `too-large` 并保留媒体类型。空文件或签名缺失时为 `binary` 并说明原因；其他扩展名的 PDF 内容仍为 `binary`。只读取文件头，不读入正文；Diff 中仍为 `metadata`。

`/tree` 与 `/files` 返回的节点带仅按扩展名推导的 `mediaType`。Diff 的 `kind` 增加 `image`（两侧均为图片，或一侧不存在；两侧都是存在的文本时仍为 `text`）。

`/raw` 不受 JSON 接口的 10 秒时限约束，返回与快照完全一致的字节：

- 工作树来源在发送前后校验文件身份、大小与修改时间；索引 / HEAD 按对象 ID 读取。
- 响应带正确的 `Content-Type`（未识别媒体类型时为 `application/octet-stream`）、`Content-Length`、`nosniff` 与 `no-store`。
- `Content-Disposition`：图片与 PDF 未带 `download=1` 时为 `inline`；其余为 `attachment` 并给出原文件名。
- SVG 与 PDF 另加 `Content-Security-Policy: default-src 'none'; style-src 'unsafe-inline'; sandbox`。
- 同时最多 4 个传输；发送中文件变化会中断连接。

错误码：400 非法路径、比较侧或跨会话快照，404 不存在，409 版本冲突或内容已变化，410 快照失效，413 超出限制，429 传输繁忙，504 取消或超时。

### 文件操作

资源管理器的文件操作作用于 `contextId` 所在的根（工作区或临时文件夹），不能跨根；文件内容、Git 索引与配置不受影响。

| 路由                                                  | 说明                                                                                      |
| ----------------------------------------------------- | ----------------------------------------------------------------------------------------- |
| `GET /node?contextId&nodeId`                          | 按树节点下载常规文件（≤100 MiB），`attachment` + 原文件名；也接受 HEAD                    |
| `GET /archive?contextId&nodeId&includeIgnored`        | 目录流式 ZIP，`application/zip`，无预知长度；未带 `includeIgnored=true` 时排除 Git 忽略项 |
| `GET /usage?contextId&nodeId`                         | `{entries, bytes, limited}`：节点及其下条目数与常规文件字节，用于删除确认                 |
| `POST /rename {contextId, nodeId, name}`              | 同目录改名，返回 `{nodes}`                                                                |
| `POST /mkdir {contextId, parentNodeId, name}`         | 新建文件夹，201 返回 `{nodes}`                                                            |
| `POST /copy {contextId, nodeIds, targetNodeId, mode}` | `mode` 为 `copy` / `move`，最多 100 项，返回按请求顺序的 `{nodes}`                        |
| `POST /upload?contextId&nodeId`                       | 单文件 multipart（字段 `file`，≤100 MiB）上传到目录，201 返回 `{nodes}`                   |
| `DELETE /node?contextId&nodeId&recursive=1`           | 删除；非空目录需 `recursive=1`，成功返回 204                                              |

- 名称只能是单个条目：拒绝空名、`.`、`..`、任意大小写的 `.git`、`/`、`\`、控制字符与超过 255 字节（400）。上传只取文件名的最后一段。
- 重名时自动取最小可用的 ` (n)` 后缀（放在扩展名前，`.env` 之类的前导点不算扩展名），从不覆盖已有条目；`{nodes}` 给出最终名称。
- 根目录不可重命名、移动或删除（403）；嵌套仓库边界与特殊文件不可修改，符号链接只能删除且不影响其目标（400）。
- 复制递归重建文件、目录与符号链接，跳过 `.git`、嵌套仓库与特殊文件；复制与 ZIP 先扫描，合计超过 100 MiB 或 10000 项返回 413。
- 不能粘贴到自身或其子目录（400）；移动是同根 `rename`，跨文件系统返回 409；移动到原目录时保持原位。
- 非空目录未带 `recursive=1`，或目录中含 `.git` / 嵌套仓库时返回 409，不会删除一部分后停止。
- 根不可写时 `writableAvailable` 为 false，写操作返回 403。上传与下载、ZIP 共用 4 个传输槽（429）；其余操作 60 秒超时。
