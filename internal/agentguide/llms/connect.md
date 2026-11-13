# 连接与身份

`webmux` 命令经本机 Unix 套接字访问服务，不需要 Token。

## 可执行文件

下文的 `webmux` 指 WebMux 可执行文件，按以下顺序确定：

1. 环境变量 `WEBMUX_BIN`，写作 `"$WEBMUX_BIN"`（带引号以容忍路径中的空格）；
2. `{{webmux}}` 简述或本实例信息中给出的绝对路径；
3. `PATH` 中的 `webmux`。

## 会话中的变量

| 变量                | 含义                                    |
| ------------------- | --------------------------------------- |
| `WEBMUX_SESSION_ID` | 当前会话 ID；命令中的 `self` 指的就是它 |
| `WEBMUX_SOCKET`     | 本实例本机 API 套接字的绝对路径         |
| `WEBMUX_BIN`        | 服务端正在运行的可执行文件的绝对路径    |

在聊天视图中运行的 agent 进程同样有这三个变量。

## 定位服务

CLI 依次使用 `--socket PATH`、`--config PATH`、`WEBMUX_SOCKET`、默认配置。指定的套接字连不上时以退出码 3 结束，不会改连其他实例。

## 会话内被禁止的操作

会话内的进程不能停止服务、读取 Token、控制 Cloudflare Tunnel 或修改 Tailscale 设置、P2P 设置与信令，这些请求返回 403。
P2P 诊断通过独立的本机只读入口提供，不放开原有控制端点。

## 排错

1. 运行 `"$WEBMUX_BIN" version` 与 `"$WEBMUX_BIN" status`，比较客户端与服务版本；同一路径也可能被升级替换；
2. 工具 shell 丢失了 `WEBMUX_*` 变量时，使用简述或本实例信息（`/llms.txt` 的“本会话 ID”）中的绝对路径与会话 ID，加 `--socket <套接字>`，并把 `self` 换成该会话 ID；
3. 没有可用的可执行文件时，直接调用本机 API：`curl -s --unix-socket <套接字> http://webmux/api/sessions`，手册本身也可以这样读取：`curl -s --unix-socket <套接字> http://webmux/llms.txt`；
4. 处在禁止访问套接字或外部程序的沙箱中时，告诉用户无法控制 WebMux，不要反复重试；
5. 版本不匹配时使用与服务匹配的客户端并核对其帮助，不能仅凭路径判断版本。

## P2P 诊断

用 `p2p status` 查看当前配置、编译能力、UDP 实际绑定与网关映射，用 `p2p peers` 查看连接状态、候选对、IP 族、RTT 与流通道数。
两者可在会话内读取，不修改配置。没有测量的指标是 unknown；RTT 与候选元数据可能来自浏览器，RTT 不一定是 ICE RTT。
流通道数不含控制和 RPC 通道。选项、输出与退出码以各命令的 `--help` 为准。

`p2p probe` 向配置的 STUN 或显式目标发送有限 UDP 绑定请求，返回本地、远端与 STUN 观察到的映射地址及失败原因。
探测来自 **CLI 的网络和临时端口**，不是运行服务的 ICE 套接字；不能据此断言 NAT 类型、网关映射可达或保证穿透。
每目标一次请求、最多 3 秒，整体有界且可取消；UDP 探测不支持需要 TLS/TCP 的 `stuns:`。

P2P 构建支持 UDP4/UDP6，浏览器 offer、服务端 full ICE answer，无 TURN；Hard NAT 可能仍失败并回落 HTTP/WS。
`nop2p` 构建会报告不支持 WebRTC，独立 CLI UDP 探测仍可使用。
