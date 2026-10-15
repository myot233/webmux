# WebMux

单用户、单进程、可重连的 Web 终端。浏览器关闭、切网不会结束服务端 PTY；重新附着会清屏并回放当前缓冲。用于在可信内网中操作 Bash、Claude Code、Codex CLI 等。目标与协议见 [WebMux.md](WebMux.md)。

**安全边界：这相当于以当前系统用户身份把 shell 暴露到网络。不要开放公网；默认只绑定 `127.0.0.1:7681`。使用 Tailscale / WireGuard 地址并设置共享 token。服务重启结束所有会话，PTY 与缓冲不落盘；仅显式添加的仓库清单持久保存。**

## 从源码构建

需要 Go **1.24+**（终端屏幕预览依赖的最低版本）、Node.js **24 LTS**、pnpm **12.11.2** 和 GNU Make（macOS 自带的 `make` 即 GNU Make）。运行端只需 Linux 或 macOS（amd64 / arm64）、可执行 shell 与所选 agent；不需要 Node.js、Go 或 tmux。

Git 是仓库 / worktree 管理的可选系统依赖（需要支持 `git worktree list --porcelain -z`）。没有 Git 时仍可直接启动终端；添加的仓库必须可信，Git 的本机配置、hooks 与 filters 不在沙箱内运行。

```sh
corepack enable
corepack prepare pnpm@12.11.2 --activate
make build
./bin/webmux version
./bin/webmux --listen 127.0.0.1:7681
```

若 Node 安装未附带 Corepack，可按 pnpm 官方安装方式安装 pnpm。前端所有依赖由 `web/pnpm-lock.yaml` 锁定，Makefile 始终使用 `pnpm install --frozen-lockfile`，只允许 esbuild 的必要安装脚本。

- `make web`：构建前端到 `web/dist`。
- `make build`：构建内嵌静态资源的单二进制 `bin/webmux`。
- `make release VERSION=0.1.0`：生成 `bin/webmux-linux-amd64`、`bin/webmux-linux-arm64`、`bin/webmux-darwin-amd64`、`bin/webmux-darwin-arm64`，CGO 禁用；darwin/arm64 由 Go 链接器自动 ad-hoc 签名。
- `make install`：构建并安装到 `~/.local/bin/webmux`。
- `make test`：Go race 测试、Vitest 单元 / Chromium 组件测试与 Playwright 真实后端端到端测试。
- `make check`：Svelte / TypeScript 检查、Vite 构建、格式检查、`go vet`、golangci-lint 默认规则。需先将 `golangci-lint` 放到 PATH。
- `make format`：gofmt 格式化所有 Go 文件，Prettier 格式化前端代码、配置与文档。
- `make format-check`：检查格式，不修改文件；构建产物和依赖目录除外。

Linux 与 macOS 是同等支持的运行平台。进程状态与清理在 Linux 读 `/proc`，在 macOS 读 `sysctl kern.proc.all` 与 `proc_pidinfo` 的运行线程数；其他平台只能编译，进程探测返回 false、关闭时只信号进程组。未构建前端时 `.gitkeep` 让 `go test` 能工作，但该状态的 HTTP 页面明确返回 503，不能当作可部署产品。

## 终端旁的只读代码工作区

点击终端顶栏“代码”打开。1024px 及以上并排显示终端和代码，分隔线可拖动或用方向键调整；窄屏使用“终端 / 代码”切换，终端保持同一 PTY，继续接收输出。代码按需加载，折叠后仍保留已打开文件。

- “文件”逐层浏览目录、通过面包屑返回，或按文件名 / 相对路径搜索；无需先修改文件。Git 忽略的未跟踪项默认隐藏，“显示忽略项”可显式开启；已跟踪文件不会因新增 ignore 规则消失。普通目录 / 无 Git 时显示能力提示。
- 完整文件用只读 CodeMirror 展示真实行号、语法高亮、文件内查找和跳转行。最多 20 个标签，以路径及 worktree / index / head 来源区分，切换保留各自阅读状态。文件内容不保存到浏览器存储。
- “修改”分为未暂存、已暂存、未跟踪；同文件暂存后继续修改会分别出现。Diff 展示双方行号、增删和省略上下文，支持每次 30 行向上 / 向下或全部展开，以及打开比较侧完整文件；删除文件的旧侧仍可读。全文“查看 Diff”会核对版本，不把新内容混入已有阅读。
- 代码可见且页面前台时每 5 秒检查变化，恢复前台 / 重连立即检查。检测只提示；点击“刷新文档 / 刷新全部”才更换内容并清除旧选区。已知更新、删除、过期或检测失败时暂停位置发送，保留旧文本。
- 点击行号选择单行，桌面 Shift 点击终点；手机再次点击终点可选连续范围，也可“引用整个文件”。跨 Diff 未展开区的范围需先展开。下方预览与实际插入使用同一格式，如 `/repo/main.go:12-18 `、`/repo/main.go:12（索引版本） `。特殊路径使用 JSON 引号与转义，不能无损表示的路径禁用引用。
- “发送位置到终端”仅向当前会话粘贴，不回车、不清空已有输入；连接未就绪时提示失败且不自动重发。手机成功后返回终端，不强制唤起键盘。请自行补充要求并提交。

