# 部署指南

## 运行要求

- Linux 或 macOS（amd64 / arm64）。
- 一个可执行的 shell，以及打算使用的 CLI agent（Claude Code、Codex CLI 等）。
- 可选：Git，用于仓库与 worktree 管理（需支持 `git worktree list --porcelain -z`）。

运行端不需要 Node.js、Go、tmux 或数据库。

## 安装

Linux/macOS 推荐使用仓库根目录的 [`install.sh`](../install.sh)。

- 依赖：POSIX `sh`、`curl`、`uname`、`mktemp`、`awk`，以及 `sha256sum` 或 `shasum`（任一即可）。
- 流程：自动识别 Linux/macOS 与 amd64/arm64（Apple Silicon 的 Rosetta 终端优先选择 arm64），下载对应二进制与 `SHA256SUMS`，校验通过后才原子替换目标文件。
- 默认安装到 `~/.local/bin/webmux`，不需要 sudo，也不会执行 `config init`、`service install` 或改写 shell 配置。

安装脚本本身不依赖 `raw.githubusercontent.com`。任选一个可访问的入口：

```sh
# GitHub 直连；install.sh 随 Release 一起发布
curl -fsSL https://github.com/mzdluo123/webmux/releases/latest/download/install.sh | sh

# 国内网络：ghfast.top 代理同一 Release 资产
curl -fsSL https://ghfast.top/https://github.com/mzdluo123/webmux/releases/latest/download/install.sh | sh

# jsDelivr 读取仓库中的脚本；脚本随后仍从 GitHub 或镜像下载二进制
curl -fsSL https://cdn.jsdelivr.net/gh/mzdluo123/webmux@main/install.sh | sh
```

| 变量                 | 默认值                                    | 说明                                                            |
| -------------------- | ----------------------------------------- | --------------------------------------------------------------- |
| `WEBMUX_VERSION`     | 最新 Release                              | 指定 `x.y.z`（可带 `v` 前缀），从 `releases/download/v...` 下载 |
| `WEBMUX_INSTALL_DIR` | `~/.local/bin`                            | 目标目录；不可写时明确报错，不自动 sudo                         |
| `WEBMUX_MIRRORS`     | `https://ghfast.top https://gh-proxy.com` | 空格分隔的 gh-proxy 前缀；空值表示只用 GitHub                   |
| `WEBMUX_SHA256`      | 空                                        | 固定二进制 SHA-256，跳过同源 `SHA256SUMS`                       |

```sh
curl -fsSL https://github.com/mzdluo123/webmux/releases/latest/download/install.sh \
  | WEBMUX_VERSION=0.2.0 WEBMUX_INSTALL_DIR="$HOME/bin" sh
```

下载顺序为 GitHub 直连，然后依次尝试镜像（地址为 `<前缀>/https://github.com/...`），每个来源都有连接、低速和总时限。

- 二进制与 `SHA256SUMS` 必须来自同一来源并通过校验；失败时保留原有二进制、清理临时文件，并汇总各来源的错误。
- 镜像是第三方服务，能看到请求者 IP。
- 同源校验只能发现截断和损坏，无法防范同时替换两者的恶意镜像；高保证或离线场景请用 `WEBMUX_SHA256` 固定摘要。

脚本只安装二进制。完成后按提示运行 `webmux --listen 127.0.0.1:7681`。

- 需要后台服务时再执行 `webmux service install`，首次启动会自动生成配置与 Token。
- 重复执行脚本会升级二进制；正在运行的服务需手动重启，重启会结束现有会话。
- Windows 不运行该脚本：请下载 `webmux-windows-*.exe` 或托盘程序，或按 [Windows（WSL）](deployment/windows-wsl.md) 在 WSL 中运行同一脚本。

## 配置

```sh
webmux config init                 # 手动预生成 0600 的 ~/.config/webmux/config.toml，含随机 Token
# 或：mkdir -p ~/.config/webmux && cp deploy/config.example.toml ~/.config/webmux/config.toml && chmod 600 ~/.config/webmux/config.toml
```

