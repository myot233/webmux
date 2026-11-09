---
name: webmux-ui
description: Use for any WebMux UI change — components, layout, styling, icons, interaction states, mobile/touch, theming, or UI screenshots. Encodes the design system, frontend guards and the maintainer's UI preferences.
---

# WebMux UI 变更指南

适用于 `web/src`、`web/stories` 中所有可见界面的修改：新增或调整组件、布局、样式、图标、交互状态、移动端适配、主题以及 UI 截图。

权威规范以下列文档为准。本 skill 只给出执行顺序、易错点和维护者偏好，与文档冲突时以文档为准：

- [设计系统](../../../docs/design-system.md)：设计原则、令牌、圆角刻度、图标、Storybook。
- [前端架构守卫](../../../docs/frontend-architecture.md)：复用顺序、分层、`no-restyle` 契约、现有 variant/size 表。
- [前端代码质量基线](../../../docs/frontend-style.md)：Svelte 5 runes 审查规则与测试分层。
- [UI PR 证据](../../../docs/agent-rules/testing.md#ui-pull-request-evidence)：截图要求。

## Workflow

1. **先理解现状。** 读相关组件及其 Storybook story。用户给出 CSS 选择器或 HTML 片段时，先定位对应 Svelte 组件，再查同类界面的写法，避免只修一处造成不一致。
2. **按顺序找组件，找到就复用：**
   1. `web/src/lib/components/ui/`
   2. `web/src/components/`
   3. shadcn-svelte registry（`pnpm --dir web exec shadcn-svelte add <name>`，生成后审查差异）

   用户嫌外观“不精致”、要求替换，或问“放在哪里更好”时，先查 shadcn-svelte 是否有现成组件，给出 2–3 个方案并说明取舍，再动手。移动、新增控件同样如此：位置没对齐就改代码，返工成本最高。

3. **需要新外观时，在 `ui/` 组件的 `tv()` 中加 variant 或 size**，并登记到前端架构守卫的扩展表。使用处的 `class` 只能写布局（margin、宽高、grid/flex 位置）。不写原始调色板颜色、内联样式，也不写非布局的任意值。
4. **同类问题全局统一。** 改一种控件的外观或用法时，搜索所有使用处并一起迁移，不留两种写法。
5. **验证**，见下方 Verification。

## Information architecture

放控件、标题和信息之前先定归属，这是返工最多的地方：

- **先写一句“它作用于哪个对象”**（页面、仓库、列表、单行），再放到对应层级。列表标题行只放作用于这份列表的东西（筛选、排序、列表内新增），不放上级对象的操作。例：仓库级的“新建工作树”“管理工作树”“仓库详情”放在工作树列表的标题行是错的，它们属于侧栏仓库行的“⋯”菜单。
- **一个功能只有一个常驻入口。** 移动控件时删除旧位置，并迁移所有入口、测试和文档，不留两份。
- **同一信息在一屏内只出现一次。** 仓库名、数量、状态不在顶栏、页面标题和分区标题里重复。手机顶栏承载页面标题和高频图标操作，页内不再重复标题。
- **同一种界面只有一个组件。** 新聊天页和已打开的聊天共用 `ComposerCard`（卡片、单行收起、文本框、工具栏尺寸、发送键都在这里）。要改输入框外观，改这个组件；两个页面只通过 slot 提供各自不同的内容，不再各抄一份样式。工具栏里发送键之外的按钮一律用 `ghost` 无边框、`size="icon"`，由 `ComposerContext` 传入。
- **分组、折叠类功能先定边界**（什么会结束一组），写进 spec，并用交错的真实数据测试（思考、工具、回复交替），不能只用同类连续项。

## Design language

- **克制的开发工具风格。** 使用中性色和一个绿色主要操作，不用渐变、装饰动画或嵌套卡片。阴影只给浮层（菜单、弹窗、`raised` 按钮）。
- **线条优先（维护者偏好）。** 选中、切换类控件用线条表达状态，不用填充色块：
  - 所有 `Tabs.List` 都写 `variant="line"`，不使用 `bg-muted` 胶囊样式的 `default` 变体。Tabs 只用于切换带关联面板的内容区域（Tailscale 模式、会话视图、代码根）。
  - 只选一个值（分屏方向、主题、起始端、状态筛选、预览/源码、列表显示方式）用 `SegmentedControl`：基于 RadioGroup（`role="radiogroup"` 与 `radio`），同样是下划线样式，方向键切换，已选项不能再次点击清空。表单中用 `size="form"`，窄屏需要换行时用 `wrap`。测试中按 `radiogroup`、`radio` 和 `aria-checked` 定位。
  - 不用一组 Button 模拟分段控件。每个选项都需要说明文字时，再考虑 RadioGroup。共用的选择抽成组件，例如 `SplitDirectionChoice`。
  - 开关按钮保持固定 variant，只切换 `aria-pressed`；展开用 `aria-expanded`，当前项用 `aria-current="true"`。Button 会统一显示前景色描边。不要写 `variant={on ? "default" : "outline"}` 这类按状态切换的 variant。
  - 新的导航或选中态组件同样优先使用下划线或边框指示。
- **按钮层级：**
  - `default`（绿色）：主要操作，每个任务区一个。
  - `outline`：独立的文字次要操作，以及需要明显可见的单个图标操作（如停止、取用）。
  - `ghost`：纯图标或低频工具按钮；需要弱化时用 `quiet`。
  - `success`（绿色填充）：对话框里“更安全的替代操作”，如删除对话框的“仅从主页隐藏”。放在右下角，作为最醒目的操作。
  - `destructive`：危险操作，并配确认弹窗。
  - 浮在终端或代码上方的按钮用 `outline` 加 `raised`。
  - Button 没有 `secondary` 变体，不要重新加回。Badge 的 `secondary` 不受影响。
- **圆角只用刻度：**
  - `rounded-sm`：标签和行。
  - `rounded-md`：控件。
  - `rounded-lg`：容器。
  - `rounded-xl`：只用于 Dialog、AlertDialog、Drawer。
- **字号层级：**
  - 12px：元数据，也是最小字号。
  - 14px：正文和常规操作。
  - 16px：分区标题。
  - 24–30px：页面标题。
- **间距**沿用 4px 基准：4 / 8 / 12 / 16 / 24 / 32。
- **状态色必须配文字：**
  - 绿色：运行、成功。
  - 琥珀色：等待、注意。
  - 红色：失败、危险。
  - 蓝色：说明。
  - 中性色：空闲。
- **图标：** 使用 Iconify `tabler`，随应用本地打包。纯图标按钮必须同时提供 `aria-label` 和可见提示（`title` 或 Tooltip），装饰性图标加 `aria-hidden="true"`。同一动作始终使用同一图标。

## Interaction preferences

以下偏好来自维护者以往的反馈，违反过的都被要求返工：

- **可点击必须看得出来。** 能点的文字或区域要有悬停态、光标变化或按钮外观，不能看起来和普通文本一样。
- **应用内导航不开新标签页。** 进入会话、聊天或 shell 等应用内跳转都在当前页完成，不加 `target="_blank"`。
- **只保留一层焦点指示。** 带焦点环的容器里，内层控件不能再画第二个焦点环（例如在容器内使用 `Textarea variant="bare"`）。
- **对齐要精确。** 并列的元数据、图标和文字按基线或网格对齐，桌面端和移动端都要检查。重复出现的元数据优先用 grid 布局，避免文字参差不齐。
- **同排按钮尺寸一致。** 同一工具栏或按钮行里并排的图标按钮必须使用同一尺寸档位，桌面档与触屏档成对设置：宽松工具栏用 `size="icon-touch"`（即 `size-(--control-touch) sm:size-(--control-default)`），紧凑工具栏（如终端 32px 顶栏、工作区行）整体用 `icon-sm`，手机上需 44px 的个别按钮用 `icon-touch-sm`。同一行不得混用 `icon-sm` 与 `icon-touch`，也不手写 `size-8`、`min-h-*` 补尺寸。共用组件（如 `QuickCommandMenu`）接收 `size` 属性，由所在工具栏传入，不写死。验证时用 `getBoundingClientRect` 检查桌面与 390×844 下同排按钮的盒尺寸。
- **工具栏按直觉排序。** 依次放高频主要操作、状态、次要工具、低频菜单。顺序在各页面间保持一致。
- **更安全的替代操作用独立按钮，不用 checkbox。** 勾选后改变主操作含义会多一次点击，状态也隐蔽。替代操作已无意义时（例如已从主页隐藏）不显示。
- **行尾操作：悬停才出现。** 沿用 `ListButton variant="file-tree-action"`：鼠标悬停或聚焦时出现在行尾，触屏常驻。新增行尾操作不另写显隐逻辑。
- **图标按钮必须能互相区分。** 同屏的“新建会话”和“新建工作树”不能都是 `+`，用不同图标（如分支加加号）；纯图标按钮保留 `aria-label` 和 `title`。
- **无障碍名称唯一。** 名称会被测试按 role 与名称匹配，新名称不要成为已有名称的子串（如 `仓库操作：x` 与 `仓库菜单 x`）。隐藏文字后改了名称，要同步更新用它定位的测试。
- **点击显示详情。** 紧凑的指标（如 token 用量）点击后弹出 Popover 显示明细，不把所有信息常驻在界面上。
- **加载时不闪现其他组件。** 首次渲染就是最终结构，不要先出现一个组件再替换成另一个；手机端尤其要检查。
- **参考成熟产品。** 聊天界面以 Codex、Claude、Cursor 网页版的常见交互为参照，并在方案里说明借鉴了哪一点。

## Mobile

- **空间预算：页头最多两行**：标题行（标题加图标操作）和一个主按钮行。小于 `sm` 时文字按钮优先变成图标（保留 `aria-label` 和 `title`），不要让一组按钮换成第二行再右对齐。
- **固定槽位不随触控尺寸变化。** 重复行里的展开箭头、图标列宽度必须固定，否则带子项的行与普通行错位。全局 `.wm-control` 在触屏上有 44px 的 `min-width/min-height`，且未分层，`size-*` 压不住它。需要覆盖时给元素加 `data-*` 属性，在 `styles.css` 的同一媒体查询里写覆盖。
- **浮层必须留在视口内。** Popover、Dropdown、Dialog、Drawer 都要设 `collisionPadding`（`ui/` 组件已默认 8），高度用 bits-ui 的 `--bits-*-available-height` 限制，内部滚动。
- **内容优先，控件紧凑。** 手机上终端和代码要尽量多占空间。主要导航（顶栏、抽屉）的触控高度 44px（`size="icon-touch"`、`size="sm-touch"`，或 `any-pointer-coarse:min-h-11`）；设置、工作区、代码等信息密集页面用 36px 紧凑触控高度，由 `.settings-layout`、`.workspace-page`、`.code-panel` 统一设置，页面内用 `size="icon"`/`default`，不要加 `icon-touch`。终端顶栏保持 32px 紧凑高度，不能把常用按钮做得过高。
- **工具栏保持单行。** 小于 `sm` 时只常驻高频操作，低频功能（例如分屏）收进 ⋯ 菜单或直接在手机上隐藏。菜单项沿用原按钮的名称、处理函数和禁用条件。
- **响应式只用断点。** 显隐靠 `max-sm:` 和 `sm:` 断点完成，不新增 JS 布局状态。
- **处理溢出。** 方框装不下的信息可以移到容器外或下一行，不能挤压或截断关键文字。错误信息、标签和警告必须允许换行。长路径可以截断，但要保留 `title` 或可访问名称。
- **表单字号。** 手机上表单字段使用 16px 文字，避免 iOS 输入时自动缩放。

## Svelte rules

- `$state` 只用于参与渲染的状态。能算出来的值用 `$derived`。`$effect` 只做副作用，并且必须返回清理函数。
- `$props()` 写明类型；事件用 `onclick`，不用 `on:`；可增删的列表用业务 ID 作 key。
- 不对可能为 `undefined` 的 map 项使用 `bind:ref`：带 fallback 的 prop 会抛 `props_invalid_value` 并让整页空白。改用容器的 `bind:ref` 加 `querySelector`。
- 运行时才能确定的尺寸，用 `style:--name` 配合 `h-(--name)` 这类类名实现，不写 `style:height`。

## Verification

1. 运行以下检查，全部通过才算完成：
   - `pnpm --dir web check`
   - `pnpm --dir web lint`
   - `pnpm --dir web check:arch`
   - `pnpm --dir web exec oxfmt --check src`

   遇到违规时修改代码。不加 `eslint-disable` 或 `jscpd:ignore`，也不扩大基线。

2. 在真实页面或 Storybook（`pnpm --dir web storybook`，地址 `http://127.0.0.1:6006`）中查看改动。至少检查：
   - 桌面视口（1280×720）和手机视口 390×844、375×667、320×568（iPhone SE），改动涉及浮层或页头时再看横屏 667×375；
   - 每个视口下 `document.documentElement.scrollWidth <= innerWidth`（没有页面级横向溢出），打开的浮层完整落在视口内；
   - 浅色和深色主题；
   - 键盘焦点和悬停状态；
   - 长文本。
3. Playwright 移动项目包含 Pixel 7 和 iPhone SE（320×568）。iPhone SE 用 iPhone UA，会触发 iOS 专属分支（例如推送要求安装到主屏幕），相关用例按项目名跳过并写明原因。移动控件或移动入口时，迁移所有 spec，重复流程抽成 helper（如 `repository-menu.ts`），不在多个 spec 里复制。
4. 改了 `ui/` 组件或设计系统场景时，更新 `web/stories/DesignSystem.svelte` 的示例，并运行 `pnpm --dir web exec vitest run --config vitest.config.ts` 中受影响的测试。交互行为有变化时，运行对应的 E2E spec。
5. 用户要求查看效果时，提供可以直接打开的 Storybook 或应用 URL。

## Delivery

- PR 必须嵌入桌面、平板和手机三种视口的最终实现截图。修改已有界面时，每种视口都提供前后对比。尺寸、主题和图片链接要求见 [UI PR 证据](../../../docs/agent-rules/testing.md#ui-pull-request-evidence)。
- PR 必须嵌入最终实现的截图：
  - 修改已有界面时给出修改前后对比；
  - 改动涉及多个主题或视口时，覆盖浅色/深色和桌面/手机；
  - 标明视口，并使用隔离的演示数据；
  - 图片链接要求见 [UI PR 证据](../../../docs/agent-rules/testing.md#ui-pull-request-evidence)。
- 交互或动画变更还必须在 PR 正文嵌入最终实现的 GIF，作为标准交付步骤：录制真实应用或 Storybook，正常速度展示初始态、操作与结束态；需要时包含关闭或反向过程。注明视口、主题和演示数据，检查公开链接能播放，后续动效变化时重新录制；GIF 不替代静态截图。
- 行为或界面变化时，同步更新 `docs/user-stories/` 中受影响的故事，并在 PR 摘要中写明故事 ID。新增 variant 或契约时，同步更新设计系统或前端架构守卫文档。
- 交付说明要写清实际做过的视觉检查和其局限。无头浏览器的 `hover` 与 `any-pointer: coarse` 都为 false，悬停显示和触屏常驻无法验证，必须如实写“未验证”；同样要写没有真机验证。
