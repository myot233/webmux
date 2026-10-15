# WebMux 产品与技术设计文档

Oct 13, 2026 · @RainChan

> 实施约定（2026-10-14）：前端采用 Svelte 5、shadcn-svelte、TypeScript、Vite；使用 Zod 校验数据边界，pnpm 管理依赖。使用 Git 提交阶段成果，Go / 前端分别用 gofmt / Prettier 格式化。本文已同步仓库 / worktree 管理及分组首页；终端二进制帧协议不变。

## 1. 项目概述

WebMux 是一个运行在单台 Linux 或 macOS 机器上的 Go 单二进制 Web 终端服务，用于在浏览器（尤其是手机和平板）里创建、查看、操作多个持久化终端会话，典型用途是远程遥控 Claude Code、Codex CLI 等命令行 agent。

**核心问题**：ttyd 一类工具在浏览器断开后会话即终止，手机锁屏、切网、关浏览器都会丢掉正在运行的 agent。WebMux 把 PTY 的生命周期与浏览器连接解耦：会话由服务端持有，浏览器只是“附着”上去，随时断开、随时重连，重连后能看到断线期间的全部输出。

**目标**

- 会话在浏览器断开后继续存活，重连后恢复完整滚动缓冲
- 欢迎页展示所有会话及其标题、工作目录、状态（运行中 / 等待输入 / 空闲 / 已退出）
- 新建会话时可选工作目录和预设启动命令（如 `claude`、`codex`、`omp`），也可输入自定义命令
- 手机 / 平板浏览器上可用：响应式布局、虚拟键盘友好、必要的软键（Esc、Ctrl、方向键、Tab 等）
- 单二进制部署，前端静态资源嵌入，Linux `systemd` 或 macOS `launchd` 一条用户服务即可运行

**非目标（本版本明确不做）**

- 不做多机中心网关，每台机器各自部署、各自访问
- 不做多用户 / 权限体系，以启动服务的系统用户身份运行，单用户
- 不做服务进程重启后的会话存活（守护进程分离架构），重启即丢会话
- 不做文件编辑保存、任意历史版本浏览、全库内容搜索或推送通知；文件上传与只读代码分屏已实现
- 不依赖系统 tmux，也不与已有 tmux 会话互通

**部署与安全前提**：服务只监听 Tailscale / WireGuard 内网地址，认证仅为一个可选的共享 token，不暴露公网。

## 2. 需求与约束

以下决策已在需求访谈中确认，实现时不要偏离。

| 维度       | 决策                                              | 对实现的影响                                                |
| ---------- | ------------------------------------------------- | ----------------------------------------------------------- |
| 会话持久化 | 自研 PTY 管理器，不依赖 tmux                      | 服务进程内用 `creack/pty` fork 子进程，自己维护环形滚动缓冲 |
| 部署形态   | 单机，每台 Linux / macOS 独立部署                 | 无主从、无注册中心，配置极简                                |
| 网络与认证 | 走 Tailscale / WireGuard 内网                     | 默认只绑定内网地址；可选共享 token，无账号体系              |
| 技术栈     | Go 单二进制，前端嵌入                             | `go:embed` 打包前端产物，`go build` 即得部署物              |
| 运行身份   | 当前系统用户，单用户                              | 不做 setuid / 用户切换，不做鉴权细粒度                      |
| 服务重启   | 会话随服务退出，仓库清单保留                      | 仅注册清单落盘；PTY、缓冲和会话列表仅在内存中               |
| 欢迎页     | 仓库标题 → 工作树卡片 → 会话列表，仅仓库折叠      | Git 快照与实时会话分离，直接目录启动不依赖 Git              |
| 会话 title | 优先终端 OSC 0/2 设置的 title，没有则用工作目录名 | 服务端解析 PTY 输出流中的 OSC 序列                          |
| 新建会话   | 选工作目录 + 预设或自定义启动命令                 | 预设列表来自配置文件，默认含 shell、claude、codex、omp      |
| 会话状态   | 运行中 / 等待输入 / 空闲 / 已退出                 | 基于输出活跃度 + 前台进程启发式判断                         |
| 移动端     | 手机 / 平板浏览器可用                             | 响应式布局、软键栏、触摸滚动、粘贴                          |

**约束与假设**

- 目标系统为 Linux 与 macOS（amd64 / arm64），不考虑 Windows
- 同时存在的会话数量级为十几个，不做大规模优化
- 单个浏览器标签可同时打开一个会话；同一会话允许多个浏览器同时附着（多端同看）
- 滚动缓冲默认保留 10,000 行（可配置），超出后丢弃最旧行
- 不做录像回放，只保证重连时能回放当前缓冲

## 3. 产品设计

整个产品只有两个页面：欢迎页（会话列表）和终端页。所有路由在前端处理，后端只提供 API 和静态资源。

### 3.1 欢迎页 `/`

顶部保留品牌、连接状态、主题与“新建会话”，提供“设置”“添加仓库”和“刷新 Git 状态”。正文按注册顺序展示无边框仓库分组标题，仓库内主工作区优先，其余工作树按规范路径排序；组内会话按最近活跃时间排序，输出更新不重排上层分组。仓库通过 shadcn Collapsible 折叠，工作树 / 目录是唯一一层 Card，内部使用无边框 Item 会话列表与 Separator，不再嵌套卡片。标题保留运行 / 等待 / 退出数量。空组仍保留操作入口；未注册会话按 cwd 展示在“直接会话”，原工作树失效的会话在仓库内“已不可用工作区”保留，每个会话只出现一次。桌面单列展示工作树，手机将会话状态和活动信息换到标题下方；路径完整换行，输出预览观察窗的高度跟随本地设置。

每条会话显示：

- **标题**：OSC title，没有则工作目录名（basename）
- **状态点 + 文字**：运行中（绿色，有输出流动）、等待输入（黄色，2 秒无输出且光标停在提示符）、空闲（灰色，30 秒以上无输出）、已退出（红色，显示退出码）
- **最近活跃时间**（相对时间，如“3 分钟前”）
- **输出预览**：默认跳过当前终端屏幕末尾一条非空行，再向上显示十条非空行；独立背景、等宽字体、高度跟随设定行数。空屏幕显示等待提示，内容不足时保留空余高度，内容全部被跳过时显示隐藏提示。光标移动、擦行和重绘更新对应屏幕内容，不把旧状态栏当作新增输出行。长行横向裁切、不再次换行，不增加嵌套卡片边框；明暗主题均与卡片背景区分。

`/settings` 设置页提供“显示行数”（1–50）和“跳过末尾行数”（0–50），默认分别为 10 和 1；0 表示保留末尾内容。按当前屏幕非空行顺序先排除末尾行，再取剩余内容的最后 N 行，不访问滚动历史。效果示例随有效输入即时更新，支持恢复默认。设置经过 Zod 验证后自动保存到当前浏览器 localStorage，刷新后保留；无效输入不覆盖已有值。设置不修改原始终端数据、不影响服务端状态探测，也不跨设备同步。

会话行交互：

- 会话行最小高度 56px，链接点击区域覆盖整行及上下留白（独立菜单除外）→ 进入终端页；键盘焦点框覆盖整行，保留链接的新标签页打开行为
- 行末独立菜单 → 关闭会话或移除已退出会话，均通过 AlertDialog 二次确认；打开菜单不会触发进入终端
- 已退出的会话保留在列表中直到用户手动移除，方便查看最后输出

列表通过 WebSocket `/ws/sessions` 实时更新，不轮询。

工作树标题展示分支或 detached 提交、主工作区 / bare / locked / 缺失标记、完整换行路径，以及修改 / 未跟踪文件状态；未知不能显示干净。卡片不显示“忽略文件”或“共享目录 · N 个独立终端”标签，但保留运行 / 等待 / 退出数量；同一目录中的终端进程独立，文件仍共享。删除预览仍检查忽略文件并保留风险提示与确认流程。Git 状态附读取时间，在挂载、可见、重连、显式刷新和操作后通过 REST 更新，旧响应不能覆盖新状态，单仓库错误不丢弃会话。

### 3.2 新建会话弹层

从欢迎页点击“新建会话”弹出底部抽屉（移动端）或居中对话框（桌面端），包含：

