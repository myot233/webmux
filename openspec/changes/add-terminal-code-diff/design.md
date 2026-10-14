# Design

## Context

动机见 [proposal.md](proposal.md)。本设计基于 `c3b46cd` 的实际实现：

- `Terminal.svelte` 只有一个终端容器，包含上传、粘贴、全屏、选择菜单与移动软键；`styles.css` 使用固定 viewport 高度和 flex 布局。
- `useTerminal` 在 `onMount` 创建 xterm，接收快照/续流，使用 ResizeObserver fit 并发送尺寸；`paste(text)` 当前返回 void，无法让调用方区分连接不可用。`ws.ts` 的 `send` 可返回本地连接是否可发送，但没有远端接收确认。
- `internal/workspace` 已提供系统 Git argv、超时、取消进程组、目录规范化、工作树识别和注册清单。现有 `git()` 将 stdout/stderr 写入无界 bytes.Buffer，不能直接用来承接大 diff。
- `workspace.Service` 显式持有 session manager；注册存储故障与 Git 可用性是不同能力。`SessionInfo.cwd` 是不可变的启动目录，`workspace` 可空，终端内 `cd` 不受跟踪。
- API/WS 沿用认证与 Origin 中间件；前端 JSON 合同统一由 Zod 验证。现有终端二进制帧不需要扩展。
- `web/test/e2e` 有真实临时 HOME、Git、PTY 和动态端口夹具；Tabs 尚未生成，已有 Button、ScrollArea 等 shadcn 组件。

### Draft Defaults

用户已明确要求桌面右侧、移动标签、diff 与快速发送位置。本草案采用：未提交修改三组、统一 diff、1024px 布局断点、文件/单行/连续范围、插入不回车、按事件与手动刷新。以上已在起草时向用户呈现，作为 draft 评审默认值，不记为用户逐项确认；分支对比另行扩展。

## Goals / Non-Goals

**Goals:**

- Git 审阅通过独立 REST 读取，不进入 PTY 读写锁或状态探测循环。
- 保持一个终端实例，通过布局和显隐改变可见区域，不通过卸载组件模拟标签切换。
- 从服务端结构化 diff 获得双方行号，避免前端按展示行偏移推算位置。
- 只保留短期前端结果和目标身份，不引入 diff 数据库或 agent 对话管理。

**Non-Goals:**

- 不提供编辑器、代码执行、暂存/回退/提交、分支基线比较、全文搜索或文件树。
- 不从运行进程 cwd、OSC 标题或 agent 专属协议推断仓库。
- 不承诺外部 Git/文件写入下的事务快照或消息到达 agent 的确认；不增加自动重发队列。
- 不改造 app/runtime 架构，也不更改原有工作树删除的会话协调规则。

## Decisions

### 1. 在 workspace 服务内增加按会话定位的只读读取

API 先取得会话信息，在锁外从真实启动目录解析 Git 顶层路径和 Git/common directory。主工作区、linked worktree、未注册 Git 仓库都适用，仓库清单故障不作为读取前提。Git 不存在、普通目录或目录失效返回明确 capability 状态。

首次读取为该会话绑定实际根目录与 Git 管理目录身份，包括规范路径及文件系统身份；后续重新解析和检查，不仅比较路径字符串。已有 workspace 关联时验证与关联根一致。外部删除/替换后绑定不自动改变。该少量会话级绑定在会话移除时释放，限制缓存并发和数量；无需持久化。

不要求用户显式注册：它会阻碍现有直接启动路径。也不使用主仓库 anchor 作为 diff cwd：linked worktree 的工作区和索引各自独立。

### 2. 两个只读 API 和完整的结构化合同

| 接口                                                   | 成功响应和目标                                                                                                |
| ------------------------------------------------------ | ------------------------------------------------------------------------------------------------------------- |
| `GET /api/sessions/{id}/changes`                       | `{availability, reason, rootPath, contextId, checkedAt, files}`；可用时 files 按来源分组、路径稳定排序        |
| `GET /api/sessions/{id}/diff?contextId=...&fileId=...` | `{contextId, fileId, checkedAt, contentVersion, kind, reason, oldSource, newSource, oldPath, newPath, hunks}` |

