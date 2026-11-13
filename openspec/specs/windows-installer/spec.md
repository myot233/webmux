# windows-installer Specification

## Purpose

为 Windows 用户提供按用户安装的 WebMux 托盘安装程序：把托盘放到固定位置并登记开始菜单、卸载项与可选的登录启动，随包预置对应架构的 WSL 服务端，并支持覆盖升级与干净卸载。

## Requirements

### Requirement: 按用户安装且无需管理员

安装程序 SHALL 只为当前 Windows 用户安装，SHALL NOT 写入 `HKEY_LOCAL_MACHINE`、`Program Files` 或其他用户的目录。默认安装目录 SHALL 为 `%LOCALAPPDATA%\Programs\WebMux`，用户可以在安装界面改为其他可写目录。安装程序界面 SHALL 使用简体中文。

安装本身 SHALL NOT 请求管理员权限。唯一的例外是“防火墙规则”中用户勾选的可选规则：添加时 SHALL 只弹出一次管理员确认，用户取消时安装照常完成。

#### Scenario: 普通用户安装

- **WHEN** 非管理员用户双击 `webmux-setup-windows-amd64.exe`，取消勾选防火墙选项并接受其他默认选项
- **THEN** 安装过程中不出现 UAC 提示，`%LOCALAPPDATA%\Programs\WebMux\webmux-tray.exe` 存在

#### Scenario: 自定义目录

- **WHEN** 用户把安装目录改为 `D:\Tools\WebMux`
- **THEN** 托盘被安装到 `D:\Tools\WebMux\webmux-tray.exe`，开始菜单快捷方式与卸载项指向该路径

### Requirement: 安装内容与入口

安装程序 SHALL 把托盘安装为安装目录下的 `webmux-tray.exe`，并在当前用户的开始菜单中创建“WebMux”快捷方式。安装程序 SHALL 在当前用户的“应用和功能”中登记卸载项，显示名称为“WebMux”，并包含发布版本号、发布者与卸载程序路径。

#### Scenario: 开始菜单与卸载项

- **WHEN** 安装完成
- **THEN** 开始菜单中出现“WebMux”，“应用和功能”中出现“WebMux”及其版本号，可从该处卸载

### Requirement: 安装时登记登录启动

安装界面 SHALL 在单独的“安装选项”页提供默认勾选的“开机后自动启动 WebMux（登录 Windows 时启动托盘）”，SHALL NOT 以组件列表的形式呈现。

勾选时 SHALL 写入与托盘菜单“登录时自动启动”相同的当前用户启动项，指向安装后的 `webmux-tray.exe`，使托盘菜单中的复选项显示为已勾选；取消勾选时 SHALL NOT 新建或修改该启动项。

#### Scenario: 默认勾选

- **WHEN** 用户保持默认选项完成安装后右键托盘图标
- **THEN** “登录时自动启动”显示为已勾选；注销并重新登录后托盘自动出现

#### Scenario: 取消勾选

- **WHEN** 用户取消勾选后完成安装，且此前不存在托盘启动项
- **THEN** 当前用户的登录启动项中没有 WebMux 托盘

### Requirement: 架构匹配

每个安装包只包含一个架构的托盘。安装程序 SHALL 在写入任何文件前检查 Windows 本机架构：与安装包架构不符时 SHALL 提示应下载的安装包名称并退出，SHALL NOT 安装任何文件。

#### Scenario: ARM 设备运行 amd64 安装包

- **WHEN** 用户在 Windows arm64 设备上运行 `webmux-setup-windows-amd64.exe`
- **THEN** 安装程序提示请改用 `webmux-setup-windows-arm64.exe` 并退出，磁盘与注册表没有变化

### Requirement: 预置 WSL 服务端

安装包 SHALL 内置一个与托盘同一次构建、架构与安装包相同的 Linux 服务端，并放到托盘的下载缓存位置：

- amd64 安装包内置 `linux-amd64`，arm64 安装包内置 `linux-arm64`；
- WSL 架构与之相同时，托盘无需联网即可完成“安装 / 升级 WSL 中的 webmux”。

安装程序 SHALL NOT 访问或修改 WSL 及其中的文件，也 SHALL NOT 自动触发托盘的 WSL 安装动作。

