# windows-tray-forwarding Specification

## Purpose

由托盘中的“允许局域网访问”开关统一控制 WSL 中 WebMux 的局域网/Tailscale 开放状态（在 `127.0.0.1:<端口>` 与 `0.0.0.0:<端口>` 之间切换），并在 WSL NAT 网络模式下自动在 Windows 主机候选网卡地址上提供受控的 HTTP/WebSocket 转发入口，同时保持 Token 认证与 Origin 同源安全边界。

## Requirements

### Requirement: 允许局域网访问开关与适用判定

托盘菜单 SHALL 提供“允许局域网访问”开关项，反映当前 WebMux `listen` 是否对非回环地址开放（例如 `0.0.0.0:7681` 为勾选，默认 `127.0.0.1:7681` 为未勾选）。

用户切换该开关时，托盘 SHALL 调用 `webmux config listen` 将 WSL 配置文件中的 `listen` 更新为 `0.0.0.0:<端口>`（开启）或 `127.0.0.1:<端口>`（关闭），并在实例由托盘管理且正在运行时重启 WebMux 使新监听地址生效（存在活动会话时先经用户确认）。

当“允许局域网访问”处于开启状态时，托盘 SHALL 按以下规则决定是否启用 Windows 侧转发：

1. WSL 处于镜像（mirrored）网络模式，或 `webmux status` 已报告可用的远程链接：无需 Windows 侧转发，其他设备直接访问 WSL；
2. 否则（典型为 NAT 网络模式）：托盘自动在 Windows 主机的所有候选非回环 IPv4 地址（排除 WSL 虚拟交换机与链路本地地址，Tailscale 置顶）上启用转发。

#### Scenario: 镜像网络模式

- **WHEN** WSL 使用镜像网络模式且“允许局域网访问”已开启
- **THEN** `PlanForward` 判定无需 Windows 侧转发，其他设备直接访问 WSL 的 `0.0.0.0:<端口>`

#### Scenario: NAT 模式下开启局域网访问

- **WHEN** WSL 使用 NAT 模式，Windows 主机有 Tailscale 地址 100.64.0.9 与局域网地址 192.168.1.20，用户开启“允许局域网访问”
- **THEN** WSL 中 WebMux 切换为监听 `0.0.0.0:7681` 并重启，托盘自动在 `100.64.0.9:7681` 与 `192.168.1.20:7681` 上监听并转发到 `127.0.0.1:7681`

### Requirement: 启用条件

只有同时满足以下条件时，托盘 SHALL 在 NAT 模式下运行转发监听：“允许局域网访问”已开启、WebMux 正在运行且健康、配置了非空 Token、Windows 侧可经本机链接访问 WebMux。任一条件不满足时，已启用的转发 SHALL 暂停监听，条件恢复后自动恢复。

#### Scenario: 关闭局域网访问

- **WHEN** 用户取消勾选“允许局域网访问”并确认重启
- **THEN** WSL 中 WebMux 切回 `127.0.0.1:7681`，托盘停止所有 Windows 侧转发监听

#### Scenario: WebMux 停止

- **WHEN** 转发已在 `100.64.0.9:7681` 启用，用户停止 WebMux
- **THEN** 托盘停止在该地址监听；WebMux 再次就绪后自动恢复监听

### Requirement: 监听范围

NAT 模式转发 SHALL 监听 Windows 主机上的候选具体非回环 IPv4 地址，端口与 WebMux 实际端口相同；转发器本身 SHALL NOT 监听通配地址或回环地址。某个地址端口已被占用时 SHALL 记录失败原因，不影响其他候选地址。Windows 主机地址变化时 SHALL 自动停止失效地址并在新地址出现后开始监听。

#### Scenario: 端口被占用

- **WHEN** Windows 主机上已有程序监听 `192.168.1.20:7681`
- **THEN** 该地址转发失败不影响 `100.64.0.9:7681` 照常转发

### Requirement: 转发的安全边界

转发 SHALL 是对 WebMux HTTP 与 WebSocket 流量的透明中转，并 SHALL 满足：

- 接受通过该转发入口到达的 IP 字面量或主机名（如 Tailscale MagicDNS `*.ts.net:<端口>`）请求；
- 请求带 `Origin` 时，只接受与请求 `Host` 同源（`http://<Host>`）的请求，否则以 403 拒绝；
- 通过校验的请求，转发 SHALL 把 `Host` 与同源的 `Origin` 改写为 WebMux 本机链接对应的值后转给 WebMux，其余请求头（含 `Authorization`、Cookie）与请求体原样传递；
- 转发 SHALL NOT 添加、读取或记录 Token，SHALL NOT 记录请求体、响应体或 WebSocket 消息内容；认证仍完全由 WebMux 判定；
- WebSocket 升级与二进制帧 SHALL 原样双向传递，不缓冲整条消息；任一方向关闭时 SHALL 关闭另一方向。

#### Scenario: 手机经转发访问

- **WHEN** 转发启用于 `100.64.0.9:7681`，手机浏览器打开 `http://100.64.0.9:7681` 或 MagicDNS 主机名并输入正确 Token
- **THEN** 手机可以列出会话、打开终端、输入并看到实时输出，与直连行为一致

#### Scenario: 错误的 Token

- **WHEN** 经转发的请求携带错误 Token
- **THEN** 返回 WebMux 的 401 响应，转发本身不改变结果

#### Scenario: 跨站请求

- **WHEN** 第三方网页向 `http://100.64.0.9:7681/api/sessions` 发起带 `Origin: https://evil.example` 的请求
- **THEN** 转发以 403 拒绝，请求不会到达 WebMux

### Requirement: 资源与生命周期

转发 SHALL 限制并发连接数（默认 64）并在连接空闲 10 分钟后关闭非 WebSocket 连接；WebSocket 连接只在任一端关闭或 WebMux 停止时结束。托盘退出时 SHALL 关闭所有转发监听与连接；退出确认对话框 SHALL 说明“仅退出托盘”会停止转发。

托盘“复制链接”子菜单中的远程链接 SHALL 同时列出转发地址并标注“经托盘转发”。

#### Scenario: 退出托盘提示

- **WHEN** 转发已启用，用户选择“退出”
- **THEN** 对话框说明转发将停止；选择“仅退出托盘”后手机无法再经转发地址访问，WSL 中的 WebMux 继续运行

### Requirement: 防火墙放行

托盘运行时 SHALL NOT 修改 Windows 防火墙。其他设备能否连到转发地址取决于防火墙：安装版可以在安装时添加专用网络与域网络的放行规则（见 `windows-installer` 的“防火墙规则”）；便携版或未添加规则时，由 Windows 在首次监听时弹出原生提示。

#### Scenario: 安装时已放行

- **WHEN** 安装时添加了防火墙规则，当前网络为专用网络，用户勾选转发地址
- **THEN** 转发开始监听时不出现 Windows 防火墙提示，同一网络中的其他设备可以连接

#### Scenario: 公用网络

- **WHEN** 当前网络被 Windows 识别为公用网络，用户勾选转发地址
- **THEN** 安装时添加的规则不适用，是否放行由 Windows 原生提示决定
