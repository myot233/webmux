## Context

`CodeMarkdownPreview.svelte` 有两个变体，共用一个 DOMPurify 实例与 `afterSanitizeAttributes` 钩子。

- 变体：`document` 在代码面板中预览 Markdown 文件，`message` 渲染聊天中的 agent 消息。
- 共用钩子的行为：图片被渲染器替换为文字，非 HTTP(S)/`mailto:` 的 `href` 被删除，`img`、`svg`、`iframe`、`style` 等标签被禁止。
- `chat-view` 规格要求聊天消息“沿用代码预览的清洗规则”。

代码面板已具备本变更需要的后端能力：

- `GET /code/locate?path=` 接受相对 shell 目录的路径或根内绝对路径，返回 `nodeId`、根相对 `path` 与 `kind`（文件或目录），根边界、符号链接与特殊文件由服务端拒绝；`CodePanel.locate()` 已据此打开文件标签或定位目录。
- `GET /code/file?nodeId=` 返回 `Document` 与快照 ID；图片文档只做头部识别，`kind=image`，不读入正文。
- `GET /code/snapshots/{snapshotId}/raw` 以认证请求流式返回原始字节，最多 4 个并发传输，忙时 429；`CodeImageView` 已用 `api` 读取 Blob 并生成对象 URL。
- 快照缓存上限 64 MiB，但计入的是 `documentSize`（文本、路径等元数据），图片字节不进入缓存，因此按需为多张图片建立快照不会挤出正在阅读的文档。

## Goals / Non-Goals

**Goals:**

- 预览中的相对链接、锚点、相对图片在代码面板内可用，且不放宽脚本与外部资源边界。
- 代码块高亮与源码阅读器一致。
- 聊天消息的渲染行为保持不变。

**Non-Goals:**

- 新增后端接口或批量读取接口。
- Mermaid、数学公式、Front matter 表格化、源码与预览滚动同步、预览中选区引用回终端。
- Markdown Diff 的渲染视图；外部图片代理或缓存。
- 持久化预览模式。

## Decisions

### D1：前端解析路径，复用 `locate` / `file` / `raw`

前端以文档的根相对路径为基准，用 POSIX 规则规范化相对地址（处理 `.`、`..`、重复斜杠与百分号解码），得到根相对路径；`/` 开头的地址直接视为根相对。规范化后仍以 `..` 开头的路径在前端直接判为“不在工作区内”，否则拼接上下文根的绝对路径交给 `locate`，由服务端做最终边界判断。

`locate` 会沿符号链接解析到真实路径，因此它返回的根相对路径与前端规范化结果不一致时，说明途经符号链接，按“符号链接”拒绝，不打开也不加载。

备选：新增“按文档解析相对路径”的后端接口。`locate` 已提供根内绝对路径解析与全部边界检查，新接口只会复制这段逻辑，因此不采用。

### D2：按变体分离清洗配置与后处理

两个变体使用各自的 DOMPurify 实例与钩子。`document` 变体在清洗阶段：

- 把相对 `href` 改写为 `data-md-href`，把 Markdown 图片与 `<img>` 的相对 `src` 改写为占位元素上的 `data-md-src`（保留 `alt`、`width`、`height`），外部图片仍渲染为文字；
- 删除作者提供的 `id`、`name`；
- 仍禁止 `style`、`srcset` 等属性与现有禁用标签。

清洗后由组件对占位元素做增强，相对地址永远不会以原值出现在 `src` / `href` 上，浏览器不会按应用路由发起请求。

- 链接通过事件委托处理点击；图片只在获得对象 URL 后才写入 `src`。
- `message` 变体保持图片、链接与标题的惰性行为，但与 `document` 变体共用标签与属性清单：两者都移除作者 `id` / `name` 以及 `map` / `area`（`area` 会保留原值相对 `href`）。
- 新增浏览器测试锁定这些行为。

备选：在一个实例里按变体切换钩子行为。DOMPurify 钩子是实例级全局状态，切换容易在并发渲染时串用，因此不采用。

### D3：图片的懒加载、并发与资源释放

占位元素由 `IntersectionObserver`（预加载边距约一屏）触发加载。

1. `locate` 得到 `nodeId`。
2. `file` 得到文档与快照 ID。
3. `kind=image` 且为 worktree 来源时，再用 `raw` 读取 Blob。

限制与释放：

- 每篇文档维持一个最多 2 个并发的队列，为下载与图片视图留出传输槽；单篇文档最多加载 50 张。
- 所有请求共享一个 `AbortController`，切换文件、离开预览或卸载时中止并回收对象 URL。
- 429 显示“传输繁忙”并提供重试，409/410 显示“内容已变化，请刷新”。

### D4：标题 slug 与锚点

在本地实现 GitHub 风格 slug，不引入 `github-slugger` 依赖。

- 规则：Unicode 字母数字保留、去除标点、空白转 `-`、小写，同名追加 `-n`。
- 元素 `id` 为 `md-h-<slug>`，链接仍写 `#<slug>`；点击时在预览容器内查找并 `scrollIntoView`，不改 `location.hash`。
- 跨文件 `path#slug` 先走 D1 打开文件，再通过面板向 `CodeDocumentView` 传递一次性的“打开后预览并定位”意图。

### D5：代码块高亮

用 `@codemirror/language-data` 的 `LanguageDescription.matchLanguageName(languages, info, true)` 按围栏信息匹配语言，按需 `load()`，再用 `@lezer/highlight` 的 `highlightCode` 配合 `codeHighlightStyle` 生成文本节点与 `span`。高亮异步执行，带渲染代次，过期结果丢弃；未匹配时保持原始文本。

### D6：目录与入口

目录数据来自 D4 生成的标题列表。桌面在文档栏使用现有 `DropdownMenu`/`Popover` 组件呈现，窄屏并入现有“代码菜单”；两个以下标题时不显示入口。不新增自定义弹层。

## Risks / Trade-offs

- [多图文档触发较多请求：每张图 3 次请求] → 懒加载、2 并发、50 张上限；请求均为本机或 VPN 内的小 JSON。
- [路径规范化与服务端不一致] → 前端只做规范化与快速拒绝，最终以 `locate` 的结果为准；测试覆盖百分号编码、`..` 越界与 `/` 开头路径。
- [清洗钩子改动影响聊天消息] → 变体使用独立实例；聊天消息新增回归场景。
- [大量代码块高亮占用主线程] → 异步按块处理，过期代次丢弃；文本文件已受 2 MiB 上限约束。
- [SVG 图片含脚本] → 仅以 `<img>` + 对象 URL 显示，浏览器不执行 SVG 中的脚本，也不加载其外部资源。

## Migration Plan

纯前端行为增强，无数据迁移。回滚时恢复 `CodeMarkdownPreview.svelte` 及相关组件即可，后端无变化。

## Open Questions

- Mermaid 渲染价值较高但体积约 1 MB 且需额外安全设计，留待后续独立变更评估。
