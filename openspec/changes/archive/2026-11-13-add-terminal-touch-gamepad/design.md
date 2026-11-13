## Context

在手机等触屏设备上操作 CLI Agent（如 Claude Code、Codex、OpenCode 等交互式 TUI）时，高频按键包括方向键（`↑`、`↓`、`←`、`→`）、`Esc`、`Ctrl`、退格键（`Backspace`）与回车键（`Enter`）。现有的 `SoftKeys.svelte` 是终端底部的一条 36px 矮滚动栏，存在三个突出问题：

1. **盲操困难**：`↑ ↓ ← →` 横向一字排开且混在其他按钮中间，用户需要低头寻找或横向滚动，无法像手柄十字键那样凭拇指肌肉记忆盲操；
2. **缺少退格与回车**：现有软键栏没有 `Backspace`（`\x7f`）和 `Enter`（`\r`），删改字符或确认菜单项必须唤起占据半屏的系统虚拟键盘；
3. **屏幕空间与控制权边界**：用户希望采用类似动作手游的浮动按钮直接悬浮在终端界面上方，并且满足五项明确约束：
   - 可以在终端菜单栏切换这两种模式；
   - 启动软键盘摇杆模式时，把对应的重复键从最底下的滚动栏里删除；
   - 浮动键应当是半透明的，不遮挡终端内容；
   - 手指没有碰到这几个键时，照常发送滚动到终端；
   - 必须在已经接管控制（`controlling === true`）的情况下才显示这些浮动键。

## Goals / Non-Goals

**Goals:**

- 在终端顶栏菜单（`TerminalHeader.svelte` 的“显示”分组）提供 **“软键盘摇杆模式”** 开关，并作为本机设备偏好持久化在 `localStorage`。
- 严格绑定控制端门禁：仅当 `touchGamepad.enabled && t.controlling && t.connection !== "missing" && t.exitCode === null && visible` 时渲染浮动摇杆层 `TouchGamepad.svelte` 并精简底部滚动栏。
- 设计符合 WebMux 设计系统（`webmux-ui` / `frontend-architecture.md`）的半透明浮层按钮（在 `ui/button` 中提供专用的半透明浮层样式，遵守 `shadcn/no-restyle` 契约），使下方终端字符依然清晰可辨。
- 支持十字方向盘滑动手势（按住向 `↑`、`↓`、`←`、`→` 方向滑动或长按连发）以及退格键长按连发，同时保证未触碰按钮的空白区域完全穿透（`pointer-events-none`），不影响 `.terminal-host` 的触摸滚动与长按选词。
- 在底部 `SoftKeys.svelte` 与浮动摇杆 `TouchGamepad.svelte` 中均加入回车键 `↵`（`\r`）；当浮动摇杆激活显示时，从底部 `SoftKeys.svelte` 的 `.soft-row` 中动态移除 `Esc`、`Tab`、`↵`、`↑`、`↓`、`←`、`→`、`Ctrl+C`（`Ctrl` 保留在底部滑动栏），减少横向滚动长度。

**Non-Goals:**

- 不改变后端 API、WebSocket 协议或服务端 `settings.json` 结构（触屏操控习惯因手机/桌面设备形态而异，与 `keyboardSizeHold` 一样属于本机设备级偏好）。
- 不在观察端（`!t.controlling`）显示浮动按键，避免观察时遮挡画面或误触抢夺控制权。

## Decisions

### D1 设备级偏好存储（复用 `keyboardSizeHold` 模式）

- **决策**：
  - 新增 `web/src/hooks/terminal/touch-gamepad-preference.svelte.ts`，使用 `stored("touchGamepad")` 与 `store("touchGamepad", value ? "1" : "0")` 在模块级 `$state` 中维护开关状态；
  - 在 `TerminalHeader.svelte` 的“显示”分组（紧随“锁定尺寸”、“键盘弹出时保持尺寸”、“强制显示底部工具栏”之后）提供 `DropdownMenu.CheckboxItem` **“软键盘摇杆模式”**。
- **原因与对比**：
  - 用户通常在手机上开启摇杆模式，而在桌面大屏浏览器上关闭；将偏好保存在设备本地 `localStorage`（而非跨设备同步的 `settings.json`）可避免手机开启后污染桌面端界面，且与现有的 `keyboardSizeHold`（`web/src/hooks/terminal/keyboard-size-hold-preference.svelte.ts`）完全一致。

### D2 必须已接管控制（`controlling === true`）才激活浮动层与底部去重

- **决策**：在 `TerminalViewport.svelte` 中计算派生状态 `const gamepadActive = $derived(touchGamepad && t.controlling && t.connection !== "missing" && t.exitCode === null && visible)`：
  - 仅当 `gamepadActive` 为 `true` 时，在 `.terminal-area` 内渲染 `<TouchGamepad />`；
  - 将 `gamepadActive` 传给 `<SoftKeys gamepad={gamepadActive} />`，仅当浮动键实际在屏幕上可用时才从底部滚动栏剔除对应按键。
