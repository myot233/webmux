## 1. 清洗配置分离

- [x] 1.1 `CodeMarkdownPreview.svelte`：为 `document` 与 `message` 变体建立独立 DOMPurify 实例与钩子；`message` 保持现有禁用标签、图片转文字与链接规则
- [x] 1.2 `document` 变体：相对 `href` 改写为 `data-md-href`，相对图片（Markdown 语法与 `<img>`）改写为带 `data-md-src`、`alt`、`width`、`height` 的占位元素；外部图片保持文字；删除作者 `id` / `name`；非 HTTP(S)/`mailto:` 协议去除链接能力
- [x] 1.3 浏览器测试：脚本、iframe、外部图片、`javascript:` 链接、作者 `id` 均不生效；DOM 中不存在原值相对 `src` / `href`；聊天消息中的图片、相对链接与标题保持惰性

## 2. 路径解析与相对链接

- [x] 2.1 新增路径工具：以文档根相对路径为基准规范化相对地址（`.`、`..`、重复斜杠、百分号解码、忽略查询串与片段），`/` 开头视为根相对，越界直接返回“不在工作区内”
- [x] 2.2 `CodePanel.svelte` 向 `CodeDocumentView` / `CodeMarkdownPreview` 提供“按根相对路径打开”回调，内部复用 `locate`：文件打开或聚焦标签，目录在导航中定位；失败原因（越界、不存在、符号链接、不可读）回传给预览就地提示
- [x] 2.3 预览容器用事件委托处理 `data-md-href` 点击（含修饰键点击），阻止默认导航
- [x] 2.4 Node 单元测试覆盖路径规范化边界：`../` 越界、编码的 `%2e%2e`、`/` 开头、目录尾斜杠、空路径与纯片段

## 3. 标题锚点与目录

- [x] 3.1 实现 GitHub 风格 slug 与重复后缀，标题写入 `md-h-<slug>` 形式的 `id`
- [x] 3.2 `#slug` 点击在预览容器内滚动定位，不修改地址栏；目标缺失时提示
- [x] 3.3 跨文件 `path#slug`：打开目标文件后切到预览并定位标题（面板向文档视图传递一次性意图）
- [x] 3.4 `CodeDocumentView.svelte`：两个及以上标题时提供“目录”入口，桌面用现有菜单/弹层组件，窄屏并入“代码菜单”
- [x] 3.5 Node 单元测试覆盖 slug（中文、标点、emoji、重复标题）；浏览器测试覆盖文档内跳转与目录点击

## 4. 相对图片

- [x] 4.1 占位元素进入可视区附近时入队；队列最多 2 并发、单文档最多 50 张；依次调用 `locate`、`file`，`kind=image` 时用快照 `raw` 读取 Blob 并写入对象 URL
- [x] 4.2 失败占位文案：不存在、不在工作区内、不是可预览的图片、超过 20 MiB 预览上限、传输繁忙（可重试）、内容已变化（请刷新）、图片过多未加载
- [x] 4.3 点击已加载图片在代码面板中打开该图片文件
- [x] 4.4 切换文件、离开预览或卸载时中止所有请求并回收对象 URL
- [x] 4.5 浏览器测试：成功显示、各失败占位、并发上限、离开时中止与回收

## 5. 代码块高亮

- [x] 5.1 用 `LanguageDescription.matchLanguageName` 匹配围栏语言并按需加载，`highlightCode` + `codeHighlightStyle` 生成文本节点与 `span`；渲染代次过期时丢弃结果
- [x] 5.2 浏览器测试：常见语言出现高亮类名、未知语言为纯文本、代码中的 HTML 不被解析、暗色主题配色切换

## 6. 端到端与收尾

- [x] 6.1 Playwright：在隔离工作区中打开含相对链接、锚点和图片的 Markdown，验证跨文件打开、标题定位与图片加载，且无对应用路由或外部地址的图片请求
- [x] 6.2 更新 `docs/user-stories/code-and-worktrees.md` 中 US-I16 的验收标准与源码引用，并同步 `docs/user-stories.md` 与 `docs/user-stories/coverage.md`
- [x] 6.3 运行 `pnpm --dir web check:arch`、`make check` 与 `make test-fast`