1. **工作目录**：文本输入框，默认值为上一次使用的目录，支持 `~` 展开；旁边列出最近使用过的 5 个目录快捷选项。浏览按钮打开路径选择器：输入前缀过滤，↑↓ 选择，Enter / Tab 进入子目录，Alt+↑ 返回上级，支持返回主目录；点击“选择当前目录”确认。完整目录列表可滚动，普通目录优先，点目录可切换显示，输入点目录前缀时直接显示匹配结果。
2. **启动命令**：单选芯片，来自配置文件 `presets`，默认提供 `shell`（用户默认 shell）、`claude`、`codex`、`omp`；选中后显示实际命令行，允许按配置追加参数。另有“自定义”输入完整命令，支持引号和转义，保存上次输入。直接执行 argv，不隐式求值 shell 语法；管道、变量等须显式使用 `bash -lc '命令'`。
3. **会话名称**（可选）：留空则自动 title
4. 确认按钮：创建成功后直接跳转终端页

启动位置提供三种模式：全局默认“直接目录”；工作树上下文预选“已有工作树”；仓库上下文预选“新工作树”。创建表单分开选择新分支 / 已有本地分支、明确基线和完整目标路径。新分支预填可修改的 `wt/<形容词>-<名词>`，不在服务端静默改名。默认位于仓库旁的 `<basename>.worktrees/<分支最后一段>`。可仅创建工作树；“创建并进入”先创建工作树再启动命令，后者失败保留前者，重试不重复创建。不自动 fetch、复制环境文件或安装依赖。同一工作树可启动多个独立 PTY，启动 cwd 可为其子目录，不追踪终端后续 cd。

已有本地分支选择器使用 shadcn `Command` + `ScrollArea`，直接显示在表单内。搜索按分支名进行不区分大小写的连续子串匹配，不将近似分支名当作匹配项；搜索框固定，列表限高滚动并提供可拖动滚动条。点击或 ↑↓ / Enter 提交选择，显示勾选及当前已选分支；搜索、悬停和键盘高亮不改变已提交选择，也不直接提交创建表单。占用分支显示原因并禁选，创建处理中禁用选择。选择更新默认目标路径，但保留用户手动修改的路径。

“从列表移除仓库”只修改登记，不关闭会话或删除文件 / 分支。“删除 worktree”默认保护修改、未跟踪 / 忽略文件和活动会话；强制删除进入独立确认页，展示完整路径、分支保留、会话列表、外部进程及全部内容丢失风险，要求完整路径输入和未预选的关闭授权。主工作区、bare、locked、缺失 / 未知状态及含子模块 / 嵌套仓库的目标不能通过页面删除；关闭会话永不触发工作树删除。

目录不存在时提示“目录不存在，是否创建？”，用户确认后 `mkdir -p`。

### 3.3 终端页 `/s/:id`

布局自上而下：

- **顶栏**（高度 44px）：返回按钮、会话 title、状态点、连接状态图标（已连接 / 重连中）、全屏 / 退出全屏按钮、上传按钮、右侧菜单（复制全部缓冲、清屏、关闭会话、字体大小）
- **上传进度**：仅上传期间显示在顶栏下方，包含文件名、百分比、进度条与取消按钮；完成或失败后消失，不占用常驻终端高度。
- **终端区**：xterm.js，占满剩余空间；横屏和竖屏自动根据容器尺寸 fit，并把新 cols/rows 发回服务端 `SIGWINCH`
- **软键栏**（仅触摸设备显示，始终保持单行）：按使用频率优先排列 `⌨`（唤起/收起系统键盘）、`粘贴`、`/`、`@`，随后为 `Esc` `Tab` `Ctrl` `Alt` `↑` `↓` `←` `→` `Home` `End` `PgUp` `PgDn`。`Ctrl` 和 `Alt` 为粘滞键：点一下后下一个按键带修饰符，再点取消
- 常用组合单独给按钮：`Ctrl+C`、`Ctrl+D`、`Ctrl+Z`、`Ctrl+L`、`Ctrl+R`，一行放不下时横向滚动

终端页交互细节：

- 进入页面先回放服务端缓冲，回放期间显示“正在恢复…”遮罩，回放完成后滚动到底部
- WebSocket 断开后自动重连（指数退避 0.5s→8s），重连后再次回放整个缓冲并清屏重绘，保证一致
- 单指滑动通过 xterm 标准滚轮路径处理：普通终端滚动历史，全屏应用遵循启用的鼠标报告或光标键模式。普通历史滚动不发送应用输入；全屏面板的滚轮由应用消费。
- 双指捏合调整字体大小（12–20px），设置存在 localStorage
- 浏览器标签页标题同步显示会话 title
- 上传按钮选择单文件，终端区域支持文件拖入；桌面原生文件粘贴、剪贴板图片读取复用同一上传流程，普通文本粘贴不被转换为文件。多个拖入 / 粘贴文件按顺序处理，上传期间不接受另一批文件。
- 上传成功后通过 xterm paste 将 shell 引号保护的绝对路径输入终端，不添加回车。终端若已断开，提示已上传路径但不假装发送成功；离开页面或点击取消会终止请求。
- 文件在服务器系统临时目录的独立 `webmux-upload-*` 子目录中保存，目录 0700、文件 0600；失败或取消清理未完成文件，成功文件由用户或系统临时目录策略清理，不自动随会话关闭删除。
- 页面隐藏（切后台）时暂停渲染但保持 WebSocket；iOS 杀掉连接后回到前台会触发重连

### 3.4 状态判定规则（用户可见的定义）

| 状态     | 判定                                                                                                                                                                                                           |
| -------- | -------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| 运行中   | 最近 2 秒内有输出                                                                                                                                                                                              |
| 等待输入 | 2 秒以上无输出，且前台进程树没有运行中的成员、叶子进程都在睡眠（Linux 读 `/proc/<pid>/stat`，macOS 读 `sysctl` 进程表与任务运行线程数），或输出末尾匹配提示符特征（行尾为 `$ `、`> `、`❯ `、`? `、`(y/n)` 等） |
| 空闲     | 30 秒以上无输出且不满足“等待输入”特征                                                                                                                                                                          |
| 已退出   | 子进程已退出，记录退出码                                                                                                                                                                                       |

“等待输入”是 agent 场景最关心的状态：Claude Code 请求权限、问是否继续时都会停在这里。启发式可能误判，UI 文案用“可能在等你”而非断言。

## 4. 技术架构

架构只有三层：浏览器、WebMux 进程、子进程。WebMux 进程内的 Session Manager 持有所有 Session，每个 Session 拥有一个 PTY、一个环形缓冲、一个 OSC 解析器和一个状态探测器。浏览器通过两条 WebSocket 分别订阅会话列表和单个会话的字节流，断开只影响浏览器自己，不影响 Session。

&#91;embedded content: WebMux 架构 · 浏览器 / Go 进程 / 子进程三层\]

浏览器断线后 Session 继续从 PTY 读取并写入环形缓冲；重连时服务端先发缓冲快照再接实时流，因此手机锁屏期间 agent 的输出不会丢。

### 4.1 技术选型

| 层          | 选型                                                                                      | 理由                                                 |
| ----------- | ----------------------------------------------------------------------------------------- | ---------------------------------------------------- |
| 语言        | Go 1.23+                                                                                  | websocket 依赖要求；单二进制、交叉编译               |
| PTY         | `github.com/creack/pty`                                                                   | 事实标准，支持 `Setsize`、`StartWithSize`            |
| HTTP / 路由 | 标准库 `net/http` + `http.ServeMux`（1.22 新路由语法）                                    | 零依赖，够用                                         |
| WebSocket   | `github.com/coder/websocket`（原 nhooyr）                                                 | 纯 Go、支持 context、二进制帧                        |
| 前端框架    | Svelte + TypeScript + Vite                                                                | 体积小、手机加载快                                   |
| UI 与校验   | shadcn-svelte + Tailwind CSS + Zod                                                        | 复用可访问组件，校验 HTTP / WebSocket / 本地存储数据 |
| 终端渲染    | `@xterm/xterm` 5.x + `@xterm/addon-fit` + `@xterm/addon-webgl`（失败回退内置 DOM 渲染器） | 完整终端仿真与 GPU 加速                              |
| 嵌入        | `go:embed web/dist`                                                                       | 一个二进制包含前端                                   |
| 配置        | 命令行参数 + 可选 TOML 文件（`github.com/BurntSushi/toml`）                               | 单用户不需要复杂配置                                 |
| 日志        | 标准库 `log/slog`                                                                         | 结构化日志，`journalctl` 直接可读                    |
| 构建        | `Makefile`：`make web` → `make build`                                                     | 前端产物进入 `web/dist` 再编译 Go                    |

### 4.2 关键设计原则

