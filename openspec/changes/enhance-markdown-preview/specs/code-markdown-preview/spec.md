## ADDED Requirements

### Requirement: Markdown 预览入口与模式

打开扩展名为 `.md`、`.markdown`、`.mdown` 或 `.mkd`（大小写不敏感）的完整文本文件时，代码工作区 SHALL 提供“预览 Markdown”入口，并可“返回源码”；窄屏时入口位于“代码菜单”。

- 文件默认显示源码。
- 切换文件、打开 Diff 或刷新为新内容后 SHALL 回到源码模式，预览状态 SHALL NOT 写入设置。
- Diff 视图 SHALL NOT 提供渲染预览。

#### Scenario: 切换到预览并返回

- **WHEN** 用户打开 `README.md`，点击“预览 Markdown”，再点击“返回源码”
- **THEN** 先显示渲染后的标题、列表、表格、代码块和链接，随后回到带行号的源码视图

#### Scenario: 切换文件后回到源码

- **WHEN** 用户在 `a.md` 的预览中打开另一个标签页 `b.md`，再切回 `a.md`
- **THEN** `b.md` 以源码显示，且预览状态不出现在设置中

### Requirement: 预览内容的安全边界

预览 SHALL 用 DOMPurify 清洗渲染结果，SHALL NOT 执行脚本、加载外部资源或嵌入框架、表单、音视频与 `<style>`。

- 原始 HTML 中的 `id`、`name` 属性 SHALL 被移除，仅保留系统生成的标题锚点。
- 外部图片 SHALL 显示为“图片：说明（地址）”文字，SHALL NOT 发起网络请求。
- HTTP(S) 与 `mailto:` 链接 SHALL 在新标签页打开并带 `noopener noreferrer`。
- `javascript:`、`data:`、`file:` 及其他协议的链接 SHALL 失去链接能力，只保留文字。
- 任何未解析的相对地址 SHALL NOT 以原值进入 DOM 的 `src` 或 `href`，以免浏览器按应用地址发起请求。

#### Scenario: 脚本与外部资源不生效

- **WHEN** 文档包含 `<script>`、`<iframe>`、`<img src="https://example.com/a.png">`、`![x](https://example.com/b.png)` 和 `<a href="javascript:alert(1)">`
- **THEN** 预览不执行脚本、不发起对 `example.com` 的请求，外部图片显示为文字，`javascript:` 链接只剩文字

#### Scenario: 作者自带 id 不覆盖页面元素

- **WHEN** 文档原始 HTML 中含 `<div id="app">` 与 `<a name="top">`
- **THEN** 渲染结果中不存在这两个属性，应用其他元素不受影响

### Requirement: 工作区内相对链接

预览中的相对链接 SHALL 在当前代码面板内打开目标，不离开当前文档。

- 解析：普通相对路径相对当前文档所在目录解析，`/` 开头的路径相对代码工作区根解析，查询串被忽略。
- 目标为文件时 SHALL 以新标签页（或聚焦已打开的标签页）打开；目标为目录时 SHALL 在文件导航中定位该目录。
- 解析结果超出工作区根、不存在、为符号链接或不可读时，SHALL 在预览中提示原因，不离开当前文档。
- 修饰键点击 SHALL 与普通点击行为相同，SHALL NOT 打开指向应用路由的新浏览器标签页。

#### Scenario: 打开同级与上级文件

- **WHEN** `docs/guide/intro.md` 的预览中点击 `[配置](config.md)` 与 `[根说明](../../README.md)`
- **THEN** 分别打开 `docs/guide/config.md` 与根目录 `README.md` 的标签页

#### Scenario: 打开目录

- **WHEN** 预览中点击 `[示例](./examples/)`
- **THEN** 文件导航定位到 `examples` 目录，当前文档仍保持打开

#### Scenario: 超出根或不存在

- **WHEN** 点击 `[外部](../../../../etc/passwd)` 或指向不存在文件的链接
- **THEN** 预览显示“链接目标不在工作区内”或“链接目标不存在”的提示，不打开任何文件，不发起超出根的读取

### Requirement: 标题锚点与目录

