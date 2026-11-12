## Context

- 现在唯一的 fetch 在 `refresh.go`：执行 `fetch --prune --no-tags <remote>`，使用可覆盖的 `fetchTimeout`（默认 30s），不持有仓库门闩；失败时只返回一句泛化提示，不透出 Git 输出。
- `Service.git` 已设置 `GIT_TERMINAL_PROMPT=0` 和 `GCM_INTERACTIVE=never`，并在 ctx 取消时杀掉整个进程组。
- 仓库快照已有按仓库的 single-flight 机制，用引用计数管理等待者，最后一个等待者离开时取消读取（`registry.go` 中的 `snapshot`）。
- 批量删除已支持未锁定、目录确认不存在（`os.ErrNotExist`）的 `prunable` 条目，并通过 `retainRecords` 回退使用登记中的 `HEAD` 保护归档记录。
- 依赖 `enrich-repository-details` 提供的 `Remotes` 读取、URL 脱敏函数、`lastFetchedAt` 和 `RepositoryPanel` / `RepositoryWorktrees`。

## Goals / Non-Goals

**Goals:**

- 获取不会挂起、不会提示认证、不会泄露凭据，单个远程失败不影响其他远程。
- 清理失效登记完全复用批量删除的保护与结果语义，不新增服务端写路径。
- 访问远程的 Git 调用（本变更的获取，以及 `add-branch-sync-actions` 的推送）共用一套非交互执行、错误分类与脱敏，不各写一份。

**Non-Goals:**

- 定时后台获取、标签管理、子模块获取。
- 快进、推送与删除分支：由 `add-branch-sync-actions` 负责。

## Decisions

### 1. 逐个远程顺序获取，而不是 `fetch --all`

先用 `remote` 取得远程清单，再按名称排序，逐个执行：

```
fetch --prune --recurse-submodules=no --end-of-options <remote>
```

`fetch --all` 在任一远程失败时只返回一个总的退出码，无法给出逐远程结果，因此不用。

不并行执行，原因有二：多个 fetch 同时写 `FETCH_HEAD` 和 packed-refs 会互相争用锁；单用户场景下远程数量很少。

不加 `--no-tags`，保持用户 Git 配置的标签跟随行为，与 IDE 的“Fetch”操作一致。现有的起始分支刷新为了速度保留 `--no-tags`，两处的目的不同。

- 备选方案：沿用 `--no-tags`。这样用户在终端看到的标签会与 WebMux 获取的结果不一致，被否决。

### 2. 时限、取消与合并

- 每个远程使用 `fetchTimeout`（默认改为 60s，测试中仍可覆盖），整体上限为 3 分钟。
- 执行上下文使用 `context.WithoutCancel`：客户端断开时不杀掉 Git。这只排除“客户端断开”这一种不必要的中断；每个远程的超时仍会结束 Git 进程。Git 以锁文件加重命名逐个更新引用，被结束时每个引用要么是旧值要么是新值，`FETCH_HEAD` 可能不完整；该远程记为 `timeout`，用户重试即可。
- 每个仓库设一个 `fetchFlight`：已有获取在进行时，新请求直接等待同一个结果。它与快照的 single-flight 不同，最后一个等待者离开时也不取消，原因同上。
- 获取不持有仓库门闩，所以创建、删除、切换、快照都不被阻塞。并发写引用的冲突交给 Git 的锁处理，失败会体现在对应远程的结果中。
- 全部远程结束后，用 `context.Background()` 和 `readTimeout` 重读一次快照，并把快照放进响应 `{results: [{remote, status, message}], repository}`，前端不必再发一次请求。

### 3. 共用的远程调用：非交互执行、错误分类与脱敏

新增 `Service.remoteGit(ctx, cwd, args...)`，在 `Service.git` 之上为单次调用追加环境变量，并把失败转成 `RemoteFailure{status, message}`。获取和 `add-branch-sync-actions` 的推送都只经由它访问远程。

**错误分类。** 按 `LC_ALL=C` 下 Git 的固定英文输出做关键字分类：

