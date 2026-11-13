# websocket-compression Specification

## Purpose

在移动网络等慢速链路上减少实时连接流量：手机与平板默认启用 WebSocket 压缩，用户可按设备覆盖，服务端只在客户端明确要求时协商。

## Requirements

### Requirement: 手机默认启用实时连接压缩

用户未在设置页选择过“压缩实时连接”时，前端 SHALL 在主指针为粗指针的设备（手机、平板）上默认启用压缩，在其他设备上默认关闭。启用时，列表、终端、聊天与设置 WebSocket 的 URL SHALL 带 `compress=1`。

#### Scenario: 手机首次打开

- **WHEN** 主指针为粗指针且本机没有保存过压缩选择
- **THEN** 所有实时连接以 `compress=1` 建立，设置页开关显示为开启

#### Scenario: 桌面首次打开

- **WHEN** 主指针为精细指针且本机没有保存过压缩选择
- **THEN** 实时连接不带 `compress=1`，设置页开关显示为关闭

### Requirement: 用户选择优先于设备默认值

用户在设置页切换压缩开关后，选择 SHALL 保存在当前浏览器并优先于设备默认值；切换 SHALL 立即重连已打开的连接并按 offset 续传。

#### Scenario: 手机上关闭压缩

- **WHEN** 用户在手机上关闭开关
- **THEN** 已打开的连接以不带 `compress=1` 的地址重连，之后刷新页面仍保持关闭

### Requirement: 服务端仅按请求协商压缩

服务端 SHALL 仅在 WebSocket 握手带 `compress=1` 且客户端提议 `permessage-deflate` 时协商该扩展；不带参数时 SHALL NOT 协商，即使浏览器总是提议它。认证与 Origin 校验 SHALL 不受影响。

#### Scenario: 未带参数

- **WHEN** 握手提议 `permessage-deflate` 但不带 `compress=1`
- **THEN** 响应不包含 `Sec-WebSocket-Extensions`
