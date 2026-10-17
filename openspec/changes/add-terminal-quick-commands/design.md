## Context

动机见 proposal.md。与实现相关的现状：

- 软键栏 `web/src/components/SoftKeys.svelte` 为 `<aside aria-label="终端软键">` 内单个 `.soft-row`（`overflow-x: auto`、不换行），仅在 `any-pointer: coarse` 或 `.force-soft-keys` 时显示（`web/src/styles.css`）。软键按钮在 `pointerdown` 上 `preventDefault` 以保留终端焦点。E2E `softkeys.spec.ts` 断言所有按钮同一行、320 px 下“粘贴”“唤起或收起键盘”完整可见。
- 浏览器本地设置的既有模式是 `web/src/previewSettings.svelte.ts`：模块级 `$state`，通过 `api.ts` 的 `stored`/`store`（`localStorage`，键前缀 `webmux.`，异常吞掉）读写，Zod 契约位于 `web/src/schemas.ts`。设置页 `Settings.svelte` 用 `Card` 分组，页面声明“设置仅保存在当前浏览器”。
- `Terminal.svelte` 已有统一文本入口 `insertText(text, operation)` → `drafts.insert(draft, text, operation, t.paste)`：草稿模式写入草稿光标处，否则走 `t.paste`（xterm `paste`，按程序状态包装 bracketed paste；未连接/恢复中/已退出返回 `false`，并清除锁定修饰键）。`sendDraft` 已确立“先粘贴、成功后单独 `t.send("\r")`、部分成功单独提示”的语义，`sendUnavailable` 已给出不可发送原因文案。
- 弹出层焦点处理已有先例：粘贴对话框 `onCloseAutoFocus={focusTerminal}` 在触屏上不回焦，否则回焦到 `focusInput()`。
- UI 组件目录已有 DropdownMenu、Dialog、AlertDialog、Item、Empty、Badge、Input、Textarea、Label；没有 Switch/Checkbox。图标库为 `@tabler/icons-svelte`，`components.json` 已配置 shadcn-svelte CLI。
- WebMux 常经 VPN IP 以纯 HTTP 访问，非安全上下文下 `crypto.randomUUID` 不可用（代码中剪贴板已显式检查 `isSecureContext`）。

## Goals / Non-Goals

**Goals:**

- 发送路径完全复用 `insertText` 与 `sendDraft` 的语义，不新增第二套 PTY 写入或错误处理约定。
- 软键栏新增入口不破坏单行布局与既有按钮可见性。
- 所有界面由现有 shadcn-svelte 组件组合，唯一新增的 Switch 通过官方 CLI 生成。

**Non-Goals:**

- 服务端存储、跨设备同步、导入导出。
- 变量/占位符模板、按会话或 agent 过滤指令、键盘快捷键触发、按会话的最近使用排序。
- 桌面端在软键栏以外的入口（桌面用户可通过既有“强制显示底部工具栏”使用）。
- 拖拽排序。

## Decisions

### 1. 数据契约与存储模块

`schemas.ts` 新增：

```ts
QuickCommandSchema = z.object({
  id: z.string().min(1).max(64),
  name: z.string().max(40),
  command: z.string().min(1).max(4096),   // 已规范化，非空白
  enter: z.boolean(),
});
QuickCommandsSchema = z.array(QuickCommandSchema).max(50);
```

新增 `web/src/quickCommands.svelte.ts`，结构对齐 `previewSettings.svelte.ts`：模块级 `quickCommands = $state<QuickCommand[]>(load())`，`load()` 用 `stored("quickCommands", "[]")` + `JSON.parse` + `safeParse`，失败返回 `[]` 且不回写；`saveQuickCommands(list)` 先 `parse` 再整体替换状态并 `store`。另导出纯函数 `normalizeCommand(text)`（CRLF/CR→LF，去末尾换行）和 `commandLabel(cmd)`（名称或正文首行），供设置页、菜单和单元测试共用。

模块在加载时注册一次 `window` 的 `storage` 事件，键为 `webmux.quickCommands` 时重新 `load()`，实现多标签页同步。`id` 用 `crypto.getRandomValues` 生成的十六进制串（非安全上下文可用），仅作 Svelte 列表 key 与编辑定位。

