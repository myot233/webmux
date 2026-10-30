## 1. 后端 PDF 识别与契约

- [x] 1.1 `internal/codeview/media.go`：增加 `MaxPDFBytes`（20 MiB）、`.pdf` 候选与前 1024 字节 `%PDF-` 签名检查，生成 `pdf` / `too-large`（保留 `application/pdf`）/ `binary`（空文件、签名缺失）文档
- [x] 1.2 `service.go` 与 `diff.go`：PDF 候选复用图片的头部读取路径，不读入正文；Diff 保持 `metadata`
- [x] 1.3 `internal/contract`、`internal/wire`、`web/src/schemas.ts`：文档种类增加 `pdf`；更新 `docs/protocol.md` 中的种类说明
- [x] 1.4 `internal/api/code.go`：`application/pdf` 原始字节与 SVG 共用 CSP `sandbox` 头
- [x] 1.5 Go 测试：有效 PDF、空文件、签名缺失、`.bin` 中的 PDF、超过 20 MiB、Git 对象来源、PDF raw 响应头；`go test ./internal/api -run TestWireSnapshots -update` 重新录制并审阅差异

## 2. 格式预览框架

- [x] 2.1 `CodeDocumentView.svelte`：把 Markdown / SVG 的视图切换泛化为“格式预览”状态，按扩展名为 HTML、CSV/TSV、JSON 提供“预览 / 返回源码”入口（窄屏并入“代码菜单”），切换文件、Diff 或刷新时复位；`kind=pdf` 直接进入 PDF 视图
- [x] 2.2 浏览器测试：入口仅对受支持扩展名的完整文本出现，超限或二进制不出现，复位行为与 Markdown 一致

## 3. HTML 静态预览

- [x] 3.1 新增 HTML 预处理函数：`DOMParser` 惰性解析，删除危险元素、`on*` 属性、链接地址、表单动作与 `javascript:` URL，在 `head` 首位注入 CSP meta 并序列化
- [x] 3.2 新增 HTML 预览组件：`<iframe sandbox srcdoc>`（无 `allow-*`），白色背景，顶部提示“静态预览，脚本与外部资源已禁用”
- [x] 3.3 浏览器单元测试覆盖预处理的删除清单（`DOMParser` 在 Node 中不可用）；浏览器测试确认脚本不执行、表单不可提交、框架不跳转

## 4. CSV / TSV 表格预览

- [x] 4.1 实现 RFC 4180 状态机解析器（分隔符按扩展名），返回表头、前 2000 条数据行、总行数或带行号的错误
- [x] 4.2 从 shadcn-svelte registry 引入 `table` 组件；表格预览支持表头吸顶、横向滚动、截断提示与解析失败说明
- [x] 4.3 Node 单元测试：引号、转义引号、字段内换行、CRLF、列数不齐、未闭合引号、空文件、BOM

## 5. JSON 树预览

- [x] 5.1 实现保留原始数字文本与位置的递归下降解析器，错误报告行列
- [x] 5.2 JSON 树组件：默认展开两层、成员计数、超过 200 成员分批展开，基于现有 `collapsible`
- [x] 5.3 Node 单元测试：大整数与小数原文、Unicode 转义、深层嵌套、各类语法错误的行列；浏览器测试覆盖分批展开

## 6. PDF 预览

- [x] 6.1 `pnpm --dir web add pdfjs-dist`，确认无需安装脚本、锁文件冻结流程不变；worker、CMap、标准字体与 wasm 以构建资源打包，经自定义读取工厂映射哈希文件名
- [x] 6.2 新增 PDF 视图组件：动态导入渲染器，认证读取 `raw` 为 `ArrayBuffer`，`enableXfa: false`（pdf.js 6 已移除 `isEvalSupported`，见 design D5），读取与加载任务共用中止信号
- [x] 6.3 页面占位与可视区渲染、远离页面释放画布、渲染比例上限 2；文本层；自建链接层（内链跳页、HTTP(S)/`mailto:` 新标签页）
- [x] 6.4 单行工具栏：页码与跳页、适应宽度、放大、缩小、下载（复用 `CodeDownloadButton`）；触摸双指缩放；窄屏布局与图片视图一致
- [x] 6.5 失败状态：受密码保护、无法解析、渲染失败、超过预览上限，均保留下载入口
- [x] 6.6 浏览器测试：多页渲染与画布回收、跳页、链接行为、密码与损坏文件提示、卸载时中止
- [x] 6.7 确认首屏构建产物不含 pdf.js；`knip` 与 dependency-cruiser 通过

## 7. 端到端与收尾

- [x] 7.1 Playwright（桌面与 Pixel 7）：隔离工作区中打开 HTML、CSV、JSON 与含中文的多页 PDF；记录网络请求，断言 HTML 预览无额外请求、未打开 PDF 时不加载渲染器
- [x] 7.2 在 `docs/user-stories/code-and-worktrees.md` 新增 US-I17（预览 HTML、表格、JSON 与 PDF），调整 US-I11 的二进制边界说明，同步 `docs/user-stories.md` 与 `docs/user-stories/coverage.md`
- [x] 7.3 运行 `pnpm --dir web check:arch`、`make check` 与 `make test-fast`
