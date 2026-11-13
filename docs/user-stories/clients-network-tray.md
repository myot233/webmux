# 原生客户端、隧道与托盘

## M. 原生终端、CLI 与本机控制面

- **US-M01 从本地终端启动**（脚本用法与启动增强见 US-S07）：作为终端使用者，我希望用 `webmux run -- COMMAND ARG...` 创建并附着会话，或引用 Web 端保存的启动配置，以便保留原来的 CLI 工作习惯并随时转到浏览器。
  验收要点：
  - `--` 后 argv 原样传递；
  - `--launch-config NAME`（`--config-id` 别名）按 Web 端显示的唯一名称选择已保存配置，以服务端保存的 argv 与环境变量创建，名称不存在时以用法错误退出并列出已有名称，且与命令、`--env` 互斥；
  - 默认当前目录，可指定名称与目录；
  - `--detach` 只创建并输出会话 ID；需要入口链接时用 `webmux open ID` 取得不含 token 的本机链接；
  - 默认附着前向 stderr 公告会话 ID 与分离键提示，stdout 保留给会话；
  - 非 TTY 的附着请求在创建前拒绝；
  - 会话进程默认看到 WebMux 的虚拟终端身份（`TERM_PROGRAM=webmux` 及服务版本），不会继承 Kitty 等宿主终端身份，启动配置显式提供的终端变量仍可覆盖默认值。引用勾选了混合模式的配置时创建从终端开始的混合会话（US-Q13）。
- **US-M02 显式传递运行环境**：作为进阶使用者，我希望指定启动目录和必要环境变量，以便让程序使用正确项目与工具环境。
  验收要点：
  - `--cwd` 使用服务器目录规则；
  - Linux/macOS 的 `webmux run -- COMMAND` 默认继承调用方 shell 的环境（跳过 `TERM`、宿主终端身份、`WEBMUX_SESSION_ID`、`PWD`/`OLDPWD` 与不合法名称），重复 `--env NAME=VALUE` 在其上覆盖；
  - Windows 客户端与启动配置不继承调用方环境；
  - 未发送的变量来自服务环境；
  - 普通网页创建与本机 CLI 的环境传递权限按接口约定处理。
- **US-M03 附着与观察**：作为使用者，我希望 `webmux attach ID` 回到已有 PTY，或只观察，以便在手机和原生终端间接续同一任务。
  验收要点：
  - 初次附着完整回放并进入原始终端模式；
  - 默认取得控制并同步尺寸，`--observe` 不输入或改尺寸；
  - 程序已开启明暗报告（DEC 2031）时，取得控制后按本地终端背景色（OSC 11）报告明暗，使程序改用本地终端配色；
  - 会话退出时返回其退出码；
  - 服务未运行返回 3，不自动拉起服务；
  - 结束后恢复控制台状态；
  - 取消、会话退出或断线会中断无输入时的终端读取，`Run` 返回前等待输入、输出与尺寸协程退出，不关闭 stdin，也不终止持久会话；
  - Unix 的 poll/read 遇到信号中断时重试，不误判为终端关闭。混合会话在聊天视图或切换中拒绝附着并以 1 退出；
  - 附着期间被切到聊天视为被动分离，以 0 退出（US-Q13）。
- **US-M04 分离与重新接管**：作为原生终端使用者，我希望浏览器接管后本地按键不会误抢控制，并可明确接管或分离，以便安全切换设备。
  验收要点：
  - 被接管后显示控制提示，丢弃普通键及终端自动应答；
  - Enter 仅接管、不发给会话；
  - Ctrl+] 分离且进程继续运行，返回 0；
  - 兼容 modifyOtherKeys、kitty 与 Windows 编码按键，退出时关闭临时上报模式。
- **US-M05 命令行发现与打开会话**：作为使用者，我希望用 `ls` 列出会话并用 `open ID` 获取入口，以便不经过首页就能找到目标。
  验收要点：
  - `ls` 显示 ID、名称、状态、命令和目录，`--json` 输出原始会话数组；
  - `open` 校验目标并打印不含 token 的本机链接，`--browser` 可打开默认浏览器。`ls` 为混合会话在状态列附加所在视图，`--json` 原样包含 `views`、`view`、`transition` 与 `agent`，不含 `hybrid`（US-Q13）。
  - `ls` 可按标签、父会话与状态筛选，`--state`/`--archived`/`--repo` 列出已归档会话（US-S15），`open` 接受 `self`（US-S01、US-S03）；`run`、`attach`、`ls`、`open` 的 `--help` 写到标准输出并以 0 退出（US-S04）。
- **US-M06 诊断运行实例**：作为部署者，我希望用 `status` 看到健康状态、访问地址和配置漂移，以便确认正在使用哪个实例。
  验收要点：
  - 优先本机 API，必要时使用运行时记录与 TCP 健康检查；
  - 区分运行、未运行、无响应或错误并给出退出码；
  - `--json` 提供版本化结果，只有同时 `--reveal-token` 才揭示 token；
  - 提示 listen/token 配置与运行实例的差异；
  - 经本机 API 时在 `urls.access` 与文本 `Access:` 行列出服务端托管且已就绪的远程访问入口（Cloudflare Tunnel、Tailscale Serve/Funnel），不含 token。