- **Session 与连接解耦**：Session 的生命周期只由子进程决定，WebSocket 连接是观察者，可以为零个、一个或多个
- **服务端只传字节，不解释终端**：PTY 原始字节直接进缓冲、直接转发给 xterm.js，服务端只“旁路嗅探” OSC 序列和活跃度，不做终端仿真
- **重连 = 清屏 + 全量回放 + 续流**：不做增量 diff，简单且一致；缓冲上限控制回放成本（10,000 行约几百 KB）
- **单进程、会话内存态**：无数据库；PTY、缓冲与会话列表重启即清空。显式注册的仓库清单写入独立状态目录，Git 自己持有工作树及分支状态；配置文件只读
- **尺寸由最后附着的客户端决定**：多端同看时，最近一次发送 resize 的客户端尺寸生效（和 tmux 的默认策略不同，但对手机 + 桌面交替使用更友好）

## 5. 后端详细设计

后端分为 `internal/session`（PTY 与缓冲）、`internal/hub`（WebSocket 分发）、`internal/api`（HTTP 路由）、`internal/config` 与 `internal/workspace`（持久仓库清单、Git 快照和目录操作协调）。`cmd/webmux` 显式构造并注入 workspace 服务；session 包不依赖 Git，Git 不进入 PTY 读写或周期状态探测。

### 5.1 Session 数据结构

```go
type Status string
const (
    StatusRunning Status = "running"
    StatusWaiting Status = "waiting"   // 可能在等输入
    StatusIdle    Status = "idle"
    StatusExited  Status = "exited"
)

type Session struct {
    ID        string    // 8 位随机 base32，URL 安全
    Cmd       []string  // 实际 exec 的 argv
    Preset    string    // 预设名，如 "claude"
    Cwd       string
    CreatedAt time.Time

    mu        sync.RWMutex
    pty       *os.File
    proc      *os.Process
    cols, rows uint16
    title     string     // OSC 解析结果，空则用 filepath.Base(Cwd)
    userTitle string     // 预留：手动改名
    lastOut   time.Time  // 最近一次有输出的时间
    preview   Preview    // 只读终端屏幕状态；底部最多 100 条非空行，末行供状态探测使用
    status    Status
    exitCode  *int

    ring      *RingBuffer          // 原始字节环形缓冲
    subs      map[*Subscriber]struct{} // 附着的 WebSocket
    done      chan struct{}
}
```

### 5.2 创建与运行

1. `Manager.Create(cwd, preset, cmd, extraArgs, name)`：校验 `cwd` 存在且是目录，后端展开 `~` 并用 `filepath.Abs` 规范化。
2. 组装命令：预设在配置中定义为 argv 数组，`extraArgs` 追加其后；`shell` 预设取 `$SHELL`，为空则 `/bin/bash`，以登录交互方式启动（`-l -i` 可配置）。`preset = "custom"` 时直接使用非空 `cmd` argv，不接受非空 `extraArgs`；普通预设不接受 `cmd`。配置预设不能使用保留名称 `custom`。
3. 环境变量：继承服务进程环境，并显式设置 `TERM=xterm-256color`、`COLORTERM=truecolor`、`LANG`（继承）、`WEBMUX_SESSION_ID=<id>`
4. `pty.StartWithSize(cmd, &pty.Winsize{Rows: 24, Cols: 80})` 启动；初始尺寸随后由第一个附着的客户端 resize 覆盖
5. 启动三个 goroutine：
   - **reader**：循环 `pty.Read` 到 32 KB 临时缓冲 → 写入 ring → 喂给 OSC 扫描器和屏幕 Preview 提取器 → 更新 `lastOut` → 广播给所有 `subs`（非阻塞，慢客户端用带缓冲 channel，满则断开该客户端让它重连回放）
   - **waiter**：`proc.Wait()` 返回后设置 `exitCode`、`status = exited`、关闭 pty、关闭 `done`，广播状态变更
   - **status ticker**：每 1 秒评估一次状态（见 5.5），变化时广播到 `/ws/sessions`
6. `Manager.Close(id)`：先发 `SIGHUP` 给进程组（`syscall.Kill(-pid, SIGHUP)`，启动时设置 `Setsid: true`），5 秒后仍未退出则 `SIGKILL`
7. `Manager.Remove(id)`：只允许移除已退出的会话，从 map 中删除

### 5.3 环形缓冲 RingBuffer

按**字节**而非按行实现，避免解析 ANSI 的复杂度：

- 固定容量，默认 2 MB（约等于 10,000 行 × 200 字节的上限，配置项 `scrollback_bytes`）
- `Write(p []byte)`：写满后覆盖最旧字节
- `Snapshot() []byte`：返回从最旧到最新的连续副本
- 回放时在快照前缀发送 `\x1b[H\x1b[2J`（清屏归位）由前端处理；快照可能从一个转义序列中间开始，xterm.js 对此容忍良好，首行偶尔乱码可接受
- 每次 `Write` 都加锁，`Snapshot` 加读锁；写入频率不高（PTY 读取已经聚合）

**为什么不按行存**：agent 输出大量使用光标移动和重绘（spinner、进度条、Claude Code 的 TUI），按行截断会破坏序列；按字节回放再由 xterm.js 重新解释是最稳的方案。

### 5.4 OSC title 解析

在 reader goroutine 里对每个 chunk 做增量状态机扫描，不影响转发：

- 匹配 `ESC ] 0 ; <text> BEL` 或 `ESC ] 2 ; <text> BEL`，终止符也接受 `ESC \`（ST）
- 跨 chunk 的序列要能拼接：状态机保存“已进入 OSC、累积到的文本”，最大累积 512 字节，超出则放弃
- 解析成功 → `title = text`（去首尾空白），与旧值不同时广播状态变更
- `Title()` 方法返回优先级：`userTitle` > `title` > `filepath.Base(Cwd)`

Claude Code 和多数 shell 配置（oh-my-zsh、starship）都会设置 OSC 0/2 title，所以列表里通常能看到 `✳ claude` 或 `user@host: ~/proj` 这样的名字。

### 5.5 状态探测

每秒评估，输入为 `lastOut`、进程状态、`lastLine`：

```
if exited → exited
else if now - lastOut < 2s → running
else if looksLikePrompt(lastLine) || foregroundBlockedOnRead(pid) → waiting
else if now - lastOut > 30s → idle
else → waiting   // 2–30s 无输出且判断不出来，倾向提示用户看一眼
```

- `looksLikePrompt`：去除 ANSI 后的最后一行，去尾空格后以 `$`、`#`、`%`、`>`、`❯`、`›`、`?`、`:` 结尾，或包含 `(y/n)`、`[Y/n]`、`Yes`/`No` 选项、`Press Enter` 等模式；模式列表放在配置里可扩展
- `foregroundBlockedOnRead`：取每秒一次的进程表（Linux 为 `/proc/*/stat`，macOS 为 `sysctl kern.proc.all` 加 `proc_pidinfo` 的运行线程数），若整棵进程树（按 ppid 遍历）里没有 `R` 状态进程且叶子进程为 `S`，视为阻塞；其他平台此函数返回 false
- 状态只是展示用，不驱动任何自动化动作，误判无害

### 5.6 可配置输出预览

`Preview` 使用固定版本的 `github.com/0magnet/xterm-go/vt` 只读跟踪终端屏幕，初始为 80×24，成功调整 PTY 尺寸后同步 resize。处理跨 chunk 的 UTF-8、光标寻址、回车、擦行/清屏、滚动、宽字符以及主/备用屏幕切换；不按收到的换行片段累计历史。仅从当前屏幕底部提取最多 100 条非空行，每行最多 120 个 Unicode 字符，以换行连接为不可变 `preview` 字符串，供浏览器按本地行数与末尾偏移裁剪；保留缩进，不截断 UTF-8 或组合字符单元。重复的实际屏幕行仍分别保留，清空屏幕后预览和提示符探测文本同时清空。状态探测始终读取真实底部单行，不受浏览器隐藏末尾行的设置影响。

预览屏幕不保留额外 scrollback，不注册向 PTY 发送数据的回调；设备查询应仅由真实浏览器终端回应。屏幕状态由 `Session.mu` 保护，不增加连接或后台 goroutine。原始 PTY 字节仍直接进入 ring 和 xterm，预览侧解析不会改写终端帧或回放。构建需要 Go 1.24+。

### 5.7 Hub 与 Subscriber