预览 SHALL 为每个标题按 GitHub 风格生成 slug，并以固定前缀写入元素 `id`，避免与应用元素冲突。

- slug 规则：小写、去除标点、空格转连字符，重复时追加 `-1`、`-2`。
- `#slug` 链接 SHALL 在预览区内滚动到对应标题，SHALL NOT 修改应用地址栏；目标不存在时 SHALL 不滚动并给出提示。
- 指向 Markdown 文件的 `路径#slug` 链接 SHALL 打开该文件、切到预览并定位标题。
- 文档含两个及以上标题时，文档栏 SHALL 提供“目录”入口，按层级列出标题，点击后滚动定位；窄屏时入口位于“代码菜单”。

#### Scenario: 文档内跳转

- **WHEN** 文档含两个 `## 安装` 标题，用户点击 `[第二个安装](#安装-1)`
- **THEN** 预览滚动到第二个“安装”标题，地址栏不变

#### Scenario: 跨文件定位标题

- **WHEN** 在 `README.md` 预览中点击 `[部署](docs/deploy.md#systemd)`
- **THEN** 打开 `docs/deploy.md`，进入预览并滚动到 `systemd` 标题

#### Scenario: 使用目录

- **WHEN** 用户在窄屏打开含多个标题的文档预览，从“代码菜单”打开“目录”并选择第三个标题
- **THEN** 目录关闭，预览滚动到该标题

### Requirement: 工作区内相对图片

Markdown 图片语法与原始 HTML `<img>` 中的相对 `src` SHALL 按相对链接相同规则解析到工作区文件，并在进入可视区附近时加载。

- 读取：被识别为图片（含 SVG）且不超过 20 MiB 的文件 SHALL 通过快照原始字节端点以认证请求读取，并以对象 URL 显示。
- SVG SHALL 只作为 `<img>` 显示，不执行其中脚本。
- 数量：同一文档同时加载的图片 SHALL 不超过 2 张，单篇文档加载的图片 SHALL 不超过 50 张，超出部分显示“图片过多，未加载”。
- 失败：超出根、不存在、非图片、过大、传输繁忙或内容已变化时 SHALL 在原位置显示带原因的文字占位，其余内容照常显示。
- 点击已加载的图片 SHALL 在代码面板中打开该图片文件。
- 切换文件或离开预览时 SHALL 取消未完成的读取并释放对象 URL。

#### Scenario: 显示仓库截图

- **WHEN** `README.md` 含 `![首页](docs/img/home.png)` 与 `<img src="docs/img/logo.svg" width="80">`，两者均为有效图片
- **THEN** 两张图片在原位置显示，`logo.svg` 宽度为 80，网络中不出现对应用路由或外部地址的图片请求

#### Scenario: 图片缺失或无效

- **WHEN** 文档引用不存在的 `a.png`、实际为文本的 `b.png` 和 30 MiB 的 `c.png`
- **THEN** 三处分别显示“图片不存在”“不是可预览的图片”“超过 20 MiB 预览上限”，文档其余部分正常渲染

#### Scenario: 离开时释放资源

- **WHEN** 含多张图片的文档仍在加载时，用户切换到另一个标签页
- **THEN** 未完成的读取被取消，已创建的对象 URL 被释放

### Requirement: 代码块语法高亮

带语言标注的围栏代码块 SHALL 按 CodeMirror 语言数据匹配语言名或别名并高亮，配色 SHALL 与源码阅读器一致并跟随明暗主题；语法按需加载。无标注或无法识别的语言 SHALL 显示为纯文本。高亮 SHALL 只生成样式包裹的文本节点，代码内容 SHALL NOT 作为 HTML 解析。

#### Scenario: 常见语言高亮

- **WHEN** 文档含 ` ```ts `、` ```go ` 与 ` ```bash ` 代码块
- **THEN** 三个代码块按各自语言显示关键字与字符串配色，切换暗色主题后配色随之变化

#### Scenario: 代码中的 HTML 不被解析

- **WHEN** ` ```html ` 代码块中含 `<img src=x onerror=alert(1)>`
- **THEN** 该内容按文本高亮显示，不创建图片元素，不执行脚本