- **US-M07 停止指定实例**：作为部署者，我希望可靠地停止自己启动的服务，以便清理后台运行实例。
  验收要点：
  - `stop` 优先经本机控制面关闭，必要时验证运行记录后退回 SIGTERM 和超时 SIGKILL；
  - 无记录不随意发信号；
  - systemd/launchd 实例默认要求使用服务管理命令，显式 `--force` 可覆盖；
  - 支持超时与 JSON 结果。
- **US-M08 本机桥接与权限边界**：作为本机使用者，我希望 CLI 和 Windows 桥接无需维护网络 token，以便直接访问当前用户的服务。
  验收要点：
  - Unix 套接字验证同用户身份，`bridge` 在标准输入输出间传输；
  - 浏览器网络入口仍要求认证；
  - 会话内进程不能用本机管理 API 读取 token、停止服务或控制隧道，也不能经套接字读取或修改 `/api/tunnel`、`/api/p2p` 及其子路径（403）；
  - 本机状态 `/local/v1/status` 汇总当前隧道、Tailscale 与 P2P 直连运行状态（开关、端口映射与活动直连对端数），不含敏感凭据；
  - 本机客户端忽略成功响应内容时，仍报告响应读取失败；非成功响应统一保留 HTTP 状态码与最多 64 KiB 错误体，供命令处理结构化错误；
  - 套接字不是不同账号间的共享授权机制。

依据：

- [会话命令](../../internal/clicmd/clicmd.go)、[会话环境](../../internal/session/env.go)、[会话管理](../../internal/session/manager.go)
- [服务启动](../../cmd/webmux/main.go)、[原生附着](../../internal/attach/)、[控制命令](../../cmd/webmux/control.go)
- [本机 API](../../internal/api/local.go)、[运行时记录](../../internal/runtime/)、[环境与本机请求](../../cmd/webmux/sessions.go)。
- [本机客户端](../../internal/localclient/client.go)、[响应处理回归](../../internal/localclient/response_test.go)。

## N. Cloudflare 公网隧道

- **US-N01 临时公网入口**：作为明确选择公网访问的部署者，我希望启用 Quick Tunnel，以便临时从无法接入 VPN 的设备连接自己的终端。
  验收要点：
  - 可在 Web 设置页保存 Quick 模式并确认启用，也可用 `--tunnel` 或 `[tunnel].enabled`；
  - 无需 Cloudflare 账号，获得随机 HTTPS 地址，不承诺固定地址或 SLA；
  - 未启用时不创建隧道。Web 设置在服务器 `<state_dir>/tunnel-settings.json` 原子保存（目录 0700、文件 0600、独占锁），跨浏览器与重启生效并优先于 TOML 的模式、主机名、凭据及开关；
  - 显式 `--tunnel` 只覆盖当前进程开关，不改写磁盘。
- **US-N02 固定域名隧道**：作为已有 Cloudflare 配置的部署者，我希望使用 Named Tunnel，以便通过预设的公共主机名访问。
  验收要点：
  - Web 页可保存 Named 模式、裸主机名及写入式隧道 token，并在 Cloudflare 配置回源；
  - 也支持 TOML 配置。省略 token 保留已有凭据，首次配置或显式空凭据不可保存 Named；
  - 切回 Quick 清除主机名与凭据；
  - 凭据不回显、不放入 URL 或浏览器持久存储，只显示是否已配置，经环境变量而非命令行传给 cloudflared；
  - 指定空配置避免本机其他 cloudflared 配置干扰。
- **US-N03 隧道认证与扫码**：作为部署者，我希望公网入口始终要求足够强的 token，并能扫码连接，以便明确控制入口凭据。
  验收要点：
  - WebMux 登录 token 与 Cloudflare 隧道 token 不同；
  - 首次启动可生成并持久化 256 位随机登录 token，已有配置缺少登录 token 时生成本次进程的随机值；
  - 运行中启用要求当前登录 token 至少 43 字符，不生成或更换登录 token，条件不足提示修复后重启；
  - 显式 CLI/TOML 隧道启动仍校验登录 token。所有启动链接与二维码均不含 token，避免凭据进入 journal 或日志文件；通过 `webmux status --json --reveal-token` 查看当前 token；
  - 持久化 token 重启保留，仅临时 token 重启更换；托盘复制链接仍可当场生成带 token 片段，并按敏感凭据处理剪贴板与浏览历史。
- **US-N04 获取 cloudflared**：作为部署者，我希望自动定位或下载匹配的 cloudflared，以便不用手工管理隧道依赖。
  验收要点：
  - 按显式路径、PATH、状态缓存定位，不存在时下载固定版本；
  - 支持有序下载镜像，空镜像列表禁用回退；
  - 各来源均校验内置 SHA-256 后执行；
  - 定位或校验错误明确报出；
  - 路径和下载镜像等运维项仍由 TOML 管理，不由 Web 设置覆盖。