浏览根首次绑定到启动 cwd 所属 worktree 顶层（包括未登记、linked worktree 的子目录启动），普通目录 / 无 Git 时绑定启动 cwd。不会跟随终端内后续 `cd`，不会在后台扩大根。根目录被替换后拒绝继续读取。`.git` 和已知 Git 管理目录始终排除，符号链接只显示目标文字，子模块 / 嵌套仓库显示边界，不递归越界。

目录每页 200 项、单目录最多检查 10000 项；路径搜索最多 200 个结果 / 100000 个条目，受限时明确提示收窄路径。全文单侧最多 2 MiB / 20000 行，二进制和不支持编码显示原因。修改列表每页 200 项，可继续按需加载，没有总项数上限；列表按可见区域虚拟渲染，支持按来源或路径筛选。Git 修改 / 状态 / 忽略列表逐条流式处理，不受累计 2 MiB 输出限制；文件与其他非列表 Git 查询仍有 2 MiB 上限，请求期限 10 秒，最多两个服务端重型读取。只读快照总预算 64 MiB、最长 10 分钟，会话移除后释放；失效返回 410，需主动刷新。原始文件、索引、对象与 Git 配置均不写入。Diff 使用浏览器 Worker 中的 jsdiff 比较两侧冻结文本，展开上下文无需网络请求；比较预算 250ms、Worker 总期限 10 秒，超限或失败会提示重试或查看两侧完整文件。切换标签、隐藏或刷新会终止未完成计算，文本比较不写临时文件。

## 配置

```sh
mkdir -p ~/.config/webmux
cp deploy/config.example.toml ~/.config/webmux/config.toml
chmod 600 ~/.config/webmux/config.toml
```

编辑配置，将 `listen` 改为自己的 Tailscale / WireGuard 地址，例如 `100.64.0.5:7681`，`token` 改为自己的随机共享密钥。默认配置文件为 `~/.config/webmux/config.toml`：不存在时用默认值；显式 `--config` 指定的文件不存在时启动失败。CLI `--listen` / `--token` 优先于 TOML，包括显式空 token。

```sh
./bin/webmux --config ~/.config/webmux/config.toml
./bin/webmux --listen 127.0.0.1:7681 --token YOUR_TOKEN
curl -H 'Authorization: Bearer YOUR_TOKEN' http://127.0.0.1:7681/api/health
```

| 配置                 | 默认                                                       | 含义                                                                |
| -------------------- | ---------------------------------------------------------- | ------------------------------------------------------------------- |
| `listen`             | `127.0.0.1:7681`                                           | HTTP / WebSocket 监听地址                                           |
| `token`              | 空                                                         | 可选共享 token，空表示无认证                                        |
| `scrollback_bytes`   | `2097152`                                                  | 每会话保留最新 2 MiB 原始终端字节，不是精确行数                     |
| `default_cwd`        | `~`                                                        | 新建会话默认目录，支持 `~` 展开                                     |
| `state_dir`          | `$XDG_STATE_HOME/webmux`，未设置时 `~/.local/state/webmux` | 仓库清单目录，支持 `~`；不保存会话、终端内容或 token                |
| `shell`              | `$SHELL` 或 `/bin/bash`                                    | 默认 shell，以 `-l -i` 启动                                         |
| `idle_after_seconds` | `30`                                                       | 无等待特征时的空闲阈值                                              |
| `prompt_patterns`    | 常见提示符 / 权限确认                                      | 可自定义的 Go 正则表达式列表                                        |
| `allowed_origins`    | `[]`                                                       | 附加允许的完整 Origin，例如 `https://myhost.example`                |
| `presets`            | shell / claude / codex / omp                               | argv 数组及 `allow_args`，配置列表替换默认列表；`custom` 为保留名称 |

预设不会自动安装 agent；`which claude`、`which codex`、`which omp` 必须能找到实际程序。追加参数按引号与反斜杠规则拆成 argv，不经 shell 求值。要更改 shell 的启动方式，可为 shell 预设显式设置 `cmd = ["/bin/zsh", "-l", "-i"]`。

仓库清单为 `state_dir/repositories.json`，通过临时文件、同步与原子替换保存；状态目录权限 `0700`，文件 `0600`。同一状态目录仅供一个实例使用，文件锁冲突、清单损坏或存储不可用会禁用该实例的仓库管理，但不阻止普通终端启动；不会把损坏清单覆盖为空清单。多个实例应配置独立 `state_dir`。

若清单文件已原子替换、随后目录同步失败，磁盘持久结果不确定：请求报告失败并禁用后续仓库写入，避免旧内存覆盖新文件。保留状态目录，排除存储故障后重启服务重新读取；直接会话仍可使用。

## systemd 用户服务

```sh
make install
mkdir -p ~/.config/systemd/user
cp deploy/webmux.service ~/.config/systemd/user/
systemctl --user daemon-reload
systemctl --user enable --now webmux
systemctl --user status webmux
journalctl --user -u webmux -f
```

先按上节创建配置。`deploy/webmux.service` 的 PATH 包含 `~/.local/bin`、pnpm / npm 全局 bin；若 agent 在 nvm、其他自定义目录中，通过用户 drop-in 增加真实路径：

```sh
which claude
which codex
systemctl --user edit webmux
```

```ini
[Service]
Environment=PATH=/home/YOU/.local/bin:/home/YOU/.nvm/versions/node/v24.20.0/bin:/usr/local/bin:/usr/bin:/bin
```

