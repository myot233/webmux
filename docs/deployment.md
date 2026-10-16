# 部署指南

## 运行要求

- Linux 或 macOS（amd64 / arm64）。
- 一个可执行的 shell，以及打算使用的 CLI agent（Claude Code、Codex CLI 等）。
- 可选：Git，用于仓库与 worktree 管理（需支持 `git worktree list --porcelain -z`）。

运行端不需要 Node.js、Go、tmux 或数据库。

## 配置

```sh
mkdir -p ~/.config/webmux
cp deploy/config.example.toml ~/.config/webmux/config.toml
chmod 600 ~/.config/webmux/config.toml
```

示例见 [`deploy/config.example.toml`](../deploy/config.example.toml)。加载规则：

- 默认路径为 `~/.config/webmux/config.toml`，不存在时全部使用默认值；通过 `--config` 显式指定的文件不存在时启动失败。
- 命令行 `--listen`、`--token` 优先于配置文件，显式传入空 token 也会覆盖配置。
- `webmux version` 输出版本号。

```sh
webmux --config ~/.config/webmux/config.toml
curl -H 'Authorization: Bearer YOUR_TOKEN' http://127.0.0.1:7681/api/health
```

### 配置项

| 配置项               | 默认值                                                     | 说明                                                                              |
| -------------------- | ---------------------------------------------------------- | --------------------------------------------------------------------------------- |
| `listen`             | `127.0.0.1:7681`                                           | HTTP / WebSocket 监听地址                                                         |
| `token`              | 空                                                         | 共享 token，空表示不认证                                                          |
| `scrollback_bytes`   | `2097152`                                                  | 每会话保留的原始终端字节数（不是行数）                                            |
| `default_cwd`        | `~`                                                        | 新建会话的默认目录，支持 `~` 展开                                                 |
| `state_dir`          | `$XDG_STATE_HOME/webmux`，未设置时 `~/.local/state/webmux` | 仓库清单目录                                                                      |
| `shell`              | `$SHELL` 或 `/bin/bash`                                    | Shell 预设使用的程序，以 `-l -i` 启动                                             |
| `idle_after_seconds` | `30`                                                       | 无等待特征时判定为空闲的秒数                                                      |
| `prompt_patterns`    | 常见提示符 / 确认模式                                      | 判定“可能在等你”的 Go 正则列表                                                    |
| `agent_protocol`     | `true`                                                     | 在会话环境中设置 `WARP_CLI_AGENT_PROTOCOL_VERSION=1`，让 agent 发出结构化提醒事件 |
| `bell_attention`     | `true`                                                     | 把终端响铃（输入 1 秒后）视为需要介入                                             |
| `allowed_origins`    | `[]`                                                       | 额外允许的完整 Origin，例如 `https://myhost.example`                              |
| `presets`            | shell / claude / codex / omp                               | `name`、`label`、`cmd`（argv 数组）、`allow_args`；配置后替换默认列表             |

`custom` 是保留预设名。追加参数按引号与反斜杠规则拆分为 argv，不经 shell 求值。要改变 shell 的启动参数，可为 shell 预设显式设置 `cmd = ["/bin/zsh", "-l", "-i"]`。

### 状态目录

`state_dir` 只保存仓库清单 `repositories.json`，不保存会话、终端内容或 token。目录权限 0700、文件 0600，通过原子替换写入。

- 同一状态目录只能由一个实例使用；多个实例请配置不同的 `state_dir`。
- 清单损坏、文件锁冲突或存储不可用时，仓库管理功能被禁用，但终端服务照常运行，损坏的清单不会被覆盖。

## systemd 用户服务（Linux）

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

## agent 的 PATH

systemd 和 launchd 都不会加载交互 shell 的初始化文件。服务模板的 `PATH` 已包含 `~/.local/bin`、pnpm / npm 全局目录（macOS 另含 Homebrew）。如果 agent 安装在 nvm 等其他位置：

```sh
which claude codex
systemctl --user edit webmux    # Linux：添加 drop-in
```

```ini
[Service]
Environment=PATH=/home/YOU/.local/bin:/home/YOU/.nvm/versions/node/v24.20.0/bin:/usr/local/bin:/usr/bin:/bin
```

macOS 直接编辑 plist 的 `EnvironmentVariables` 后重新加载。

## 手机访问（Tailscale）

1. 在服务器和手机上安装 Tailscale 并加入同一网络。
2. 将 `listen` 设置为服务器的 Tailscale 地址（如 `100.64.0.5:7681`），并设置 `token`。
3. 手机浏览器打开 `http://<Tailscale IP 或主机名>:7681`，输入一次 token（保存在浏览器 localStorage 中）。
4. 可“添加到主屏幕”以独立模式运行。

防火墙应只允许可信 VPN 接口访问该端口。

## HTTPS

WebMux 不内置 TLS。需要浏览器安全上下文（系统剪贴板 API）时，可使用 `tailscale serve` 或 Caddy 反向代理 HTTPS：

- 保留原始请求 `Host`；
- 把公开的 HTTPS Origin 加入 `allowed_origins`。

## 升级

替换二进制后重启服务。**重启会结束所有终端会话**，升级前请保存工作；仓库清单和磁盘上的 worktree 会保留。只替换磁盘上的文件不会让正在运行的进程加载新版本。

## 平台差异

Linux 与 macOS 同等支持。进程状态在 Linux 读取 `/proc`，在 macOS 使用 `sysctl` 与 `proc_pidinfo`。

macOS 上：

- 没有 subreaper，脱离父进程的后代由 launchd 回收。
- 内核不公开其他进程的环境变量，`WEBMUX_SESSION_ID` 无法用于清理；关闭会话时按终端会话 ID 与进程树查找后代。先 `setsid` 再让父进程退出的自守护程序不会被清理。
- 临时目录位于 `/var → /private/var` 符号链接下，WebMux 报告规范化后的真实路径。
- Apple 自带的 `/bin/bash` 为 3.2 版本；默认 Shell 预设使用 `$SHELL`（通常为 `/bin/zsh`）。