#### Scenario: 离线安装服务端

- **WHEN** 在无法访问网络的 amd64 Windows 上完成安装，WSL 发行版为 `x86_64` 且缺少 webmux，用户点击托盘菜单“安装 WSL 中的 webmux”
- **THEN** 安装成功，托盘日志记录使用了本地文件，没有发起网络下载

#### Scenario: 安装程序不改动 WSL

- **WHEN** 安装程序完成，用户尚未操作托盘菜单
- **THEN** WSL 中配置路径上的 webmux 与安装前相同

#### Scenario: WSL 架构不同

- **WHEN** amd64 安装包完成安装，但目标发行版为 `aarch64`
- **THEN** 托盘按原有流程联网下载 `webmux-linux-arm64` 并校验后安装

### Requirement: 覆盖安装即升级

已安装时再次运行安装程序 SHALL 视为升级或重装，并按以下顺序处理正在运行的托盘：

1. 替换文件之前，安装程序 SHALL 请求当前用户的托盘退出，效果等同“仅退出托盘”：WebMux 与保持发行版存活的进程继续运行，终端会话不受影响。
2. 托盘 10 秒内未退出时，安装程序 SHALL 强制结束从安装目录运行的托盘进程。
3. 安装完成后，安装程序 SHALL 启动新托盘。

#### Scenario: 运行中升级

- **WHEN** 托盘正在运行，WebMux 有 2 个会话，用户运行新版本安装程序
- **THEN** 旧托盘退出，文件被替换，新托盘启动并显示运行中；2 个会话保持可用

#### Scenario: 托盘无响应

- **WHEN** 已安装的托盘在收到退出请求后 10 秒内仍未退出
- **THEN** 安装程序结束该进程后继续安装，不弹出“文件被占用”错误

### Requirement: 与其他位置的托盘共存

当前用户正在运行一个不在安装目录、且不响应退出请求的托盘（例如旧版本或手工存放的 exe）时，安装程序 SHALL NOT 结束它，SHALL 提示用户先从托盘菜单退出并提供重试与取消；用户取消时 SHALL NOT 留下部分安装。

#### Scenario: 手工存放的旧托盘

- **WHEN** 用户运行着 `Downloads\webmux-tray-windows-amd64.exe`（不支持退出请求的旧版本），然后运行安装程序
- **THEN** 安装程序提示先退出该托盘；用户退出后点击重试，安装继续完成并启动新托盘

### Requirement: 卸载

卸载程序 SHALL 先按“覆盖安装即升级”的方式让托盘退出，再删除安装程序写入的内容：安装目录中的文件、开始菜单快捷方式、卸载项与预置的服务端文件。登录启动项只有指向本次安装的 `webmux-tray.exe` 时才 SHALL 删除。

安装程序为本次安装添加过防火墙规则时，卸载 SHALL 删除该规则（弹出一次管理员确认）；用户取消时 SHALL 说明如何手工删除，卸载照常完成。

卸载 SHALL NOT 停止 WebMux，SHALL NOT 删除托盘设置、托盘日志、保活记录或 WSL 中的任何文件。完成页 SHALL 说明保留了哪些内容；WebMux 正在运行时还 SHALL 说明它仍在 WSL 中运行。

#### Scenario: 卸载保留数据

- **WHEN** WebMux 正在运行，用户从“应用和功能”卸载 WebMux
- **THEN** 托盘消失，安装目录、快捷方式与卸载项被删除；`%APPDATA%\WebMux\tray.json`、`%LOCALAPPDATA%\WebMux\tray.log` 与 WSL 中的 webmux 保留，浏览器中的会话继续可用

#### Scenario: 启动项指向其他路径

- **WHEN** 登录启动项指向另一个位置的托盘 exe，用户卸载安装版
- **THEN** 该启动项保持不变

#### Scenario: 删除防火墙规则

- **WHEN** 安装时添加过防火墙规则，用户卸载并在管理员确认中选择“是”
- **THEN** 以“WebMux tray forwarding”开头、指向该托盘的规则被删除

### Requirement: 静默安装

安装程序与卸载程序 SHALL 支持无界面的静默模式：