重载并重启服务。用户未登录时仍需运行，可执行 `loginctl enable-linger "$USER"`（可能需要管理员授权）。服务使用 `KillMode=mixed`，先让 WebMux 正常清理会话，再由 systemd 在超时后清理整个 cgroup。终止 / 重启服务会永久丢失会话与缓冲。

## macOS launchd 用户代理

```sh
make install
mkdir -p ~/Library/LaunchAgents ~/Library/Logs
sed "s#/Users/YOU#$HOME#g" deploy/com.webmux.plist > ~/Library/LaunchAgents/com.webmux.plist
plutil -lint ~/Library/LaunchAgents/com.webmux.plist
launchctl bootstrap "gui/$(id -u)" ~/Library/LaunchAgents/com.webmux.plist
launchctl print "gui/$(id -u)/com.webmux"
tail -f ~/Library/Logs/webmux.log
```

先按「配置」创建配置。launchd 不展开 `~` 与 `$HOME`，模板里的 `/Users/YOU` 必须替换为真实主目录。`EnvironmentVariables` 的 PATH 已包含 `~/.local/bin`、pnpm / npm 全局 bin 与 Homebrew；agent 在其他目录时直接编辑 plist 后重新加载：

```sh
launchctl bootout "gui/$(id -u)/com.webmux"
launchctl bootstrap "gui/$(id -u)" ~/Library/LaunchAgents/com.webmux.plist
```

代理在登录后运行（用户会话内，非开机前），`RunAtLoad` 启动、异常退出后 `KeepAlive` 重启。plist 设置 `SHELL=/bin/zsh` 与 `LANG=en_US.UTF-8`：launchd 不加载登录 shell 环境，缺少 UTF-8 locale 时中文会显示为转义。`ExitTimeOut` 15 秒内 WebMux 收到 SIGTERM 后清理会话，超时由 launchd SIGKILL。停止 / 重载代理会永久丢失会话与缓冲。系统睡眠会暂停所有会话进程；需要长时间运行 agent 时用「系统设置 → 电池」或 `caffeinate` 防止睡眠。

macOS 平台差异：

- 没有 subreaper；脱离父进程的后代由 launchd 回收，不会成为 WebMux 的僵尸进程。
- 内核不向其他进程公开环境变量（即使同一用户），`WEBMUX_SESSION_ID` 只能用于 Linux。关闭会话时按终端会话 ID 与进程树查找后代；先 `setsid` 脱离终端会话、再让父进程退出的自守护程序不会被清理。
- 临时目录位于 `/var → /private/var` 符号链接之下；会话与工作树报告规范化后的真实路径。
- Apple 的 `/bin/bash` 是 3.2，交互启动时提示改用 zsh，可设置 `BASH_SILENCE_DEPRECATION_WARNING=1`。默认 shell 预设使用 `$SHELL`（通常为 `/bin/zsh`）。

## Tailscale 与手机访问

1. 在服务器和手机安装 Tailscale，加入同一可信网络。
2. 绑定服务器的实际 Tailscale 地址，防火墙只允许可信 VPN 接口。
3. 手机浏览器打开 `http://<Tailscale-IP或名称>:7681`。
4. 启用 token 时输入一次，浏览器 localStorage 保存；只在可信设备上保存密钥。
5. iOS Safari / Android Chrome 可“添加到主屏幕”。

WebMux 不内置 TLS。需要安全上下文与系统剪贴板 API 时，可用 `tailscale serve` 或 Caddy 代理 HTTPS；保持请求 Host，并把公开 HTTPS Origin 加入 `allowed_origins`。不能让不可信客户端伪造同源域名绕过网络隔离。HTTP 内网下不提供离线缓存或 service worker。

## 使用

首页采用「仓库分组标题 → 工作树卡片 → 会话列表行」，只有工作树 / 目录使用一层 shadcn Card，不嵌套会话卡片。仓库按注册顺序、工作树按路径稳定排列，只有组内会话按最近活跃时间排序。仓库标题可折叠，展开后工作树和会话直接可见；标题保留运行 / 等待 / 退出数量，空仓库和空工作树不会消失。没有注册关联的会话按启动目录显示在「直接会话」；失去工作树的会话保留在「已不可用工作区」。目录完整换行展示，不依赖悬停。桌面为单列工作树卡片，手机将会话状态和活动信息换到标题下方，不使用横向滚动表格。

会话通过 WebSocket 实时更新；Git 状态在首次加载、返回页面 / 可见、重连、手动刷新及操作结束时重新读取，显示读取时间和局部错误，不把读取失败显示为干净。新建会话提供「直接目录」「已有工作树」「新工作树」三种模式，保留最近目录、预设参数、自定义 argv 和可选名称。

- 目录选择器采用路径导航：输入完整路径或目录前缀，↑↓ 选择、Enter / Tab 进入子目录、Alt+↑ 返回上一级；也可返回主目录。点击“选择当前目录”后才更新工作目录。
- 目录列表不截断，普通目录优先，支持符号链接目录；默认不显示点目录，可用“显示隐藏目录”切换，也可直接输入点目录前缀。长列表使用固定可见、可拖动的滚动条；只滚动目录区域，路径输入和确认按钮保持可见。
- 启动预设包括 Shell、Claude Code、Codex CLI、omp。自定义命令支持引号和反斜杠，记住上次输入；直接执行 argv，不展开变量、通配符或管道。需要 shell 语法时显式输入 `bash -lc 'npm run dev | tee dev.log'`。

