## Context

代码工作区按 `Document.kind` 分派视图：`text` 进 CodeMirror 阅读器，`image` 进 `CodeImageView`，`binary`、`unsupported`、`too-large`、`symlink` 显示元信息。

媒体类型识别集中在 `internal/codeview/media.go`：图片候选按扩展名筛选后只读取 4 KiB 头部做签名识别，不读入正文；`service.go` 与 `diff.go` 分别为工作树和 Git 对象调用这条路径。

所有不超过 100 MiB 的常规文件都已 `downloadable`，可通过 `GET /code/snapshots/{snapshotId}/raw` 以认证请求读取；`api/code.go` 只在 `MediaType` 非空时以 `inline` 返回，SVG 额外带 CSP `sandbox`。`CodeImageView` 已演示“认证读取 Blob → 对象 URL → 中止与回收”的模式。

Markdown 与 SVG 已有“源码 / 预览”切换的先例：默认源码（SVG 例外），切换文件后复位，状态不持久化。

## Goals / Non-Goals

**Goals:**

- HTML、CSV/TSV、JSON 在不改后端的前提下提供只读预览。
- PDF 在桌面与手机上都能在面板内阅读，包括中文文档。
- 所有预览不执行文档脚本，不发起外部或同源的额外请求，不能接触应用令牌。

**Non-Goals:**

- 交互式 HTML（允许脚本）、HTML 相对资源加载、独立预览来源。
- PDF 表单填写、PDF 内脚本、批注编辑、打印、全文搜索、Diff 对比。
- `.jsonl`、`.ipynb`、Excel 等其他格式；CSV 排序、筛选与类型推断。
- 改变文本 2 MiB / 20000 行或下载 100 MiB 上限。

## Decisions

### D1：文本类格式只改前端

HTML、CSV/TSV、JSON 已作为 `kind=text` 全文下发，预览仅是前端的另一种视图。`CodeDocumentView` 把 Markdown 与 SVG 的切换泛化为“格式预览”状态，按扩展名选择预览组件。备选是由后端解析并下发结构化结果，但这会扩大契约且没有收益，因此不采用。

### D2：HTML 三层隔离

1. 惰性预处理：用 `DOMParser` 解析（不执行脚本、不加载资源），再删除以下内容：
   - 元素：`script`、`base`、`link`、`template`、`meta[http-equiv=refresh]`、`object`、`embed`、`iframe`、`frame`、`portal`。
   - SVG 动画元素：`animate`、`animateMotion`、`animateTransform`、`set`。
   - 属性：`on*` 属性、`a`/`area` 的 `href`、`form` 的 `action` / `formaction`、`javascript:` URL、全部 `srcset`。
   - 经 URL 解析后协议不是 `data:` 的 `src`、`poster`、`background`、`href`。
   - `srcset` 是候选列表，单次 URL 判断无法覆盖每个候选，因此整体删除，元素回退到 `src`；以 URL 解析而非前缀匹配判断协议，前导不间断空格等字符会被视为相对地址而删除。
   - CSP 虽会拦截外部引用，但浏览器仍会尝试发起请求，因此在预处理阶段直接移除。
   - `template` 的内容不在遍历范围内，而 `shadowrootmode` 会让它在框架中成为未经处理的影子树；SVG 动画可在不运行脚本的情况下把 `href` 写回链接。两者整体删除。
2. CSP：在 `head` 首位插入 `<meta http-equiv="Content-Security-Policy" content="default-src 'none'; style-src 'unsafe-inline'; img-src data:; font-src data:">`。
3. 框架：`<iframe sandbox srcdoc=...>`，不加任何 `allow-*`，因此为不透明来源，无法运行脚本、提交表单、导航顶层或读取应用存储。

预处理负责导航类风险（CSP 不约束框架自身导航与 meta 刷新），CSP 负责资源加载，sandbox 兜底脚本与来源隔离。框架背景固定为白色，与普通网页预期一致。备选“允许脚本 + 独立来源”需要新的服务来源与资源读取接口，已在讨论中否决。

### D3：CSV 与 JSON 使用本地小型解析器

CSV 与 JSON 都使用本地小型解析器，不新增运行时依赖。

- CSV 解析器按 RFC 4180 状态机实现，记录行列号以报告未闭合引号。它最多构建 2000 条数据行的表格，但继续扫描以给出总行数。
- JSON 使用本地递归下降解析器，生成带原始数字文本、成员数与位置信息的节点树，从而保留大整数精度并报告行列。
- 备选 `papaparse` / `JSON.parse` 不采用：前者为单一用途引入依赖，后者丢失数字精度且错误位置在各浏览器不一致。
- 树的折叠复用现有 `collapsible` 组件。
- `web/src/lib/components/ui/` 目前没有表格组件，按设计系统约定从 shadcn-svelte registry 引入 `table`，不自造样式化替代品。

