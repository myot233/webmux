# 连接、认证与启动会话

## A. 连接、认证与访问边界

- **US-A01 连接服务**：作为使用者，我希望在浏览器中连接自己的 WebMux，以便在不安装终端客户端的情况下操作服务器。
  验收要点：
  - 登录页、工作区、设置页与浏览器标签页使用统一的 WebMux 标志，明暗主题下均可辨认；
  - 打开页面后检查服务可用性；
  - 显示连接中、连接失败或可用状态；
  - 未设置 token 的服务可直接进入；
  - 设置页按需加载，显示加载状态，下载失败时提示刷新重试。
- **US-A02 共享 token 登录**：作为使用者，我希望输入并在本机记住共享 token，以便以后方便地访问自己的会话。
  验收要点：
  - 登录页在共享 Token 右侧提供问号提示，默认收起说明；
  - 鼠标悬停、键盘激活或手机点击可查看，指针可移入说明，Escape 或点击外部可关闭；
  - 说明涵盖共享密码用途、初始化生成与保存、配置路径及启动参数覆盖；
  - 配置路径独立成行、窄屏可换行且无横向溢出；
  - 启用认证后，正确 token 才能访问 HTTP API 与 WebSocket；HTTP 只接受 `Authorization: Bearer`，WebSocket 只接受 Bearer 或 token 子协议，`webmux_token` Cookie 与 URL 参数都不能认证；
  - 端口转发凭据 Cookie `webmux_forward` 独立于 token，只能访问 `/forward/` 网关，不能认证 `/api/` 与 `/ws/`，见 US-R01；
  - 当前浏览器保存 token；
  - 错误或变更后的 token 触发重新认证提示；
  - 本机套接字采用同用户身份校验，见 US-M08。
- **US-A03 可信网络访问**：作为部署者，我希望控制监听地址和允许的浏览器来源，以便只从预期的本机或可信 VPN 入口操作终端。
  验收要点：
  - 默认仅监听本机回环（127.0.0.1:7681），局域网或可信 VPN 访问可改为 0.0.0.0:7681；
  - 未显式指定配置且默认文件缺失时，首次启动生成并保存 256 位随机 token，非隧道启动不打印该 token，可经本机状态显式查询；
  - 已有配置的 token 为空且监听地址不是 loopback 时，每次启动生成临时随机 token；启动链接与二维码不携带 token，通过 `webmux status --json --reveal-token` 查看；
  - 只有 loopback 且未启用隧道时允许无 token；
  - 隧道启动链接与二维码同样不含 token，持久 token 不因重启失效；
  - 监听 loopback 时校验请求 Host（防止无 token 下的 DNS 重绑定），非 loopback 监听（强制要求 token）接受任意合法 Host（如局域网主机名或 Tailscale MagicDNS），并始终校验 Origin（同源、allowed_origins 或受控公网来源）；
  - 认证和来源检查同时覆盖终端与文件、仓库、代码接口。
- **US-A04 查看服务与能力**：作为部署者，我希望知道服务是否在线、会话数量及 Git/仓库能力是否可用，以便定位连接或环境问题。
  验收要点：
  - 健康检查返回可用状态和会话数；
  - 配置接口提供默认目录、主机名和工作区能力，不返回 token；
  - 首页展示连接与能力异常。
  - 侧栏底部的主机状态入口可点击查看主机名、连接状态、当前传输路径（`WebSocket` 或含候选类型与 IP 族的 `P2P 直连`，见 US-H12）、往返延时（直连生效时优先采用 P2P 控制通道 RTT，否则采用健康检查 RTT）、系统负载（1/5/15 分钟与核心数）和可用/总内存；
  - 往返延时高于 200 毫秒，或 1 分钟负载高于核心数的 50% 时，状态灯变黄并在提示中说明原因；平台无法提供负载或内存时对应行不显示。
- **US-A05 脚本访问**：作为进阶使用者，我希望通过 REST 和 WebSocket 操作会话，以便接入自己的脚本或客户端。
  验收要点：
  - 提供会话、配置、目录、上传、仓库与代码（含文件操作）接口；
  - 列表和终端分别提供实时流；
  - 遵循相同认证、错误和生命周期边界，详见 [协议](../../protocol.md)。