- `availability` 为 `available / not-git / git-unavailable / unavailable`；非 available 时 rootPath/contextId 可空、files 为空，并提供原因。真正 Git 读取失败使用现有 JSON error；404 表示会话不存在，400 表示非法参数，409 表示上下文/条目已改变，413 表示修改列表超限，504 表示读取超时。
- 文件条目包含 opaque `fileId`、`group: unstaged/staged/untracked`、`status: added/modified/deleted/renamed/copied/type-changed/unmerged`、可空 oldPath/newPath（工作树相对路径）。同一路径不同 group 有不同 fileId。
- fileId 保存原始路径字节和来源的稳定编码，仅作为标识；服务端解码后必须匹配当前会话工作树重新读取的变化条目，不能作为任意文件读取授权。contextId 来源于会话和绑定的目录身份。拒绝绝对/越界路径及不属于当前列表的标识。
- diff 的 kind 为 `text / binary / metadata / unmerged / too-large / unsupported`；非 text 提供原因，hunks 为空，文件级引用仍可用。非法 UTF-8 路径采用可辨识转义显示且禁用发送；不使用 JSON 损失替换后的路径读取文件。
- 文本块包含 oldStart/oldCount/newStart/newCount 和 lines；每行包含 `kind: context/addition/deletion/no-newline`、text、可空 oldLine/newLine。代码内容以普通字符串呈现，控制字符显示为转义，不使用 HTML 或 ANSI 解释器。
- oldSource/newSource 明确为 `head/index/worktree/empty`；contentVersion 为比较来源、路径、元数据与完整展示内容的摘要。列表读取和文件读取不宣称属于同一文件系统事务。
- 扩展 `schemas.ts` 与 `api.ts`；按需为共享 request/api 增加 AbortSignal，继续使用统一 401 行为，不另建认证传输。

不直接返回一份巨大 patch：分组列表加单文件读取允许局部故障、取消和限流，也避免浏览器解析带特殊路径的 patch 头。

### 3. 让 Git 负责比较，让有界读取负责资源保护

使用 NUL 分隔的 Git status/raw/name-status 信息定位路径、双方对象和模式。未暂存使用索引到工作区，已暂存使用 HEAD 到索引；unborn HEAD 采用空树语义。保留重命名两侧路径，unmerged 独立提示，子模块第一版只展示摘要。

文本 diff 使用固定参数关闭颜色、external diff 和 textconv（`--no-color --no-ext-diff --no-textconv`）、统一 3 行上下文；路径经 `--` 与 literal pathspec 传入，避免 Git pathspec 魔法。对象内容通过服务端获得的 OID 读取，不拼接用户给定 revision。按已知单文件解析 hunk 头、行前缀与无末尾换行标记，不从 patch 路径头反推文件身份。

未跟踪文本在服务端按空内容到新内容生成单个新增块，保留真实行号/末尾换行。拒绝 device/FIFO，符号链接通过 lstat/readlink 读取链接文本；常规文件采用从绑定根逐段禁止符号链接跟随的有界打开，避免路径检查后再打开的越界竞争。所有目标路径均验证在绑定工作树内。

新读取路径沿用现有进程组取消和 WaitDelay 逻辑，但使用有界 stdout/stderr；不扩大现有 git() 的无界缓存风险。第一版设定：一次请求最长 10 秒，修改列表最多 1000 条且机器输出最多 2 MiB，单文件 patch/内容最多 1 MiB、文本最多 10000 行，错误摘要最多 8 KiB。触达上限立即终止并回收 Git 进程组，返回受限状态；不将任意截断 patch 当成完整 diff。最多两个并发 Git 读取，等待也计入超时。

不运行文件 watcher 或固定轮询。首次可见、重新显示、前台恢复、连接恢复和手动刷新按相同入口取消/合并在途读取；读取完成后显示时间。agent 持续写入时用户可以手动刷新，不能由 PTY 高频输出触发 Git。

### 4. 终端实例常驻，响应式布局改变可见性

`Terminal.svelte` 持有唯一 useTerminal；新增独立 diff 面板与状态模块。宽度至少 1024px 时左终端、右面板，右面板默认约 40% 宽、可折叠；第一版无需拖拽分隔线。窄屏使用 shadcn Tabs，默认终端；长代码在 Diff 自身水平滚动，文件列表在自身区域滚动。整个页面继续适配 VisualViewport、安全区和全屏。