| 关键字 | 分类 |
|---|---|
| `Authentication failed`、`could not read Username`、`Permission denied (publickey)` | `auth` |
| `Host key verification failed` | `auth`，说明为“主机密钥未受信任，请先在终端连接一次并确认指纹” |
| `Could not resolve host`、`Connection refused`、`Network is unreachable` | `network` |
| ctx 超时 | `timeout` |
| `rejected`、`does not appear to be a git repository` | `rejected` |
| 其他 | `error` |

`message` 由固定的中文说明加脱敏后的 Git 输出第一行组成，最多 200 字符，使用 `enrich-repository-details` 中同一个 URL 脱敏函数，额外替换 `https?://[^\s]+` 中的 userinfo。

日志中只记录仓库 ID、远程名和分类，不记录 URL 和 Git 输出原文。

**SSH 的非交互。** 服务进程没有终端，ssh 仍可能等待主机指纹确认或口令，直到超时。Git 选择 ssh 命令的优先级是 `GIT_SSH_COMMAND` 环境变量 > `core.sshCommand` 配置 > `GIT_SSH` 环境变量 > 默认 `ssh`（本机 Git 2.47.3 用 `ls-remote` 加三个探针脚本实测）。处理如下：

- 远程调用额外设置 `SSH_ASKPASS_REQUIRE=never`；
- 只有 `GIT_SSH_COMMAND`、`core.sshCommand`（用 `config --get core.sshCommand` 读取）和 `GIT_SSH` 三者都未设置时，才设置 `GIT_SSH_COMMAND="ssh -o BatchMode=yes"`；
- 任何一项已设置时保持原样：注入的 `GIT_SSH_COMMAND` 会覆盖后两者，从而替换掉用户自定义的 ssh 程序或包装脚本。

- 备选方案：完全不改 ssh 环境。ssh 可能在没有终端的情况下等到超时，与规格中“立即以认证失败结束”冲突，被否决。
- 备选方案：只检查 `GIT_SSH_COMMAND` 和 `core.sshCommand`。设置了 `GIT_SSH`（如 plink 或包装脚本）的用户会被注入的命令覆盖，被否决。

### 4. 清理失效登记只是前端的快捷入口

`RepositoryWorktrees` 用 `worktree-batch.ts` 中的现有分类筛出 `prunable && !locked && kind === "linked"`，并且这些条目被归为“可安全清理”。目录是否已缺失由服务端在批量预览时用 `os.Lstat` 复核，前端的计数只用于展示。点击后设置选择集合并直接进入预览阶段。

CLI 的 `worktree prune` 在本地按同样的条件筛选，然后调用同一个批量预览和执行接口。

- 备选方案：服务端新增执行 `git worktree prune` 的接口。`prune` 一次处理全部可清理条目，不经过归档记录保留，也无法给出逐项结果，被否决。

### 5. 入口位置

“获取远程”作用于整个仓库，因此放在仓库面板的标题栏，在各分区之间常驻，不在某个分区里重复放置。按钮带图标与文字（tabler `cloud-download`），窄屏只显示图标并保留 `aria-label`/`title`。逐远程结果以 `InlineNotice` 显示在标题栏下方，直到关闭面板或再次获取。

## Risks / Trade-offs

- [风险] `BatchMode=yes` 会让依赖 ssh-agent 之外交互方式的用户无法获取 → 这正是非交互的本意；结果归为“认证失败”，并提示在终端执行。用户自己设置的 `GIT_SSH_COMMAND` 保持不变。
- [风险] 远程很多或很慢时，整体 3 分钟的上限仍可能不够 → 超过上限的远程标为 `timeout`；已完成的远程结果照常返回。
- [风险] 获取与外部 `git fetch`/`gc` 同时运行，引用锁冲突 → 该远程报告错误，用户重试即可，不做自动重试。
- [取舍] 获取期间面板的领先/落后仍是旧数据 → 按钮显示进行中，完成后一次性刷新，不做流式更新。

## Migration Plan

先归档 `enrich-repository-details`，再实现本变更；本变更与 `switch-worktree-branch` 互不依赖，可以并行实施。没有持久化变化，回滚只需还原代码。
