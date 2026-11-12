## 1. 模式偏好与终端顶栏菜单入口

- [x] 1.1 创建 `web/src/hooks/terminal/touch-gamepad-preference.svelte.ts`，使用 `stored("touchGamepad")` 与 `store("touchGamepad", ...)` 实现本机持久化的响应式偏好状态
- [x] 1.2 在 `web/src/components/terminal/TerminalHeader.svelte` 的“显示”分组中新增“软键盘摇杆模式”复选项，并在 `web/src/components/terminal/SessionTerminal.svelte` 中绑定该偏好

## 2. 半透明浮动摇杆组件与底部软键栏去重

- [x] 2.1 在 `web/src/lib/components/ui/button/button.svelte` 中扩展半透明浮层样式，并在 `docs/frontend-architecture.md` 登记；实现 `web/src/components/terminal/TouchGamepad.svelte`。
  - 左侧提供 `Esc`、`Tab`、`Ctrl+C`、`Enter` 功能键簇。
  - 右侧提供十字方向摇杆 `↑` `↓` `←` `→` 及左上角空格键，支持滑动方向切换与长按连发。
- [x] 2.2 更新 `web/src/components/terminal/TerminalViewport.svelte`，仅在软键盘摇杆模式开启且当前客户端已接管控制（`t.controlling === true`）、会话未退出且终端视图可见时渲染 `TouchGamepad.svelte`，在触摸或滑动终端时自动变为半透明，并保证未命中按钮区域完全穿透以维持终端触摸滚动
- [x] 2.3 更新 `web/src/components/terminal/SoftKeys.svelte`，在底部软键栏中加入回车键 `↵`（`\r`），并在浮动摇杆激活显示时从底部 `.soft-row` 动态移除重复的 `Esc`、`Tab`、`↵`、`↑`、`↓`、`←`、`→`、`Ctrl+C` 按键（保留 `Ctrl`、`Alt`、`Shift`）

## 3. 测试、文档与架构守卫校验

- [x] 3.1 在 `web/test/` 中新增单元测试与组件/E2E 测试，覆盖菜单切换、观察端隐藏与接管后显示、浮动按键与滑动连发、未触碰区域终端滚动以及底部栏去重
- [x] 3.2 更新 `docs/user-stories/` 中受影响的用户故事，运行 `pnpm --dir web check:arch`、`make docs-check` 与 `make check` 确保全部检查通过
