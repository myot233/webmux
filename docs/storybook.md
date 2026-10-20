# Storybook 与防溢出测试

Storybook 直接渲染业务组件，使用生产环境的 Tailwind 样式、字体与明暗主题。示例数据覆盖长会话标题、无空格路径、多语言内容、长命令、错误消息及大量草稿；不需要启动 Go 服务，也不继承应用开发服务器的 API/WebSocket 代理。

```sh
pnpm --dir web storybook          # http://127.0.0.1:6006
pnpm --dir web build-storybook    # 类型检查并构建 web/storybook-static
pnpm --dir web test:storybook     # 构建后在真实 Chromium 中检查布局
```

开发预览使用 6006 端口，自动测试由系统分配独立的回环端口，并将同一端口传给预览服务、就绪检查和测试 URL。自动测试始终测试静态构建，不复用已运行的服务。

`make test` 与 Ubuntu/macOS CI 都包含 Storybook 测试。截图、trace 与 HTML 报告位于 `web/test-results/storybook/` 和 `web/playwright-report/storybook/`；Storybook 与 E2E 使用不同子目录，复用 CI 已上传的诊断根目录。这避免后运行的套件清理另一套报告，也无需修改工作流权限。这些输出不提交到 Git。

## 测试边界

- 8 个业务场景，分别运行明暗主题：320×568、390×844、768×800、1280×900、667×320，以及 320×568 下 200% 根字号。大字号测试不等同于真实设备或浏览器缩放测试。
- 页面宽度、元素边界和文字范围都不能超出视口。仅检查 `scrollWidth` 会漏掉被父元素 `overflow-hidden` 遮住的内容，因此额外检查实际几何范围及祖先的剪裁边界（包括横向 `auto` / `scroll`），并用超宽元素、零宽 flex 子项、视口内剪裁和滚动容器剪裁的反例验证检测器确实会报错。
- 弹窗必须位于视口内；内容较长时允许内部纵向滚动。每个场景先检查必需按钮的数量与可见性，再检查操作按钮滚动后仍完整可见并通过无副作用的试点击；工作区抽屉还实际勾选表单项。反例会验证隐藏必需按钮确实使检查失败。
- 终端预览有意截断、单行输入框/文本域内部滚动、`.truncate` 和屏幕阅读器专用文字不按普通文本越界处理。对应容器仍检查边界；会话与代码路径截断后仍保留完整可访问名称或 `title`。
- 测试阻断 `/api/` 请求，避免预览中的交互访问真实后端。Storybook 中的启动、发送、删除等演示回调不执行终端操作。

## 添加场景

在 `web/stories/Overflow.stories.ts` 导出 Story，并在 `web/test/storybook/cases.ts` 登记其 ID。注册表测试会检查两边完全一致，新增场景不能绕过布局矩阵。示例使用 `web/src/schemas.ts` 验证数据；如需扩大有意溢出的白名单，应在 `overflow.ts` 精确限定元素并说明产品原因，不能用全局 `overflow-x-hidden` 掩盖问题。