示例见 [`deploy/config.example.toml`](../deploy/config.example.toml)。加载规则：

- 默认路径为 `~/.config/webmux/config.toml`。
  - 未显式指定 `--config` 且该文件不存在时，首次 `webmux` / `webmux serve` / `webmux start` 会用与 `config init` 相同的模板和随机 token 生成它（目录 0700、文件 0600），然后正常启动。
  - 启动输出会说明是新建还是使用现有配置，并提示用 `webmux status --json --reveal-token` 查看已持久化的 token。
  - 并发首次启动通过原子发布只创建一个文件，各进程读取同一 token。

- 通过 `--config` 显式指定的文件只读：文件存在时只读取该文件，不存在时启动失败且绝不创建。`--config` 与 `--log-file` 支持 `~/`。
- 默认配置已存在时直接读取，不修改、不重写。生成失败（主目录只读、权限不足、磁盘满等）会报错退出，不会静默回退到内存默认值。
- 命令行 `--listen`、`--token` 优先于配置文件，显式传入空 token 也会覆盖配置；覆盖只影响本次进程，不写入新生成的默认配置。仅 `config show`、`config validate`、`status`、`stop` 等客户端命令只读取配置，不会触发生成。
- `webmux version` 输出版本号；`webmux version --json` 另含控制契约版本（见 [控制面 JSON 契约](protocol/control-json.md#控制面-json-契约)）。

```sh
webmux --config ~/.config/webmux/config.toml   # 等同于 webmux serve --config ...
curl -H 'Authorization: Bearer YOUR_TOKEN' http://127.0.0.1:7681/api/health
```

### 命令行

无子命令或使用 `--listen` 等服务参数时启动服务，与 `serve` 相同；未知子命令退出码 2。客户端子命令都接受 `--config`，按与服务相同的规则找到状态目录、运行时记录与本机套接字。`webmux start` 在不改变前台默认语义的前提下后台启动，可用 `webmux stop` 停止。

| 命令                                              | 说明                                                                                                                               |
| ------------------------------------------------- | ---------------------------------------------------------------------------------------------------------------------------------- |
| `webmux serve [--log-file PATH]`                  | 启动服务；`--log-file` 把标准输出与标准错误写入文件（0600），旧文件保留为 `.1`，启动方关闭管道后服务照常运行                       |
| `webmux start [--log-file PATH]`                  | 后台启动并按 `serve` 规则加载配置；默认日志为 `<state_dir>/webmux.log`，等待就绪后输出 PID、本机链接与日志路径；已运行时可重复执行 |
| `webmux status [--json] [--reveal-token]`         | 优先经本机 API，退回运行时记录 + TCP 健康检查；退出码 0 健康、3 未运行、1 其他；只有 `--json --reveal-token` 输出 Token            |
| `webmux stop [--timeout 15s] [--force] [--json]`  | 优先经本机 API 停止，卡死时退回 SIGTERM，超时 SIGKILL；systemd / launchd 实例需 `--force`；没有记录时不向任何进程发信号            |
| `webmux bridge`                                   | 把本机套接字桥接到标准输入输出，供 Windows 客户端经 `wsl.exe --exec webmux bridge` 使用                                            |
| `webmux config path\|show\|validate\|check\|init` | 定位、查看生效配置（不含 Token）、校验（含 TOML 行列号）、以模板和随机 Token 初始化（已存在需 `--force`，原文件保留为 `.bak`）     |

`webmux config check` 是 `config validate` 的别名；`webmux help` 显示全部命令。

JSON 输出契约见 [控制面 JSON 契约](protocol/control-json.md#控制面-json-契约)。配置只在启动时读取；`status` 的 `drift` 列出已修改但尚未通过重启生效的 `listen`、`token`。

### 配置项

| 配置项               | 默认值                                                     | 说明                                                                                                                                                                                                                                                    |
| -------------------- | ---------------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `listen`             | `0.0.0.0:7681`                                             | HTTP / WebSocket 监听地址                                                                                                                                                                                                                               |
| `token`              | 空                                                         | 共享 token；首次启动自动生成并持久化，可用 `webmux status --json --reveal-token` 查看；已有配置监听 loopback 且未启用隧道时空表示不认证；其他监听地址或启用隧道且为空时每次启动临时生成，并打印带 `#token=` 的本地链接；隧道模式显式 token 至少 43 字符 |
| `scrollback_bytes`   | `2097152`                                                  | 每会话保留的原始终端字节数（不是行数）                                                                                                                                                                                                                  |
| `default_cwd`        | `~`                                                        | 新建会话的默认目录，支持 `~` 展开                                                                                                                                                                                                                       |
| `state_dir`          | `$XDG_STATE_HOME/webmux`，未设置时 `~/.local/state/webmux` | 仓库清单、启动配置等服务端状态目录                                                                                                                                                                                                                      |
| `idle_after_seconds` | `30`                                                       | 无等待特征时判定为空闲的秒数                                                                                                                                                                                                                            |
| `prompt_patterns`    | 常见提示符 / 确认模式                                      | 判定“可能在等你”的 Go 正则列表                                                                                                                                                                                                                          |
| `agent_protocol`     | `true`                                                     | 在会话环境中设置 `WARP_CLI_AGENT_PROTOCOL_VERSION=1`，让 agent 发出结构化提醒事件                                                                                                                                                                       |
| `bell_attention`     | `true`                                                     | 把终端响铃（输入 1 秒后）视为需要介入                                                                                                                                                                                                                   |
| `program_status`     | `true`                                                     | 识别 `OSC 7501` 程序状态报告，并由服务端应答 `OSC 7501;?` 探测                                                                                                                                                                                          |
| `allowed_origins`    | `[]`                                                       | 额外允许的完整 Origin，例如 `https://myhost.example`                                                                                                                                                                                                    |
| `tunnel`             | 未启用                                                     | Cloudflare Tunnel，见下文“公网访问（Cloudflare Tunnel）”                                                                                                                                                                                                |
| `push`               | 启用                                                       | 注意力 Web Push，见下文“手机推送（Web Push）”                                                                                                                                                                                                           |

所有启动命令都由使用者自定义：在设置页保存“名称 + 完整命令 + 环境变量”，新建会话时选用，或直接填写不保存的临时命令。

- 新安装首次启动（`launch-configs.json` 不存在）时自动写入六条启动配置：Shell（`$SHELL -l -i`，未设置时为 `/bin/sh`）、Claude Code、Codex CLI、Gemini CLI、Open Code、omp。
- agent 预设以 `bash -ic 'exec -- "$@"' webmux <程序>` 加载 `~/.bashrc`。
- 它们是普通配置，可编辑或删除。不再读取 `[[presets]]` 或 `shell`；旧配置请改为在设置页保存。

新安装的 omp 启动配置额外预置 `PI_FORCE_IMAGE_PROTOCOL=kitty`，以在浏览器终端启用 Kitty 图片传输；其他 agent 配置不增加该环境变量。已经存在的 `launch-configs.json` 不自动修改，升级后可手动编辑自己的 omp 配置。

完整命令按引号与反斜杠规则拆分为 argv，不经 shell 求值。

- 需要加载 `~/.bashrc` 的 Open Code 配置可填写 `bash -ic 'exec opencode "$@"' opencode`，对应 argv 为 `["bash", "-ic", "exec opencode \"$@\"", "opencode"]`。
- 末尾的程序名是 `bash -c` 的 `$0` 占位符，后接参数经 `"$@"` 原样传给 agent。
- WebMux 不改写 `~/.bashrc`；其中的输出、环境覆盖或提前退出同样会影响启动。

### 状态目录

完整文件、锁、所有者和故障行为见 [配置与持久化清单](persistence.md)，由 `make persistence-check` 约束；下文仅说明部署边界。

`state_dir` 保存仓库清单 `repositories.json`、启动配置 `launch-configs.json`（及其锁 `launch-configs.json.lock`）与运行时子目录 `run/`（运行时记录 `server.json`、锁 `server.json.lock`、本机套接字 `webmux.sock`），不保存会话、终端内容或 token。目录权限 0700、文件 0600，通过原子替换写入。

- 同一状态目录只能由一个实例使用；多个实例请配置不同的 `state_dir`。第二个实例照常提供终端服务，但不写运行时记录、不创建本机套接字，并在日志中警告。
- 清单损坏、文件锁冲突或存储不可用时，仓库管理功能被禁用，但终端服务照常运行，损坏的清单不会被覆盖。
- 启动配置同理：文件损坏、`launch-configs.json.lock` 被另一实例持有或目录不可读写时，只有保存的启动配置功能返回 503，显式临时命令与已运行会话照常可用；损坏的文件不会被覆盖或改名，修复或删除后重启服务即可。启动配置的环境变量以明文保存在该 0600 文件中。
- 首次启动时写入上述默认预设并创建 `launch-configs.json`；文件已存在（包括删空后的空列表）时不再补写，也不会自动导入旧 TOML 的启动预设。
- `launch-configs.json` 当前为版本 `2`，每条配置可带可选的 `preview: {lines, skip}`。
  - 本次破坏性升级只接受版本 `2`；版本 `1` 文件保留原样且启动配置功能返回 503，不会自动转换。
  - 升级前备份旧配置；需保留内容时先用支持 v1 的旧版本保存为 v2，或备份后移走旧文件并在新版重新创建配置。
  - 回滚前停止新版并恢复对应版本的独立备份。

- 混合会话记录 `hybrid-sessions.json`（及其锁）只保存 webmux 会话与 agent 会话的对应关系、命令快照与状态，不含环境变量值、消息、排队内容或 token，最多 200 条。
  - 文件损坏、锁冲突或不可读写时只禁用混合会话的创建与恢复，普通会话不受影响，原文件不被覆盖。
  - 启动配置的 `hybrid` 字段只在勾选时写入，文件仍为版本 `2`；回退到不认识该字段的旧版本前，先取消所有混合勾选，否则旧版本报告启动配置文件损坏。
  - 详见 [混合会话](hybrid-sessions.md)。

## 服务管理命令

先将二进制放到稳定位置（源码构建可执行 `make install`），再以日常用户运行，不要使用 sudo：

```sh
webmux service install
webmux service status
webmux service logs            # 最近 100 行
webmux service logs --follow   # 持续查看，Ctrl-C 退出；也支持 -f
webmux service stop
webmux service start
webmux service restart
webmux service uninstall
```

- `install` 写入用户服务定义并立即启用、启动；缺少默认配置时与首次 `serve` 共用同一生成逻辑，创建带随机 token 的 0600 配置后重新读取并校验。显式 `--config PATH` 指定的文件必须已经存在且有效，不会自动创建。
- 服务使用当前可执行文件和配置文件的绝对路径，不复制二进制。移动二进制后，需在新位置执行 `service install --force` 更新定义。
- 已有服务定义需要 `service install --force` 才能替换，并会重启服务。`service install --dry-run` 只输出服务定义，不写文件、不创建配置、不操作服务；可与 `--config` 配合使用。
- `stop`、`restart`、覆盖安装和卸载会结束当前所有终端会话，滚屏不会在重启后恢复。`uninstall` 保留二进制、配置、日志和状态目录。
- 安装会保存当前 PATH 中的绝对目录，并补充常见 agent 安装目录，只保存 PATH、shell、locale 和 `XDG_STATE_HOME` 等指定环境变量。更新 agent 安装路径后可重新执行 `install --force`。
- 服务工作目录为用户主目录；配置中的 `default_cwd`、`state_dir` 建议使用绝对路径或支持的 `~` 路径；自定义启动命令的可执行程序使用绝对路径或 PATH 中的命令名，避免依赖安装时的工作目录。
- Linux 需要可用的 systemd 用户管理器；macOS 需要已登录的 GUI 会话。服务启用成功不代表 HTTP 端口已成功监听，遇到端口冲突等启动失败时查看 `status` 和 `logs`。

`webmux help` 显示命令概览；`webmux service install --help` 等显示子命令参数。

## systemd 用户服务（Linux）

推荐使用 `webmux service install`，生成位置为 `$XDG_CONFIG_HOME/systemd/user/webmux.service`（默认 `~/.config/systemd/user/webmux.service`）。也可手工使用模板：

```sh
make install
mkdir -p ~/.config/systemd/user
cp deploy/webmux.service ~/.config/systemd/user/
systemctl --user daemon-reload
systemctl --user enable --now webmux
journalctl --user -u webmux -f
```

用户未登录时也保持运行：

```sh
loginctl enable-linger "$USER"
```

服务使用 `KillMode=mixed`：WebMux 先自行清理会话，超时后由 systemd 清理整个 cgroup。

## launchd 用户代理（macOS）

推荐使用 `webmux service install`，自动生成 `~/Library/LaunchAgents/com.webmux.plist` 并填写绝对路径，日志位于 `~/Library/Logs/webmux.log`。也可手工使用模板：

```sh
make install
mkdir -p ~/Library/LaunchAgents ~/Library/Logs
sed "s#/Users/YOU#$HOME#g" deploy/com.webmux.plist > ~/Library/LaunchAgents/com.webmux.plist
plutil -lint ~/Library/LaunchAgents/com.webmux.plist
launchctl bootstrap "gui/$(id -u)" ~/Library/LaunchAgents/com.webmux.plist
tail -f ~/Library/Logs/webmux.log
```

修改 plist 后重新加载：

```sh
launchctl bootout "gui/$(id -u)/com.webmux"
launchctl bootstrap "gui/$(id -u)" ~/Library/LaunchAgents/com.webmux.plist
```

- launchd 不展开 `~` 和 `$HOME`，plist 中必须使用绝对路径。
- plist 设置了 `LANG=en_US.UTF-8`；缺少 UTF-8 locale 时中文会显示为转义字符。
- 代理在用户登录后运行，`KeepAlive` 在异常退出后重启。
- 系统睡眠会暂停所有会话进程；长时间运行 agent 时可使用 `caffeinate` 防止睡眠。

## Windows（WSL）

WebMux 运行在 WSL 2 发行版中，由 Windows 托盘程序或 `webmux-windows` 命令行客户端管理。前置条件、安装与托盘设置、启动方式、远程转发、Token 与剪贴板及故障诊断见 [Windows（WSL）部署](deployment/windows-wsl.md)。

## agent 的 PATH

systemd 和 launchd 都不会加载交互 shell 的初始化文件。

- 需要从 `~/.bashrc` 获取 PATH 与环境变量时，在自定义启动配置中显式填写 `bash -ic 'exec opencode "$@"' opencode`（其他 agent 替换程序名）。
- 不使用 Bash 包装时，可直接指定程序绝对路径或调整服务的 PATH。服务模板的 `PATH` 已包含 `~/.local/bin`、pnpm / npm 全局目录（macOS 另含 Homebrew）。

如果 agent 安装在 nvm 等其他位置：

```sh
which claude codex opencode
systemctl --user edit webmux    # Linux：添加 drop-in
```

```ini
[Service]
Environment=PATH=/home/YOU/.local/bin:/home/YOU/.nvm/versions/node/v24.20.0/bin:/usr/local/bin:/usr/bin:/bin
```

macOS 直接编辑 plist 的 `EnvironmentVariables` 后重新加载。

Windows 托盘经 `wsl.exe --exec` 启动 WebMux，同样不经过登录 shell；托盘启动的实例会在 `PATH` 前补上服务模板中的 `~/.local/bin`、`~/.local/share/pnpm`、`~/.npm-global/bin`（已存在的保持原位）。安装在其他位置的 agent 请改用 systemd 启动方式并按上面添加 drop-in，或把它链接到 `~/.local/bin`。

## 手机访问（Tailscale）

1. 在服务器和手机上安装 Tailscale 并加入同一网络。
2. 推荐在 `[tailscale]` 设置 `mode = "serve"`，让 WebMux 通过 `127.0.0.1` 监听并托管自动证书的 HTTPS 入口；也可以手工把 `listen` 设为服务器 Tailscale 地址（如 `100.64.0.5:7681`）。
3. 托管 Serve 时手机打开 `https://<节点>.ts.net/`；手工方式打开 `http://<Tailscale IP 或主机名>:7681` 并输入一次 token（保存在浏览器 localStorage 中）。也可以打开启动时打印的访问链接并把主机换成 Tailscale 地址。
4. 托管 Serve 的 HTTPS 地址可以安装为应用；手工 HTTP 地址只能当作普通网页使用，见 [安装为应用（PWA）](#安装为应用pwa)。

详细配置、端口、WSL Windows provider 与故障排查见 [Tailscale Serve / Funnel 入口](tailscale.md)。

防火墙应只允许可信 VPN 接口访问该端口。

## 访问链接

每次启动都会在标准输出打印 `WebMux local: http://<主机>:<端口>/`。

- 启用隧道时会附带 `#token=<token>`；非隧道模式不附带片段，以免固定的已配置 token 被 service manager 捕获并写入 journal 或日志文件。
- 片段不会发送到服务器，前端读取并保存 token 后立即从地址栏和当前历史记录中移除。
- WebSocket 握手通过子协议携带 token，token 不出现在请求 URL 或 Cookie 中。
- 浏览器的自动补全或同步历史仍可能记住带片段的原始链接。

## 公网访问（Cloudflare Tunnel）

```sh
webmux --tunnel
```

推荐打开网页“设置 → 公网访问”，保存 Quick/Named 配置并启用，无需重启。首次网页保存前也可在配置中设置：

```toml
[tunnel]
enabled = true
token = "" # 空 = Quick Tunnel；填写 = Named Tunnel
hostname = "" # Named Tunnel 的公共主机名（ASCII/punycode，不支持通配符）
cloudflared = "" # 可选：cloudflared 路径
# download_mirrors = ["https://ghfast.top", "https://gh-proxy.com"]
```

- **Quick Tunnel**：无需账号，每次启动得到随机 `https://*.trycloudflare.com` 地址，无 SLA，适合临时访问。
- **Named Tunnel**：在 Cloudflare 控制台创建隧道，把公共主机名指向 `http://127.0.0.1:<端口>`，在网页保存凭据与主机名（首次保存前也可用 TOML）。凭据经环境变量传给 cloudflared，不出现在命令行中。固定的 HTTPS 地址（Named Tunnel、Tailscale Serve/Funnel 等）也是手机推送的前提。
- 启用隧道要求至少 43 字符的 WebMux 登录 Token；启动需要认证且未配置时生成随机 Token。网页管理配置的恢复失败不会结束本地服务：弱 Token 或 cloudflared 故障显示 failed，可修复后重新开启。网页复制的公网地址不含登录 Token。
- cloudflared 依次取 `tunnel.cloudflared`、`PATH`、`<state_dir>/cloudflared/<版本>/` 缓存；都没有时下载 webmux 固定的版本。
  - 下载先连 GitHub，10 秒无响应或 20 秒无数据时依次改用 `download_mirrors` 镜像（默认 `ghfast.top`、`gh-proxy.com`；`[]` 表示只用 GitHub）。
  - 任何来源的文件都必须与内置 SHA-256 一致才会执行。
  - 镜像是第三方服务，能看到请求者 IP 和所下载的文件；不希望回退到它们时设置 `download_mirrors = []`。

- webmux 以 `--config /dev/null` 运行 cloudflared，`~/.cloudflared/config.yml` 不会影响隧道。
- cloudflared 意外退出时公网地址立即失效（返回 403），本地访问和会话不受影响；webmux 退出时 cloudflared 随之结束。Linux 上即使 webmux 被 SIGKILL 或 OOM 终止，内核也会结束 cloudflared；macOS 没有等价机制，依赖 launchd 结束作业的全部进程，手动运行时需自行确认没有残留。
- 网页配置保存于 `<state_dir>/tunnel.json`（0600），优先于 TOML；`--tunnel` / `--tunnel=false` 只覆盖本次启动。恢复配置文件设置会删除网页配置并立即关闭，下一次启动按 TOML。
- 存储损坏、锁冲突或不可读时保持本地服务可用且公网关闭，不退回 TOML 意外重开。运行中关闭即使保存失败也先撤销公网授权并停止隧道，页面提示尚未持久化；修复存储后须重新保存关闭状态。

Cloudflare 控制台步骤、两个 token 的区别、Windows/WSL 托盘操作、验证方法与故障排查见 **[Cloudflare Named Tunnel 配置指南](cloudflare-tunnel.md)**。

> 隧道模式或本次进程临时生成 token 时打印的链接与二维码包含 token；以 systemd / launchd 运行时它们会写入 journal 或日志文件，请限制日志访问权限。
>
> - 首次启动写入配置文件的 token 会跨重启保留，不会每次更换；只有未持久化的临时 token 会在重启时更换。
> - 非隧道模式不打印固定的已配置 token，启动输出只提示用 `webmux status --json --reveal-token` 查看。

## 安装为应用（PWA）

浏览器只为安全上下文（HTTPS 或本机回环地址 `127.0.0.1` / `localhost`）提供完整 PWA 能力，按访问地址分两类：

| 访问地址                                                                 | 能力                                                                                                                 |
| ------------------------------------------------------------------------ | -------------------------------------------------------------------------------------------------------------------- |
| HTTP 局域网或 VPN 地址，如 `http://100.64.0.5:7681`                      | 只能作为普通网页使用：Chromium 不提供安装，没有推送和系统剪贴板 API。iOS Safari 仍可“添加到主屏幕”，但推送仍不可用。 |
| HTTPS：Tailscale Serve、Cloudflare Named Tunnel、Caddy 等 HTTPS 反向代理 | 完整 PWA 能力：可安装、独立窗口运行、系统剪贴板；推送还要求来源通过 Host/Origin 校验，见下一节。                     |

- 设置 → 安装应用显示当前状态。Chrome、Edge（含 Android）提供安装提示时显示“安装 WebMux”按钮；也可从地址栏安装图标或浏览器菜单安装。iOS / iPadOS 在 Safari 中“分享 → 添加到主屏幕”，从主屏幕打开后需重新输入 Token。
- 当前地址不是安全上下文时，该分区提示“当前地址不能提供完整 PWA 能力”。
- 不支持离线启动：会话与 PTY 都在服务端，断网时应用无法显示终端。Service Worker 只在开启推送时注册，不缓存页面或 API。
- 只支持部署在域名根路径；`start_url`、`scope` 与 Service Worker 作用域都是 `/`。

## 手机推送（Web Push）

会话出现提问、等待授权、本轮完成、错误或终端通知时，WebMux 可以经浏览器推送服务（APNs、FCM、Mozilla）提醒已开启推送的设备；页面关闭或手机锁屏后仍能收到，点击通知回到对应会话。

```toml
[push]
enabled = true
subject = "https://github.com/mzdluo123/webmux" # VAPID 联系方式：https:// URL 或 mailto:
ttl_seconds = 86400 # 推送服务为离线设备保留提醒的秒数，0..2419200
kinds = ["question", "permission", "error", "done", "notify"] # 还可加 "bell"
include_title = true # false 时通知标题固定为 WebMux
```

- Web Push 只在安全上下文可用，可从以下入口开启：
  - 手机：**Cloudflare Named Tunnel**、**Tailscale Serve/Funnel** 等已授权的 HTTPS 入口。Quick Tunnel 每次启动更换地址，旧订阅点开即失效，因此不支持。
  - 桌面浏览器：也可从 `127.0.0.1` / `localhost` 开启。
  - 非回环 HTTP（VPN / 局域网）与其他未授权的来源不支持；手工 `tailscale serve` 或自建 TLS 反向代理需把 HTTPS origin 加入 `allowed_origins`。

- 隧道或 Tailscale 入口关闭期间推送照常发出，但点开通知要等入口恢复后才能打开页面。修改 `tunnel.hostname` 或 Tailscale 入口端口后需在各设备重新开启。
- 开启步骤：用 Named Tunnel、Tailscale Serve/Funnel 等 HTTPS 入口打开 WebMux → 设置 → 通知推送 → 开启。iOS / iPadOS 需要 16.4 以上，先在 Safari 中“分享 → 添加到主屏幕”，从主屏幕图标打开并重新输入 Token（主屏幕应用不共享 Safari 的登录）。
- 通知只含会话标题、提醒种类与会话链接，不含工作目录、命令、预览或终端输出。同一信号只提醒一次，spinner 输出与重连不会重复提醒；启发式“可能在等你”和进程退出不推送。
- VAPID 密钥与订阅保存在 `<state_dir>/push.json`（0600）。文件损坏时改名为 `push.json.corrupt-<时间>` 后重建，所有设备需重新开启。没有设备开启时不产生任何出站请求。
- 推送是尽力送达：省电、勿扰或离线超过 `ttl_seconds` 时可能延迟或丢失；无 Google Play 服务的 Android 设备不支持；中国大陆网络通常无法访问 FCM；桌面 Chrome、Edge 与 Firefox 需要浏览器进程在运行。会话列表仍是唯一准确的状态来源。

## HTTPS

WebMux 不内置 TLS。需要浏览器安全上下文（系统剪贴板 API）时，推荐使用托管的 `[tailscale] mode = "serve"`（仅 tailnet）或 `mode = "funnel"`（显式公网），WebMux 会自动处理证书、端口、Host/Origin 与启动对账，详见 [Tailscale Serve / Funnel 入口](tailscale.md)。

也可以手工使用 `tailscale serve`、Cloudflare Tunnel 或 Caddy 反向代理 HTTPS：

- 保留原始请求 `Host`；
- 把公开的 HTTPS Origin 加入 `allowed_origins`；非 443 端口必须写成带端口的完整 Origin。

## 升级

替换二进制后重启服务。**重启会结束所有终端会话**，升级前请保存工作；仓库清单和磁盘上的 worktree 会保留。混合会话在重启后标记为已中断，可从首页“可恢复”分组恢复到聊天或终端（对话由 agent 保存，排队消息不保留）。只替换磁盘上的文件不会让正在运行的进程加载新版本。

## 平台差异

Linux 与 macOS 同等支持。进程状态在 Linux 读取 `/proc`，在 macOS 使用 `sysctl` 与 `proc_pidinfo`。

macOS 上：

- 没有 subreaper，脱离父进程的后代由 launchd 回收。
- 内核不公开其他进程的环境变量，`WEBMUX_SESSION_ID` 无法用于清理；关闭会话时按终端会话 ID 与进程树查找后代。先 `setsid` 再让父进程退出的自守护程序不会被清理。
- 临时目录位于 `/var → /private/var` 符号链接下，WebMux 报告规范化后的真实路径。
- Apple 自带的 `/bin/bash` 为 3.2 版本；如需 zsh，请在自定义启动命令中显式填写 `/bin/zsh -l -i`。

## 持久化升级与诊断

完整路径、锁与故障策略见 [持久化清单](persistence.md)。新版统一使用数据文件名加 `.lock`，包含新增 `settings.json` 与 `settings.json.lock`。升级前停止旧进程并备份，不支持新旧版本共用状态目录。旧 Tailscale 所有权文件须先核实其外部入口；不能仅删除记录后继续暴露服务。

`config show --json` 查看磁盘配置，`config show --live --json` 查看运行进程实际采用值。输出包含字段来源与脱敏的存储健康；离线检查不创建或修复文件。推送文件损坏保留原位并禁用推送，修复后重启；不会自动丢弃订阅并生成新密钥。
