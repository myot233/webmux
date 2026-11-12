# 安装为应用与手机推送

把 WebMux 安装为 PWA，并通过 Web Push 在手机上收到会话提醒。

## 安装为应用（PWA）

浏览器只为安全上下文（HTTPS 或本机回环地址 `127.0.0.1` / `localhost`）提供完整 PWA 能力，按访问地址分两类：

| 访问地址                                                                 | 能力                                                                                                                 |
| ------------------------------------------------------------------------ | -------------------------------------------------------------------------------------------------------------------- |
| HTTP 局域网或 VPN 地址，如 `http://100.64.0.5:7681`                      | 只能作为普通网页使用：Chromium 不提供安装，没有推送和系统剪贴板 API。iOS Safari 仍可“添加到主屏幕”，但推送仍不可用。 |
| HTTPS：Tailscale Serve、Cloudflare Named Tunnel、Caddy 等 HTTPS 反向代理 | 完整 PWA 能力：可安装、独立窗口运行、系统剪贴板；推送还要求来源通过 Host/Origin 校验，见下一节。                     |

- 设置 → 安装应用显示当前状态。Chrome、Edge（含 Android）提供安装提示时显示“安装 WebMux”按钮；提示打开失败或用户取消时显示备用安装说明。也可从地址栏安装图标或浏览器菜单安装。iOS / iPadOS 在 Safari 中“分享 → 添加到主屏幕”，从主屏幕打开后需重新输入 Token。
- 当前地址不是安全上下文时，该分区提示“当前地址不能提供完整 PWA 能力”。
- 不支持离线启动：会话与 PTY 都在服务端，断网时应用无法显示终端。Service Worker 只在开启推送时注册，不缓存页面或 API。
- 只支持部署在域名根路径；`start_url`、`scope` 与 Service Worker 作用域都是 `/`。

## 手机推送（Web Push）

会话出现提问、等待授权、本轮完成、错误或终端通知时，WebMux 可以经浏览器推送服务（APNs、FCM、Mozilla）提醒已开启推送的设备；页面关闭或手机锁屏后仍能收到，点击通知回到对应会话。

```toml
[push]
enabled = true
subject = "https://github.com/mzdluo123/webmux" # VAPID 联系方式：https:// URL 或 mailto:
ttl_seconds = 86400 # 推送服务为离线设备保留提醒的秒数，0..2419200
kinds = ["question", "permission", "error", "done", "notify"] # 还可加 "bell"
include_title = true # false 时通知标题固定为 WebMux
```

- Web Push 只在安全上下文可用，可从以下入口开启：
  - 手机：**Cloudflare Named Tunnel**、**Tailscale Serve/Funnel** 等已授权的 HTTPS 入口。Quick Tunnel 每次启动更换地址，旧订阅点开即失效，因此不支持。
  - 桌面浏览器：也可从 `127.0.0.1` / `localhost` 开启。
  - 非回环 HTTP（VPN / 局域网）与其他未授权的来源不支持；手工 `tailscale serve` 或自建 TLS 反向代理需把 HTTPS origin 加入 `allowed_origins`。

- 隧道或 Tailscale 入口关闭期间推送照常发出，但点开通知要等入口恢复后才能打开页面。修改 `tunnel.hostname` 或 Tailscale 入口端口后需在各设备重新开启。
- 开启步骤：用 Named Tunnel、Tailscale Serve/Funnel 等 HTTPS 入口打开 WebMux → 设置 → 通知推送 → 开启。iOS / iPadOS 需要 16.4 以上，先在 Safari 中“分享 → 添加到主屏幕”，从主屏幕图标打开并重新输入 Token（主屏幕应用不共享 Safari 的登录）。
- 通知只含会话标题、提醒种类与会话链接，不含工作目录、命令、预览或终端输出。同一信号只提醒一次，spinner 输出与重连不会重复提醒；启发式“可能在等你”和进程退出不推送。
- VAPID 密钥与订阅保存在 `<state_dir>/push.json`（0600）。文件损坏时原文件保留原位并禁用推送（终端与其他功能不受影响），修复文件后重启；不会自动丢弃订阅或重建 VAPID 密钥。没有设备开启时不产生任何出站请求。
- 推送是尽力送达：省电、勿扰或离线超过 `ttl_seconds` 时可能延迟或丢失；无 Google Play 服务的 Android 设备不支持；中国大陆网络通常无法访问 FCM；桌面 Chrome、Edge 与 Firefox 需要浏览器进程在运行。会话列表仍是唯一准确的状态来源。
