# 从本机终端使用

服务所在机器（含 WSL 内）的 shell 里可以直接用 `webmux` 命令创建、附着和分离会话：在常用终端里启动 agent，`Ctrl+]` 分离后到手机上继续，回来再 `webmux attach` 接续，全程是同一个进程。

最快路径：

```sh
webmux run -- claude      # 在当前目录创建会话并立即附着；Ctrl+] 分离
webmux ls                 # 查看会话 ID
webmux attach <ID>        # 回来接续
```

用脚本或 agent 批量控制会话（`send`、`wait`、`screen`、`chat` 等）见 [用脚本和 agent 控制 WebMux](shell-control.md)。

## 启动与附着

在服务所在机器（含 WSL 内）的 shell 中，`webmux` 经本机套接字访问服务，无需 Token：

```sh
webmux run -- claude --resume          # 在当前目录创建会话并立即附着
webmux run --detach --name review -- codex   # 只创建，输出会话 ID
webmux run --env VIRTUAL_ENV=/w/.venv --env PATH=/w/.venv/bin:/usr/bin -- python app.py
webmux run --launch-config omp --detach --name review  # 使用 Web UI 保存的启动配置（按名称）
webmux ls [--json]                     # 列出会话；--json 为服务端原始数组
webmux attach <ID> [--observe]         # 附着；Ctrl+] 分离，会话继续运行
webmux open <ID> [--browser]           # 输出（并打开）不含 Token 的会话链接
```

- `--` 之后的参数原样作为 argv，不经 shell 展开；`--cwd` 指定启动目录（默认当前目录，`~` 由服务端展开）。
- 自定义命令的环境变量：
  - 默认继承**调用方 shell 的环境**（如 `PATH`、`VIRTUAL_ENV`、`SSH_AUTH_SOCK`、API key），在服务进程环境之上覆盖同名变量，`--env NAME=VALUE` 再覆盖继承值。
  - `TERM`、`COLORTERM`、宿主终端身份变量、`WEBMUX_SESSION_ID`、`WEBMUX_SOCKET`、`WEBMUX_BIN`、`PWD`/`OLDPWD` 与不合法的变量名不继承，会话仍以 WebMux 终端身份和 `--cwd` 目录启动。
  - `--launch-config` 只使用配置中保存的环境，不继承调用方环境。
  - Windows 客户端（`webmux.exe`）也不把 Windows 环境复制到 WSL，只发送 `--env` 显式列出的变量。
- `--launch-config <NAME>`（也可写作 `--config-id <NAME>`）使用 Web 端“启动配置”中保存的完整 argv 与环境变量：
  - `NAME` 是设置页显示的配置名称（名称唯一，区分大小写）；名称不存在时以用法错误（退出码 2）结束并列出已有名称。
  - CLI 只读取 `/api/launch-configs` 摘要把名称换成 ID，命令由服务端按 ID 展开，因此敏感环境值不会经过 CLI。
  - 它与 `--` 后的命令、`--env` 互斥，但仍可配合 `--cwd`、`--name` 和 `--detach`。
  - 不带该选项时仍使用临时命令模式。
- 会话终端身份：
  - 默认以 `TERM=xterm-256color`、`COLORTERM=truecolor` 和 `TERM_PROGRAM=webmux` 启动，并报告当前 WebMux 版本；宿主 Kitty、tmux、Warp、iTerm、WezTerm、Ghostty、Windows Terminal 等身份变量不会泄漏到子进程。
  - 需要显式启用 Kitty 专用行为时，应在 Web 端启动配置中保存相应的 `TERM`/终端变量；这不会改变浏览器对 Kitty 图形协议的支持，也不会把普通会话伪装成 Kitty。
- 附着时终端切换为原始模式，默认取得控制权并同步窗口尺寸；`--observe` 只观察。会话退出时以其退出码结束；分离以 0 结束；服务未运行以 3 结束，不会自动启动服务。附着开始时以终端复位后的完整回放呈现会话内容。
- 浏览器等其他客户端接管后，原生终端左上角显示 `Controlled by <客户端> · Enter to take control · Ctrl+] to detach`：
  - 期间的按键（包括终端对焦点、设备、颜色查询的自动应答）一律丢弃，不会抢回控制。
  - 按 Enter 重新接管（该 Enter 不发送给会话），随后按本终端尺寸重绘。程序开启 modifyOtherKeys、kitty 键盘协议或 Windows Terminal 的 win32-input-mode 后，Enter 与 `Ctrl+]` 的编码形式同样识别。
  - 附着结束时关闭这些键盘、鼠标、焦点与粘贴上报模式，避免外层 shell 收到无法解析的按键编码。