- **US-A06 带凭据链接登录**：作为使用者，我希望打开带 token 片段的访问链接就能登录，以便从终端或托盘把入口带到手机。
  验收要点：
  - 前端读取并保存 `#token=` 后立即移除地址栏和当前历史记录中的片段；
  - 片段不发送到服务器；
  - WebSocket 认证通过子协议携带 token，不放入请求 URL 或 Cookie；
  - 链接仍是敏感入口。

依据：

- [统一品牌标识](../../../web/src/components/shell/Brand.svelte)、[品牌图标](../../../web/public/icon.svg)、[浏览器图标入口](../../../web/index.html)
- [应用入口](../../../web/src/App.svelte)、[Token 说明](../../../web/src/components/shared/TokenHelp.svelte)、[配置命令](../../../cmd/webmux/configcmd.go)
- [启动与 token 生成](../../../cmd/webmux/main.go)、[API 路由与安全检查](../../../internal/api/router.go)、[Host 校验](../../../internal/api/host.go)
- [安全说明](../../../SECURITY.md)。

## B. 创建会话与选择启动位置

- **US-B01 启动 Shell 或 agent**：作为使用者，我希望通过自定义启动配置或显式命令运行 Shell、Open Code、Claude Code、Codex、omp 等程序，以便快速开始终端工作或遥控 agent。
  验收要点：
  - 新建会话对话框以“位置”与“运行”两个选择行组成一页，“运行”在同一列表中提供保存的启动配置与末尾的“自定义命令…”；新安装预置常见 agent 的启动配置，没有不可修改的内置预设；
  - Claude Code、Codex CLI、Antigravity CLI、Open Code、Qoder、QoderCN、Kimi 与 omp 的预置配置默认自动批准工具调用：
    - Claude Code `--dangerously-skip-permissions`；
    - Codex CLI `--dangerously-bypass-approvals-and-sandbox`；
    - Antigravity CLI `--dangerously-skip-permissions`；
    - Open Code `--auto`；
    - Qoder `--dangerously-skip-permissions`；
    - QoderCN `--dangerously-skip-permissions`；
    - Kimi `--auto`；
    - omp `--auto-approve`；
  - Cursor、Copilot、Qwen、CodeBuddy 使用各自默认授权行为；
  - 十一款 CLI 的启动命令见 [接入说明](../../cli-agents.md)；
  - Claude Code、Codex CLI、Antigravity CLI、Open Code、omp、Qoder、QoderCN 与 Kimi 预置配置默认为混合模式（US-Q01）；
  - 命令选择器可用键盘操作（Enter 打开、方向键与搜索、Esc 只关闭选择器），选中行和每个选项同时显示图标、名称与命令，长命令保留完整提示；
  - 保存的配置按 ID 启动，临时命令按完整 argv 启动，均创建真实 PTY；
  - Open Code 可通过“加载 shell 启动文件”保存为 Bash（`~/.bashrc`）或 Zsh（`~/.zshrc`）包装，新安装按服务端 `$SHELL` 预选，其他 shell 回退为不包装或手动选择，不自动安装程序；
  - 勾选了混合模式的配置可选择从终端或聊天开始（US-Q02）；
  - 首页与工作树统一使用“新建会话”，分屏菜单复用同一对话框；
  - 无论哪种启动方式，对话框都有可选的“用户指令”，创建后自动发送：
    - 聊天端作为第一条消息；终端端若所选混合模式 agent 已支持命令行初始提示参数（且启动命令本身未带初始提示），通过首次 TUI 启动参数直接传入（不写入 `SessionInfo.Cmd` 或持久化记录），尚未迁移的 agent、不支持该消息形式的边界情况与自定义命令仍在程序输出安静后输入并自动回车；
    - 未填会话名称的终端启动以该指令为会话名（US-B07）；
    - 位置属于已登记仓库且该仓库设置了初始化指令时，另有“使用初始化指令”（从聊天开始时默认勾选，从终端开始时默认不勾选），指令先于用户指令发送（模板规则见 US-J12）；
    - 引用无效、临时文件夹不可用等发送前即可发现的问题使创建失败并保留输入，不留下半启动的会话；
  - 底部汇总“将在 X 运行 Y”，默认值齐全时打开即可直接创建；会话名称收在“高级：会话名称”中；
  - 创建成功后按入口进入终端或加入当前布局。