- 会话行至少 56px 高，除行末菜单外，整行含上下留白均可点击进入终端；键盘聚焦时显示整行焦点框。行末菜单独立提供关闭或移除，shadcn AlertDialog 二次确认。列表复用 Item 和 Separator，终端观察窗使用独立背景、等宽字体，默认跳过当前终端屏幕末尾一条非空行，再向上显示十条非空行，而不是最近收到的输出片段；光标重绘覆盖旧内容，清屏移除旧预览，备用屏幕切换与 PTY 尺寸变化同步跟踪。每行最多 120 个 Unicode 字符，预览窗口横向裁切、不再次换行，点击可进入完整终端。空屏幕显示等待提示，内容不足时保留设定行数的高度；内容全部被跳过时显示隐藏提示。同一操作区的文字按钮与图标按钮使用对应的 shadcn 尺寸档位。
- 首页顶部“设置”进入 `/settings`，可设置显示 1–50 行、跳过末尾 0–50 行（0 表示不隐藏）。默认显示 10 行、跳过 1 行，页面提供效果示例和恢复默认。有效整数立即保存到当前浏览器的 localStorage，无效输入不覆盖已保存值；刷新或返回工作区后继续生效，不改变 PTY 输出、状态探测或其他设备的设置。
- 终端页头使用 Tabler 图标按钮返回会话列表，保留“返回会话列表”无障碍标签。终端菜单提供复制全部原始缓冲、清屏、重新同步、字体大小和关闭会话。
- 终端顶栏全屏按钮右侧提供“上传文件”，也可把文件拖入终端区域。桌面粘贴剪贴板中的文件 / 图片使用同一流程，普通文本仍按 xterm 的粘贴模式输入。上传时显示文件名、百分比与进度条，支持取消；成功后进度消失，并把经过 shell 引号保护的服务器绝对路径粘贴到终端，不自动按回车。多个拖入 / 粘贴文件依次上传。
- 上传文件放在服务器系统临时目录（遵循 `TMPDIR`）的独立 `webmux-upload-*` 子目录中，不写入工作区，不覆盖同名文件。单文件上限 100 MiB，上传期限 5 分钟，目录 / 文件权限为 0700 / 0600；失败、取消或会话退出时清理未完成文件。成功文件保留供终端使用，由用户或系统临时目录清理策略删除，不随会话关闭自动删除。
- 已退出会话保留退出码与最后输出，直到手动移除。
- 同一会话可多端同时连接；**最后一次 resize 的客户端决定 PTY 尺寸**。
- 默认服务端缓冲 2 MiB、xterm 历史 10,000 行；超过上限丢弃旧数据。

### 仓库与 worktree

- 「添加仓库」只登记已有仓库，不 clone / init；主目录、linked worktree、子目录和符号链接按 Git common directory 去重。登记会跨服务重启保留，已有会话不会因此重启。
- 「新建 worktree」支持从明确的本地基线创建新分支，或选择未被占用的已有本地分支。新分支预填可修改的建议名 `wt/<形容词>-<名词>`，避开已有本地分支和已有工作树目录段；「换一个」重填。默认路径为仓库旁的 `<仓库名>.worktrees/<分支最后一段>`，提交前可修改完整路径；不覆盖目录、不重置分支、不自动 fetch。
- 已有本地分支使用 shadcn 搜索列表：按名称不区分大小写匹配，搜索框固定、列表限高并带滚动条；支持点击或 ↑↓ / Enter 选择。已选分支显示勾选，已被工作树占用的分支禁选；搜索本身不修改已选分支，手动修改过的目标路径不会被后续选择覆盖。
- 可仅创建工作树，或创建后启动 Shell / agent。后者是两个独立结果：启动命令失败时保留已创建的工作树，换命令重试只启动会话。没有可解析提交的仓库不能新建 linked worktree，但可在主工作区启动。
- 同工作树可开多个独立 PTY，但它们**共享文件**。新工作树只检出版本控制内容，不复制 `.env`、未提交修改或依赖目录，不安装依赖、不执行 WebMux 自定义初始化脚本。
- 「关闭 / 移除会话」不删除工作树；「从列表移除仓库」只移除登记，不关闭会话、不删除目录或分支，原终端仍可从直接会话访问。
- 「删除 worktree」先预览。默认拒绝未提交修改、未跟踪 / 忽略文件以及活动会话（包括从子目录直接启动的会话）。主工作区、bare、locked、缺失 / 未知状态、含子模块 / 嵌套仓库的目录不能通过页面删除。
- 强制删除是独立危险确认：输入**完整路径**并明确授权关闭列出的会话后，等待子进程清理和 PTY 排空，再执行 Git 单次 force 删除。确认 5 分钟内单次有效，目标或会话集合改变后必须重新预览。分支和已退出会话记录保留；失败不会被伪装成完整成功，也不会用递归删除兜底。
- 强制删除会永久丢弃删除时的全部目录内容，包括 `.env` 和确认后新写入的文件。WebMux 只知道启动 cwd，不追踪终端内 `cd`，不能停止外部进程，也不提供跨外部 Git 操作的文件系统事务。detached 提交可能没有分支保护。
- 服务重启后仓库清单和磁盘上的 worktree 保留；所有会话、PTY 与终端缓冲消失。

### 移动端