终端宿主保持挂载。useTerminal 增加可见性/有效尺寸处理：隐藏时暂停 fit/resize，仍接收输出；重新显示后等待布局完成，再 fit 并发送有效尺寸。不得通过新建 xterm 或重连来恢复标签。软键栏、终端选择浮层和拖放覆盖只出现在终端区域/标签；Diff 不抢终端键盘焦点。

state 以会话 ID、contextId、group/fileId 和请求序号隔离。切换文件/会话取消前一请求，页面离开取消所有 Diff 请求；旧响应和错误均不能写入新选择。布局变化保留仍有效的文件与滚动状态，成功刷新只有 contentVersion 不变时才保留行选择。

### 5. 生成版本准确的引用，复用终端粘贴通道

选择状态保存 fileId、contentVersion、side 和连续行范围。context 行默认新侧，用户可以明确选择旧侧；仅对存在行号的侧允许扩选。点击另一侧/文件开启新选择；连续范围不可跨越未展示的折叠间隔，避免误把不相连的可见行当成完整选区。手机用行号按钮选择起点，再用明确的“设为终点”动作扩选，另有清除和文件级引用动作。

引用预览和最终插入共享一个格式化函数：

| 选择            | 示例文本                                           |
| --------------- | -------------------------------------------------- |
| 未暂存新侧范围  | `/repo/src/main.go:12-18 `                         |
| 未暂存旧侧      | `/repo/src/main.go:12-18（索引旧版本） `           |
| 已暂存新侧      | `/repo/src/main.go:12-18（索引版本） `             |
| 已暂存旧侧/删除 | `/repo/src/main.go:12-18（HEAD 旧版本） `          |
| 文件级          | `/repo/src/main.go `，存在版本歧义时附相同来源说明 |

绝对路径由服务器验证的 root 和该侧路径构造，适用于子目录启动。含空格、引号、冒号歧义、反斜杠或控制字符的路径使用 JSON 字符串转义的引号形式，例如 `"/repo/a b.go":12`；禁止原样 CR/LF/ESC/NUL。此文本用于指出位置，不包装为 shell 命令。发送不包含代码块或自动生成的修复要求。

useTerminal 提供可观测的 paste 结果：检查已初始化、连接 connected、非 restoring、未退出，再调用 xterm paste 以保留 bracketed paste；复用 WebSocket send 的本地布尔结果，不能只凭 void paste 假装成功。所有终端输入仍经原有队列；不在 backend 新增“send-to-agent”接口，不依赖 preset 名称或第三方 SDK。

不追加回车、不清空输入、不自动重发。成功仅提示“已插入终端”；没有传输确认时不提示 agent 已收到。桌面保持终端可继续输入但不强制改变焦点；手机成功切回终端并 fit，不主动 focus 唤起键盘。失败保留 Diff 标签及选择供用户重试。后台外部写入造成的行移动仅在刷新时检测；引用明确针对当前展示快照，不承诺原子精确定位。

## Risks / Trade-offs

- [agent 写文件期间 diff 与行位置变化] → 有读取时间、contentVersion、乱序保护和刷新后选择失效；不宣称实时事务快照。
- [隐藏终端造成零尺寸或恢复后缺输出] → 终端实例保持挂载，单独处理可见性、非零 resize 和续流；使用真实 PTY 输出回归。
- [大 patch、二进制、Git 配置导致读取阻塞] → 读取时限/字节/行数/并发限制，禁用 external diff/textconv，局部降级。
- [特殊路径或旧侧行号误导 agent] → 服务端结构化双方路径/行号，引用预览、可逆转义和明确来源标签。
- [同一工作树多个 agent 同时修改] → 展示工作树共享修改，不声称按 agent 归属过滤；页面始终显示根路径。
- [Git 是多进程读取而非快照] → 变化条目重验证；消失/来源变化返回冲突后刷新，不自动读另一个文件。
- [位置发送到 Shell 或 agent 正忙] → 始终只粘贴文本，由用户选择时机并提交；不从等待启发式自动执行。

## Migration Plan

1. 先补充只读后端能力、数据合同和隔离测试，再增加响应式 Diff UI 与位置发送。
2. 保持原 REST/WS 合同、配置与部署模板兼容，不新增持久化数据；有 Git 即可启用，无 Git 只降级面板。
3. 验证 Linux/macOS 的真实 Git/PTY、桌面/Pixel 7 浏览器和原终端回归后更新产品文档。
4. 回滚到旧二进制即可恢复原终端页，Git 工作区/索引不需迁移；本草案 PR 不包含实施代码。