- `Subscriber` 封装一个 WebSocket 连接 + `out chan []byte`（容量 256）
- 附着流程：`hub.Attach(sessionID, conn)` → 发送快照（一帧二进制，前缀见协议节）→ 加入 `subs` → 之后 reader 广播的每个 chunk 都写入 `out`
- 写 goroutine 把 `out` 的数据合并成尽量大的帧发送（最多聚合 50 ms 或 64 KB），减少手机端帧数
- 读 goroutine 处理客户端上行：输入字节直接 `pty.Write`，resize 消息调用 `pty.Setsize` 并记录
- 连接关闭或写超时（10 s）→ 从 `subs` 移除；Session 不受影响
- `/ws/sessions` 的订阅者维护在 Manager 一级，收到的是 JSON 事件

### 5.8 并发与资源

- Manager 持有 `map[string]*Session` + `sync.RWMutex`
- 每个 Session 固定 3 个 goroutine + 每个订阅者 2 个 goroutine，20 个会话 × 2 端约 150 个 goroutine，可忽略
- 服务收到 `SIGTERM` 时：对所有 Session 执行 `Close`，等待最多 10 秒，再退出；会话与缓冲不持久化，已保存的仓库清单不受影响
- 单个会话内存上限 ≈ 2 MB 缓冲 + 订阅者 channel，总内存约 50 MB 量级

### 5.9 仓库清单与目录协调

- `state_dir/repositories.json` 为版本化注册清单，保存稳定 ID、common directory、anchor 和名称，目录 `0700` / 文件 `0600`，同目录临时文件同步后原子替换，写失败不发布成功。进程文件锁防止同目录多实例覆盖；损坏文件不按空清单覆盖。
- Git 为可选系统依赖；缺失 Git、状态目录故障或个别仓库读取失败不阻止普通终端服务。仓库由 common directory 归一化去重，不扫描、clone、init、自动 repair 或修改 safe.directory。
- Git 使用 argv、NUL 机器输出和禁用交互环境；读操作最多 10 秒，写操作最多 30 秒，关闭会话与强制删除共享写期限。分支通过 Git 校验，基线解析为本地提交 OID 后执行。创建 / 删除失败或取消后刷新实际状态，不递归删除目录或删除残留分支回滚。
- 所有 API 会话创建经统一协调层登记规范化 cwd；删除预留阻止目标 / 子目录 / 符号链接别名的新启动，并等在途启动结束。仓库写操作串行，Git 和进程等待不持有 session 内部锁。
- 删除预览保存 5 分钟单次 confirmationId，绑定目录和 Git 管理身份、分支 / HEAD、锁及会话集合，执行前重新验证；变化时在关闭进程前拒绝。强制删除等待关联 Session.Done（子进程清理与 PTY 排空），再次验证后执行单次 `git worktree remove --force`。分支、退出记录与回放保留，错误如实说明部分结果，无双重 force、自动解锁或递归删除兜底。
- 仅保护已知启动目录，外部进程、终端内 cd 和外部 Git 竞态不受服务内锁控制；不承诺文件系统事务或进程沙箱。仓库必须可信，Git 可运行其配置、hooks 和 filters。

## 6. 通信协议

所有 API 以 `/api` 为前缀，WebSocket 以 `/ws` 为前缀，其余路径返回嵌入的前端 `index.html`（SPA 回退）。启用 token 时，HTTP 请求带 `Authorization: Bearer <token>` 或 Cookie `webmux_token`，WebSocket 通过同名 Cookie 或 query `?token=` 认证。

### 6.1 HTTP API

| 方法       | 路径                                                            | 说明                         | 请求 / 响应                                                                                                                                                         |
| ---------- | --------------------------------------------------------------- | ---------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| GET        | `/api/sessions`                                                 | 列出会话                     | 响应 `SessionInfo[]`，按 `lastActiveAt` 倒序                                                                                                                        |
| POST       | `/api/sessions`                                                 | 创建会话                     | 预设请求 `{cwd, preset, args?: string[], name?}`；自定义请求 `{cwd, preset: "custom", cmd: string[], name?}`；201 返回 `SessionInfo`；400 目录 / 命令错误或字段冲突 |
| GET        | `/api/sessions/{id}`                                            | 单个会话信息                 | `SessionInfo`；404                                                                                                                                                  |
| DELETE     | `/api/sessions/{id}`                                            | 关闭会话（SIGHUP → SIGKILL） | 204                                                                                                                                                                 |
| DELETE     | `/api/sessions/{id}?remove=1`                                   | 从列表移除已退出会话         | 204；409 会话仍在运行                                                                                                                                               |
| GET        | `/api/sessions/{id}/buffer`                                     | 下载完整缓冲（原始字节）     | `application/octet-stream`，用于“复制全部”                                                                                                                          |
| POST       | `/api/sessions/{id}/upload`                                     | 上传文件到服务器临时目录     | 恰好一个 multipart 文件字段 `file`；单文件最多 100 MiB，期限 5 分钟；201 `{path}`；400 无效请求，404 会话不存在，409 已退出，413 超限，408 读超时                   |
| GET        | `/api/presets`                                                  | 预设列表                     | `Preset[]`                                                                                                                                                          |
| GET        | `/api/fs/dirs?prefix=/home/u/pro`                               | 目录补全                     | 全部匹配子目录的绝对路径（包含符号链接目录），普通目录优先、各组按名称排序；无静默截断                                                                              |
| POST       | `/api/fs/mkdir`                                                 | 创建目录                     | `{path}`；204                                                                                                                                                       |
| GET        | `/api/health`                                                   | 健康检查                     | `{ok: true, sessions: n}`                                                                                                                                           |
| GET        | `/api/config`                                                   | 可选 workspace 能力          | `{defaultCwd, workspace: {gitAvailable, registryAvailable, gitError, registryError}}`；不包含 token                                                                 |
| GET / POST | `/api/repositories`                                             | 列表 / 注册                  | GET 返回逐仓库快照与局部错误；POST `{path}`，新增 201、重复 200                                                                                                     |
| DELETE     | `/api/repositories/{id}`                                        | 仅移除登记                   | 204，不关闭会话、不删除目录或分支                                                                                                                                   |
| GET        | `/api/repositories/{id}/worktrees`                              | 新鲜仓库快照                 | Repository 含 `checkedAt` 与 `worktrees`                                                                                                                            |
| GET        | `/api/repositories/{id}/branches`                               | 本地分支与基线               | `{branches: [{name, head, worktreePaths}], bases: [{name, head}], defaultBase, defaultParent}`，不 fetch                                                            |
| POST       | `/api/repositories/{id}/worktrees`                              | 创建工作树                   | `{path, mode:"new", branch, base}` 或 `{path, mode:"existing", branch}`；201 Worktree                                                                               |
| POST       | `/api/repositories/{id}/worktrees/{worktreeId}/removal-preview` | 只读删除预览                 | 请求 `{}`；响应 `{confirmationId, expiresAt, worktree, sessions, blockers, forceAllowed}`                                                                           |
| POST       | `/api/repositories/{id}/worktrees/{worktreeId}/remove`          | 单次确认删除                 | `{confirmationId, mode:"safe"}`；force 另要求 `confirmPath` 与 `closeSessions:true`；成功 204                                                                       |

会话创建保留原 cwd 合同，可增加 `workspace: {repositoryId, worktreeId}`；省略 cwd 时使用工作树根，提供 cwd 时必须属于同一工作树，允许真实子目录。明确目标无效时不回退默认目录。新接口错误仍为 `{error}`：400 输入、404 不存在、409 冲突 / 过期确认、503 Git / 清单不可用、504 超时、500 存储或非预期错误；沿用 401 / 403 认证与 Origin 边界。完整 JSON 形状由 `internal/workspace/types.go` 和 `web/src/schemas.ts` 同步维护。

```ts
interface SessionInfo {
  id: string;
  title: string; // 已按优先级算好
  cwd: string;
  preset: string; // "shell" | "claude" | "codex" | "omp" | "custom" | 配置中的预设名称
  cmd: string[];
  status: "running" | "waiting" | "idle" | "exited";
  exitCode: number | null;
  preview: string; // 当前屏幕底部最多 100 条非空行，以 \n 分隔；每行最多 120 个 Unicode 字符
  createdAt: string; // RFC3339
  lastActiveAt: string;
  cols: number;
  rows: number;
  clients: number; // 当前附着的浏览器数
  workspace: {
    repositoryId: string;
    worktreeId: string;
    worktreePath: string;
  } | null;
}

interface Preset {
  name: string; // "claude"
  label: string; // "Claude Code"
  cmd: string[]; // ["claude"]
  allowArgs: boolean;
}
```

