## Why

在手机等触屏设备上操作 CLI Agent（如 Claude Code、Codex、OpenCode 等 TUI）时，频繁需要按下方向键（`↑` `↓` `←` `→`）、`Esc`、`Ctrl`、退格键（`Backspace`）和回车键（`Enter`）。现有的终端底部软键栏（`SoftKeys`）存在明显局限：

- 只有一条很矮的单行横向滚动栏，按键密集，方向键无法凭拇指肌肉记忆盲操；
- 缺少退格键与回车键，删改字符或确认选项必须唤起占据半个屏幕并遮挡终端输出的系统软键盘。

引入类动作手游的**浮动虚拟手柄（软键盘摇杆模式）**，在终端区域上方提供半透明的浮动方向盘与高频功能键（`Esc`、`Ctrl`、`Backspace` 等），并与底部滚动软键栏联动去重，能够让手机用户在已接管控制的情况下顺畅完成 TUI 导航、中断与简单修改，同时保留未触碰按键区域的正常终端滚动。

## What Changes

- **终端菜单新增模式切换（`web/src/components/terminal/TerminalHeader.svelte`）**：
  - 在终端菜单（`⋯`）的“显示”分组中新增复选项 **“软键盘摇杆模式”**（本机浏览器偏好，默认在触屏设备上可随时开启/关闭并持久化于 `localStorage`）。
- **接管控制门禁与显隐条件（`web/src/components/terminal/TerminalViewport.svelte`）**：
  - 浮动摇杆与功能键**仅在当前客户端已接管终端控制权（`t.controlling === true`）**、会话连接正常且未退出、处于终端视图（非代码独占或聊天视图）时显示。
  - 当处于观察端（`!t.controlling`）时隐藏浮动按键，避免观察端误触抢夺控制权或遮挡观察画面；一旦用户点击“接管控制”或输入成为控制端，浮动按键立即显现。
- **半透明浮动方向盘与高频按键布局（新增 `web/src/components/terminal/TouchGamepad.svelte`）**：
  - 在 `.terminal-area` 左右下角对称浮动两组带毛玻璃胶囊底座（`backdrop-blur-md`）与高对比描边的半透明控件，且在用户点击或滑动终端区域时自动变为低不透明度（`opacity-30`）退让视野：
    - **左侧高频功能与确认键组（2×2）**：包含 `Esc`、`Tab`、`Ctrl+C` 与回车键 `↵ Enter`（`\r`）；
    - **右侧方向盘（D-Pad / 滑动摇杆）与空格键**：包含十字方向键 `↑` `↓` `←` `→`（采用清晰 Tabler 图标，支持点按及按住向四向滑动连发）以及位于方向盘左上角的空格键 `␣`（`" "`，用于 TUI 多选切换选择）。
- **触摸穿透与终端滚动互不干扰**：
  - 浮动控件容器本身设为 `pointer-events-none`，仅各独立按钮/摇杆响应指针事件并调用 `stopPropagation()` 与 `preventDefault()`（同时阻止兼容层 `mousedown` 夺取或丢失焦点）。
  - 手指未碰到这几个浮动按钮时，在终端任意位置（包括按钮之间的空隙）滑动，照常由底层 `attachTouchGestures` 处理终端滚屏与长按选词，且浮动摇杆在点击/滑动终端期间自动淡化为半透明；手指按在方向盘或浮动键上滑动时，指针被该控件捕获，彻底屏蔽底层终端滚动。
- **底部软键栏加入回车键并动态去重（`web/src/components/terminal/SoftKeys.svelte`）**：
  - 在底部软键栏（`SoftKeys.svelte`）中新增回车键 `↵`（`\r`），使传统底栏模式与浮动摇杆模式都具备回车键；
  - 当浮动摇杆模式处于激活显示状态时，自动从最底下的横向滚动栏（`.soft-row`）中移除已在浮动层提供的重复按键（`Esc`、`Tab`、`↵`、`↑`、`↓`、`←`、`→`、`Ctrl+C`），而将 `Ctrl`、`Alt`、`Shift` 修饰键保留在底部滑动栏中。

## Capabilities

### New Capabilities

- `terminal-touch-gamepad`: 终端触屏浮动摇杆与高频虚拟键模式，涵盖菜单切换、控制端门禁、半透明浮层按键与连发手势、未命中区域穿透滚动，以及底部软键栏的动态去重。

### Modified Capabilities

<!-- 无既有 capability 的需求级破坏性变更 -->

## Impact

- 前端：
  - 新增偏好与组件：`web/src/hooks/terminal/touch-gamepad-preference.svelte.ts`、`web/src/components/terminal/TouchGamepad.svelte`；
  - 终端视图与软键栏调整：`web/src/components/terminal/TerminalHeader.svelte`、`web/src/components/terminal/SessionTerminal.svelte`、`web/src/components/terminal/TerminalViewport.svelte`、`web/src/components/terminal/SoftKeys.svelte`、`web/src/styles.css`；
  - 测试：`web/test/` 下的单元测试与组件/E2E 测试。
- 后端：无后端或协议变更（设备级 UI 交互偏好保存在浏览器本地存储）。
- 文档与用户故事：`docs/specs.md`、`docs/user-stories/`。
