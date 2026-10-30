## Why

agent 产出的 README、设计文档与 OpenSpec 产物都是 Markdown，用户在代码工作区里核对这些文档时依赖预览（US-I16）。现有预览把图片降级为“图片：xxx”文字、删除相对链接的 `href`、代码块没有语法高亮，也没有标题锚点和目录，长文档在手机上几乎只能退回源码阅读。

## What Changes

- 文档中的相对链接在代码面板内打开目标文件或目录：路径相对当前文档目录解析，`/` 开头的路径相对工作区根解析；超出根、不存在或不可读时就地提示原因，不离开当前视图。
- `#标题` 链接在预览区内滚动到对应标题；指向其他 Markdown 文件的 `文件#标题` 打开该文件的预览并定位标题。
- 标题生成稳定锚点；文档含两个以上标题时提供“目录”入口，窄屏收入“代码菜单”。
- 工作区内的相对图片（Markdown 图片语法与原始 HTML `<img>`）按需加载并显示，复用现有图片识别、20 MiB 预览上限与快照原始字节端点；外部图片仍只显示文字，不发起任何外部请求。
- 带语言标注的代码块按 CodeMirror 语言数据高亮，颜色跟随主题；未知语言保持纯文本。
- 聊天消息的 Markdown 渲染保持现有边界：不加载任何图片、不解析工作区链接、不生成锚点。
- 不新增后端路由，不引入 Mermaid、数学公式、源码/预览滚动同步或渲染版 Markdown Diff。

## Capabilities

### New Capabilities

- `code-markdown-preview`：代码工作区中 Markdown 文件的渲染预览，包括入口与模式、清洗边界、工作区内链接与锚点、目录、相对图片和代码块高亮。

### Modified Capabilities

- `chat-view`：时间线中 agent 消息的 Markdown 渲染不再表述为“沿用代码预览的清洗规则”，而是明确保持原有资源边界，避免文件预览放宽的能力（图片、锚点、工作区链接）扩散到聊天消息。

## Impact

- 前端：`web/src/components/CodeMarkdownPreview.svelte`（按变体分离清洗配置与后处理）、`CodeDocumentView.svelte`（目录入口、链接与图片回调）、`CodePanel.svelte`（复用 `locate` / `openFile` 打开相对路径）、`web/src/code-language.ts` 与 `web/src/code-highlight.ts`（代码块高亮复用）。
- 后端：不新增接口；复用 `GET /code/locate`、`GET /code/file` 与 `GET /code/snapshots/{snapshotId}/raw`，受现有根边界、符号链接拒绝、4 个传输槽与 20 MiB 图片上限约束。
- 依赖：不新增 npm 依赖；标题 slug 在本地实现。
- 用户故事：修改 US-I16，补充链接、图片、目录与高亮的验收标准。
- 测试：Vitest 浏览器测试覆盖渲染、清洗与交互；E2E 覆盖相对链接与图片在真实后端下的加载。
