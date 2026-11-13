# linux-packages Specification

## Purpose

为 Linux 用户提供 deb 与 rpm 安装包，通过系统包管理器安装、升级和卸载 WebMux，并随包提供可由用户自行启用的 systemd 用户服务。

## Requirements

### Requirement: 安装包内容

每个安装包 SHALL 只包含以下文件，并 SHALL NOT 声明运行时依赖：

- 可执行文件 `/usr/bin/webmux`（权限 0755），与同一 Release 中对应架构的 `webmux-linux-<架构>` 字节相同；
- systemd 用户单元 `/usr/lib/systemd/user/webmux.service`（权限 0644）；
- 系统单元 `/usr/lib/systemd/system/webmux-update.service` 与 `/usr/lib/systemd/system/webmux-update.timer`（权限 0644）。

安装包 SHALL 附带启用、保留与停用更新定时器的安装脚本，行为见 `linux-package-auto-update`。

#### Scenario: 查看 deb 内容

- **WHEN** 用户对 `webmux_0.2.0-1_amd64.deb` 执行 `dpkg-deb -c`
- **THEN** 列表中只有 `/usr/bin/webmux`、`/usr/lib/systemd/user/webmux.service`、`/usr/lib/systemd/system/webmux-update.service` 与 `/usr/lib/systemd/system/webmux-update.timer` 四个文件，安装后 `webmux version` 输出 `webmux 0.2.0`

### Requirement: 包内用户单元

包内单元 SHALL 以 `/usr/bin/webmux serve` 运行默认配置（不存在时由首次启动生成带随机 Token 的配置），工作目录为用户主目录，`PATH` 包含 `~/.local/bin`、`~/.local/share/pnpm`、`~/.npm-global/bin`、`/usr/local/bin`、`/usr/bin`、`/bin`，异常退出时重启，`WantedBy=default.target`。

#### Scenario: 用户启用开机自启

- **WHEN** 用户安装包后执行 `systemctl --user enable --now webmux`
- **THEN** WebMux 启动并在该用户下次登录时自动启动，`~/.config/webmux/config.toml` 被生成

### Requirement: 安装与升级不改动用户服务

安装、升级与卸载 SHALL NOT 为任何用户启用、启动、停止或重启 `webmux.service`，因为重启会结束所有终端会话。升级后正在运行的服务 SHALL 继续使用旧进程，直到用户执行 `systemctl --user restart webmux`（或 `webmux service restart`）。

#### Scenario: 运行中升级

- **WHEN** 用户的 `webmux.service` 正在运行，管理员用新版本安装包升级
- **THEN** 服务进程不变，终端会话继续可用；`/usr/bin/webmux version` 报告新版本

### Requirement: 服务命令识别包内单元

在 Linux 上，`~/.config/systemd/user/webmux.service` 不存在而 `systemctl --user cat webmux.service` 能找到单元时（例如安装包提供）：

- `webmux service start|stop|restart|status` SHALL 作用于该单元；
- `webmux service uninstall` SHALL 只禁用并停止它，SHALL NOT 删除该单元文件，并说明如何彻底移除。

两者都找不到时，命令 SHALL 像以前一样提示先执行 `webmux service install`。

`webmux service install` SHALL 仍写入 `~/.config/systemd/user/webmux.service`，该文件按 systemd 规则优先于包内单元。

#### Scenario: 用包内单元查看状态

- **WHEN** 用户只安装了 deb 包并执行过 `systemctl --user enable --now webmux`，然后运行 `webmux service status`
- **THEN** 命令输出 `systemctl --user status webmux.service` 的结果，而不是提示先执行 `service install`

#### Scenario: 卸载包内单元

- **WHEN** 只有包内单元时用户执行 `webmux service uninstall`
- **THEN** 服务被禁用并停止，`/usr/lib/systemd/user/webmux.service` 仍在，输出说明删除需卸载软件包

#### Scenario: 没有任何单元

- **WHEN** 既没有用户定义也没有包内单元，用户执行 `webmux service status`
- **THEN** 命令失败并提示先执行 `webmux service install`