VisualViewport 跟随键盘高度与安全区域，横竖屏自动 fit；支持单指滚动、双指字体缩放（12–20 px）、单行软键栏。单指向下滑查看更早内容，向上滑查看后续内容：普通终端滚动历史，omp 等全屏面板按应用启用的鼠标 / 光标键模式处理滚轮。指针捕获到稳定容器，应用重绘行节点不会中断手势；滑动或缩放不会唤起键盘，轻点仍可聚焦。双指变单指、取消手势时重新建立基线，避免跳动。Ctrl / Alt 为一次性修饰键。终端输入禁用自动大写、拼写检查与自动纠正。常用 Ctrl+C/D/Z/L/R、Esc、Tab、方向、Home/End、PgUp/PgDn 都有按钮。

长按终端文字约 0.5 秒选中词语，保持按住并拖动可跨行扩选，松手后点击“复制选中”；“取消选择”清除选区。中文、emoji 的双宽字符按完整字符选择，不会因长按或拖动唤起键盘或发送终端输入。无剪贴板权限时展示选中的纯文本，可在文本框内使用系统选择与复制；滑动和双指缩放仍按原行为工作。

软键栏始终显示并占满一行，按键横向滚动，不换行，不提供收起按钮；`⌨`、粘贴、`/`、`@` 排在最前，无需滚动即可操作。按键统一描边样式、36 px 高度，Ctrl / Alt 激活时高亮。键盘按钮切换终端输入焦点；`/`、`@` 按普通字符输入，按下时保持终端焦点。

终端顶部菜单左侧提供全屏 / 退出全屏按钮，通过浏览器原生 Fullscreen API 请求隐藏导航界面；浏览器或系统退出全屏时，按钮状态自动同步，离开终端页也会退出本页开启的全屏。菜单、字体设置等弹层在全屏内仍可使用。浏览器原生地址栏 / 工具栏不能用页面 CSS 强制隐藏；不支持网页全屏时会提示，可尝试“添加到主屏幕”以独立模式打开（页面已配置 standalone 与 iOS 主屏幕元信息）。

HTTP 私网通常无法读 Clipboard API：点“粘贴”后在输入框粘贴并“发送”。xterm 的 paste API 遵循应用启用的 bracketed paste 模式：Bash / Claude 开启时多行文本不会被逐行误提交。桌面使用 Ctrl+Shift+C/V；复制全部在剪贴板不可用时提供可选中文本。

文件粘贴优先使用原生 Ctrl+V / Cmd+V 的 paste 事件，不依赖 Clipboard API 读取权限；桌面 Ctrl+Shift+V / 菜单“粘贴”也会通过 Clipboard API 读取图片。权限与可读取的文件类型取决于浏览器和操作系统。没有剪贴板读取权限时，仍可在粘贴弹层内粘贴文件 / 图片，或使用上传按钮、拖拽。离开终端页面会取消正在进行的上传。

切后台停止写入 xterm，连接与 PTY 保持；回前台主动重新附着并回放，避免半失效连接仍显示 OPEN。指数退避 0.5–8 秒，回前台 / 恢复网络时立即尝试。真机 iOS 输入法、VPN 切换与锁屏恢复不能用桌面设备模拟证明。

### 标题与状态

OSC 0/2 title 支持 BEL / ST 与跨块拼接，创建时的手动名称优先。预览只读跟踪当前终端屏幕，最多提供底部 100 条非空行，每行最多 120 个 Unicode 字符；浏览器按本地设置裁剪。进程表（Linux `/proc`、macOS `sysctl`）每秒刷新，时间与提示符状态每 500 ms 评估，为 3 秒等待提示留出传输余量；列表更新每会话最多 500 ms 一次，预览变化最多 2 秒一次。

- 最近 2 秒有输出：运行中。
- 之后匹配提示符，或进程树无 running 成员、叶子全 sleeping（Linux / macOS）：可能在等你。
- 2–30 秒不确定：同样提示查看；超过空闲阈值且没有等待特征：空闲。
- 子进程结束：已退出，记录真实退出码；信号退出记为 `128 + signal`。

启发式可能把 sleep / 网络等待判为等待，也可能把不断重绘的 TUI 判为运行中。状态只展示，不自动执行动作。

## 协议与实现

REST 与二进制 WebSocket 帧遵循设计文档第 6 节。只读 `GET /api/config` 返回 `{defaultCwd, workspace: {gitAvailable, registryAvailable, gitError, registryError}}`，不暴露 token。HTTP 接受 Bearer 或 `webmux_token` Cookie；WebSocket 也可用 query token。静态登录页面公开，所有 API / WebSocket（包括仓库与 worktree 管理）受认证和 Origin 保护。token 以定长摘要常量时间比较。

创建会话支持两种互斥形式：预设 `{cwd, preset, args?: string[], name?}`，或自定义 `{cwd, preset: "custom", cmd: string[], name?}`。自定义命令必须包含非空可执行程序，不能同时传非空 `args`；预设形式不能传 `cmd`。`GET /api/fs/dirs?prefix=` 返回全部匹配目录，普通目录排在点目录之前，各组按名称排序。

`POST /api/sessions/{id}/upload` 接受恰好一个 `multipart/form-data` 文件字段 `file`，成功返回 201 `{path}`（服务器临时文件的绝对路径）。上传流式写入磁盘，保留二进制内容；文件名仅取 basename 并拒绝控制字符，超限返回 413，无效 multipart 返回 400，会话不存在 / 已退出返回 404 / 409。该接口沿用 Token 与 Origin 检查，仅此路由将读期限延长到 5 分钟。