- 备选：服务端 TOML 配置。需要新增写配置 API、改变“配置只在启动时加载”的模型，并扩大认证/Origin 面；与现有设置页“仅当前浏览器”的约定冲突。暂不采用。
- 备选：每条指令单独存储键。整体数组更简单，50 × 4 KB 上限约 200 KB，在 `localStorage` 容量内。

### 2. 软键栏右侧固定区

`SoftKeys.svelte` 新增可选 snippet prop `trailing`，在 `.soft-row` 之后、`aside` 之内渲染，前面加竖向 `Separator`。`.soft-row` 已是 `flex: 1; min-width: 0`，trailing 区域 `shrink-0` 即可固定在右侧、不参与滚动。SoftKeys 不感知快捷指令业务，菜单组件由 `Terminal.svelte` 注入，保持 SoftKeys 只负责布局与按键。

放在右侧而非左侧：左端已固定“⌨”“粘贴”（E2E 要求可见），右侧对右手拇指更易达，也与顶栏“⋯”菜单的右侧习惯一致。按钮使用 `IconBolt` + “快捷指令”文字而非纯图标：文字让首次使用者无需猜测含义，宽度约 90 px，320 px 视口下仍留出可滚动的软键区。

### 3. 菜单组件 `QuickCommandMenu.svelte`

基于 `DropdownMenu`：`Content side="top" align="end"`，`class` 限制 `max-h-[min(60dvh,24rem)] w-72 max-w-[calc(100vw-1rem)] overflow-y-auto`。Props：`commands`、`target: "terminal" | "draft"`、`unavailable: string`、`run(cmd)`、`onCloseAutoFocus`。

- 顶部 `DropdownMenu.Label`：`target === "draft"` 时“插入草稿”，否则“发送到终端”；`unavailable` 非空时在其下以 muted 文本显示原因。
- 条目：`DropdownMenu.Item`，两行布局（名称/首行 + 等宽截断预览），直接模式且 `enter` 时右侧 `DropdownMenu.Shortcut` 显示 `IconCornerDownLeft`（`aria-hidden`，可访问名称追加“并回车”）。触屏下条目 `min-h-11` 保证点击目标。`disabled={target === "terminal" && !!unavailable}`。
- 空状态：`DropdownMenu.Label`“还没有快捷指令”+ 一行说明，随后“添加快捷指令…”。
- 底部：`Separator` + “管理快捷指令…”（`IconSettings`），导航到 `/settings?session=<会话 ID>#quick-commands`；“添加快捷指令…”同一目标。
- 触发按钮不加 `pointerdown preventDefault`（会干扰 bits-ui 的打开逻辑）；菜单打开时焦点进入菜单是预期行为。关闭时由 `Terminal.svelte` 传入的 `onCloseAutoFocus` 复用 `focusTerminal`：阻止默认回焦，触屏不回焦，否则 `focusInput()`。

`unavailable` 在 `Terminal.svelte` 中按与 `sendUnavailable` 相同的条件派生，但使用面向快捷指令的文案：已退出为“会话已退出，无法发送”，未就绪/重连/恢复中为“终端未就绪，暂不能发送”。按钮仅在 `t.connection === "missing"` 时禁用。

- 备选：Popover + Command（可搜索）。50 条上限下搜索价值低，且搜索框会在手机上弹出软键盘遮挡列表；DropdownMenu 与顶栏“⋯”一致，键盘导航和 typeahead 由 bits-ui 提供。
- 备选：Drawer（底部抽屉）。手机上更大，但与软键栏同侧会遮住刚发送后的终端输出，且桌面强制工具栏时体验不一致。

### 4. 发送逻辑放在 `Terminal.svelte`

```ts
function runQuickCommand(cmd: QuickCommand) {
  t.resetModifiers();
  const result = insertText(cmd.command, drafts.begin(draft));
  if (result === "failed") error = "终端未就绪，快捷指令未发送；请恢复连接后重试。";
  else if (result === "terminal" && cmd.enter && !t.send("\r"))
    error = "快捷指令已粘贴，但回车未发送；请检查终端后手动提交。";
  else error = "";
}
```

直接复用 `insertText`，草稿模式的光标插入、选区替换、会话隔离和操作有效性检查全部由 `terminal-draft.svelte.ts` 既有逻辑保证；先 `resetModifiers()` 确保草稿模式下锁定的修饰键也不残留。`"draft"` 结果从不追加回车。

- 备选：用 `t.send(cmd.command)` 模拟逐字输入。多行正文会被逐行提交、无法被 bracketed paste 保护，且与草稿发送语义分叉；不采用。