- **US-N05 隧道故障隔离**：作为使用者，我希望隧道断开时本机会话仍继续，以便恢复远程入口后继续任务。
  验收要点：
  - 隧道退出立即撤销对应公网来源授权，本地访问和 PTY 保留，不自动重试；
  - Web 保存的启用状态在启动时遇到短登录 token、依赖下载或进程启动失败，只把隧道置为 failed，HTTP/本机 API 继续可用；
  - 设置文件损坏、不可读或锁冲突时保持原文件并禁止公网启动，不回退到 TOML/CLI 开启；
  - 关闭操作即使落盘失败也立即停止当前进程的隧道并撤销授权，返回 503 与 `notPersisted: true`，明确提醒修复存储后再重启，不能声称下次仍关闭；
  - 正常退出关闭隧道而不改持久开关。
- **US-N06 在线切换隧道**：作为浏览器或托盘使用者，我希望服务运行中开启或关闭公网隧道，以便控制入口而不中断会话。
  验收要点：
  - Web 页与本机控制面共用持久化开关，启用前确认，要求已有至少 43 字符登录 token；
  - 成功保存后异步应用，无需重启，重复启用不产生第二个 cloudflared；
  - 更换已启用的模式、主机名或凭据先撤销旧授权并停止旧进程，再启动新配置，迟到事件不能恢复旧入口；
  - 普通保存/启用落盘失败不应用新设置，关闭例外见 US-N05。托盘仅在 `settingsAvailable: true` 时提供 Web 设置入口；
  - 勾选以可选 `enabled` 表示的当前进程期望（含 CLI 覆盖）为准，不等于磁盘开关或 ready，旧服务缺该字段时回退到 starting/ready。
- **US-N07 配置说明与指南**：作为不熟悉 Cloudflare 的使用者，我希望在设置页和文档中按步骤完成 Named Tunnel 配置，以便独立获得固定域名入口。
  验收要点：
  - 设置入口打开 `/settings/tunnel`，区分 Quick/Named，显示配置来源、凭据是否已配置、启用前提与存储问题；
  - 保存与启用分开，提供 Named 配置步骤、文档链接和写入式凭据框，不读取真实凭据；
  - 恢复配置文件默认值前确认，成功后删除 Web 覆盖并停止当前隧道，不立即按 TOML 自动开启，下次启动才遵循 TOML；
  - 恢复失败不假装成功；
  - 文档覆盖 Windows/WSL、验证、排错与安全。
- **US-N08 查看隧道运行状态**：作为部署者，我希望在设置页看到当前公网隧道的运行状态、模式与主机名，以便确认隧道是否启用时不必查看托盘或服务日志。
  验收要点：
  - 独立显示当前进程期望 `enabled` 与 off/starting/ready/failed，启动失败仍可保留启用期望，不能把它当成可用公网入口；
  - 状态轮询反映托盘和其他设备变化，ready 才显示无 token 公网地址；
  - 只读 `/api/config` 的旧 `tunnel.enabled` 仍仅表示 starting/ready，模式和主机名动态跟随有效设置，推送资格按当前有效 HTTPS 入口（含 Named Tunnel、Tailscale Serve/Funnel）与本机回环判定；
  - GET 设置即使存储损坏仍可读状态及错误，不能启用时给出原因；
  - 接口不返回 Cloudflare 凭据，桌面和移动布局无横向溢出。

依据：