会话创建可附加 `workspace: {repositoryId, worktreeId}`；省略 cwd 使用该工作树根目录，提供 cwd 时必须属于同一工作树（允许子目录），失效目标不会退回默认目录。`SessionInfo.workspace` 返回可空 `{repositoryId, worktreeId, worktreePath}`，`cwd` 仍为真实启动目录。仓库相关路由、删除预览与单次确认合同见 [协议 6.1](WebMux.md#61-http-api)，终端二进制帧不变。

Session 持有 PTY 与有界字节环形缓冲，连接只是观察者。快照与订阅在同一锁内切换，无回放缺口；慢客户端断开后可重连。PTY 使用 Go netpoll 支持的非阻塞描述符，断开连接能取消阻塞输入，不会关闭持久会话；每连接输入队列有上限，单次写入最长 10 秒。会话退出先清理子进程并排空 PTY，再发送退出帧。关闭先 SIGHUP，5 秒后 SIGKILL；Linux 与 macOS 按会话 ID 与进程树找到其他 job-control 进程组，Linux 另按继承的 WEBMUX_SESSION_ID 查找并用 subreaper 回收孤儿子进程，macOS 由 launchd 回收孤儿；不因主进程先退出而取消清理。完全脱离终端会话且父进程已退出的自守护程序（Linux 上还需显式清掉环境）不属于可靠可控的 PTY 子进程；本服务不是进程沙箱。不会把终端输入输出写入日志。

输入在等待写入锁之前和获得锁之后都检查取消状态，避免已经断开的连接仍把排队输入写进 PTY。Origin 必须是完整的 HTTP(S) origin，不接受缺失主机、路径、query 或 fragment；API 不接受 query token。

快照将类型前缀与缓冲内容组装到一次分配中；每连接复用最多 64 KiB 的输出合并工作区，避免每个小输出帧都重新分配大缓冲。共享 PTY 输出片段保持不可变，合并和 WebSocket 写入只由该连接的单个 writer 执行。

屏幕预览通过固定版本的 `github.com/0magnet/xterm-go/vt` 只读解析 PTY 输出，按当前 PTY 尺寸维护屏幕，不保留额外滚动历史，也不响应设备查询或向 PTY 注入输入。原始字节缓冲、WebSocket 回放与浏览器 xterm 仍走原有路径，不由预览解析器改写。

前端使用 Svelte 5 / TypeScript / Vite / xterm 5，基础控件来自官方 [shadcn-svelte](https://www.shadcn-svelte.com/docs/installation/vite)，采用 Tailwind CSS 主题与 Svelte runes 状态。桌面 Dialog / 移动 Drawer、下拉菜单、关闭确认、目录补全和字体 Slider 均复用库组件。终端页懒加载，WebGL 不可用、上下文丢失，或浏览器报告的实际像素尺寸与 CSS 尺寸 × DPR 不一致时，回退 xterm 内置 DOM 渲染器并重绘现有缓冲，不重置终端或重连会话。正常的像素取整误差不会触发回退；DPR 改变时重新测量。终端与组件使用同一组 GitHub Dark / Light CSS 变量。

终端文字启用最低 4.5:1 对比度校正，应用显式输出黑色 ANSI / RGB 输入文字时也能在暗色背景上辨认；不改写 PTY 数据或应用的颜色转义序列。

`web/src/schemas.ts` 使用 Zod 定义会话、预设、列表事件、退出信息、健康检查、目录和本地设置契约，类型由 schema 推导。HTTP 响应和 WebSocket JSON 必须解析成功才进入页面状态；损坏的本地设置回到有效默认值。PTY 原始字节不转成 JSON，仍按二进制透传。

## 开发

```sh
pnpm --dir web install --frozen-lockfile
pnpm --dir web dev
go test -race ./...
pnpm --dir web test # unit + browser + e2e
pnpm --dir web test:unit # Vitest Node 单元测试
pnpm --dir web test:browser # Vitest 浏览器组件测试
pnpm --dir web test:e2e # Playwright 真实后端端到端测试
pnpm --dir web test:e2e:report # 查看上一次 HTML 报告
make format
make check
```

首次安装依赖后执行 `make test-browsers-install`（安装 Chromium；Linux 上同时安装系统依赖，可能需要 sudo，macOS 无需额外系统库）。已有系统库时可只执行 `pnpm --dir web exec playwright install chromium`。`make test-unit`、`make test-browser`、`make test-e2e` 分别运行三个测试层。

- **Vitest 单元测试**：Node 环境，覆盖 Ctrl / Alt 控制字节、组合顺序与 Unicode / 多字符输入边界。
- **Vitest 浏览器组件测试**：真实 Svelte / Tailwind / xterm 渲染，不需要后端。覆盖目录选择器导航 / 滚动 / 错误恢复、WebGL 与 DOM 渲染回退、DPR、历史保留、触控滚动 / 缩放 / 取消、暗色对比度与中文长按扩选。
- **Playwright 端到端测试**：桌面与 Pixel 7 触控 Chromium，每个 worker 自动启动真实 Go 服务与 Vite，使用临时 HOME / 工作目录、无 rc 的 Bash、独立动态端口，结束自动关闭进程和清理会话；不连接开发服务器或已有会话。覆盖 UI 创建、真实 PTY 键盘输入、刷新回放、退出码、关闭 / 移除、暗色 ANSI 输入、移动长按 / 跨行复制和剪贴板拒绝后的文本回退。端到端使用 DOM 渲染器断言文字，WebGL 由组件测试覆盖。

- **Worktree 验收**：`web/test/e2e/worktrees.spec.ts` 在临时真实 Git 仓库与状态目录中验证两工作树、多 PTY / 回放、dirty / ignored 保护、强制删除与分支保留、启动失败重试、移除登记、跨重启恢复清单、无 Git 直接启动、陈旧确认和旧响应竞态。后端 race 测试另覆盖存储失败 / 锁、NUL 路径、并发启动 / 删除、身份变化、关闭超时及子进程回收。

GitHub Actions 在 Ubuntu 与 macOS 上分别自动安装依赖和浏览器，运行构建及全部测试；失败保留截图、trace、video、服务日志与 PTY 缓冲，报告位于 `web/playwright-report`，附件位于 `web/test-results`。移动模拟不能替代 iOS Safari / Android 系统原生选择菜单与键盘的真机验证。

Vite 使用 `127.0.0.1:5173`，代理到 `127.0.0.1:7681`。**开发配置需** `allowed_origins = ["http://127.0.0.1:5173"]`，否则带 Origin 的写请求会被拒绝。生产不需要开发 Origin。

shadcn 配置位于 `web/components.json`，生成的基础组件位于 `web/src/lib/components/ui/`；业务组件只组合这些基础控件。增加库组件使用 `pnpm --dir web exec shadcn-svelte add <组件名>`，随后 `make format`。不要重新引入旧前端框架或手写替代基础弹层 / 菜单。

当前主题基于 [shadcn 预设 `b3FOWw2Iza`](https://www.shadcn-svelte.com/create?preset=b3FOWw2Iza)：Lyra 样式、Mist 底色、Green 主色、Cyan 图表色、无圆角、半透明菜单与 subtle 菜单强调，图标统一为 Tabler。正文、标题和控件使用自托管的 Inter（拉丁）与 Noto Sans SC（中文）；路径、输出预览和终端使用自托管的 Maple Mono NF CN。字体文件按 unicode-range 切片，首屏只预加载拉丁切片。明暗模式共用这套规则。迁移预设时先运行 `pnpm --dir web exec shadcn-svelte apply b3FOWw2Iza --yes`，再用 `add <已有组件名> --overwrite --yes` 重新生成已有基础组件；`apply` 不会自动替换这些组件。保留 `styles.css` 中的终端样式、无衬线正文 / 标题字体配置、等宽字体入口和零圆角比例映射，并校验桌面 / 移动端与明暗主题。

### OpenSpec

项目已按 [OpenSpec 官方初始化流程](https://openspec.dev/docs/setup) 接入 **1.13.0**，使用默认 `spec-driven` schema 和 core 工作流。CLI 是开发工具，不加入前端运行依赖；其他开发机器安装：

```sh
npm install -g @fission-ai/openspec@1.13.0
openspec --version
openspec list
```

`openspec/config.yaml` 配置项目上下文，新建规格文档使用简体中文，结构标题与 SHALL / MUST 保持英文。`openspec/specs/` 保存规格，`openspec/changes/` 保存在途变更；现有 `README.md` 与 `WebMux.md` 不迁移或删除。

在相应 AI 工具的聊天界面启动提案（不是终端命令）：

| 工具           | 提案入口                         | 工作流文件                                  |
| -------------- | -------------------------------- | ------------------------------------------- |
| omp / Oh My Pi | `/opsx-propose "变更需求"`       | `.omp/skills/`、`.omp/commands/`            |
| Claude Code    | `/opsx:propose "变更需求"`       | `.claude/skills/`、`.claude/commands/opsx/` |
| Codex          | `$openspec-propose` 加上变更需求 | `.agents/skills/`                           |

core 包含 explore、propose、apply、update、sync、archive 六个工作流。重启或重新加载 AI 工具以发现新命令；更新 CLI 后，在项目根目录运行 `openspec update` 刷新生成的工作流。验证已有规格和变更可运行 `openspec validate --all --strict --no-interactive`；刚初始化时尚无待验证内容。

## FAQ

**创建失败，找不到 claude / codex？** 检查服务用户的 PATH；systemd 与 launchd 都不会执行你的交互 shell 初始化文件。预设显示实际 argv，程序缺失不会被误报为目录不存在。

**断线后旧内容有一点乱码？** 字节环形缓冲可能从 UTF-8 / 转义序列中间开始。前端全量重放，首行偶发残片在设计允许范围内。

**关闭手机页面是否关闭会话？** 不会。只有子进程退出、手动关闭或服务退出会结束会话。

**两个设备的布局为何互相影响？** 两端共享同一 PTY，最新 resize 生效。不是独立终端视图或分屏。

**无 token 监听所有地址为何能启动？** 为遵循设计，不拒绝但打印醒目 WARN。请改为 VPN IP 并配置 token；默认仅 loopback。

**粘贴 / 复制没有权限？** HTTP 不是安全上下文，使用内置文本框回退或配置 HTTPS。复制全部是原始缓冲，可能包含 ANSI 转义序列。

**重启之后找不到会话？** 这是本版本的明确边界：无数据库、无守护进程分离、不与 tmux 互通。

## 验收记录（2026-10-14）

以下为本次实际运行结果，不把模拟浏览器视为真机验收。

| 项目             | 验证结果                                                                                                                                                |
| ---------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Go 检查          | `go test -race ./...`、`go vet ./...`、`golangci-lint run ./...` 通过，lint 0 issues                                                                    |
| 前端与格式       | Svelte / TypeScript 0 errors、0 warnings；Vite 构建仅有 chunk 体积提示（见 macOS 验收）；控制键测试、`make format-check` 通过                           |
| pnpm 与打包      | frozen lockfile 安装、`make build`、`make release` 通过；JS + CSS 合计约 255 kB gzip                                                                    |
| HTTP / WebSocket | 真实服务验证 token / Origin、创建、输入、resize、双客户端、回放、退出码与移除                                                                           |
| 持久会话         | 零客户端持续 610.026 秒，重连快照完整包含 60 轮 `claude --version` 真实输出及结束标记；随后浏览器实际接收全部 60 轮并显示回放                           |
| 状态与标题       | OSC 标题、预览更新通过；测试提示符 2048 ms、真实 Claude 目录权限确认 2283 ms 收到 waiting 事件                                                          |
| 进程清理         | 实际 Bash、Claude 和忽略 HUP 的 sleep 子进程，5128 ms 内全部退出，`ps` 确认无残留                                                                       |
| 内存上限         | 20 个运行中会话，每个缓冲填满 2 MiB，服务 RSS 75.9 MiB                                                                                                  |
| 桌面 UI          | Token 登录、创建目录、创建会话、终端输入、卡片菜单、关闭确认、保留退出输出、移除通过                                                                    |
| 移动视口 UI      | 390px Chromium：Drawer、目录补全、预设参数、中文多行粘贴、粘滞键、捏合与字体持久化通过；844×390 横屏自动 resize 到 102×13，无横向溢出                   |
| 真实 Claude TUI  | 方向键切换 No / Yes，Esc 正常取消；Ctrl+C 提示再次按键后退出，退出码 1                                                                                  |
| 前台恢复         | Chromium 触发 visibilitychange 后 4 ms 收到真实 WebSocket 快照，历史输出保留；非真实手机锁屏                                                            |
| Zod              | 实际 HTTP 响应通过解析；畸形会话事件被拒绝；损坏目录列表被拒绝，损坏字体设置回退                                                                        |
| 跨平台           | `make release` 生成 Linux / macOS amd64 / arm64 静态二进制；darwin-arm64 原生、darwin-amd64 经 Rosetta、linux-arm64 在 Debian 容器中运行 `version` 通过 |
| 干净 Linux       | Debian bookworm-slim 仅挂载发布二进制，无 Node / Go，验证内嵌 SPA、认证、真实 Bash PTY、输出与退出码                                                    |
| systemd          | `systemd-analyze --user verify deploy/webmux.service` 通过；未替换当前用户的常驻服务                                                                    |

**仍需真机环境**：iOS Safari / Android Chrome 的中文输入法、`--help` 自动替换行为、VPN 切换、系统键盘与锁屏解锁后 3 秒恢复；当前只确认了输入属性、视口处理和浏览器恢复机制。没有连接真实移动设备，不将这些条目标记为已通过。4G 首屏 1 秒目标也未在真实蜂窝网络测量。

Chrome 移动设备模拟器可能报告与 DPR 不一致的 `device-pixel-content-box` 尺寸，导致 WebGL 画布空白；现在会自动切换 DOM 渲染器。已在 412×924、DPR 2.625 的 Chromium 模拟中，通过现有真实 omp 会话观察到 logo、更新提示与终端提示符正常显示。该验证不代替实际 iOS GPU / Safari 真机验证。

### macOS 验收（2026-10-14，macOS 26.7.1 arm64）

| 项目            | 验证结果                                                                                                                                                                       |
| --------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------ |
| Go 检查         | `make test` 中 `go test -race ./...` 全部通过；`internal/session` 另以 darwin/amd64（Rosetta）通过；`make check` 通过，`golangci-lint` 在 darwin 与 `GOOS=linux` 下均 0 issues |
| Linux 回归      | `golang:1.23-bookworm` 容器内 `go vet ./...`、`go test -race ./...` 全部通过，含仅 Linux 的 subreaper 回收测试                                                                 |
| 前端与 E2E      | Vitest 单元 22 / 浏览器 21 通过；Playwright 桌面 + Pixel 7 共 25 通过、3 个仅移动端用例在桌面项目按设计跳过                                                                    |
| 打包与安装      | `make build`、`make release`、`make install HOME=<临时目录>`（BSD `install`）通过；darwin-arm64 `codesign -v` 通过                                                             |
| 真实服务        | 默认 shell 预设为 `/bin/zsh -l -i`；空闲阈值 3 秒时，`read` 阻塞的 Bash 判为 waiting，忙循环判为 idle；cwd 报告规范化真实路径                                                  |
| 进程清理        | 独立 job-control 组中忽略 HUP 的 `sleep` 在关闭后 5106 ms 退出（退出码 137），`ps` 无残留；适配前 macOS 只信号进程组，worktree 强制删除测试报告子进程存活                      |
| launchd         | `plutil -lint` 通过；以临时标签 `bootstrap` 模板后健康检查、zsh 会话与 `LANG` / `SHELL` 生效，`bootout` 后服务与会话进程全部退出；未替换当前用户的常驻代理                     |
| 已知差异 / 警告 | macOS 不公开其他进程环境，`WEBMUX_SESSION_ID` 无法用于清理（见 macOS 平台差异）；Vite 构建提示 `index` chunk（509 kB）超过 500 kB，与平台无关                                  |