### 5. 设置页卡片与编辑对话框

`Settings.svelte` 新增 `<Card.Root id="quick-commands">`，放在“会话输出预览”卡片之前（新功能更常用）。`onMount` 中若 `location.hash === "#quick-commands"` 则 `scrollIntoView`。若查询参数 `session` 匹配会话 ID 格式 `^[a-z2-7]{8}$`（与 `App.svelte` 路由一致），顶栏按钮改为“返回终端”并导航到 `/s/<会话 ID>`，否则保持“返回工作区”；只接受会话 ID 而非任意路径，避免开放跳转。独立 PWA 没有浏览器后退按钮，这一入口保证管理完能直接回到原会话。卡片内容：

- 空列表：`Empty`（`IconBolt` 媒体、标题“还没有快捷指令”、说明“把常用命令或提示词存起来，在终端底部工具栏一键发送。”、`Button`“添加快捷指令”）。
- 非空：`Item.Group` 内每条 `Item.Root variant="outline"`；`Item.Content` 渲染为 `button`（点击编辑，含名称和等宽预览，`enter` 时 `Badge variant="secondary"`“回车”）；`Item.Actions` 放 `icon-sm` 的上移（`IconArrowUp`）、下移（`IconArrowDown`）、删除（`IconTrash`）按钮，均带 `aria-label`。删除按钮打开 `AlertDialog`，标题“删除快捷指令？”，描述显示 `commandLabel`，操作按钮为 destructive“删除”。
- 列表下方 `Button variant="outline"`“添加快捷指令”（`IconPlus`），满 50 条时禁用并显示“最多 50 条”。
- 卡片描述：“在终端底部工具栏点击“快捷指令”即可发送；仅保存在当前浏览器。桌面端可在终端菜单开启“强制显示底部工具栏”。”

编辑对话框抽为 `web/src/components/QuickCommandDialog.svelte`：`Dialog` + `Label`/`Input`（`maxlength=40`，placeholder“例如：查看状态”）+ `Label`/`Textarea`（`font-mono`，`rows=4`，`maxlength=4096`，placeholder“例如：git status”）+ `Switch` 与说明“关闭后只填入指令，由你确认后再回车”。对话框持有本地副本，“保存”时 `normalizeCommand` 并校验后回调；正文 `trim()` 为空时“保存”禁用。新建时 `onOpenAutoFocus` 聚焦正文输入。

设置页与预览卡片的“自动保存”不冲突：列表的每次结构变化（排序、删除）即时保存；对话框内是一次编辑事务，显式“保存”比逐字自动保存更符合对话框的通用习惯，也避免半成品条目出现在终端菜单中。

- 备选：在卡片内联编辑全部字段并逐字自动保存。手机上 50 条指令会形成很长的表单，且必须处理“空白正文条目”这种半成品状态；不采用。
- Switch 通过 `pnpm --dir web dlx shadcn-svelte@1.7.0 add switch` 生成到 `web/src/lib/components/ui/switch/`，依赖已安装的 bits-ui，不改 lockfile 依赖集。

## Risks / Trade-offs

- [用户把含密码/令牌的命令存为快捷指令，明文留在 localStorage] → 卡片说明“仅保存在当前浏览器”；不发送到服务端、不记录日志；不额外提示以免增加说明负担。
- [自动回车默认开启，误触即执行] → 菜单条目带回车标记、顶部写明去向；DropdownMenu 需要“打开→选择”两步才触发，误触概率低；危险命令可由用户关闭自动回车。
- [右侧固定按钮占用约 90 px，窄屏软键可视数量减少] → 软键本就横向滚动；E2E 继续在 320 px 下断言单行与关键按钮可见。
- [桌面默认不显示软键栏，桌面用户可能找不到入口] → 设置卡片说明强制显示工具栏的路径；后续若有需求再加顶栏入口（不在本次范围）。
- [不同浏览器/设备的指令不互通] → 与现有设置一致，明确写在卡片描述中；服务端同步列为非目标。
- [部分 TUI 对粘贴与随后的 CR 间隔敏感] → 与草稿发送使用同一序列，已在既有草稿功能中验证；不加延时或重试。

## Migration Plan

纯前端新增功能，无数据迁移。旧版本浏览器没有 `webmux.quickCommands` 键时视为空列表。回滚只需移除前端代码，残留的本地存储键无副作用。