- **原因**：
  - 满足用户“必须在已经接管控制的情况下显示这些键”的要求；同时如果当前处于观察端，浮动层隐藏，底部栏保留原有完整按键（或在接管控制后立即联动切换），不会出现“浮动层没显示、底部栏也找不到按键”的死角。

### D3 半透明浮层视觉与前端架构守卫（`ui/button` 扩展）

- **决策**：
  - 为遵守 `shadcn/no-restyle`（业务组件不得在 `<Button class="...">` 中写颜色、透明度、背景或圆角），在 `web/src/lib/components/ui/button/button.svelte` 的 `tv()` 中新增专用的半透明浮层变体，并在 `docs/frontend-architecture.md` 扩展表中登记；
  - 浮层按钮静置时采用低不透明度边框与表面（如 `border-border/50 bg-background/45 text-foreground/85 hover:bg-background/75 active:bg-background/85 dark:border-input/50 dark:bg-background/40`），确保下方的终端字符清晰可读。
  - 左右浮动簇放置在 `.terminal-area` 底部两侧（`absolute inset-x-3 bottom-3 z-10 pointer-events-none flex items-end justify-between`），容器本身 `pointer-events-none`，仅内部的按钮组开启 `pointer-events-auto`。
  - 当窄屏出现“回到最新”浮动按钮（`!t.atBottom`）时，“回到最新”按钮位于顶部居中或方向盘上方，避免与右下角方向盘重叠。

### D4 左右分区的动作手游式布局与摇杆滑动连发手势

- **决策**：
  - **左侧拇指区（高频控制与确认簇，2×2 毛玻璃胶囊底座）**：
    - 第一行：`Esc`（`\x1b`）、`Tab`（`\t`）；
    - 第二行：`Ctrl+C`（`\x03`）、`↵ Enter`（回车 `\r`，带 `IconCornerDownLeft`）；`Ctrl` 保留在底部横向滑动栏中。
  - **右侧拇指区（十字方向摇杆 D-Pad + 左上角空格键，毛玻璃胶囊底座）**：
    - 采用紧凑的 3×2 网格布局（使用高辨识度 Tabler 矢量图标）：
      - 左上：`␣`（空格 `" "`，带 `IconSpace`，方便在多选菜单中切换勾选），上中：`↑`（`\x1b[A`）；
      - 下排：`←`（`\x1b[D`）、`↓`（`\x1b[B`）、`→`（`\x1b[C`）。
  - **触摸/滑动终端时自动变半透明退让**：
    - 当用户点击或滑动 `.terminal-host` 时，`TouchGamepad` 立即切换至低不透明度（`opacity-30`），松手后延迟恢复，避免阅读或滚屏时遮挡视线。
    - **摇杆滑动与长按连发机制**：
      - 用户既可以直接点按/长按单个方向键（按下立即发送一次，按住 350ms 后每 75ms 自动连发）；
      - 也可以把右侧十字区当作**滑动摇杆**：手指在方向盘任意键按下后（通过 `setPointerCapture` 捕获指针），无需抬起手指，直接向 `↑`、`↓`、`←`、`→` 方向拖动偏移超过阈值（如 14px），即自动切换为目标方向并持续连发对应方向键，带来动作手游虚拟摇杆的顺滑手感。

### D5 指针隔离与终端滚动穿透

- **决策**：
  - 在 `TouchGamepad.svelte` 的按钮上监听 `onmousedown={(e) => e.preventDefault()}`（与 `SoftKeys.svelte` 相同的 `keepFocus` 机制，防止 iOS Safari 或桌面点击夺走终端/草稿框焦点导致软键盘收起）。
  - 在 `onpointerdown` / `ontouchstart` / `ontouchmove` 上对命中的按钮执行 `event.stopPropagation()`，阻止事件冒泡或被 `.terminal-host` 捕获；而 `.terminal-host` 的 `attachTouchGestures` 仅绑定在 `.terminal-host` 元素自身上（位于 `.terminal-area` 内，与 `TouchGamepad` 为兄弟节点）。
  - 因此：手指点在浮动按钮以外的任何终端区域时，事件直接命中 `.terminal-host`，终端滚动与长按选词 100% 保持原样；手指点在浮动按钮上时，事件不会进入 `.terminal-host`，绝不触发终端滚动。

## Risks / Trade-offs

- **[右下角方向盘与“回到最新”按钮位置冲突]** → 当 `!t.atBottom` 显示“回到最新”按钮且 `gamepadActive` 为 `true` 时，将“回到最新”按钮置于终端底部水平居中或上移至摇杆上方，确保两者互不遮挡。
- **[长按连发在指针取消或切出页面时未清理定时器]** → 在 `pointerup`、`pointercancel`、`lostpointercapture`、`blur` 以及组件 `onDestroy` 时统一清除连发定时器，确保不会产生幽灵输入。