- [隧道控制](../../cmd/webmux/tunnelctl.go)、[隧道令牌与启动](../../cmd/webmux/main.go)、[持久设置](../../internal/tunnelsettings/store.go)
- [设置契约](../../internal/contract/tunnel.go)、[设置 API](../../internal/api/tunnel.go)、[设置 API 用例](../../internal/api/tunnel_test.go)
- [启动与故障用例](../../cmd/webmux/tunnelsettings_test.go)、[本机权限用例](../../internal/api/local_test.go)
- [入口与二维码](../../cmd/webmux/access.go)、[隧道进程与下载](../../internal/tunnel/)、[公网来源授权](../../internal/api/public.go)
- [动态配置](../../internal/api/config.go)、[推送来源](../../internal/api/push.go)、[Web 设置页](../../web/src/pages/TunnelSettingsPage.svelte)
- [设置状态管理](../../web/src/hooks/settings/tunnel-settings.svelte.ts)、[设置端到端用例](../../web/test/e2e/tunnel-settings.spec.ts)
- [Named Tunnel 指南](../cloudflare-tunnel.md)、[部署说明](../deployment/remote-access.md#公网访问cloudflare-tunnel)。

## O. Windows/WSL 托盘管理

- **US-O01 安装或升级 WSL 服务**：作为 Windows 使用者，我希望从托盘安装或升级 WSL 中的 WebMux，并在开启自动更新时让较旧的服务自动跟上托盘，以便减少手工下载步骤。
  验收要点：
  - WSL 中的 webmux 缺失、缺少本机 API、控制契约过旧或版本低于托盘时提供入口；
  - WSL 2 与发行版可用时选择匹配架构和托盘版本；
  - 校验发布时嵌入的服务摘要才安装；
  - 托盘下载缓存中已有同版本且摘要匹配的文件（包括 Windows 安装程序预置的服务端）时直接安装、不联网，摘要不符时丢弃并重新下载；
  - 支持 GitHub 与镜像回退；开发版托盘从 `dev` Release 下载同一次构建的带版本号服务端，已被删除时报告“开发版 <版本> 的服务端已不在 dev Release，请先更新托盘”且不改动 WSL；
  - 缺失时从不在后台安装；
  - 发布构建且开启“自动更新”时，检查发现较旧或契约过旧（含缺少本机 API）即自动升级，失败在状态行显示“自动升级 WSL 中的 webmux 失败：…”且到下次检查时机才重试，过新或无法判断时不自动改写；
  - 普通本地托盘构建无摘要时不提供安装；
  - 运行中的旧服务到重启才更新。
- **US-O02 启动方式与恢复**：作为 Windows 使用者，我希望托盘管理 WebMux 的启动和故障恢复，以便无需一直开着 WSL 窗口。
  验收要点：
  - 可选 auto、tray、systemd；
  - auto 优先已有可用用户服务，否则托盘托管；
  - 按方式保持 WSL 存活；
  - 托盘托管意外退出后退避重启，10 分钟内最多 5 次；
  - systemd 重启策略交给单元。
- **US-O03 管理所属实例**：作为 Windows 使用者，我希望托盘区分自己管理的服务和外部实例，以便不误停其他终端启动的工作。
  验收要点：
  - 可启动、停止、重启所属实例；
  - 有会话的停止、重启或切换目标先确认；
  - 外部或启动方式不符的实例只提供访问操作；
  - 不擅自接管停止权限。
- **US-O04 登录启动与单实例**：作为 Windows 使用者，我希望登录后出现一个托盘实例，以便随时找到服务入口。
  验收要点：
  - 同一 Windows 用户只运行一个托盘，再次启动通知已有实例；
  - 可选择注册当前 exe 为登录启动；
  - exe 移动后需重新登记；
  - 安装程序可登记同一启动项，复选项随之显示为已勾选；
  - `webmux-tray.exe quit` 请求已运行的托盘按“仅退出托盘”退出且不询问，退出码 0 表示已退出、1 表示仍在运行、2 表示参数错误、3 表示没有运行中的托盘，`--kill-path` 超时后只结束当前用户从该路径运行的托盘；
  - 无需以管理员启动托盘。
- **US-O05 选择发行版与编辑设置**：作为 Windows 使用者，我希望选择 WSL 发行版、启动方式和路径，以便使用自己配置的开发环境。
  验收要点：
  - 设置写入 `%APPDATA%\WebMux\tray.json`，保存后自动重载；
  - 无效文件提示并使用默认值，不覆盖文件；
  - 提供打开 WebMux 配置与托盘设置入口；
  - 配置缺失才提供初始化，已有配置不被自动改写。
- **US-O06 查看状态和日志**：作为 Windows 使用者，我希望托盘图标、菜单和日志解释运行问题，以便判断该重启还是修正配置。
  验收要点：
  - 区分运行、启动、停止和错误；
  - 提示 WSL 缺失、版本不兼容（见 US-O11）、配置及健康错误；
  - 可打开服务日志或 systemd journal 和托盘日志；
  - 托盘日志轮转且不记录 token。
- **US-O07 诊断 Windows 可达性**：作为 Windows 使用者，我希望知道 WSL 内服务健康但 Windows 仍无法连接的情况，以便定位网络模式或 localhost 转发问题。
  验收要点：
  - 独立从 Windows 回环地址检查可达性，不带 token 且不走系统代理；
  - 故障时提示 localhostForwarding/networkingMode；
  - 不把 WSL 内健康等同于 Windows 浏览器可达。
- **US-O08 允许局域网访问与转发可信远程地址**：作为 Windows 使用者，我希望通过托盘的“允许局域网访问”开关控制 WSL 服务是否向局域网与 Tailscale 开放，以便从手机或其他设备继续操作。
  验收要点：
  - 开关默认关闭（`127.0.0.1:7681`），切换时通过 `webmux config listen` 修改 WSL `config.toml` 的 `listen`（开启为 `0.0.0.0:<端口>`，关闭为 `127.0.0.1:<端口>`）并自动重启托盘托管的 WebMux；
  - NAT 模式下开启时自动把当前所有可用 Windows 候选地址（Tailscale 地址优先展示）转发到本机链接，仅在服务健康、配置 token 且 Windows 可达时监听，条件失效暂停、恢复后继续；
  - 转发 HTTP 与 WebSocket 并按请求 `Host` 校验同源 `Origin`，不限制请求 `Host`，支持任意 IP 或主机名（如 Tailscale MagicDNS）；
  - 退出托盘即停止本机转发；
  - 托盘运行时不修改防火墙；安装版可在安装时添加专用网络与域网络的放行规则（US-O12），公用网络或未添加规则时由 Windows 原生提示处理。
- **US-O09 打开和复制受保护入口**：作为 Windows 使用者，我希望从托盘打开页面、在统一的“复制链接”子菜单中复制本机/远程/经托盘转发链接以及所有已就绪的远程访问入口（Cloudflare Tunnel、Tailscale Serve/Funnel 等）的域名链接，或复制 token，以便把访问入口带到另一设备。
  验收要点：
  - 操作时从 WSL 获取 token，不在 Windows 配置中保存；
  - 复制链接可带 `#token=`，不配置 token 时复制普通链接；
  - 远程访问入口按服务端 `urls.access` 的顺序逐项显示“URL（入口名称 · 公网/仅私有网络）”，托盘不按入口类型写专门代码，新入口无需修改托盘；旧服务端缺少该字段时回退为就绪的 Cloudflare 公网链接；
  - 仅服务明确返回 `settingsAvailable: true` 时提供“公网访问设置”，打开本机 `/settings/tunnel` 并沿用 token 片段交接；
  - 敏感剪贴板排除历史和云同步，60 秒后仅在内容未被替换时清空；
  - 通知只显示无 token 地址。
- **US-O10 退出托盘保留任务**：作为 Windows 使用者，我希望只退出托盘而保留 WebMux，以便临时关闭桌面客户端却不中断任务。
  验收要点：
  - “仅退出托盘”不停止服务，重新打开后恢复可用控制；
  - 托盘转发随退出停止；
  - Windows 注销或关机不承诺保留任务；
  - 需要停服务时使用单独停止操作。
- **US-O11 判断 WSL 服务兼容与版本对应**：作为 Windows 使用者，我希望托盘告诉我它与 WSL 中的 webmux 是否兼容、版本是否对应、该更新哪一边，以便两边分别升级后不再遇到含糊的失败。
  验收要点：
  - 托盘启动后首次取得状态、切换发行版或修改 `webmux` 路径、安装完成后，以及处于缺失或不兼容状态时打开菜单，在后台以 `version --json` 检查（旧版本回退文本并视为契约 v1），单次不超过 30 秒，日志记录两侧版本与契约；
  - 兼容只看控制契约，对应只比较版本号（`X.Y.Z` 与 `X.Y.Z-dev.N` 按 semver 预发布规则比较），`dev` 等无法解析的版本无法判断且不提示；
  - 结果只出现在菜单状态行，不发通知或弹窗：较旧一方、“比托盘新，请升级托盘”或“版本过旧，请升级”；
  - WSL 较新且为发布版本时，安装版托盘提供“更新托盘”（开启自动更新时自动检查一次，见 US-O13），便携版提供“下载托盘 <版本>”打开对应正式版或 `dev` Release 页面；
  - WSL 为较新的开发版而托盘通道为正式版时，提示“WSL 中为开发版 <版本>，托盘使用正式版通道”，安装版托盘不自动检查，改为提供“切换到开发版通道并更新托盘”（见 US-O14）；
  - 契约不兼容仍可启动、停止、重启，以 Windows 侧可达性判断运行并显示“运行中（不兼容，功能受限）”，状态无法解析不判失败、不自动重启；
  - 缺少本机 API 时禁用启动；
  - 运行实例与已安装文件版本不同时提示重启后生效，只有托盘管理的实例可从菜单重启。
- **US-O12 用安装程序安装与卸载托盘**：作为 Windows 使用者，我希望用安装程序安装、升级和卸载托盘，以便获得固定位置、开始菜单入口和卸载项，而不必手工放置 exe。
  验收要点：
  - `webmux-setup-windows-<架构>.exe` 按当前用户安装、安装本身不需要管理员，默认目录 `%LOCALAPPDATA%\Programs\WebMux`（可更改，升级沿用上次目录）；
  - 安装 `webmux-tray.exe`、开始菜单“WebMux”与“应用和功能”卸载项（名称、版本、发布者）；
  - “安装选项”页的“开机后自动启动 WebMux”默认勾选，写入与托盘菜单“登录时自动启动”相同的启动项；
  - “安装选项”页的“允许其他设备通过托盘转发访问 WebMux”默认勾选：安装完成后弹出一次管理员确认，由 `webmux-tray.exe firewall allow` 为该托盘路径添加只适用于专用与域网络的入站 TCP 允许规则；规则已存在时不弹窗，取消或失败时安装照常完成并说明手工命令；
  - 再次交互安装时该选项沿用上次的选择，取消勾选或改换安装目录时删除之前记录的规则；静默安装不弹管理员确认、不改动防火墙规则，自更新在原路径覆盖后规则继续有效；卸载时删除安装程序添加的规则（再弹一次确认）；
  - `webmux-tray.exe firewall allow|remove [路径]` 退出码：0 已是目标状态、1 失败、2 参数错误、3 取消确认；
  - 预置同架构 Linux 服务端到托盘下载缓存，不访问或改动 WSL；
  - 安装包架构与 Windows 不符时提示正确的安装包并以退出码 2 退出，不写任何文件；
  - 覆盖安装先以 `quit` 关闭本用户托盘（WebMux 与会话保持），安装目录中的托盘 10 秒无响应才强制结束，其他位置不响应的托盘只提示用户退出并可重试，静默模式以退出码 3 结束且不写文件；
  - 完成后启动托盘；
  - 卸载同样先关闭托盘，删除程序、快捷方式、卸载项与预置服务端，登录启动项仅在指向本次安装时删除，保留托盘设置、日志与 WSL 中的 webmux，不停止 WebMux；
  - 支持 `/S` 静默安装与卸载：首次安装用默认选项，覆盖安装沿用登记的安装目录并保持登录启动项现状；失败时总恢复升级前的 `webmux-tray.exe`，关闭过运行中的托盘时重新启动它并以退出码 4 结束，没有关闭托盘时以退出码 2 结束且不启动托盘。
- **US-O13 托盘自动更新**：作为用安装程序安装托盘的 Windows 使用者，我希望托盘自己安装经过签名验证的新版本，且不结束 WSL 中的终端会话，以便不必手动下载安装程序。
  验收要点：
  - 托盘设置 `autoUpdate` 默认开启（旧设置缺省视为开启），菜单提供“自动更新”勾选项与随时可用的“检查更新”，非发布构建禁用并说明原因；
  - 开启时启动 2 分钟后检查、之后每 24 小时检查，读取签名清单（GitHub 与镜像，只信内置公钥）；
  - 从登记的安装目录运行的安装版发现更高版本时下载同架构安装程序到 `%LOCALAPPDATA%\WebMux\updates` 并校验，等无 WSL 安装、启停重启或对话框时以分离进程运行 `/S`，旧托盘按“仅退出托盘”退出，WebMux PID 与会话不变，经转发的连接短暂断开、转发在新托盘恢复；
  - 下载或等待期间关闭“自动更新”时不运行安装程序，恢复为“有新版本”；
  - 运行安装程序前在设置中记录 `updateAttempt`（版本与时间），无法记录时不运行；
  - 安装程序结束后托盘仍在运行或 15 分钟未结束时显示“更新失败：<原因>”并恢复检查；前者与安装程序无法启动时清除 `updateAttempt`，超时保留；
  - 安装程序关闭托盘后失败、重新启动原托盘时，原托盘显示一次“更新到 <版本> 失败，已恢复原托盘”，24 小时内自动检查不再安装该版本，“更新到 <版本>”与更高版本不受限制；
  - 关闭自动更新时只提示“有新版本”并提供“更新到 <版本>”；
  - 便携版只提示并提供“下载托盘 <版本>”，从不下载；
  - 下载或校验失败显示“更新失败：<原因>”，下次计划检查重试；
  - 不发系统通知；
  - 新托盘按 `lastVersion` 显示一次“已更新到 <版本>”，清除 `updateAttempt` 并清理下载目录。
- **US-O14 托盘选择更新通道**：作为 Windows 使用者，我希望在托盘中选择跟随正式版或开发版，以便托盘与 WSL 服务端一起尝试新构建，并能随时退回正式版。
  验收要点：
  - `tray.json` 的 `updateChannel` 默认 `stable`，菜单“更新通道”下有“正式版”“开发版”单选项，切换后立即写入设置并检查一次；
  - 开发版通道同时读取 `dev` 与正式版清单，采用较高的已验证版本，安装程序文件名带版本号；
  - 切到开发版后 WSL 服务端按原有自动升级规则跟随到同一次开发版构建；
  - 点击“切换到开发版通道并更新托盘”把 `updateChannel` 设为 `dev` 并立即检查，发现更高版本时视同点击“更新到 <版本>”立即安装；
  - 切回正式版不降级：当前开发版高于最新正式版时不安装，状态说明正式版超过它后自动更新。

隧道设置的增量兼容同样归入 US-O11：不得从版本号或存在 `tunnel` 字段推断设置页可用，只有 `settingsAvailable: true` 才显示入口；缺少该字段的旧服务仍保留原有隧道控制。可选 `enabled` 表示当前进程期望（包括 `--tunnel` 覆盖），不是持久磁盘值；缺失时才按 starting/ready 推断开关。

Windows 原生终端客户端复用 US-M01–US-M06：提供 run、attach、ls、open、status 与 version，服务仍在 WSL 中。

- 发行版与 webmux 路径：按命令行、环境变量、托盘设置、默认值依次解析。
- 路径换算：用 `wslpath` 换算盘符路径，识别同发行版的 WSL UNC 路径，无法换算时要求显式 Linux `--cwd`。
- 连接：经 `wsl.exe --exec webmux bridge`，不依赖 localhost 转发。
- 附着：使用 UTF-8/VT 控制台，恢复后还原模式与代码页。

依据：

- [Windows CLI](../../cmd/webmux-windows/main_windows.go)、[WSL 桥接与路径](../../internal/wslclient/)、[托盘服务与安装](../../internal/tray/)
- [托盘自更新](../../internal/tray/selfupdate.go)、[安装版识别与启动安装程序](../../internal/tray/traysys/install_windows.go)、[自动更新说明](../deployment/updates.md)
- [兼容判定](../../internal/tray/compat.go)、[托盘菜单](../../internal/tray/winui/menu.go)、[转发](../../internal/tray/forward/)
- [敏感剪贴板](../../internal/tray/winui/clipboard.go)、[退出请求](../../internal/tray/traysys/quit.go)、[单实例与退出事件](../../internal/tray/traysys/instance_windows.go)
- [安装程序脚本](../../deploy/windows/webmux.nsi)、[安装程序冒烟](../../.github/scripts/windows-installer-smoke.ps1)、[Windows 部署](../deployment/windows-wsl.md)。

## P. Tailscale Serve / Funnel

- **US-P01 检测 Tailscale 环境**：作为部署者，我希望只读查看 Tailscale 是否可用、节点 DNS 与证书状态，以便判断能否使用自动 HTTPS 入口。
  验收要点：
  - `GET /api/tailscale` 需要认证，返回 provider（linux/wsl-native/windows/none）、后端状态、节点 DNS、HTTPS 就绪、Funnel 属性、现有入口 host/port/public 与 WSL 网络模式；
  - 只执行 `status`/`serve status`/`funnel status`，短超时、无交互 stdin、缓存并合并并发请求；
  - CLI 缺失、未登录、输出损坏或超时不 panic，也不把缺失字段当作已就绪；
  - 响应不含 token 或其他入口 target。
- **US-P02 托管 Serve**：作为部署者，我希望通过配置让 WebMux 在 tailnet 内提供自动证书的 HTTPS，以便手机等可信设备安全访问。
  验收要点：
  - `[tailscale] mode = "serve"` 在监听开始后以 argv 执行 `tailscale serve --bg --https=<port> http://127.0.0.1:<实际端口>`；
  - 执行前要求 `CertDomains` 包含节点 DNS；
  - 自动端口按 443、8443、10000 选择，显式冲突失败不覆盖；
  - 回读确认唯一 `/` handler 指向当前 target 且 `AllowFunnel` 为假后才登记来源并报告 ready；
  - 停止/失败只移除精确端口，绝不调用 reset。
- **US-P03 显式 Funnel**：作为明确需要公网访问的部署者，我希望显式启用 Funnel 并看到风险，以便在无 VPN 的设备上连接。
  验收要点：
  - `mode = "funnel"` 只允许 443/8443/10000，要求强 Token；
  - 不因检测到 Tailscale 就自动公开；
  - 命令失败把 stderr 映射为 HTTPS 未启用、缺少 funnel 属性、权限不足或端口冲突；
  - 回读 `AllowFunnel[host:port] == true` 后才登记公开来源；
  - 从 Funnel 切回 Serve 或端口变化先关闭旧精确目标，失败时确认不残留并保持 failed；
  - 设置页持续显示公网风险。
- **US-P04 WSL NAT 阻塞**：作为 WSL 使用者，我希望知道为什么不能托管 Windows Tailscale，并看到可行替代方案。
  验收要点：
  - `provider = "windows"` 且 `wslinfo --networking-mode` 为 nat/unknown 时失败关闭，不执行任何 Windows 写命令；
  - 设置页与错误说明 mirrored（`.wslconfig` + `wsl --shutdown` 会结束会话）、WSL 内安装 Tailscale、Windows 托盘转发三种方案；
  - 不自动修改 `.wslconfig`、重启 WSL 或防火墙。
- **US-P05 显式 Windows provider**：作为 WSL 使用者，我希望显式选择 Windows 主机上的 Tailscale，以便在 mirrored 网络中使用 Windows 节点。
  验收要点：
  - `auto` 只检测并优先正在运行的 WSL 原生 Tailscale，永不被用来代管 Windows；
  - 只有显式 `provider = "windows"` 才允许写命令，URL/Host/Origin 使用 Windows 节点 `Self.DNSName`；
  - 非 WSL 配置 windows 启动失败；
  - 权限不足提示在 Windows 管理员终端执行 `tailscale set --operator=<用户>`，不 sudo、不自动提权；
  - WSL 关闭后保留的入口在下一次启动按状态文件对账。
- **US-P06 所有权与失败关闭**：作为部署者，我希望 WebMux 只清理自己创建的入口，并在无法确认时拒绝启动，以便不误改其他应用或暴露旧端口。
  验收要点：
  - `<state_dir>/tailscale-ownership.json`（0600、原子替换、独立锁）记录 provider、DNS、模式、端口、target 与 pending/active；
  - 写命令前先落盘 pending，命令与回读成功后标记 active；
  - `mode = off`、模式/端口/target/provider 变化或状态损坏时先对账，删除前用 `serve status` 复核 provider、DNS/端口与 target；
  - 读取失败则不删除并拒绝启动（错误给出状态文件路径）；
  - 记录端口已无入口或改指其他 target 时判定记录过期、不动该入口并删除记录；
  - 无法移除旧入口则拒绝启动并给出精确的 `serve --https=<port> off` 命令；
  - `serve off` 同时清除 Funnel 标记，重复移除视为成功；
  - 状态文件缺失或损坏但已有入口指向当前监听地址时仍视为所有权未知并拒绝接管，错误给出该入口端口与精确的 `serve --https=<port> off` 命令，供确认废弃后手工移除；
  - 绑定监听失败时关闭本次入口或报告风险；
  - 来源撤销后旧 Host/Origin 立即 403（若在 `allowed_origins` 手工保留了 ts.net 主机名，迁移时需删除），Cloudflare 与 Tailscale 可并存。 旧 tailscale.json 即使与新版记录并存也阻止启动，须先核实外部入口。
- **US-P07 网页修改 Tailscale 设置**：作为部署者，我希望在设置页修改 Tailscale 的模式、端口与 provider 并立即生效，以便不改配置文件、不重启服务就能开启、切换或关闭入口。
  验收要点：
  - `/settings/tailscale` 可选关闭/Serve/Funnel、端口（Serve 留空自动，Funnel 仅自动/443/8443/10000）与 provider（Windows 仅 WSL 显示）；
  - `PUT /api/tailscale/settings` 保留认证与 Host/Origin 校验，按 TOML 规则校验后写入 `<state_dir>/tailscale-settings.json`（0600、原子替换、独立锁），之后启动优先于 TOML，`command` 只来自 TOML；
  - 保存后先撤销来源并移除不再符合新设置的本实例入口再应用，`starting` 期间页面轮询，移除失败保持 failed 且不应用新设置；
  - 从非 Funnel 切到 Funnel 前确认公网风险，未确认 400；
  - 登录 Token 少于 43 字符或监听不可托管时 409 且不保存，页面显示原因；
  - 从当前 Tailscale 地址替换入口前提示断线；
  - 恢复配置文件设置前确认，删除覆盖并关闭本实例入口，TOML 下次启动生效；
  - 设置文件损坏或被锁时禁止写入且托管保持关闭；
  - 会话内进程经本机 API 修改返回 403。
- **US-P08 托盘一键开启 Tailscale**：作为 Windows 托盘使用者，我希望在托盘“公网访问”子菜单中一键开启或关闭 Tailscale Serve，以便不打开网页就能得到 tailnet HTTPS 入口。
  验收要点：
  - 托盘把 Cloudflare Tunnel 与 Tailscale 的开关、状态行和各自设置入口收进“公网访问”二级菜单；
  - `/local/v1/status` 的 `tailscale` 提供不触发检测的模式、端口、provider、状态、地址与错误，旧服务端缺少该字段时不显示 Tailscale 项；
  - 勾选跟随保存的模式，failed 仍勾选以便一键关闭；
  - 开启前经本机 API 刷新检测，未安装或未登录、未运行、HTTPS 证书未就绪、WSL 非 mirrored 却只有 Windows 节点、设置不可保存或 Token 过弱时弹窗说明原因并可打开 `/settings/tailscale`；
  - 只开启 Serve（Funnel 仍由网页确认），保留已配置端口；
  - 配置为 `auto` 而只检测到 Windows 节点时先确认，同意后才以 `provider = "windows"` 保存；
  - 保存失败或应用后转为 failed 同样引导到网页，ready 时通知地址；
  - 会话内进程经本机 API 修改仍为 403。

依据：

- [Tailscale 检测与命令](../../internal/tailscale/)、[状态集合](../../internal/contract/tailscale.go)、[控制器与对账](../../cmd/webmux/tailscalectl.go)
- [控制器用例](../../cmd/webmux/tailscalectl_test.go)、[网页设置存储](../../internal/tailscalesettings/store.go)、[配置](../../internal/config/config.go)
- [启动与 Token](../../cmd/webmux/main.go)、[来源注册表](../../internal/api/public.go)
- [状态与设置 API](../../internal/api/tailscale.go)、[API 用例](../../internal/api/tailscale_test.go)、[设置页](../../web/src/pages/TailscaleSettingsPage.svelte)
- [设置表单](../../web/src/components/settings/TailscaleConfiguration.svelte)、[设置卡片](../../web/src/components/settings/TailscaleSettings.svelte)、[Tailscale 指南](../tailscale.md)。
- 托盘一键开启：[托盘菜单视图](../../internal/tray/menuview.go)、[视图用例](../../internal/tray/menuview_test.go)、[托盘操作](../../internal/tray/winui/links.go)、[本机客户端](../../internal/localclient/client.go)、[本机状态](../../cmd/webmux/local.go)。