### 6.2 WebSocket `/ws/sessions`（会话列表）

文本帧，JSON。连接建立后服务端先发一条 `snapshot`，之后推送增量事件：

```json
{"type":"snapshot","sessions":[SessionInfo, ...]}
{"type":"created","session":SessionInfo}
{"type":"updated","session":SessionInfo}   // 状态、title、preview、clients 变化
{"type":"removed","id":"abc123"}
```

`updated` 事件节流：同一会话 500 ms 内最多发一次，`preview` 的变化单独再节流到 2 s。客户端不需要发任何消息，可定期发 ping 帧保活。

### 6.3 WebSocket `/ws/s/{id}`（终端流）

服务端 → 客户端，**二进制帧**，第一个字节是类型：

| 首字节 | 含义 | 载荷                                                                         |
| ------ | ---- | ---------------------------------------------------------------------------- |
| `0x00` | 输出 | PTY 原始字节                                                                 |
| `0x01` | 快照 | 附着时发一次，载荷为整个 ring 内容；客户端收到应先 `terminal.reset()` 再写入 |
| `0x02` | 状态 | JSON `SessionInfo`（UTF-8），用于顶栏标题与状态点                            |
| `0x03` | 退出 | JSON `{"exitCode": n}`，之后服务端关闭连接                                   |

客户端 → 服务端，同样**二进制帧**，第一个字节是类型：

| 首字节 | 含义     | 载荷                                                         |
| ------ | -------- | ------------------------------------------------------------ |
| `0x00` | 输入     | 要写入 PTY 的字节（xterm.js `onData` 的字符串按 UTF-8 编码） |
| `0x01` | resize   | JSON `{"cols": n, "rows": n}`                                |
| `0x02` | 重放请求 | 空；服务端回一帧 `0x01` 快照（前端重连或主动“重新同步”时用） |

采用二进制帧而非 JSON 包裹输出，是为了避免 base64 开销和 UTF-8 截断问题（PTY 输出可能在多字节字符中间切断，二进制帧原样透传给 xterm.js 的 `write(Uint8Array)` 可正确处理）。

### 6.4 错误与关闭码

- 认证失败：HTTP 401；WebSocket 握手阶段直接 401，不升级
- 会话不存在：HTTP 404；WebSocket 关闭码 `4404`
- 会话已退出时附着：仍允许，发快照和 `0x03` 退出帧后关闭，关闭码 `4410`
- 服务端写超时或客户端 channel 满：关闭码 `4408`，前端应立即重连并清屏回放

## 7. 前端详细设计

前端是一个 Svelte 5 + TypeScript 的 SPA，使用 shadcn-svelte 组件与 Zod 数据校验，用 Vite 构建，产物放到 `web/dist` 由 Go 嵌入。终端页按路由懒加载；总包体目标 < 400 KB gzip，首屏在 4G 网络 1 秒内可交互。

### 7.1 页面与路由

用 `history` 与 `popstate` 管理页面路由：

- `/` → `SessionListPage`
- `/s/:id` → `TerminalPage`
- `/settings` → `Settings`

`index.html` 需要：`<meta name="viewport" content="width=device-width, initial-scale=1, viewport-fit=cover, interactive-widget=resizes-content">`，`interactive-widget=resizes-content` 让 Android Chrome 弹出键盘时缩小视口而不是遮挡终端；iOS 用 `visualViewport` API 监听处理（见 7.4）。加 `manifest.webmanifest` 和 `apple-mobile-web-app-capable`，用户可“添加到主屏幕”全屏运行。

### 7.2 SessionListPage

- 挂载时读取 `/api/presets`、`/api/config`、`/api/repositories` 并连接 `/ws/sessions`，首个会话 snapshot 到达前显示骨架屏；Git 快照加载独立于会话流
- 会话列表保存在 Svelte `$state` 数组中，事件经 Zod discriminated union 校验后按 id 更新
- `WorkspaceGroup` 使用单层 shadcn Card 展示工作树 / 目录，内部 `SessionRow` 使用无边框 Item 和 Separator：会话标题、状态点（CSS 类 `status-running` 等，运行中带呼吸动画）、退出码、连接设备数、相对时间（每 30 秒刷新一次）、按本地设置裁剪的 `preview` 终端观察窗。观察窗使用 `pre` 保留逻辑换行，独立背景，高度跟随设定行数，长行裁切且不撑宽移动页面；正文、标题和控件使用自托管 Inter 与 Noto Sans SC，路径和终端输出使用 Maple Mono NF CN。
- 仓库标题使用 Collapsible，工作树和会话不再二次折叠；稳定分组、局部错误与空工作树行为见 3.1。只有既无仓库又无会话时显示 shadcn Empty 整页空状态，无需先登记仓库即可直接启动。
- `NewSessionSheet`：移动端使用 shadcn Drawer，桌面端使用 Dialog；字段见产品设计 3.2。`DirectoryPicker` 使用 Popover + Command + ScrollArea，仅打开时按路径输入 `debounce 150ms` 请求 `/api/fs/dirs`，支持键盘导航、确认当前目录、隐藏目录切换、加载与错误状态；浏览中的路径与表单工作目录分离。输入、列表与确认区保持间距，目录区域限制最大高度并使用固定可见、可拖动的滚动条，路径和确认区不随目录列表滚动。最近目录存 localStorage `webmux.recentDirs`（最多 10 个），自定义命令存 `webmux.customCommand`。
- WebSocket 断开时顶部显示黄色细条“连接已断开，重连中…”，自动重连

### 7.3 TerminalPage

核心是 `hooks/terminal.svelte.ts` 的 `useTerminal`，使用 Svelte runes 与 `onMount` 管理 xterm 实例和 WebSocket 生命周期：

```ts
const term = new Terminal({
  fontFamily: '"Maple Mono NF CN", sans-serif',
  fontSize: settings.fontSize, // 默认 14，手机 13
  lineHeight: 1.15,
  scrollback: 10000,
  cursorBlink: true,
  allowProposedApi: true,
  theme: darkTheme, // 跟随 prefers-color-scheme
  macOptionIsMeta: true,
});
term.loadAddon(fitAddon);
try {
  term.loadAddon(new WebglAddon());
} catch {
  /* 保留 xterm 5 内置 DOM 渲染器 */
}
```

WebGL 渲染需要浏览器报告的实际像素尺寸与 CSS 尺寸 × DPR 一致。容器观察器发现差异超过 1 个物理像素时，释放 WebGL addon 并 `term.refresh()`，让 xterm 内置 DOM 渲染器显示现有缓冲；不 reset、不重连、不改变输入协议。忽略隐藏容器的零尺寸与缺少实际像素 API 的情况。正常尺寸继续使用 WebGL，上下文丢失也走同一回退路径。监听 resolution media query；DPR 改变时重新观察容器，以新测量判断一致性，避免使用旧 DPR 的尺寸。组件卸载时清理观察器、media query listener 与 fit 定时器。

消息处理：

- 收到 `0x01` 快照：`term.reset()` → `term.write(payload)` → 写入完成回调里 `term.scrollToBottom()`，隐藏“正在恢复”遮罩
- 收到 `0x00` 输出：`term.write(payload)`，注意 `write` 接受 `Uint8Array`
- 收到 `0x02` 状态：更新顶栏 title、状态点、`document.title`
- 收到 `0x03` 退出：显示“进程已退出（code n）”横幅，提供“返回列表”和“移除会话”按钮，终端保留可滚动
- `term.onData(s)` → 发送 `0x00` + `TextEncoder.encode(s)`
- `term.onResize({cols, rows})` → 发送 `0x01` + JSON
- `ResizeObserver` 监听容器的实际像素尺寸，先检查 WebGL 尺寸一致性，再 `fitAddon.fit()`（debounce 100 ms）

重连策略：`close` 事件后按 500 ms × 2^n（上限 8 s）重试，4408 慢客户端立即重连。回到前台主动重新附着，即使旧 socket 仍显示 OPEN，也不信任可能已失效的连接。收到快照时 reset 并回放。

### 7.4 移动端适配

这是产品能否真正用起来的关键，按以下规则实现：