- **US-B02 自定义命令与参数**：作为进阶使用者，我希望填写完整命令及参数，或选用已保存的配置，以便运行自己的 CLI 工作流。
  验收要点：
  - 在启动配置与“自定义命令…”之间切换时保留手动输入和已选配置，但只提交当前方式；
  - 空手动命令不能误用保存配置；
  - 正确保留引号、空参数与参数边界，不进行 shell 展开；
  - 未闭合引号或空可执行程序阻止提交；
  - `configId` 与非空 `cmd`、`env` 互斥；
  - 临时命令不落盘，保存配置不可用或仍在加载时可改选“自定义命令…”。
- **US-B03 选择直接目录**：作为使用者，我希望直接指定任意可访问目录并复用最近使用的位置，以便启动普通项目而不必先登记 Git 仓库。
  验收要点：
  - “位置”选择器在一处列出主页当前仓库的工作树、最近目录、其他仓库的工作树与“输入或浏览目录…”，选中直接目录时显示可编辑的工作目录输入与浏览按钮；
  - 从首页的“新建会话”打开时范围收窄：选中仓库只列该仓库的工作树与“新建工作树…”；选中工作树（或点工作树行的“新建会话”）只列该工作树与“从当前工作树分支新建…”，起始分支默认为该工作树的分支；选中“全部仓库”、“直接会话”或从分屏、会话页打开时不收窄；
  - 默认位置优先级：上下文入口（工作树的“新建会话”、分屏源会话目录）> 主页选中仓库的可用主工作区（否则该仓库其他可用工作树）> 最近目录 > 服务默认目录；
  - 主页选中仓库但没有可用工作区时保留仓库并要求选择，不回退到其他项目；对话框内改选位置不改变主页选中的仓库；
  - 保留最近 10 个启动目录，并在成功创建后记住最近一次启动方案（启动配置及混合起始端，或临时命令），下次打开直接填入，对所有位置通用；
  - 记住的配置已删除或不可用时显示“选择启动方式”并禁止提交，不隐式启动另一配置；创建失败不覆盖记忆；
  - 分屏以源会话的完整启动目录覆盖最近目录，目录未改时保留其工作区归属。
- **US-B04 浏览目录**：作为使用者，我希望逐层浏览和补全服务器目录，以便在手机上也能准确选中启动位置。
  验收要点：
  - 支持路径筛选、主目录、上一级、显示隐藏目录；
  - 支持方向键、Enter/Tab 进入和 Alt+↑ 返回；
  - 显示读取错误并能选择当前目录。
- **US-B05 创建缺失目录**：作为使用者，我希望在目录不存在时确认创建再启动，以便直接为新任务准备工作位置。
  验收要点：
  - 直接目录启动失败且目录不存在时才提示创建；
  - 确认后创建目录并重试；
  - 不因此登记仓库。
- **US-B06 在已有或新工作树启动**：作为使用者，我希望从已有 worktree 启动，或先创建 worktree 再启动 agent，以便在指定的代码副本中工作。
  验收要点：
  - 在“位置”选择器中直接选择已有工作树（显示分支与路径），或在对应仓库分组中选择“新建工作树…”；
  - 不可用仓库、不可用或 bare 工作树显示但禁用；
  - 新工作树在位置行下方展开：“创建新分支 / 使用已有分支”、起始分支与新分支名称，起始提交与存放目录收在“高级：起始提交与存放目录”，位置行摘要显示“新分支 ← 起始分支”；改选其他位置再选回同一仓库的“新建工作树…”时保留已填写内容；
  - 新工作树成功后可启动会话或仅保留工作树；
  - agent 启动失败时保留新工作树与分支，位置自动变为该工作树，重试不重复创建；改选其他位置后仍可在列表中选回它；
  - 侧栏仓库菜单的“新建工作树…”默认只创建工作树，勾选“创建后启动会话”才显示命令并启动；未勾选时改选已有工作树或目录会禁用“创建工作树”并提示，不会无操作地关闭；
  - 启动配置不可用不阻止单独创建工作树。
