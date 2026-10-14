# Tasks

## 1. Session-scoped Git Reads

- [ ] 1.1 在 `internal/workspace` 增加从会话启动 cwd 解析并绑定实际 Git 工作树/管理目录身份的只读入口，不要求注册清单可用；真实临时仓库测试覆盖 main、linked 子目录、未注册、无 Git、普通目录、注册存储故障和绑定后目录替换。
- [ ] 1.2 为 Diff 读取实现有界 stdout/stderr、10 秒请求期限、两个并发读取、等待取消及 Git 进程组回收，复用现有取消语义；受控大输出/超时/取消测试验证内存限制、子进程无残留及 PTY 输出不受阻。
- [ ] 1.3 实现 NUL 分隔修改列表和稳定的 group/fileId，区分索引/工作区、untracked/unmerged、双方重命名路径；真实 Git 测试覆盖同文件双组、unborn HEAD、删除、模式变化、特殊路径及不包含忽略文件，验证列表 1000 条/2 MiB 超限行为。
- [ ] 1.4 实现单文件统一 diff 与结构化双方行号，固定关闭颜色/external diff/textconv、使用 literal pathspec；真实 Git 测试覆盖 HEAD→索引、索引→工作区、重命名、纯元数据、无末尾换行、空文件、二进制和子模块摘要。
- [ ] 1.5 实现未跟踪文件空内容→新增的有界读取，逐段限制符号链接跟随，链接本身用 readlink，拒绝 device/FIFO；真实文件测试覆盖外部链接、目录替换竞争、非法 UTF-8 路径、1 MiB/10000 行限制，验证不会读取工作树外内容或伪造截断行。
- [ ] 1.6 建立 contextId/contentVersion 与当前变化条目重新验证，移除会话时释放绑定；并发测试验证文件消失、来源变化和工作树身份改变得到冲突/不可用且不会重绑另一目标。按上述合同补充后端读取模块的维护说明。

## 2. Read-only API and Frontend Contracts

- [ ] 2.1 在 `internal/api` 增加会话范围 changes/diff GET 路由，沿用构造注入、认证/Origin 与 JSON error；API 测试覆盖成功合同、404/400/409/413/504、未认证、非法 contextId/fileId 和越界路径，并验证 GET 不修改工作区/索引。
- [ ] 2.2 在 `web/src/schemas.ts` 定义 capability、文件、来源、hunk、行号、受限状态及响应的 Zod schemas，在 `api.ts` 扩展现有传输以支持取消；单元测试验证正常响应、无效行号/枚举、局部降级和统一 401 行为，`pnpm --dir web check` 通过。
- [ ] 2.3 新增 Diff 前端状态模块，按会话/context/file 和请求序号隔离，处理显示/前台/重连/手动刷新、取消和错误保留；状态测试验证 A/B 乱序、离开页面、重复触发合并、读取时间、变化后选择失效，以及隐藏/后台没有 Git 轮询。
- [ ] 2.4 更新 `WebMux.md` 的只读接口、三组比较来源、限制和降级说明；对照真实 API 测试响应逐项核验字段及错误合同，保持现有二进制终端协议描述。

## 3. Responsive Diff Review UI

- [ ] 3.1 使用 pnpm 与仓库既有 shadcn-svelte 流程生成 Tabs，新增只读 Diff 面板，展示三组文件、旧/新路径、双方行号、增删色彩及各局部状态；真实浏览器测试覆盖代码文本转义、无修改、错误/旧结果、超大/二进制/冲突、长路径和独立滚动。
- [ ] 3.2 在 `Terminal.svelte` 和 `styles.css` 实现 >=1024px 左终端/右可折叠 Diff、窄屏“终端 / Diff”标签，保持唯一宿主与 hook；浏览器测试覆盖 1440px、1024px、1023px、390px、折叠、旋转和全屏，验证页面没有被长代码撑宽。
- [ ] 3.3 完善 useTerminal 对隐藏与非零可见尺寸的处理，标签恢复布局后 fit，Diff 显示时隐藏终端软键/浮层且不抢焦点；真实 PTY 测试验证隐藏期间输出继续、返回后可输入/resize、会话 ID 不变且不发送零尺寸。
- [ ] 3.4 更新 `README.md` 与 `WebMux.md` 的终端布局、Git 可选依赖和刷新方式；用实际桌面与窄屏页面核对文案及截图，保留原上传、粘贴、全屏和重连入口。

## 4. Code Location Selection and Terminal Paste

- [ ] 4.1 实现文件/单行/同侧连续范围选择、桌面 Shift 扩选、触屏起点/终点、预览与清除；真实浏览器测试验证不同文件/比较侧/未展示间隔不会错误合并，以及刷新变化、会话导航和目标失效清除旧选择。
- [ ] 4.2 实现共享位置格式化函数，使用绑定根绝对路径、该侧行号和 HEAD/索引来源标签；单元测试覆盖新增/删除/重命名、子目录启动、空格/冒号/引号/换行/ESC 路径和非法 UTF-8，验证预览与实际插入完全一致。
- [ ] 4.3 为 useTerminal 增加可观测粘贴结果，复用 xterm 粘贴模式与现有 WebSocket 输入，发送前检查初始化、连接、restoring、退出状态；真实 PTY 测试验证原输入保留、bracketed paste、位置无自动回车、发送只到当前会话以及断开不自动补发。
- [ ] 4.4 接入“发送位置到终端”，成功仅反馈已插入；窄屏成功切回终端并 fit，失败保留 Diff 和选择，手机不强制打开键盘；浏览器/E2E 验证点击时断连、发送后断连、已退出与尚未初始化状态，不声称 agent 已确认接收。
- [ ] 4.5 更新产品文档的位置选择、版本标签和插入语义，说明用户自行补充要求/按回车、共享工作树差异和读取时间限制；以真实 Shell 捕获输入的隔离演示核对示例，不要求第三方 agent 登录。

## 5. Integration Acceptance

- [ ] 5.1 在现有隔离 E2E 夹具中添加真实 Git/PTY 审阅流程：两个工作树各自修改、一个文件同时 staged/unstaged、untracked、查看双方行号、发送引用、修改后刷新、乱序/局部失败；桌面与 Pixel 7 项目验证工作树不串、终端实例/输入/回放正确。
- [ ] 5.2 回归原终端上传/粘贴、软键、选择复制、全屏、断线恢复及工作树删除协调；使用真实临时服务运行相关测试，确认 Diff 读取或布局切换不改变现有安全和生命周期行为。
- [ ] 5.3 运行相关 Go race、Vitest 单元/浏览器与 Playwright 套件，再完成 `make test`、`make build`、`make format-check`、`go vet ./...` 和 `openspec validate add-terminal-code-diff --strict`；记录 Ubuntu/macOS CI 结果与实际失败修复，不将规划完成当作实施通过。
- [ ] 5.4 记录独立桌面/窄屏真实浏览器的可视验收；有 iOS/Android 真机时验证切标签、键盘与位置发送，没有真机时明确记载限制，不以 Pixel 7 模拟代替真机结论。

## Workflow follow-up

- 本 draft PR 仅提交 proposal、design、两份 capability spec、tasks 和 change 元数据；所有实施任务保持未完成。
- 用户要求实施后再进入 apply 流程；完成验证与评审后按项目流程归档该变更。