- 首次安装：SHALL 使用默认选项（含登录启动）。
- 覆盖已有安装：SHALL 沿用已有的安装目录，并 SHALL 保持登录启动项的现状（已删除的不重新登记，指向本次安装的保持不变）。
- 架构不符或无法关闭其他位置的托盘：SHALL 以非零退出码结束且不留下部分安装。
- 升级失败：SHALL 恢复升级前的 `webmux-tray.exe`（即使新文件已部分写入）。安装程序关闭过运行中的托盘时 SHALL 重新启动它；没有运行中的托盘时 SHALL NOT 启动托盘。
- 退出码：`0` 成功；`2` 架构不符，或未关闭任何托盘时失败；`3` 无法关闭运行中的托盘，未写入任何文件；`4` 关闭托盘之后失败，已重新启动原托盘。

#### Scenario: 静默安装成功

- **WHEN** 用户在命令行运行 `webmux-setup-windows-amd64.exe /S`
- **THEN** 进程以退出码 0 结束，安装内容与交互式默认安装相同

#### Scenario: 静默安装失败

- **WHEN** 在 arm64 设备上运行 `webmux-setup-windows-amd64.exe /S`
- **THEN** 进程以非零退出码结束，磁盘与注册表没有变化

#### Scenario: 静默升级保留登录启动选择

- **WHEN** 用户安装时取消了“开机后自动启动 WebMux”，之后托盘以 `/S` 运行新版本安装程序
- **THEN** 升级完成后没有登录启动项，安装目录与之前相同

#### Scenario: 静默升级失败

- **WHEN** 静默升级关闭托盘后因文件写入失败而中止
- **THEN** 进程以非零退出码结束，安装目录中恢复为升级前的托盘并被重新启动

#### Scenario: 没有运行托盘时安装失败

- **WHEN** 当前用户没有运行托盘，覆盖安装在写入文件时失败
- **THEN** 进程以退出码 2 结束，安装目录中恢复为升级前的托盘，没有启动托盘

### Requirement: 防火墙规则

交互安装的“安装选项”页 SHALL 提供“允许其他设备通过托盘转发访问 WebMux（添加 Windows 防火墙规则）”，并说明只放行专用网络与域网络、会弹出一次管理员确认。首次安装时 SHALL 默认勾选；覆盖已有安装时 SHALL 默认沿用上次是否添加了规则。

- 勾选时，安装完成后 SHALL 为安装目录中的 `webmux-tray.exe` 添加入站 TCP 允许规则，只适用于专用网络与域网络，SHALL NOT 适用于公用网络。
- 规则已存在时 SHALL NOT 再次请求管理员权限；成功后 SHALL 在卸载项中记录该托盘路径，供以后删除。
- 覆盖安装时取消勾选，或安装目录与记录的路径不同，SHALL 删除记录的规则（弹出管理员确认）。
- 用户取消管理员确认或添加失败时，安装 SHALL 照常完成，并说明之后如何手工添加或删除。
- 静默安装（`/S`）SHALL NOT 请求管理员权限，SHALL NOT 添加或删除规则；静默覆盖原路径时已有规则继续有效。

#### Scenario: 首次安装时授权

- **WHEN** 用户交互安装并保持勾选防火墙选项，在管理员确认中选择“是”
- **THEN** 防火墙中出现指向 `%LOCALAPPDATA%\Programs\WebMux\webmux-tray.exe` 的入站允许规则，配置文件为“专用、域”；之后托盘开始转发时不再出现 Windows 防火墙提示

#### Scenario: 取消管理员确认

- **WHEN** 用户在管理员确认中选择“否”
- **THEN** 安装成功完成，安装程序提示已跳过防火墙规则及手工添加的命令

#### Scenario: 重新安装时取消勾选

- **WHEN** 之前安装时添加过规则，用户再次交互安装并取消勾选防火墙选项，在管理员确认中选择“是”
- **THEN** 之前的规则被删除，卸载项不再记录规则

#### Scenario: 托盘自更新

- **WHEN** 已添加规则的安装版托盘以 `/S` 静默更新到新版本
- **THEN** 没有管理员确认，原规则继续适用于新托盘