- **US-B07 命名会话**：作为使用者，我希望给新会话填写名称，以便在多个相似终端中识别任务。
  验收要点：
  - 创建名称优先于首条提示标题与程序标题（终端 OSC 0/2、聊天端 ACP `session_info_update`）；
  - 未命名且从终端视图启动、填写了用户指令时，以该指令为名（折叠空白，最多 80 个字符，超出加“…”；不含初始化指令，去掉 `{{dir}}`、`{{webmux}}` 引用，普通 Shell 命令同样适用），之后程序标题不再覆盖；名称输入框的占位提示“留空则用首条提示，没有则跟随终端标题”；
  - 没有用户指令（含只有初始化指令）时使用程序标题，再回退到目录名；聊天端从首条消息取临时标题，见 US-Q02；
  - 首页、终端、聊天页头与浏览器标签使用会话标题；终端或 agent 标题中的 Nerd Font 私有区图标在首页、侧栏、会话行和页头等界面沿用内置图标的字形显示。
- **US-B08 管理启动配置**：作为使用者，我希望把常用的命令、参数和环境变量保存为服务端启动配置，以便在任何设备上一键选用而不必重复输入。
  验收要点：
  - 设置页的启动配置以不带边框的行列表呈现（行高不低于 44px）：图标、名称、一行摘要（终端或混合会话、环境变量数、预览行数；完整命令只在编辑对话框中显示），整行点击编辑，行尾“⋯”菜单提供编辑与删除；
  - 设置页可新建、编辑、删除自定义启动配置，并选择自动匹配、通用终端或 LobeHub 品牌图标；配置最多 100 条且文件总大小不超过 4 MiB，超出时保存被拒绝并提示精简（400），其余配置与功能不受影响；
  - 图标随配置持久保存并在设置列表、新建会话选择器中显示，缺少图标的旧配置按可识别的直接命令或标准 Bash/Zsh 包装自动匹配，不改写原配置；
  - “加载 shell 启动文件”复选框将普通命令安全包裹为所选 Bash 或 Zsh 的交互式 argv，保留空参数、空格和字面量 shell 字符；
  - 重新编辑还原普通命令并保持勾选与所选 shell，保存不重复包裹，取消勾选恢复直接执行；
  - 新建时按服务端只读 `defaultShell` 预选，空值回退 Bash；
  - 包装参数计入 argv 上限；
  - 可为每条配置选择跟随全局默认或单独设置输出预览行数与跳过末尾行数；
  - 可为每条配置选择跟随全局默认或单独设置“完成与介入提醒”：
    - 支持设置提醒范围（`all`/`urgent`/`off`）、四类信号源开关（`OSC 7501`、Warp Agent 事件、`OSC 9·777` 通知、BEL 响铃）、最短回合耗时（`0–300` 秒）与完成确认延迟（`0–60` 秒）；
    - 编辑器显式说明信号源优先级（`OSC 7501 > Agent 事件 > 桌面通知 > 终端响铃`），配置列表摘要显示“仅需介入”“提醒已关闭”或“自定义提醒”；
  - 新安装首次启动时自动写入 Shell、十一款 CLI agent 与 omp 共 13 条带图标的启动配置（完整列表见 [接入说明](../../cli-agents.md)），其中 omp 附带 `PI_FORCE_IMAGE_PROTOCOL=kitty` 以启用 US-D12 图片显示；
    - Shell 为通用终端，agent 使用本地打包的 LobeHub 图标（Antigravity CLI 为 `antigravity`，同时保留旧 `geminicli` 兼容渲染），omp 使用 Pi；
    - agent 预设按服务端 `$SHELL` 经“加载 shell 启动文件”包裹（Bash 或 Zsh，未设置时 Bash，其他 shell 直接执行）；
    - 各命令的授权参数见接入说明，Claude Code、Codex CLI、Antigravity CLI、Open Code、omp、Qoder、QoderCN 与 Kimi 为混合模式，可编辑或删除；
  - 文件已存在时启动服务（包括用户编辑或删空后）不再自动补写或覆盖，但可在设置页「新建启动配置」旁的「添加系统预设」菜单中补回单个或全部缺失的系统预设（已有同名配置时菜单项置灰并标记“已存在”，不覆盖用户已修改的配置）；
  - 配置跨重启与设备共享，环境变量值默认遮挡；
  - 按配置 ID 启动，修改或删除配置不影响运行中会话的命令与环境，会话信息保留配置 ID 供首页解析预览；
  - 存储不可用时显示原因，临时命令仍可用。编辑器可勾选混合模式，可用性由服务端判定并显示原因，列表显示“混合”标记（US-Q01）。