- `run --detach --json` 输出 `{session, configWarnings}`，新建 worktree 时另含 `worktree`；不再包含 `link`，需要链接时用 `webmux open <ID>`。
- 默认附着的 `run` 在接管终端前，向标准错误输出 `webmux: session <ID> · Ctrl+] to detach`；标准输出只属于会话。需要在浏览器打开该会话时，用 `webmux open <ID>` 取得不含 Token 的本机链接。
- 没有终端时（例如 agent 的工具调用或管道）`run` 自动按 `--detach` 处理并在标准错误说明；`--attach` 强制附着，此时没有终端仍以退出码 2 结束且不创建会话。
- 典型流程：在原生终端 `webmux run -- claude` → `Ctrl+]` 分离 → 在能访问该主机的浏览器打开会话（本机可用 `webmux open <ID>` 取得链接）→ 输入即接管 → 回来后 `webmux attach <ID>` 接续，全程是同一个进程。
- 脚本与 agent 的用法（`self`、`WEBMUX_SOCKET`/`WEBMUX_BIN`、`get`、标签、`help --json`）见 [用脚本和 agent 控制 WebMux](shell-control.md)。

## 在 Windows 终端中使用

`webmux-windows-<arch>.exe`（建议放入 `PATH` 并重命名为 `webmux.exe`）在 PowerShell / Windows Terminal 中提供 `run`、`attach`、`ls`、`open`、`status` 与 `version`，会话运行在 WSL 中的 WebMux 里。

- **连接**：每条连接都会启动 `wsl.exe --exec webmux bridge` 访问本机 API，因此不需要 Token，也不依赖 WSL 的 localhost 转发或网络模式。
- **服务**：需已在 WSL 中运行（托盘或 `webmux serve`）。
- **发行版**：`--distro`、环境变量 `WEBMUX_DISTRO`、托盘设置 `%APPDATA%\WebMux\tray.json`、WSL 默认发行版，依次生效。WSL 内 `webmux` 路径同理：`--webmux`、`WEBMUX_WSL_PATH`、托盘设置、`~/.local/bin/webmux`。
- **目录换算**：未指定 `--cwd` 时把当前目录换算为 WSL 路径：盘符路径按发行版的自动挂载规则（`wslpath`，默认 `C:\work` → `/mnt/c/work`）；`\\wsl.localhost\<发行版>\…` 或 `\\wsl$\<发行版>\…` 换算为对应 Linux 路径；网络共享、其他发行版与设备路径无法换算，以退出码 2 结束，请用 `--cwd /linux/path` 指定。
- **控制台**：附着时控制台进入 VT 原始模式并使用 UTF-8 代码页，`Ctrl+C` 发送给会话，`Ctrl+]` 分离；窗口尺寸每 250 毫秒同步一次；退出时恢复原模式与代码页。需要 Windows 10 22H2 及以上，推荐 Windows Terminal。
- **引号**：PowerShell 在把参数交给原生程序前会再处理一次引号。需要字面传递 `$`、`;` 等字符时用单引号（`webmux run -- printf '%s;$(id)' 'a b'`），或在 `--` 前使用 `--%` 停止解析。

## 在后台运行服务

没有 systemd / launchd 时，可以用 `webmux start` 让服务脱离当前 shell 继续运行：

```sh
webmux start --listen 127.0.0.1:7681
webmux status
webmux stop
```

`start` 接受与 `serve` 相同的参数：

- 日志：默认把标准输出与标准错误写入 `<state_dir>/webmux.log`（0600，旧文件保留为 `.1`），显式 `--log-file` 优先。
- 输出：命令等待子进程发布运行时记录后输出 PID、不含 token 的本机链接与日志路径，然后退出。
- 重复启动：同一实例已运行时只打印状态并以 0 结束，不会启动第二个进程。
- 隧道：启用隧道时首次启动还会等待 cloudflared 下载。
- 停止：子进程清除启动方继承的会话与服务管理环境，运行时记录保持为手动启动，因此 `webmux stop` 可直接停止。

生产环境仍推荐 `webmux service install`。