- **布局**：`height: 100dvh` 的 flex 列：顶栏 44px、终端 `flex: 1; min-height: 0`、单行软键栏约 49px（含 36px 按键、边框及内边距）；safe-area 用 `env(safe-area-inset-*)` 补 padding
- **全屏**：顶部菜单旁通过 `document.documentElement.requestFullscreen({navigationUI: "hide"})` 请求浏览器原生全屏，使用 `document.exitFullscreen()` 退出；对整个文档请求，确保 body 下的菜单 / Dialog portals 仍可见。监听 `fullscreenchange` 同步状态，阻止重复请求，离开终端页退出本页开启的全屏。平台不支持或拒绝请求时给出明确提示，不用 CSS 伪装成功；原生浏览器底栏能否隐藏由浏览器决定，可尝试主屏幕 standalone 模式。
- **键盘弹出**：监听 `window.visualViewport` 的 `resize`，把根容器高度设为 `visualViewport.height`，并 `fit()`；iOS Safari 弹键盘时页面可能被整体推上去，需要在 resize 回调里 `window.scrollTo(0,0)`
- **输入法**：xterm.js 自带的隐藏 textarea 在手机上行为正常，但要设置 `term.textarea.setAttribute('autocapitalize','off')`、`autocorrect=off`、`spellcheck=false`，否则 iOS 会自动大写首字母、把 `--` 换成破折号
- **唤起键盘**：点击终端区域 `term.focus()`；软键栏的 `⌨` 按钮在已聚焦时 `term.blur()` 收起键盘
- **软键栏**：键盘按钮置于单行最左侧，随后为粘贴、`/` 和 `@` 快捷输入。全部按键共用一行横向滚动区域，不提供收起按钮。按钮 `onPointerDown` 时 `preventDefault()` 防止终端失焦导致键盘收起；`/`、`@` 作为普通字符发送，控制字节：`Esc`=`\x1b`，`Tab`=`\t`，方向键=`\x1b[A/B/C/D`，`Home`=`\x1b[H`，`End`=`\x1b[F`，`PgUp`=`\x1b[5~`，`PgDn`=`\x1b[6~`；`Ctrl` 粘滞后下一个字母 `c` 发送 `c.charCodeAt(0) & 0x1f`；`Alt` 粘滞后发送 `\x1b` + 字母
- **粘贴**：`navigator.clipboard.readText()` 需要 HTTPS 或 localhost，Tailscale 下常是 HTTP，因此粘贴按钮回退方案是弹出一个 textarea 让用户粘贴后点“发送”；多行内容用 bracketed paste（`\x1b[200~` … `\x1b[201~`）包裹，Claude Code 能正确识别
- **滚动**：仅在终端区域设置 `touch-action: none`，避免浏览器原生滚动接管手势。用 Pointer Events 跟踪触摸指针，并捕获到稳定容器，避免 DOM 渲染器替换起始行节点后丢失后续事件。按实际行高累计单指移动，将整行位移转换成标准 `WheelEvent`，由 xterm 决定历史滚动、鼠标协议或备用屏光标键输入；滚轮坐标固定为手势起点。阻止 xterm 原生 TouchEvent 重复处理。抬起、取消、丢失捕获及卸载时释放状态与捕获；滑动结束不聚焦键盘，轻点可聚焦。
- **字体缩放**：两条触摸指针移动时按距离变化调整字体（12–20），由 `setFontSize()` 更新、fit 并存 localStorage；指针数量变化时重新记录距离、位置及位移余数，避免双指结束后剩余单指造成滚屏跳动。
- **横屏**：无特殊处理，`fit()` 自动适应；软键栏仍保持单行，按键横向滚动
- **桌面端**：默认隐藏软键栏，仅 `@media (pointer: coarse)` 显示；`Ctrl+Shift+C/V` 复制粘贴，其余与普通终端一致

### 7.5 主题与样式

- 跟随 `prefers-color-scheme`，深色为默认优化目标（手机夜间使用）
- xterm 主题取 GitHub Dark / Light 两套色板，与页面 CSS 变量同源
- 复用官方 shadcn-svelte Button、Card、Input、Dialog、Drawer、AlertDialog、DropdownMenu、Popover、Command、ScrollArea、Slider 等组件，不重新实现基础控件。自定义 CSS 只负责主题、终端布局和触摸行为。

### 7.6 前端状态与存储

只用 localStorage 存：`webmux.fontSize`、`webmux.recentDirs`、`webmux.lastPreset`、`webmux.token`。用 Zod 校验读取值，异常设置回到有效默认值；HTTP 响应和 WebSocket JSON 也通过 Zod 校验，TypeScript 类型从 schema 推导。页面状态使用 Svelte runes，不引入额外全局状态库。

## 8. 配置、安全与部署

### 8.1 配置

命令行参数优先于配置文件，配置文件默认路径 `~/.config/webmux/config.toml`，不存在则全部用默认值。

```toml
# ~/.config/webmux/config.toml
listen = "100.64.0.5:7681"        # 默认 "127.0.0.1:7681"；填 Tailscale IP 或 "0.0.0.0:7681"
token = ""                        # 空 = 不认证；非空则所有请求必须携带
scrollback_bytes = 2097152        # 每会话环形缓冲，2 MB
default_cwd = "~"                 # 新建会话默认目录
state_dir = "~/.local/state/webmux" # 可省略；默认优先 $XDG_STATE_HOME/webmux，仅保存仓库清单
shell = ""                        # 空 = $SHELL
idle_after_seconds = 30
prompt_patterns = ['\$ ?$', '❯ ?$', '> ?$', '\(y/n\)', '\[Y/n\]', 'Press Enter']

[[presets]]
name = "shell"
label = "Shell"
cmd = []                          # 空 = 使用 shell 配置
allow_args = false

[[presets]]
name = "claude"
label = "Claude Code"
cmd = ["claude"]
allow_args = true

[[presets]]
name = "codex"
label = "Codex CLI"
cmd = ["codex"]
allow_args = true

[[presets]]
name = "omp"
label = "omp"
cmd = ["omp"]
allow_args = true
```

命令行：`webmux --listen 0.0.0.0:7681 --token xxx --config path.toml`，另有 `webmux version`。

### 8.2 安全边界

这是一个以当前用户身份执行任意命令的服务，等价于把 shell 暴露在网络上。安全完全依赖网络层隔离：

- **默认只绑 `127.0.0.1`**，用户必须显式改为 Tailscale 地址；绑定 `0.0.0.0` 且 `token` 为空时启动打印醒目警告（不拒绝启动，但日志 WARN）
- `token` 比较用 `crypto/subtle.ConstantTimeCompare`
- WebSocket 握手校验 `Origin`：必须与 `Host` 同源，或在配置 `allowed_origins` 中；防止其他网站页面通过浏览器发起连接
- `/api/fs/dirs` 和 `mkdir` 只在当前用户可访问的路径上操作，不做额外沙箱（本来就是本人 shell）
- 不记录终端输入输出到日志；日志只含会话创建 / 退出 / 连接事件
- 响应头加 `X-Content-Type-Options: nosniff`、`Referrer-Policy: no-referrer`
- 不做 HTTPS：Tailscale 已加密；若用户要 HTTPS，让他们前置 Caddy 或用 `tailscale serve`

### 8.3 构建

```makefile
web:            # 构建前端
	pnpm --dir web install --frozen-lockfile
	pnpm --dir web build

build: web      # 构建 Go 二进制，嵌入 web/dist
	CGO_ENABLED=0 go build -trimpath -ldflags "-s -w -X main.version=$(VERSION)" -o bin/webmux ./cmd/webmux

release:        # 交叉编译 linux/amd64、linux/arm64、darwin/amd64、darwin/arm64
	GOOS=linux GOARCH=amd64 ... && GOOS=linux GOARCH=arm64 ... && GOOS=darwin GOARCH=amd64 ... && GOOS=darwin GOARCH=arm64 ...
```

`web/dist` 加入 `.gitignore`，仅跟踪 `.gitkeep`；`go:embed all:dist` 支持未构建时的 Go 检查，但此时页面返回 503。Vite 每次清空旧产物，`web/public/.gitkeep` 随静态资源复制，防止丢失占位文件或将旧前端打入二进制。

### 8.4 部署（systemd / launchd 用户服务）

```ini
# ~/.config/systemd/user/webmux.service
[Unit]
Description=WebMux web terminal
After=network-online.target tailscaled.service

[Service]
ExecStart=%h/.local/bin/webmux --config %h/.config/webmux/config.toml
Restart=on-failure
Environment=PATH=%h/.local/bin:%h/.npm-global/bin:/usr/local/bin:/usr/bin:/bin
KillMode=mixed
TimeoutStopSec=15

[Install]
WantedBy=default.target
```

