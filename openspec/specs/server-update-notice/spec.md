# server-update-notice Specification

## Purpose

让正在运行的 WebMux 在其可执行文件被更新程序或用户替换为其他版本后，通过 CLI、本机状态 API 与 Web UI 告诉用户“新版本重启后生效，重启会结束所有会话”，而不自行重启。

## Requirements

### Requirement: 识别已安装版本

服务端 SHALL 在启动时记录自身可执行文件的路径与文件标识。之后在被查询状态时，若距上次检查已超过 60 秒且该路径上的文件标识发生变化，SHALL 以 argv 运行该路径的 `version --json`（5 秒超时，不访问网络）得到已安装版本。

- 运行失败、超时或输出无法解析时 SHALL 视为未知，SHALL NOT 报告待生效版本，且下一次检查 SHALL 重新运行。
- 只有成功读取的版本 SHALL 按文件标识缓存。

#### Scenario: 包升级后

- **WHEN** 0.4.0 的服务端运行中，`/usr/bin/webmux` 被软件包升级为 0.5.0，用户在 60 秒后执行 `webmux status`
- **THEN** 服务端识别出已安装版本 0.5.0

#### Scenario: 文件被删除

- **WHEN** 运行中服务端的可执行文件被删除且原路径没有新文件
- **THEN** 服务端不报告待生效版本，照常运行

#### Scenario: 读取超时后重试

- **WHEN** 文件被替换为 0.5.0 后第一次读取因机器繁忙超时，60 秒后用户再次执行 `webmux status`
- **THEN** 服务端重新运行 `version --json`，识别出已安装版本 0.5.0

### Requirement: 状态中的待生效版本

已安装版本已知且与运行版本不同时，本机状态 API `/local/v1/status` 与 `webmux status --json` SHALL 包含可选字段 `installedVersion`；相同或未知时 SHALL 省略该字段。`webmux status` 的文本输出 SHALL 增加一行“已安装 <版本>，重启 WebMux 后生效，重启会结束所有会话”。

#### Scenario: 状态 JSON

- **WHEN** 运行 0.4.0，已安装 0.5.0，执行 `webmux status --json`
- **THEN** 输出对象中 `version` 为 `0.4.0`，`installedVersion` 为 `0.5.0`

#### Scenario: 无更新

- **WHEN** 运行版本与已安装版本相同
- **THEN** 状态 JSON 不包含 `installedVersion`

### Requirement: Web UI 提示

已认证的配置响应 SHALL 在存在待生效版本时包含 `installedVersion` 与启动方 `launcher`。Web UI SHALL 在主页顶部与设置页显示提示，SHALL NOT 提供重启按钮。提示内容为已安装的版本、重启后生效、重启会结束所有会话，以及按启动方给出的重启方式：

- `systemd`：`systemctl --user restart webmux`；
- `launchd`：`webmux service restart`；
- `windows-tray`：“在托盘菜单中重启”；
- `manual`：“停止后重新运行 webmux”。

#### Scenario: systemd 服务

- **WHEN** 由 systemd 用户服务运行的 0.4.0 已被替换为 0.5.0，用户打开主页
- **THEN** 主页顶部提示已安装 0.5.0、重启会结束所有会话，并显示 `systemctl --user restart webmux`

#### Scenario: 旧服务端

- **WHEN** 新版前端连接到不返回该字段的服务端
- **THEN** 前端不显示提示，其他功能正常

### Requirement: 提示可按版本关闭

用户 SHALL 能关闭主页上的提示；关闭记录 SHALL 只保存在当前设备，并按版本区分：同一版本不再在主页显示，出现更新的待生效版本时 SHALL 重新显示。设置页中的提示 SHALL 始终显示。

#### Scenario: 关闭后出现新版本

- **WHEN** 用户关闭了 0.5.0 的提示，之后已安装文件又更新到 0.6.0
- **THEN** 主页重新显示 0.6.0 的提示

### Requirement: 不自动重启

服务端 SHALL NOT 因检测到待生效版本而退出、重启或改变任何会话。

#### Scenario: 有会话时

- **WHEN** 已安装版本变化，服务端有 3 个会话
- **THEN** 3 个会话与服务端进程保持不变