- **US-B09 启动 agent 时不卡在“信任此文件夹”**：作为使用者，我希望 Claude Code、Codex、GitHub Copilot CLI、Antigravity CLI、Qoder、QoderCN 和 Kimi 在新目录启动时不停在信任对话框，以便无人看着的会话也能直接开始工作。
  验收要点：
  - 默认开启 `agent_pretrust`；新建会话的命令被识别为这七款 CLI（直接执行、绝对路径、版本化二进制名或 Bash/Zsh 启动文件包装）时，先把启动目录记入该 CLI 自己的配置，或为 Kimi 注入进程级目录信任，再启动进程；
  - 写入位置优先跟随会话环境变量，其次是服务进程的同名变量，最后回退到主目录下的默认位置：
    - Claude Code：`CLAUDE_CONFIG_DIR` 或 `~/.claude.json`；
    - Codex：`CODEX_HOME` 或 `~/.codex/config.toml`；
    - GitHub Copilot CLI：`COPILOT_HOME` 或 `~/.copilot/config.json`；
    - Antigravity CLI：固定写主目录下的 `~/.gemini/antigravity-cli/settings.json`（`trustedWorkspaces`）；
    - Qoder 与 QoderCN：分别跟随 `QODER_CONFIG_DIR`（默认 `~/.qoder/settings.json`）与 `QODERCN_CONFIG_DIR`（默认 `~/.qoder-cn/settings.json`），写入 `permissions.trustDirectories`（不跟随命令行 `--config-dir`）；
  - Kimi 不写持久配置，只在会话环境补充 `KIMI_CODE_TRUST_WORKSPACE=1`；恢复 TUI 与 ACP 继承同一环境，会话或服务已显式设置该变量（包括 falsy 值）时不覆盖；
  - 已有配置的其他内容与权限保持不变；已经信任时不写；Codex 保留注释且不改写用户显式设为 `untrusted` 的目录；
  - Claude Code、Copilot、Antigravity CLI、Qoder 与 QoderCN 的配置文件缺失、或无法解析时不创建、不改写；Codex 只在 `~/.codex` 已存在时创建 `config.toml`；
  - 主目录、其上级目录与文件系统根永不写入或注入；目录先解析符号链接；
  - 任何失败只记录日志，会话照常启动；`agent_pretrust = false` 时不写任何文件，也不注入 Kimi trust 环境；
  - 其他 CLI 与已运行会话不受影响，说明与限制见 [CLI agent 接入](../../cli-agents.md#文件夹信任预写)。

依据：

- [启动图标](../../../web/src/components/settings/LaunchConfigIcon.svelte)、[LobeHub 图标来源与许可](../../../web/src/icons/lobehub/README.md)
- [新建会话](../../../web/src/components/sessions/NewSessionDialog.svelte)、[位置选择](../../../web/src/components/workspace/LocationPicker.svelte)、[命令选择](../../../web/src/components/sessions/LaunchPicker.svelte)、[默认值解析](../../../web/src/domains/sessions/new-session-defaults.ts)
- [启动配置编辑](../../../web/src/components/settings/LaunchConfigDialog.svelte)、[系统预设菜单](../../../web/src/components/settings/LaunchPresetMenu.svelte)、[启动配置存储](../../../internal/launch/store.go)、[首次启动预设](../../../internal/launch/presets.go)
- [启动配置接口](../../../internal/api/launch.go)、[CLI agent 接入](../../cli-agents.md)
- [信任预写](../../../internal/agenttrust/agenttrust.go)、[会话钩子](../../../internal/session/manager.go)
- [会话标题来源](../../../internal/hybrid/actor.go)、[会话覆盖层](../../../internal/session/lifecycle.go)、[聊天页头](../../../web/src/hooks/sessions/hybrid.svelte.ts)
- [目录选择](../../../web/src/components/workspace/DirectoryPicker.svelte)、[命令解析](../../../internal/session/run_command.go)、[工作区会话创建](../../../internal/workspace/sessions.go)。

来源补充：[统一创建回归](../../../web/test/e2e/session-creation.spec.ts)。
