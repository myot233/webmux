# Proposal

## Why

用户在终端中运行 agent 时，需要查看它对当前工作树的修改，并把具体文件或行位置交给同一个 agent 继续处理。目前只能离开 WebMux 查看 diff，或手动输入路径和行号，桌面与手机上的审阅反馈流程都不连贯。

## What Changes

- 在桌面终端页右侧增加可折叠的 Diff 面板，同时保留左侧实时终端；窄屏使用“终端 / Diff”两个标签页。
- 第一版查看会话启动目录所在 Git 工作树的未提交修改，分开呈现未暂存、已暂存和未跟踪文件；支持主工作区、linked worktree，以及未注册仓库中的直接会话。
- 展示文件列表和只读统一 diff，包括增删行、双方行号、重命名、删除、二进制及受限内容提示。切换文件、刷新和布局不改变 agent 生命周期。
- 支持选择文件、单行或连续行范围，将文件位置插入当前会话终端输入；移动端成功插入后回到终端标签，不自动追加回车，不自动生成 agent 专属命令。
- 提供首次加载、手动刷新、面板重新显示、页面回到前台和终端重连后的刷新；显示读取时间与局部失败，不将旧数据或读取错误描述为最新/干净状态。
- 保持认证、Origin 校验、二进制终端协议、断线重连、终端复制/粘贴、文件上传与工作树删除保护。
- 本 PR 仅新增规划规格；后续实施不包含分支比较、提交/暂存/回退、代码编辑、文件浏览器或 app/runtime 生命周期解耦。

## Capabilities

### New Capabilities

- `terminal-diff-review`：会话范围的 Git 修改读取、只读 diff、桌面右侧面板、窄屏标签及刷新/降级行为。
- `terminal-code-references`：从 diff 选择文件与双方行位置，生成准确引用并通过现有终端输入路径交给当前 agent。

### Modified Capabilities

无。`openspec list --specs` 未发现主能力规范。现有 `add-worktree-management` 变更中的“不解析完整 diff”属于当时的范围取舍；本变更新增按需 diff，不改动其首页与生命周期合同。

## Impact

- 后端：扩展 `internal/workspace` 的按需 Git 读取，在 `internal/api` 增加以会话 ID 定位的只读 changes/diff 路由；Git 可用但注册清单不可用时仍可读取直接会话所在仓库。
- 前端：修改 `web/src/pages/Terminal.svelte`、`web/src/styles.css`，新增 diff 展示/状态模块，扩展 `web/src/schemas.ts` 与 `web/src/api.ts`；复用并完善 `useTerminal` 的可观测粘贴结果和可见性尺寸处理。
- 组件：使用项目既有 shadcn-svelte 基础组件；实施时按需生成 Tabs。第一版采用统一 diff，不预设引入 Monaco 或其他编辑器运行时。
- 验证：真实临时 Git 仓库与 PTY 的 Go race / E2E、真实浏览器响应式与键盘交互测试；覆盖 Linux/macOS，不依赖已登录的第三方 agent。
- 文档：实施后更新 `README.md` 与 `WebMux.md` 的终端页、只读接口和发送语义；无配置迁移和现有 API 破坏性变更。