```bash
systemctl --user daemon-reload
systemctl --user enable --now webmux
loginctl enable-linger $USER     # 让用户服务在未登录时也运行
```

`Environment=PATH` 必须包含 `claude` / `codex` 的安装位置，否则预设命令找不到；文档里要明确提示用户检查 `which claude`。

macOS 使用 launchd 用户代理 `deploy/com.webmux.plist`（`~/Library/LaunchAgents/`）：launchd 不展开 `~`，路径写绝对路径；`EnvironmentVariables` 同样要包含 agent 所在目录，并设置 `LANG` 为 UTF-8 locale；`ExitTimeOut` 给 WebMux 处理 SIGTERM、清理会话的时间。

### 8.5 手机访问

手机装 Tailscale 并登录同一账号 → 浏览器打开 `http://<机器 Tailscale 名或 IP>:7681` → 启用 token 时输入一次 → “添加到主屏幕”。iOS 上 Tailscale 作为 VPN 运行，Safari 直接访问 100.x.x.x 地址即可。

## 9. 项目结构与实施计划

### 9.1 目录结构

```text
webmux/
├── cmd/webmux/                 # CLI、配置优先级、HTTP 服务、信号退出
├── internal/
│   ├── config/config.go        # TOML、默认值、校验
│   ├── session/                # PTY、缓冲、OSC、状态、进程树清理及测试
│   ├── hub/hub.go              # 串行 WS 写入、合并、输入与取消
│   └── api/                   # REST、WS、认证、Origin、静态资源及测试
├── web/
│   ├── embed.go               # go:embed all:dist
│   ├── index.html
│   ├── public/                # manifest、图标、构建占位文件
│   ├── src/
│   │   ├── main.ts
│   │   ├── App.svelte
│   │   ├── api.ts
│   │   ├── schemas.ts         # Zod 数据契约和推导类型
│   │   ├── ws.ts
│   │   ├── pages/             # SessionList.svelte、Terminal.svelte
│   │   ├── components/        # 工作树卡片、会话列表行、创建/关闭弹层、软键业务组件
│   │   ├── lib/components/ui/ # 官方 shadcn-svelte 生成组件
│   │   ├── hooks/             # terminal.svelte.ts、viewport.svelte.ts
│   │   └── styles.css
│   ├── components.json
│   ├── package.json
│   ├── pnpm-lock.yaml
│   ├── svelte.config.js
│   └── vite.config.ts
├── Makefile
├── go.mod
├── .prettierrc.json
├── README.md
└── deploy/
    ├── config.example.toml
    └── webmux.service
```

### 9.2 里程碑

按顺序实现，每个里程碑结束时都应该能运行并手工验证。

| #   | 里程碑       | 交付内容                                                                            | 验证方式                                                                                  |
| --- | ------------ | ----------------------------------------------------------------------------------- | ----------------------------------------------------------------------------------------- |
| M1  | 核心 Session | `session` 包：PTY 启动、ring、reader/waiter、Close；单元测试覆盖 ring 和 osc 状态机 | `go test ./...` 通过；一个临时 main 能启动 bash 并把输出打到 stdout                       |
| M2  | 协议与 API   | `api` + `hub`：REST 全部端点、两个 WebSocket、二进制帧协议、token 中间件            | `curl` 创建会话；用 `websocat` 附着并能交互；断开再附着能收到快照                         |
| M3  | 最小前端     | 终端页可用：xterm + fit + 重连回放；欢迎页能列出、新建、进入、关闭                  | 桌面浏览器完整走通；杀掉浏览器标签后重开能看到历史                                        |
| M4  | 状态与 title | OSC 解析、状态探测、可配置输出预览、列表实时更新                                    | 开 `claude` 后列表显示其 title；等待权限确认时状态变为“等待输入”                          |
| M5  | 移动端打磨   | 软键栏、键盘视口处理、粘贴回退、字体缩放、PWA manifest、深色主题                    | iOS Safari 和 Android Chrome 真机：能在 `claude` 里输入中文、按 Esc、粘贴多行、横竖屏切换 |
| M6  | 打包与文档   | Makefile、交叉编译、systemd 文件、README（安装、配置、Tailscale 访问、FAQ）         | 在一台干净的 Linux 上按 README 从零部署成功                                               |

### 9.3 验收标准

已勾选项有当前实现的运行证据，详见 README「验收记录」。手机 / iOS 真机项保留未勾选；Chromium 移动视口不代替真机。

- [x] 浏览器关闭 10 分钟后重新打开终端页，能看到这期间 `claude` 的全部输出（不超过缓冲上限）
- [ ] 同一会话在手机和桌面同时打开，两边输出一致，任一端输入另一端可见
- [ ] 手机锁屏 → 解锁后 3 秒内自动恢复连接，无需手动刷新
- [x] 欢迎页在 agent 停下等权限确认后 3 秒内显示“等待输入”状态
- [x] 新建会话选目录时能补全、能创建不存在的目录、能选预设 `claude` 并追加参数
- [x] 关闭会话后 Claude Code 进程及其子进程全部退出，无僵尸进程（`ps` 检查）
- [x] 软键栏 `Esc`、`Ctrl+C`、方向键在 Claude Code 的 TUI 里行为正确
- [ ] iOS Safari 中输入 `--help` 不会被自动替换为破折号
- [x] 启动时 `listen=0.0.0.0` 且无 token 打印 WARN；带错误 token 的请求返回 401
- [x] `go vet`、`golangci-lint`（默认规则）、`pnpm --dir web build` 零警告；`go test -race ./...` 与 `make format-check` 通过
- [x] 20 个会话同时运行时服务进程 RSS < 100 MB

### 9.4 后续可选扩展（不在本版本范围）

- 会话手动重命名（`userTitle` 字段已预留）
- 等待输入时通过 ntfy / Bark 推送通知到手机
- 文件上传到会话 cwd、下载 cwd 下的文件
- 守护进程 + 前端分离，支持服务热升级不丢会话
- 分屏同时看两个会话

## 10. 给 Agent 的实现指令

把本文档整体交给编码 agent 时，附上以下指令作为开场。

```markdown
完成本文档描述的 WebMux，并自行验收。持续推进所有里程碑，
用 Git 提交阶段成果，报告实际验证证据与无法覆盖的环境边界。

硬性要求：

1. Go 1.23+，模块名 github.com/rainchan/webmux；第三方依赖：
   creack/pty、coder/websocket、BurntSushi/toml。
2. 前端 Svelte 5 + TypeScript + Vite，使用 shadcn-svelte 和 Zod；
   终端使用 xterm.js、fit、webgl。依赖使用 pnpm，代码统一 gofmt / Prettier。
3. 第 6 节的协议是接口契约，前后端都按它实现，不要擅自改字段名或帧格式。
4. 第 5.3 节的 RingBuffer 和第 5.4 节的 OSC 状态机必须有单元测试，
   包括跨 chunk 拼接和缓冲回绕的用例。
5. 所有 goroutine 必须能被 context 或 close(done) 干净地结束，用
   go test -race 跑测试。
6. 平台相关的进程逻辑放在 _linux.go / _darwin.go 文件里（Linux 读 /proc，
   macOS 用 sysctl 与 proc_pidinfo），其他平台给降级实现，保证都能 go build。
7. 不要实现第 9.4 节列出的扩展功能。
8. 遇到文档没写清楚的地方，先按最简单的实现做，并在汇报里列出你做的假设。

保留已有代码与未提交改动；迁移时删除旧框架与旧调用路径，不留兼容壳。
最终按第 9.3 节逐项记录已验证项目和真机 / 部署环境限制。
```

**常见坑提醒**（agent 容易犯的错，提前写在这里）：

- `pty.Read` 在子进程退出后返回 `EIO`，这是正常的结束信号，不是错误
- `cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true, Setctty: true}`：`creack/pty` 的 `StartWithSize` 已经处理了 `Setctty`，不要重复设置导致 `inappropriate ioctl`
- 杀进程要杀进程组（`-pid`），否则 `claude` 启动的子进程会变孤儿
- WebSocket 写必须串行，`coder/websocket` 的 `Write` 不是并发安全的，所有写都走订阅者的单个写 goroutine
- xterm.js 的 `write` 是异步的，快照写完后要用回调再 `scrollToBottom`，否则会停在顶部
- Vite 构建产物路径带 hash，`static.go` 的 SPA fallback 要对 `/assets/*` 返回 404 而不是 `index.html`，否则缺失资源会拿到 HTML 导致难以排查
- `go:embed` 不能嵌入以 `_` 或 `.` 开头的文件，Vite 默认不生成这类文件，但 `.vite` 目录要排除

