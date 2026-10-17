## 1. 数据契约与存储

- [ ] 1.1 在 `web/src/schemas.ts` 新增 `QuickCommandSchema`（`id`、`name` ≤40、`command` 1–4096、`enter`）与 `QuickCommandsSchema`（≤50 条），导出 `z.infer` 类型
- [ ] 1.2 新增 `web/src/quickCommands.svelte.ts`：基于 `api.ts` 的 `stored`/`store` 读写 `webmux.quickCommands`，损坏数据视为空列表且不回写；导出 `quickCommands` 状态、`saveQuickCommands`、`normalizeCommand`（CRLF/CR→LF、去末尾换行）、`commandLabel`、`newQuickCommandId`（`crypto.getRandomValues`）
- [ ] 1.3 在同一模块注册 `storage` 事件，键匹配时重新加载，实现多标签页同步
- [ ] 1.4 新增 `web/test/quickCommands.test.ts`：覆盖正文规范化（末尾多换行、CRLF、保留开头/中间空白）、超长与超数量拒绝、损坏 JSON 回退为空、`commandLabel` 无名称时取首行

## 2. 设置页管理

- [ ] 2.1 用 `pnpm --dir web dlx shadcn-svelte@1.7.0 add switch` 生成 `web/src/lib/components/ui/switch/`，确认未新增 npm 依赖、lockfile 依赖集不变
- [ ] 2.2 新增 `web/src/components/QuickCommandDialog.svelte`：Dialog 内名称 Input（maxlength 40）、指令等宽 Textarea（maxlength 4096）、“发送后自动回车”Switch（新建默认开启）、取消/保存；正文空白时禁用保存，新建时聚焦正文，取消/Esc 丢弃修改
- [ ] 2.3 在 `Settings.svelte` 新增 `id="quick-commands"` 卡片（位于预览卡片之前）：Empty 空状态、Item 列表（点击主体编辑、回车 Badge、上移/下移/删除按钮及首尾禁用）、AlertDialog 删除确认、底部“添加快捷指令”及 50 条上限提示、卡片描述含桌面强制工具栏说明
- [ ] 2.4 `Settings.svelte` 支持 `#quick-commands` 滚动定位；`session` 查询参数匹配 `^[a-z2-7]{8}$` 时顶栏按钮显示“返回终端”并导航到 `/s/<id>`，否则保持“返回工作区”

## 3. 终端菜单与发送

- [ ] 3.1 `SoftKeys.svelte` 增加可选 `trailing` snippet，在 `.soft-row` 之后以 Separator 分隔、`shrink-0` 固定于右侧，不参与横向滚动
- [ ] 3.2 新增 `web/src/components/QuickCommandMenu.svelte`：`IconBolt`+“快捷指令”触发按钮；DropdownMenu `side="top" align="end"`、限制最大高度与视口宽度；去向标签（发送到终端/插入草稿）、不可用原因、两行条目与直接模式回车标记、触屏 44 px 条目高度、空状态与“添加快捷指令…”、底部“管理快捷指令…”
- [ ] 3.3 在 `Terminal.svelte` 实现 `runQuickCommand`：`resetModifiers` 后经 `insertText` 发送/插入；直接模式成功且 `enter` 时 `t.send("\r")`；正文失败与回车失败分别提示，成功清除提示，不重试
- [ ] 3.4 在 `Terminal.svelte` 派生菜单 `unavailable` 文案（已退出/未就绪），会话不存在时禁用按钮；`onCloseAutoFocus` 复用 `focusTerminal`；管理入口导航到 `/settings?session=<id>#quick-commands`；把菜单通过 `trailing` 注入 SoftKeys

## 4. 验证

- [ ] 4.1 更新 `web/test/e2e/softkeys.spec.ts` 单行布局用例：320/390/844 px 下“快捷指令”按钮完整可见、位于最右且与其他软键同一行；软键行滚到最右端后按钮位置不变
- [ ] 4.2 新增 `web/test/e2e/quick-commands.spec.ts`（移动端项目；桌面项目通过“强制显示底部工具栏”覆盖一次发送）：空状态菜单 → 添加入口进入设置并可“返回终端”；设置页添加 `echo quick-ok`（回车开启）后在终端执行并看到输出；回车关闭的指令只出现在输入行不执行；Ctrl 锁定后发送不受影响；草稿模式下插入光标处且 PTY 无输入；断线/已退出时条目禁用；排序与删除确认反映到菜单；刷新后指令仍在
- [ ] 4.3 运行 `pnpm --dir web check`、`make format-check`、`make test-unit`、`make test-e2e`，并在真实手机浏览器手动确认菜单弹出方向、点击目标和关闭后不意外弹出软键盘
