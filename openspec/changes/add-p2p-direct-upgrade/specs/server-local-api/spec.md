# server-local-api Specification Delta

## MODIFIED Requirements

### Requirement: 会话内进程的限制

服务端 SHALL 识别来自 WebMux 会话内部的本机连接（对端进程属于某个会话的进程树，或在 Linux 上携带会话标识环境变量）。

对这类会话内连接，以下接口 SHALL 返回 403，其余允许的接口（包括创建会话、列出会话、附着终端）SHALL 照常可用：
- `POST /local/v1/shutdown` 与 `GET /local/v1/token`；
- `/api/p2p` 与其所有子路径（`/api/p2p/*`）。

macOS 无法读取其他进程环境，识别 SHALL 以进程树为准，并接受已脱离会话进程树的后台进程无法被识别这一限制。

#### Scenario: 会话中的 agent 取 Token

- **WHEN** WebMux 会话中运行的程序请求本机 `/local/v1/token`
- **THEN** 返回 403

#### Scenario: 会话中的程序尝试控制或协商 P2P

- **WHEN** WebMux 会话中运行的程序通过本机套接字请求 `/api/p2p` 或 `/api/p2p/signal`
- **THEN** 返回 403

#### Scenario: 会话中启动兄弟会话

- **WHEN** WebMux 会话中运行的程序通过本机套接字请求 `POST /api/sessions`
- **THEN** 新会话按正常规则创建