## 只读代码工作区（2026-10-14）

终端顶栏“代码”懒加载工作区；桌面（至少 1024px）使用 shadcn Resizable 并排显示，窄屏切换终端 / 代码，文件导航可收起。唯一 xterm 宿主不卸载：隐藏只暂停 fit / resize，输出继续进入缓冲；返回后仅发送有效尺寸，不抢键盘焦点。上传、软键和终端文本选择仍限于终端区域。

`cmd/webmux` 显式构造 `internal/codeview.Service` 并注入 API。session manager 提供会话读取与关闭通知；代码服务复用 workspace 的有界只读 Git runner，独立于仓库清单存储。首次成功读取时绑定规范根路径与文件系统身份，Git 根为启动 cwd 所属 worktree 顶层；没有 Git 或普通目录以 cwd 为根，能力字段明确表示差异 / ignore 不可用。后续 cwd 不跟踪、不自动重绑。每个路径从根文件描述符逐段 `openat(O_NOFOLLOW)`，拒绝链接遍历、特殊文件、Git 内部目录与嵌套仓库边界。

目录逐层分页；nodeId 是根内原始路径的 base64url，独立于修改列表。Git ignore 过滤仅针对未跟踪项，已跟踪文件继续可见；`.git` 与解析到的 Git 管理路径始终排除。路径搜索遵守相同边界和忽略策略，`complete=false` / `limited=true` 表示未穷尽，不等于没有匹配。

代码 API 全部位于 `/api/sessions/{id}/code`，沿用 token / Origin 检查，返回 `Cache-Control: no-store`，客户端先经 Zod 验证：

| 路由                                               | 合同                                                                                                                                            |
| -------------------------------------------------- | ----------------------------------------------------------------------------------------------------------------------------------------------- |
| `GET /context`                                     | contextId、rootPath、rootKind、browseAvailable、diffAvailable、ignoreAvailable、reason                                                          |
| `GET /tree?contextId&nodeId&includeIgnored&cursor` | children、nextCursor、limited、complete、contentVersion、checkedAt；游标绑定目录版本                                                            |
| `GET /files?contextId&q&includeIgnored`            | 有界文件路径搜索，完整相对路径及完成状态                                                                                                        |
| `GET /file?contextId&nodeId&source`                | 不可变 File 快照，source 可为当前 worktree / index / head，document 含 opaque nodeId、摘要、种类、全文、行数与可引用状态                        |
| `GET /changes?contextId&cursor&q&nodeId&group`     | 按未暂存 / 已暂存 / 未跟踪顺序分页，entries、nextCursor、匹配 total、contentVersion；可筛选路径 / 精确 nodeId / 来源组                          |
| `GET /diff?contextId&changeId`                     | 冻结双方完整文本及来源文档、snapshotId、checkedAt                                                                                               |
| `GET /snapshots/{snapshotId}/file?side=old或new`   | 读取已捕获比较侧全文，不重新读取工作区                                                                                                          |
| `POST /check`                                      | contextId、snapshotIds、directories（nodeId → version）、changesVersion、includeIgnored；返回 unchanged / updated / missing / expired / unknown |

非法路径 / 跨会话快照返回 400；会话 / 文件不存在 404；根身份、来源或分页版本冲突 409；快照失效 410；工作量 / 输出超限 413；取消 / 超时 504。读取失败与空目录、无修改分开表达，单个文档失败不影响 PTY。文件名非法 UTF-8 时转义展示并禁用位置发送。

修改列表每页 200 项，无总条数限制；浏览器在滚动接近末尾时加载下一页，也可点击“加载更多修改”，只为可见区和少量缓冲行创建 DOM。固定高度路径行可横向截短，完整路径通过标题和打开后的文档显示。方向键 / Home / End 支持已加载条目的键盘导航；来源 / 路径筛选在服务端扫描完整 Git 列表，不只筛选已加载部分。

游标绑定 contextId、完整修改版本及筛选条件；列表改变返回 409，保留已加载内容并提示刷新，禁止把不同版本页混合。服务端流式计算完整列表摘要与匹配总数，仅保留当前页；打开 Diff、全文切 Diff / 刷新使用全流或精确 nodeId 查找，批量检查仅保留打开快照涉及的修改，后续页的文件与第一页同样可用。文件导航的 Git 状态 / ignore 查询也流式读取，避免累计输出超过 2 MiB 阻断导航。每页仍需遍历 Git 元信息以校验版本和统计总数，请求取消 / 10 秒期限和两项重型操作并发继续适用。

完整文件捕获有界字节，校验读取前后身份、大小和修改时间，并再次验证当前目标；并发变化返回 409。文本支持 UTF-8 / LF / CRLF；空文件、末尾无换行保留真实行语义。非文本、编码不支持、超过 2 MiB / 20000 行不提供伪完整内容。unstaged 是 index blob / 工作区捕获，staged 是 HEAD blob / index blob，untracked 是 empty / 工作区捕获；unborn 以 empty 为旧侧。先解析不可变 OID，再 `cat-file` 捕获，复核修改来源。需要用户过滤器 / 编码转换时明确降级为元信息，不执行转换。

Diff 计算在浏览器 Web Worker 中使用 jsdiff（`diff` 包）`diffLines`，服务端只返回同一 snapshotId 的新旧原始文本及来源元信息，不生成 patch、不写比较临时文件。比较使用 `stripTrailingCr: true` 归一化 LF/CRLF 行尾，末尾无换行仍显示不占代码行号的标记；结果中的计数映射回原文件，新增 / 删除行只含本侧行号。浏览器局部模型保留三行上下文，向上 / 向下每次 30 行或全部展开只读取同一冻结模型，没有上下文网络接口；快照失效后禁用展开和引用。差异块划分可能与 Git CLI 不同。

只为代码区可见的活动标签启动计算；切换标签、隐藏、刷新或卸载终止未完成 Worker。每标签缓存当前版本的结果；回包携带 snapshotId，经过 Zod 校验和取消 / 当前文档核对后才安装。详细比较预算 250ms，Worker 总看门狗 10 秒；超限 / 不可用 / 出错时明确提示，允许重试或打开两侧冻结全文，不显示伪完整差异。全文和 Diff 均用 CodeMirror 虚拟化显示，整份删除也不生成全部行 DOM。

只读 Git 查询设置 `GIT_OPTIONAL_LOCKS=0`，不改索引 / 对象 / safe.directory；临时禁用 status / diff 的 clean / process filters 和 fsmonitor，避免原始状态查询运行自定义转换。服务端最多两个重型读取；单侧内容及非列表 Git 查询输出上限 2 MiB，修改 / status / ignore 列表采用 NUL 逐条流式处理，没有累计输出或总项数上限，错误最多 8 KiB，请求含排队最多 10 秒。

服务内存快照随机标识且归属会话，LRU 总预算 64 MiB、最长 10 分钟，移除会话 / 服务关闭释放。过期返回 410，绝不临时重读新文件冒充旧内容。浏览器保留已加载只读文本，停用失效引用，不将内容写入 localStorage / IndexedDB。每标签保留独立阅读、查找、选区与模式；最多 20 标签，不自动替用户关闭。只读 CodeMirror 虚拟渲染全文 / Diff，按需加载语言，其余纯文本，不执行 HTML 或源码。

可见前台每 5 秒单在途批量检查，恢复前台、代码重新显示、连接恢复立即检查，隐藏 / 后台取消检查。相同请求内 Git 修改列表只读取一次，多个来源共用当前列表；检测错误返回 unknown。检查不返回新全文，旧文本和选区保留；用户刷新后安装新版本并清除旧选择。session / context / document / 请求序号隔离响应，关闭标签和离开会话取消请求，旧检测不能覆盖刷新后的文档。

行引用绑定当前文档、快照、比较侧及连续范围，普通行、修改行、展开上下文均可用；跨 gap 先展开。预览和粘贴共享格式化函数，绝对路径加 `:行号` / `:起始-结束`，index 标“索引版本”，head 标“HEAD 旧版本”，文件级不虚构行号。空格、引号、换行与控制字符使用 JSON 引号 / 转义；无损路径不可表达时禁止发送。`useTerminal.paste` 检查初始化、连接、回放及退出状态，经 xterm 保留 bracketed paste，并返回本地 WS send 结果；成功仅表示已插入终端，没有远端 agent 确认，不自动回车或补发。
