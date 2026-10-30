## Why

agent 经常产出 HTML 报告、CSV 数据、JSON 结果和 PDF 文档。代码工作区目前只能以源码查看前三者，PDF 只显示“二进制内容”和元信息，用户必须下载到本地或借助终端命令才能确认结果，在手机上尤其不便。

## What Changes

- HTML（`.html`、`.htm`）文本文件提供“预览”模式：先在惰性 DOM 中去除脚本、`<base>`、`<meta http-equiv="refresh">`、表单动作与链接地址，再放入 `sandbox=""` 的 `srcdoc` 框架并注入 CSP，只显示静态排版，标注“静态预览，脚本与外部资源已禁用”。
- CSV / TSV（`.csv`、`.tsv`）文本文件提供表格预览：按 RFC 4180 解析引号字段，首行为表头，最多渲染 2000 行并提示截断，解析失败时说明原因并保留源码。
- JSON（`.json`）文本文件提供可折叠树预览：默认展开两层，大集合分批展开，数字按源码原文显示以免精度丢失，非法 JSON 给出出错位置并保留源码。
- PDF：服务端按 `.pdf` 扩展名与 `%PDF-` 文件头识别为新的文档种类 `pdf`，20 MiB 内在代码面板中用懒加载的 pdf.js 渲染，支持翻页、适应宽度、缩放、双指缩放、文本选择与文档内链接；超过上限、受密码保护或损坏时显示原因并保留下载入口。PDF 原始字节响应增加 CSP `sandbox`。
- 文本类预览默认显示源码，切换文件后回到源码；PDF 打开即预览。所有预览不执行文档脚本、不加载外部资源。
- 不预览 Diff 中的上述格式，不支持交互式 HTML、PDF 表单与 PDF 内脚本。

## Capabilities

### New Capabilities

- `code-document-preview`：代码工作区中 HTML、CSV/TSV、JSON 与 PDF 文件的预览入口、渲染边界、上限与失败说明。

### Modified Capabilities

- `code-image-preview`：“按扩展名与文件签名识别图片”中 PDF 不再归为“二进制内容”，而由 `code-document-preview` 识别为 PDF 文档；仍不被识别为图片。

## Impact

- 后端：`internal/codeview/media.go`（PDF 候选与签名识别、`MaxPDFBytes`）、`service.go` 与 `diff.go`（读取候选文件头）、`internal/api/code.go`（PDF 原始字节 CSP），`internal/contract` / `internal/wire` 与 `web/src/schemas.ts` 的文档种类增加 `pdf`；重新录制 `internal/api/testdata/wire`。
- 前端：`CodeDocumentView.svelte` 增加格式预览切换；新增 HTML、表格、JSON 树与 PDF 预览组件；`CodeDownloadButton.svelte` 复用于 PDF 工具栏。
- 依赖：新增 `pdfjs-dist`（Apache-2.0），worker、CMap 与标准字体随前端打包、按需加载，不进入首屏包，不访问公共 CDN。
- 与 `add-file-download` 的关系：复用现有快照原始字节端点与 100 MiB 下载上限，不依赖临时目录根；两者可按任意顺序合入。
- 用户故事：新增“预览 HTML、表格、JSON 与 PDF”故事（US-I17），同步修改 US-I11 中二进制内容的边界说明。
