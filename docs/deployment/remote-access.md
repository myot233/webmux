# 远程访问与 HTTPS

手机、局域网与公网访问：Tailscale、访问链接、Cloudflare Tunnel 与 HTTPS。

## 手机访问（Tailscale）

1. 在服务器和手机上安装 Tailscale 并加入同一网络。
2. 推荐在 `[tailscale]` 设置 `mode = "serve"`，让 WebMux 通过默认的 `127.0.0.1:7681` 监听并托管自动证书的 HTTPS 入口；也可以把 `listen` 设为 `0.0.0.0:7681`（或在 Windows 托盘中勾选“允许局域网访问”）后直连。
3. 托管 Serve 时手机打开 `https://<节点>.ts.net/`；HTTP 直连时打开 `http://<Tailscale IP 或 MagicDNS 主机名>:7681` 并输入一次 token（保存在浏览器 localStorage 中）。也可以打开启动时打印的访问链接并把主机换成 Tailscale 地址。
4. 托管 Serve 的 HTTPS 地址可以安装为应用；手工 HTTP 地址只能当作普通网页使用，见 [安装为应用（PWA）](pwa-and-push.md#安装为应用pwa)。

详细配置、端口、WSL Windows provider 与故障排查见 [Tailscale Serve / Funnel 入口](../tailscale.md)。

防火墙应只允许可信 VPN 接口访问该端口。

## 访问链接

每次启动都会在标准输出打印 `WebMux local: http://<主机>:<端口>/`。

- 启动打印的本机、隧道与 Tailscale 链接及二维码均不附带 Token，避免凭据进入 journal 或日志文件；在终端执行 `webmux status --json --reveal-token` 查看当前 Token。
- 托盘“复制链接”仍会当场读取 Token 并生成 `#token=<token>` 片段。片段不会发送到服务器，前端读取并保存后立即从地址栏和当前历史记录中移除；剪贴板排除历史并在 60 秒后清除。
- WebSocket 握手通过子协议携带 token，token 不出现在请求 URL 或 Cookie 中。
- 浏览器的自动补全或同步历史仍可能记住托盘复制的带片段链接。

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
- 网页配置保存于 `<state_dir>/tunnel-settings.json`（0600），优先于 TOML；`--tunnel` / `--tunnel=false` 只覆盖本次启动。恢复配置文件设置会删除网页配置并立即关闭，下一次启动按 TOML。
- 存储损坏、锁冲突或不可读时保持本地服务可用且公网关闭，不退回 TOML 意外重开。运行中关闭即使保存失败也先撤销公网授权并停止隧道，页面提示尚未持久化；修复存储后须重新保存关闭状态。

Cloudflare 控制台步骤、两个 token 的区别、Windows/WSL 托盘操作、验证方法与故障排查见 **[Cloudflare Named Tunnel 配置指南](../cloudflare-tunnel.md)**。

> 启动链接与二维码不包含 token，以 systemd / launchd 运行时也不会把登录凭据写入 journal 或日志文件。
>
> - 首次启动写入配置文件的 token 会跨重启保留，不会每次更换；只有未持久化的临时 token 会在重启时更换。
> - 当前 token 可通过 `webmux status --json --reveal-token` 查看；托盘复制链接仍按敏感凭据处理。

## HTTPS

WebMux 不内置 TLS。需要浏览器安全上下文（系统剪贴板 API）时，推荐使用托管的 `[tailscale] mode = "serve"`（仅 tailnet）或 `mode = "funnel"`（显式公网），WebMux 会自动处理证书、端口、Host/Origin 与启动对账，详见 [Tailscale Serve / Funnel 入口](../tailscale.md)。

也可以手工使用 `tailscale serve`、Cloudflare Tunnel 或 Caddy 反向代理 HTTPS：

- 保留原始请求 `Host`；
- 把公开的 HTTPS Origin 加入 `allowed_origins`；非 443 端口必须写成带端口的完整 Origin。