### D4：PDF 识别与契约

PDF 作为新增文档种类 `pdf` 识别，复用图片的头部读取路径，不读入正文。

- 识别：`media.go` 增加 PDF 候选（扩展名 `.pdf`）与签名检查（前 1024 字节内 `%PDF-`）。
- 分类：
  - 超过 `MaxPDFBytes`（20 MiB）时为 `too-large` 并保留 `application/pdf`。
  - 签名缺失或空文件为 `binary` 并给出原因。
  - Diff 仍为 `metadata`。
- 契约：`contract`、`wire`、`schemas.ts` 同步增加 `pdf`，重新录制 `internal/api/testdata/wire`。
- 响应头：`api/code.go` 对 `application/pdf` 与 SVG 共用 CSP `sandbox` 头。

备选：保持 `kind=binary`、仅靠 `mediaType` 区分。前端分派与“超过预览上限”的语义都基于 `kind`，沿用图片的做法更一致，因此新增种类。

### D5：pdf.js 的加载与渲染

- 依赖 `pdfjs-dist`（6.x），使用其 `legacy` 构建：
  - 现代构建依赖 `Map.prototype.getOrInsertComputed` 等很新的内置方法。
  - legacy 构建仍要求约 Chrome 125 / Safari 18；低于此的浏览器在 `getDocument()` 同步抛错，视图显示“当前浏览器无法预览 PDF，请下载后查看”并保留下载。
  - 仅在 PDF 视图中 `import()`，形成独立分包。
  - worker 通过 Vite `?url` 引入；CMap、标准字体与 wasm 解码器以 `?url&no-inline` 构建资源随 `web/dist` 嵌入。
- 代码面板保留非活动标签的视图：未完成的读取与解析在标签离开时取消、返回时重新开始；已加载的文档保持不变。
- pdf.js 6 把三类数据合并为单一 `BinaryDataFactory`：设 `useWorkerFetch: false`，由自定义工厂把固定文件名映射为哈希 URL，不访问公共 CDN。
- `getDocument({ data, enableXfa: false, ... })`：pdf.js 6 已移除 `isEvalSupported`，不再用 `eval` 编译字体，因此无需该选项。字节通过现有 `api` 认证读取为 `ArrayBuffer`，读取与 `PDFDocumentLoadingTask` 共用中止信号。
- 每页一个占位容器，按页面尺寸预留高度；`IntersectionObserver` 只渲染可视区前后各 1 页，离开 3 页以外的页面释放画布。渲染比例上限为 `min(devicePixelRatio, 2)`。
- 文本层使用 pdf.js `TextLayer`；链接层自行根据 `getAnnotations()` 中的 `Link` 生成覆盖元素：内部目标跳页，HTTP(S)/`mailto:` 新标签页打开，其他忽略。不引入 `pdf_viewer` 的表单层。
- 工具栏沿用 `CodeImageView` 的单行布局与触摸手势约定，下载复用 `CodeDownloadButton`。

### D6：依赖与架构护栏

确认 `pdfjs-dist` 不需要安装脚本（保持 `esbuild` 唯一允许列表）；可选的 Node 画布依赖不进入浏览器构建。knip 与 dependency-cruiser 需识别动态导入；首屏包体积以构建产物对比确认不变。

## Risks / Trade-offs

- [HTML 预处理遗漏某种导航或请求方式] → CSP 与无 `allow-*` 的 sandbox 兜底；E2E 记录网络请求，断言预览期间无额外请求。
- [依赖 JS 的报告预览为空白] → 明示“静态预览”，保留下载入口；这是选择安全边界的已知代价。
- [pdf.js 体积与移动端内存] → 懒加载分包、可视区渲染与画布回收、20 MiB 上限、渲染比例上限。
- [pdf.js 依赖较新的浏览器特性] → 实施时核对所选版本的浏览器要求与 iOS Safari 支持，必要时固定到兼容版本。
- [CJK 文档缺字] → 打包 CMap 与标准字体，E2E 使用含中文的测试 PDF。
- [新增文档种类影响旧客户端] → 内嵌前端与后端同版本发布，不存在版本错配；`internal/` 中除 `codeview`、`api`、`wire`、`contract` 外没有代码文档种类的消费者。

## Migration Plan

无数据迁移。回滚时移除 `pdf` 种类与前端预览组件即可，PDF 回到“二进制内容”。

## Open Questions

- 是否为 HTML 框架提供“浅色/跟随主题”切换，留待使用反馈。
