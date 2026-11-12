# 官网建设指南

本目录是为 WebMux 建设官方网站的对照指南：从定位与信息架构，到静态站点生成器选型、页面清单、视觉规范、交互与可访问性、SEO、站点功能与验收清单。它描述“官网应当做成什么样、如何检查”，建站代码本身放在独立仓库（原因见 [技术方案](website/stack.md#仓库位置)），不进入本仓库。

## 阅读顺序

| 文件                                     | 回答的问题                                           |
| ---------------------------------------- | ---------------------------------------------------- |
| [定位与信息架构](website/strategy.md)    | 官网给谁看、说什么、怎么证明、怎么度量               |
| [技术方案](website/stack.md)             | 用哪个静态站点生成器、放哪、怎么构建与部署           |
| [页面清单与首页区块](website/pages.md)   | 有哪些页面、每个区块放什么、文案规则                 |
| [视觉与品牌规范](website/design.md)      | 颜色、字体、Logo、截图与深色模式如何与产品保持一致   |
| [交互与可访问性](website/ux.md)          | 导航、首屏任务、反馈、键盘与屏幕阅读器、移动端、性能 |
| [SEO 规范](website/seo.md)               | URL、元信息、结构化数据、关键词、外链与搜索平台提交  |
| [站点功能清单](website/features.md)      | 安装命令生成器、下载表、更新日志等功能的优先级与验收 |
| [验收清单](website/checklist.md)         | 按阶段逐项打勾的对照表，含上线日与每次发布后的维护项 |
| [官网原型](website/prototypes/README.md) | 候选方向与选定方向的静态 HTML 原型及其截图           |

## 使用方式

1. 先读 [定位与信息架构](website/strategy.md)，确认价值主张与页面范围，再动手搭骨架。
2. 按 [技术方案](website/stack.md) 建立站点仓库与部署流水线，先把一个空首页部署到正式域名。
3. 按 [技术方案](website/stack.md#文档镜像) 把主仓库 `docs/` 镜像到官网 `/docs/`，这是第一阶段的内容主体，也是 SEO 权重的主要来源。
4. 按 [页面清单](website/pages.md) 与 [视觉规范](website/design.md) 逐页实现，并对照 [交互](website/ux.md)、[SEO](website/seo.md) 的规则自检。
5. 用 [验收清单](website/checklist.md) 做上线前检查；上线后每次发布版本按“维护”一节更新站点。

## 本指南依据的仓库事实

以下事实在 2026-11-03 按仓库内容与 GitHub Release 核对；官网上出现的数字应由构建脚本自动取得（见 [技术方案](website/stack.md#构建期数据)），不要手写。

- 最新发布：`v0.10.1`（2026-11-02），资产包括 Linux / macOS 的 amd64、arm64 二进制、Windows 托盘与控制台客户端、`install.sh` 与 `SHA256SUMS`。Windows 安装程序与 deb / rpm 打包配置在该 Release 后才加入，自包含这些资产的后续版本起提供；下载页不得为当前 Release 显示空分组。
- Linux amd64 二进制约 20.4 MB；根 README 写的“约 15 MB”已过时，官网用 Release 资产的实际大小。
- 预置 [十一款 CLI agent](cli-agents.md) 加 Shell 与 omp；[混合会话](hybrid-sessions.md) 支持 OpenCode、omp、Codex CLI、Claude Code、Antigravity CLI、Qoder、QoderCN 与 Kimi。
- 文案与截图的单一来源：[README](../README.md)、[项目介绍](overview.md)、[常见问题](faq.md)、`docs/images/`。
- 视觉方向已于 2026-11-03 选定为“叙事长卷”，规格见 [视觉与品牌规范](website/design.md#选定方向叙事长卷)，原型见 [官网原型](website/prototypes/README.md)。
- 品牌资产：`docs/images/brand/webmux-logo-{light,dark}.svg`、`web/public/icon.svg`；设计令牌：`web/src/design-tokens.css`；字体：`web/public/fonts/`（OFL 许可，见 `licenses/`）。
- 安全边界必须原样呈现：WebMux 等同于把 shell 暴露到网络，不是沙箱；服务重启结束终端会话；公网只经隧道且 Token 是唯一屏障（[安全策略](../SECURITY.md)）。
