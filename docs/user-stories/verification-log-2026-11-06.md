# 核对记录：2026-11-06 起

本文收录 2026-11-06 起各次局部核对的记录：对应的故事、已运行的验证、范围限制与未核对的部分，条目按日期升序排列。前一份记录见 [verification-log-2026-11](verification-log-2026-11.md)。

本次局部核对（2026-11-06）：实现 OpenSpec 变更 `release-archived-worktree-space`（已归档工作树释放空间与恢复前重建），新增 US-J13，更新 US-Q12、US-Q19、US-C15 与覆盖索引。

- 本地检查：`go build ./...`、`go vet`（hybrid、workspace、api、wire、contract）、`gofmt`、`svelte-check`、Oxlint、`check:arch`、格式检查与 `make docs-check`。
- 快照：用 `go test ./internal/api -run TestWireSnapshots -update` 重新录制 wire 快照。
- CI：Go race、Vitest 单元与浏览器、Storybook、性能与四个 E2E 分片在 PR #344 上通过。
- 隔离演示服务（临时状态目录与 `/tmp/demo` 仓库，Claude Code 记录不启动 agent）上实际走通：
  - 侧栏“释放空间”预览与释放：目录删除、分支保留、私有引用建立，记录标注“已释放空间”并归入“已不可用工作区”；
  - `rebuild-preview`/`rebuild` 接口在原路径检出原分支，并清理私有引用；
  - 截取了侧栏与首页的桌面、Pixel 7、明暗主题前后对比截图。
- 限制：未在本机运行测试套件；未在演示中点击“重建并恢复”启动真实 agent；未做真机核对；未重新核验其余故事能力。

本次局部核对（2026-11-06）：删除 worktree 保留已归档记录并标为已释放、删除与释放前的推送检查、侧栏树显示已归档记录；更新 US-J08、US-J09、US-J13、US-Q12、US-C17 与覆盖索引，不新增故事。

- 本地：`go test -race` 覆盖 workspace、hybrid、wire、contract，api 的强制删除用例与 `TestWireSnapshots`；`svelte-check`、Oxlint、`check:arch`、格式与文档检查；受影响的浏览器用例（树、对话框、外壳）。
- 隔离实例（假 ACP agent、本地 bare 远程）：未推送时两个预览均为 `unpushed`，推送后为 `pushed`；删除后记录为已释放并出现在首页与侧栏树，点击后重建并恢复成功；无提交的 worktree 删除成功、记录原样保留。
- 限制：完整套件与 E2E 交给 CI；未做真机核对。

本次局部核对（2026-11-07）：`webmux send` 在聊天视图排队时向标准错误报告队列项与位置（`--json` 输出 `position`），并新增 `--mode refuse`（忙时 409 `busy`、不排队）与 `--mode now`（取消当前回合并立即发送）；对应更新 US-Q08、US-S14 与覆盖索引，不新增或重编号故事。

- 依据：hybrid 的 `refuse`/`now`/位置与去重用例，clicmd 的排队提示、`--json` 静默与 `refuse` 退出码用例，api 的 `busy` 映射与端到端 `refuse` 冲突用例；重新录制 `internal/api/testdata/wire/chat-prompt-queued.json`。
- 本地：`go test -race ./internal/hybrid ./internal/clicmd ./internal/api ./internal/contract ./internal/wire`、`make test-go`、`pnpm --dir web test:unit`、`make docs-check`、`make go-lint`、`pnpm --dir web check:arch` 与 `make check` 通过。
- 环境：在宿主 WebMux 会话内运行完整 Go 测试需清除 `WEBMUX_*` 变量，否则两个既有的进程归属用例会被环境误判；清除后 `make test-go` 通过。
- 隔离冒烟（独立 `state_dir`、`127.0.0.1:17681`、假 ACP agent）：`auto` 排队在标准错误显示 `q1 (position 1)` 且标准输出为空；`--json` 返回 `position: 2` 且无提示；`--mode refuse` 以 1 退出并报告 `busy`；`--mode now` 排队后取消当前回合并发送。
- 限制：未连接真实第三方 agent，只使用假 ACP agent；未做浏览器界面与真机核对；未重新核验其余故事能力。

本次局部核对（2026-11-07）：侧栏收起为窄栏时只保留展开按钮和当前模式的主要操作，聊天/工作台模式切换仅在展开的侧栏中显示，对应更新 US-C16 与 `app-shell` 主规格，不新增故事或改变覆盖索引。

- 范围：桌面端与平板窄栏；展开后的侧栏、手机抽屉与模式记忆不变。
- 依据：`AppShell` 浏览器组件测试（窄栏保留“展开侧栏”和“新建会话”，不渲染模式按钮；展开后模式按钮恢复）通过；`pnpm --dir web check`、`lint`、`check:arch`、格式检查与 `make docs-check` 通过。
- 限制：未运行全量 E2E 与 Storybook；未在真机核对；未重新核验其余故事能力。

本次局部核对（2026-11-08）：代码查看器（`CodeReader`）与 Diff 查看器（`CodeDiffReader`）支持代码字号缩放，对应更新 US-I05 与覆盖索引，不新增故事。

- 范围：工具栏新增“缩放”按钮，点击在工具栏下方展开带滑块（10px–24px，默认 13px）与重置按钮的缩放条；桌面端支持 `Ctrl + 鼠标滚轮` 缩放，移动端支持双指捏合缩放，字号偏好持久保存在本机浏览器 `localStorage`（`webmux.codeFontSize`）。
- 依据：`CodeReader` 与 `CodeDocument` 浏览器组件测试（覆盖缩放按钮与滑块、重置、`Ctrl + wheel`、双指捏合 `touchstart`/`touchmove`/`touchend`）通过；`make check` 通过。
- 限制：未在 iOS/Android 真机核对双指手势；未重新核验其余故事能力。

本次局部核对（2026-11-08）：实现 OpenSpec 变更 `add-terminal-touch-gamepad`（终端底部软键栏补充回车键 `↵`，并新增“软键盘摇杆模式”半透明浮动操控层、控制权门控、底栏去重与空白区滚动穿透），对应更新 US-H01 与覆盖索引，不新增故事。

- 依据：`SoftKeys` 与 `TerminalViewport` 浏览器组件测试（覆盖回车键、软键盘摇杆开关、观察端/控制端显示门控、底栏重复键过滤、十字键滑动切向与退格按住连发）以及 Playwright `softkeys.spec.ts` 端到端用例（菜单开关、控制权门控、底栏去重与空白区触摸滚动穿透）通过；`make check` 通过。
- 限制：未在 iOS/Android 真机核对握持手感；未重新核验其余故事能力。

本次局部核对（2026-11-09）：草稿软键与从终端转入草稿的编辑键按完整字符移动或删除，对应更新 US-F02，覆盖 US-F01 的编辑场景。故事数量和覆盖索引不变。

- 依据：Playwright `composer.spec.ts` 的新用例在桌面与 iPhone SE 的 Chromium 项目均通过，覆盖组合重音、旗帜、多码位表情、选区及未写入 PTY。
- 限制：未在移动真机核对，未重新核验其他故事。

本次局部核对（2026-11-09）：修复 iOS 终端偶发输入无回显、回车后无刷新的问题，对应更新 US-D03、US-H06，不新增故事或改变覆盖索引。

- 范围：
  - `terminal-input.ts` 输入法状态重置：失焦或页面隐藏时清空辅助 `textarea` 并结束未完成的输入法组合，避免 `composing` 与 xterm `CompositionHelper._isComposing` 卡死；
  - `terminal-input.ts` 直接输入与回车：同轮 `input` 后清除 `keypress` 标记，避免软键盘吞掉 `keyup` 阻塞后续输入；支持无 `compositionstart` 的 `insertFromComposition` 候选词提交，以及 `keyCode=229` 下以 `insertLineBreak`/`insertParagraph` 到达的回车（转换为 `\r`）；
  - `ws.ts` 与 `terminal-socket.svelte.ts`：在 `OPEN` 连接上发送终端输入时按需触发一次无副作用提交查询探测，3 秒内无任何服务端帧响应则判定为静默半开连接并自动重连恢复快照。
- 依据：`ws-retry.test.ts` 单元测试、`TerminalInput`、`TerminalReplay`、`TerminalTransport` 浏览器测试通过；`make check` 与 `pnpm --dir web check:arch` 通过。
- 限制：事件序列在 Chromium 浏览器测试中重放验证，未在 iOS 真机核对；未重新核验其余故事能力。

本次局部核对（2026-11-09）：修复手机浏览器标签页在混合会话聊天视图中滚动消息时浏览器地址栏与工具栏无法收起的问题（#386），对应更新 US-Q05、US-H06 与覆盖索引，不新增故事。

- 范围：
  - `capabilities.ts` 新增纯函数 `chatDocumentScroll(state)`，门控窄屏（`< 1024px`）、聊天视图、未打开代码栏、单窗格、非独立模式（PWA）且未处于输入或弹层交互态时的文档级滚动模式；
  - `viewport.svelte.ts` 的 `useViewport(() => !documentScroll)` 在文档滚动阅读态下解除 `document.documentElement.style.overflow = "hidden"` 与 `window.scrollTo(0, 0)` 强制归位，并在切换锁状态时重置视口高度基线，防止浏览器工具栏高度变化被误判为软键盘；
  - `styles.css`、`Terminal.svelte`、`SessionTerminal.svelte` 与 `ChatView.svelte` 在 `data-document-scroll="true"` 下改为自然流式高度，顶栏 `position: sticky; top: 0`、底部输入停靠区 `position: sticky; bottom: 0`，“回到最新”浮动在停靠区上方；
  - 聚焦消息框或表单控件、打开底部工具栏弹层、打开代码栏、分屏、桌面宽度或切到终端视图时自动回退到固定壳层与内部容器滚动，并在焦点与失焦切换时同步 `window.scrollY` 与 `scroller.scrollTop` 以保持阅读位置，切回终端前先执行 `window.scrollTo(0, 0)` 再适配 `t.fit()`。
- 依据：`capabilities.test.ts` 单元测试（18 项）、`ChatView.test.ts` 浏览器组件测试（25 项）与 Playwright `hybrid.spec.ts` 移动端端到端用例通过；`make check`、`pnpm --dir web check:arch` 与 `make docs-check` 通过；隔离演示数据下截取 390×844 手机与 1280×720 桌面、浅色与深色主题前后对比截图。
- 限制：在 Chromium 移动模拟下验证文档滚动与焦点回退，iOS Safari / Android Chrome 原生工具栏收起动画仍需真机体验；未重新核验其余故事能力。

本次局部核对（2026-11-09）：代码工作区的文件标签获得焦点时自动显示对应内容，更新 US-I04 与覆盖索引。34 项浏览器组件测试和 24 项代码工作区、独立窗口 E2E 通过。核对 3 种视口的浅色与深色截图，未在真机验证或重新核验其他故事。

本次局部核对（2026-11-09）更新 US-L03 与覆盖索引。浏览器 UI 的 PR 须同时附桌面、平板和手机截图，修改已有界面时逐视口提供前后对比。

- 核对三种截图尺寸与 Storybook 的现有视口设置一致，并检查故事编号唯一、两份索引引用和来源链接。
- 文档门禁、`TestAgents`、修改文档的格式及空白检查通过。验证范围为文档规范与故事引用。

本次局部核对（2026-11-10）：修复移动端快速开始页 `/new` 展开输入卡片时软键盘遮挡与单行紧凑输入框底部多余内边距问题，对应更新 US-Q18，不新增故事或改变覆盖索引。

- 范围：
  - `QuickStart.svelte` 接入 `useViewport()` 并将 `<main>` 高度绑定至 `--viewport-height`（`visualViewport.height`），软键盘弹起时收缩页面可视高度并固定底部展开后的 `ComposerCard` 在软键盘上方；
  - `ComposerCard.svelte` 在紧凑单行态下点击卡片外框非按钮区域时自动聚焦 `textarea`；
  - 外层 `space-y-2` 改为 `flex flex-col gap-2` 以消除隐藏的底部选项行在紧凑卡片下留下的 `margin-bottom`，紧凑态内边距工具类改为 `composer-mobile:pt-0`、`composer-mobile:px-0 composer-mobile:pb-0` 以覆盖 `pt-1`、`px-2 pb-2`，使单行输入框上下间距对称。
- 依据：
  - `QuickStart.test.ts` 与 `ChatView.test.ts` 浏览器组件测试通过，覆盖单行卡片对称内边距、外框点击聚焦展开、可视视口收缩后展开卡片保持在视口底部上方；
  - `make check`、`pnpm --dir web check:arch` 与 `make docs-check` 通过，并截取桌面（1280×900）、平板（768×800）、手机（390×844）前后对比截图与交互 GIF。
- 限制：在 Chromium 视口收缩模拟下验证软键盘避让，未在 iOS Safari 真机核对；未重新核验其余故事能力。

本次局部核对（2026-11-10）：统一聊天模式快速开始页 `/new` 与手动新建 worktree 的分支命名逻辑并扩充词表，对应更新 US-Q18，不新增故事或改变覆盖索引。

- 分支命名：快速开始页复用 `web/src/domains/workspace/branch-name.ts` 的 `suggestWorktreeBranch` 生成 `wt/<形容词>-<名词>`。
- 冲突重试：在 `step: "worktree"`、`reason: "conflict"` 时自动换名重试，仅在 3 次重试用尽后展示失败状态。
- 词表：组合扩充至 120×240（28,800 组），改用随机抽取加回退扫描，避免每次复制洗牌全量词组。
- 依据：`branch-name.test.ts` 单元测试、`QuickStart.test.ts` 与 `WorktreeCreateFields.test.ts` 浏览器组件测试通过；`pnpm --dir web check:arch` 与 `make check` 通过。
- 限制：未运行全量 E2E 与 Storybook；未重新核验其余故事能力。

本次局部核对（2026-11-10）更新 US-C09 的明暗主题切换。

- 范围：US-C09 的侧栏主题按钮。核对双向圆形过渡、键盘焦点、连续切换、无动画回退和服务端偏好。
- 依据：10 项桌面与手机 Chromium E2E 通过，含跨设备同步和终端主题回归。隔离演示服务截图覆盖桌面、平板和手机，手机 GIF 记录双向切换。
- 限制：仅使用 Chromium 和移动视口模拟，未在真机浏览器上核对。

本次局部核对（2026-11-10）：修复代码质量扫描发现的 Token 日志暴露、混合记录持久化与关闭竞态、聊天发送和预览资源问题，并补齐 CI 与测试治理；更新 US-A03、US-N03、US-K12、US-Q08、US-Q12、US-Q14、US-Q18、US-H09、US-L02、US-L05、US-L08 与覆盖索引，不新增或重编号故事。

- 范围：启动链接和二维码不再携带 Token；混合记录状态锁不再覆盖文件 I/O，失败保留 dirty 重试并记录关闭写入错误；Create/Resume 关闭窗口受控；聊天发送失败保留两段输入，成功释放图片 Object URL；安装提示 rejection 显示备用说明；紧凑输入卡片不再充当嵌套按钮。
- 贡献者治理：Go wire 快照同时触发前端契约测试并有分类器用例；fake-agent 名称统一清单并补 Qoder/QoderCN 双向 E2E；PTY 取消写入真实 drain、attach 取消结果归一、Codex 前提失效改为失败；新增故事 ID、范围、引用与退役一致性守卫。
- 已运行：`make check`、完整 `make test`（Go race、640 项浏览器组件测试、58 项 Storybook、375 项 E2E 与 18 项性能测试通过）、Darwin/Windows vet、分类器 7 个路径场景、attach 取消 100 次重复，以及 Qoder/QoderCN 桌面与手机双向 E2E。
- UI 证据：隔离真实应用完成安装提示与聊天发送失败在桌面、平板、手机的明暗主题前后对比及两段交互 GIF，检查无横向溢出；修复版启动日志不含 Token 或 `#token=`。
- 限制：未在真实 macOS、iOS 或 Android 设备核对；无头捕获不证明 hover 与 coarse pointer 行为，未重新核验其余故事能力。

本次局部核对（2026-11-10）：修复终端标题里的 Nerd Font 私有区图标在聊天侧栏、会话行、页头等界面显示为缺字方框或空白的问题，对应更新 US-B07，不新增故事或改变覆盖索引。

- 范围：界面字体栈在 Inter 与系统字体之间加入 `WebMux UI Icons` 回退，仅覆盖 Maple Mono NF CN 的 Nerd Font 图标码位；普通拉丁文字、中文与代码字体不变，图标 WOFF2 按需加载。
- 依据：
  - `ChatHistory.test.ts` 浏览器组件测试新增标题字体栈顺序与图标字体加载断言；
  - `pnpm --dir web check`、`pnpm --dir web lint`、`pnpm --dir web check:arch`、Oxfmt 与 `make docs-check` 通过；
  - `make test-frontend` 全量通过（730 项单元测试、639 项浏览器测试）；
  - 隔离组件页面下桌面（1280×900）、平板（768×1024）和手机（390×844）的浅色与深色主题前后对比截图，确认旧字体栈缺字、新字体栈显示实际图标。
- 限制：浏览器标签页与系统通知的字体由操作系统/浏览器控制，不在应用样式覆盖范围内；`make test-fast` 的 Go 段因当前进程运行在 WebMux 会话内触发隧道/服务控制保护与进程关系用例失败（`cmd/webmux`、`internal/session`、`internal/wslclient`），与本次前端改动无关；未在真机核对，未运行全量 E2E。

本次局部核对（2026-11-10）更新 US-C17 与覆盖索引。置顶顺序保存到服务端，通过会话菜单调整，并在侧栏和快速切换面板共用。

- Chromium 桌面与手机在两个独立浏览器上下文验证置顶、排序、取消置顶、快速切换、刷新与退出清理。单元测试核对不同客户端的顺序变更，Go 协议测试核对中断记录保留与删除后的清理。
- 本次使用隔离服务与浏览器模拟，未在真机上核对。

本次局部核对（2026-11-10）：聊天视图与快速开始页 `/new` 的模型选择支持文本搜索，对应更新 US-Q07、US-Q18 与 `docs/frontend-architecture.md`，不新增故事或改变覆盖索引。

- 范围：
  - 新增 `ChatChoiceCommandList.svelte` 与 `isModelOption(option)`（识别 `category: "model"` 或 `id: "model"`），按选项名称、值与说明忽略大小写过滤，展示 `reason ?? description`，无匹配时显示“没有匹配的模型”；
  - 桌面端 `ChatSelect.svelte` 新增 `searchable` 属性，启用时以 `Popover` 承载 `ChatChoiceCommandList` 并在关闭时清空搜索词；
  - 移动端 `ChatSessionOptions.svelte` 在“会话选项”底部面板内为模型项提供内联可折叠搜索列表（`Collapsible` + `ChatChoiceCommandList`），选择后自动收起并提交，展开时按 Esc 先收起列表并把焦点归还模型按钮。
- 依据：
  - `ChatView.test.ts`（28 项）与 `QuickStart.test.ts`（18 项）浏览器组件测试通过，覆盖桌面端与移动端模型搜索过滤、空状态、关闭重置搜索词及 409 错误回滚保留原值；
  - Storybook `acp-chat.spec.ts` 与 `design-system.spec.ts` 多视口（含 320×568、375×667、390×844、568×320、667×375、768×800、1280×900）布局与无横向溢出测试通过；
  - `make check`、`pnpm --dir web check:arch` 与 `make docs-check` 通过，并截取桌面（1280×900）、平板（768×800）、手机（390×844）前后对比截图与交互 GIF。
- 限制：在 Chromium 桌面与移动视口模拟下验证，未在 iOS/Android 真机核对；未重新核验其余故事能力。

本次局部核对（2026-11-10）：工作台导航树（`WorkbenchTree.svelte`）记住每个文件夹节点的展开状态，并在节点下运行中会话集合发生变化时按内容自动重算默认展开状态，对应更新 US-C17，不新增故事或改变覆盖索引。

- 范围：
  - `webmux.workbenchTree` 保存手动展开/收起状态 `open` 与各文件夹节点的运行中会话 ID 列表 `sessions`（兼容旧版扁平键值格式）；
  - 有可直达运行中会话的文件夹（下属现存工作树有运行中会话的仓库、有运行中会话的工作树与“直接会话”）默认展开到可直接点击会话项，无运行中会话的文件夹默认折叠；
  - “已不可用工作区”（`m:<repoId>`）与已归档区域始终默认折叠，会话归档或仅落入“已不可用工作区”时不自动展开仓库、不可用工作区或已归档区域；
  - 当某文件夹下的运行中会话集合变化时清除该节点的手动折叠覆盖以恢复按内容展开/收起，内容未变化的节点保留用户的手动展开/收起状态。
- 依据：`WorkbenchTree.test.ts`（11 项）与 `AppShell.test.ts`（17 项）浏览器组件测试通过；`make check`、`pnpm --dir web check:arch` 与 `make docs-check` 通过，并截取桌面（1280×900）、平板（768×800）、手机（390×844）前后对比截图与交互 GIF。
- 限制：未在移动真机核对；未重新核验其余故事能力。

本次局部核对（2026-11-10）：打开已在聊天端的运行中会话不再先出现终端加载，对应更新 US-C14，不新增故事。

- 范围：
  - 会话页代码还在加载时，聊天端提示“正在加载聊天…”，切换中提示“正在加载会话…”，终端端仍提示“正在加载终端…”，不再把聊天会话写成正在加载终端；
  - 该页代码加载过一次后，再次从首页打开会话不再重复显示加载行。
- 依据：`web/test/hybrid.test.ts`（9 项）通过；`svelte-check --fail-on-warnings`、`eslint src`、`pnpm --dir web check:arch` 与 `go test ./internal/docguard` 通过。
- 限制：未在浏览器里对真实会话做点击核对；未重新核验其余故事能力。

本次局部核对（2026-11-10）：修复 WebKit（Safari、iPadOS）终端中 omp 等程序的 Nerd Font 图标显示为空白的问题，对应更新 US-D08，不新增故事或改变覆盖索引。

- 原因：图标、制表符与中文切片在终端首次绘制后才按需加载，`font-display: block` 期间 WebGL 字形图集缓存了不可见字形；重绘依赖 `loadingdone` 事件，而 WebKit 没有 `FontFaceSetLoadEvent`，切片加载完成也未派发 `loadingdone`，图集从不重建。桌面 Chromium 正常派发，故只在平板上出现。
- 范围：
  - `web/src/platform/fonts.ts` 新增 `onTerminalFontLoad`：每个页面对 `Maple Mono NF CN` 的待加载切片各订阅一次 `FontFace.loaded`，并通知已打开的终端；未加载的切片不持有已关闭的终端；
  - `terminal-renderer.ts` 以此替换 `loadingdone` 监听，仍清空图集、重新测量并适配；
  - 重绘后再补一帧 `term.refresh`：重绘大量迟到字形时图集会新增页面，WebGL 要到之后的帧才显示，而触摸设备上终端默认无焦点，没有光标闪烁带来的后续帧。
- 依据：
  - omp 18.18.6 Nerd 预设的 217 个私有区码位全部在内置图标与 Powerline 切片的 `unicode-range` 内，排除缺字；
  - 隔离实例（main 与修复版）+ Playwright WebKit（WPE，WebGL 渲染）输出上述全部码位：
    - 修复前除前几个字形外全部空白、无 `loadingdone`，轻点终端后仍空白；
    - 只订阅 `FontFace.loaded` 时非触摸视口正常，触摸视口（`hasTouch`）要等一次交互才显示；
    - 补一帧刷新后，平板（768×1024）与手机（390×844）触摸视口在明暗主题下无需交互即全部显示，连续 3 次一致；
  - `TerminalRenderer.test.ts` 新增吞掉 `loadingdone` 后加载未加载切片的回归用例（修复前图集清空 0 次而失败）；终端相关 13 个浏览器测试文件 153 项通过；
  - `make build`（含 `pnpm --dir web check`）、`check:test`、`lint`、`check:arch`、`make format-check` 与 `make docs-check` 通过。
  - 使用者在真实 iPad 上打开修复版独立实例（未含补一帧刷新），确认 omp 的 Nerd Font 图标正常显示。
- 限制：真机只核对了 iPad 上的 omp 图标（该构建尚未补一帧刷新），未覆盖 iPhone 与中文切片的真机表现；未运行全量 E2E 与完整 `make test`。

本次局部核对（2026-11-10，仓库面板与工作树同步详情）：实现 OpenSpec 变更 `enrich-repository-details`，并归档已完成的 `batch-worktree-management`。

- 更新的故事：新增 US-J15；更新 US-J02、US-J03、US-J08、US-J14、US-C01、US-C17 与覆盖索引，同步 `docs/usage/worktrees.md`、`docs/usage/sessions.md`、`docs/usage/shell-control.md`、`docs/protocol/http-workspace.md`。
- 范围：
  - 状态读取改为 `status --porcelain=v2 -z --branch`，补充上游、领先/落后、上游已删除、修改计数与进行中操作（读取标记文件，不增加 Git 进程）；每次快照多一次批量 `log` 读取最近提交；`lastFetchedAt` 取 `FETCH_HEAD` 修改时间；
  - 分支清单补充上游与提交信息，新增 `GET /api/repositories/{id}/remotes`（服务端脱敏）与 `webmux repo remotes`；
  - “仓库详情…”改为四分区仓库面板，“管理工作树”弹窗并入“工作树”分区；工作树详情在面板内作为子视图打开；侧栏工作树行改用“⋯”菜单，首页与侧栏显示领先/落后指示。
- 依据：
  - 测量：500 个带上游分支的 `Branches` 约 93 ms；与上游相差 10000 个提交时 `status --branch` 约 29 ms，因此未启用 `--no-ahead-behind`；
  - `make format`、`make check`（含 `check:arch`、`golangci-lint` v2.15.0 与 `make docs-check`）通过；`make test-fast` 的 Go race 全部通过；
  - Vitest Node 套件 736 项中只有 `base64url.test.ts` 1 项失败：本机 Node 24 没有 `Uint8Array.prototype.toBase64`（仓库要求 Node 26），与本变更无关；浏览器套件 671 项中 `TerminalGraphics.test.ts` 1 项在整套并发运行时失败、单独重跑 18 项全部通过；
  - Storybook `overflow.spec.ts`/`contracts.spec.ts` 22 项通过（含 320×568 双倍字号）；Playwright E2E `worktrees.spec.ts`、`repository-scope.spec.ts`、`workspace-discovery.spec.ts` 与 `hybrid.spec.ts` 通过；
  - 在隔离演示仓库上采集桌面（1280×900）、平板（768×800）、手机（390×844）明暗主题的前后对比截图。
- 限制：未在 macOS 与移动真机上运行；全量 E2E 分片与 Storybook 构建交给 CI。

本次局部核对（2026-11-11，切换工作树分支的前端与文档）：实现 OpenSpec 变更 `switch-worktree-branch` 的 Web 端（后端、CLI 与快照已在前一提交）。

- 更新的故事：新增 US-J16；更新 US-J05 与覆盖索引，同步 `docs/usage/worktrees.md`、`docs/usage/shell-control.md`、`docs/protocol/http-workspace.md` 与前端架构守卫的 `Button wrap` 登记。
- 依据：
  - `pnpm --dir web check`、`check:test`、`check:storybook`、`lint`、`check:arch` 与 `make docs-check` 通过；`wire-contract`、`wire-arrays` 与 `new-session-defaults` 单测通过；
  - 浏览器用例 `SwitchBranchDialog`、`RepositoryPanel`、`WorkbenchTree`、`RepositoryGroup`、`WorktreeCreateFields`、`NewSessionDialog` 等 10 个文件 148 项通过；
  - Storybook `overflow.spec.ts` 与 `design-system.spec.ts` 25 项通过（含切换弹窗三种场景在 320×568 双倍字号下）；
  - Playwright E2E `worktree-switch.spec.ts` 在桌面与 iPhone SE 项目 6 项通过，其中已打开代码标签页在切换后由变化检测标为“内容已更新”；`worktrees`、`repository-scope`、`workspace-ui`、`session-creation` 共 38 项通过。
  - 集成：`make format`、`make check`（含 `golangci-lint` v2.15.0）通过；`make test-go` 全部通过；
  - Vitest Node 套件只有 `base64url.test.ts` 因本机 Node 24 缺少 `Uint8Array.prototype.toBase64` 失败；浏览器套件中 `RepositoryPanel.test.ts` 一项因前一个菜单仍在退出动画而偶发失败，已改为等待菜单关闭，之后连续 3 次通过；
  - E2E `worktree-switch`、`worktrees`、`repository-scope`、`workspace-discovery` 共 48 项通过；在隔离演示仓库上采集桌面、平板、手机明暗主题截图与切换流程 GIF。
- 限制：未运行全量 E2E 与 Storybook 构建（交给 CI）；未在真机上验证；无头浏览器无法验证悬停与触屏常驻。

本次局部核对（2026-11-11，获取远程与清理失效登记）：实现 OpenSpec 变更 `add-repository-maintenance`，并把仓库与 worktree 故事从 `code-and-worktrees.md` 原文迁到 `code-and-worktrees/worktrees.md`（原文件超过 48 KiB）。

- 更新的故事：新增 US-J17；更新 US-J14 与覆盖索引，同步 `docs/usage/worktrees.md`、`docs/usage/shell-control.md`、`docs/protocol/http-workspace.md`。
- 依据：
  - `remoteGit` 单测覆盖 `GIT_SSH_COMMAND`、`core.sshCommand`、`GIT_SSH` 三种情况下不注入 `BatchMode`、都未设置时注入，以及 `auth`/`network`/`timeout`/`rejected`/`error` 分类与令牌脱敏；日志断言不含 URL；
  - `FetchRemotes` 用本地 bare 远程验证清除远端已删分支、`lastFetchedAt` 前进、单个远程失败不影响其他远程、请求取消后仍完成、并发两次只执行一次 fetch，且获取期间 `CreateWorktree` 不被阻塞；
  - `make format`、`make check`（含 `golangci-lint` v2.15.0）通过；`make test-go` 全部通过；
  - Vitest Node 套件只有 `base64url.test.ts` 因本机 Node 24 失败；浏览器套件中 `TerminalGraphics`、`TerminalSyncOutput` 各 1 项在整套并发运行时失败，单独重跑 24 项全部通过；
  - Playwright E2E `repository-maintenance.spec.ts`、`worktrees.spec.ts`、`worktree-switch.spec.ts` 通过；在隔离演示仓库上采集桌面、平板、手机明暗主题截图与获取、清理流程 GIF。
- 限制：未在真机上验证；全量 E2E 与 Storybook 构建交给 CI。

本次局部核对（2026-11-11，快进、推送与清理分支的前端与文档）：实现 OpenSpec 变更 `add-branch-sync-actions` 的 Web 端（后端、CLI 与快照已在前一提交），并与切换分支、获取远程的前端合并。

- 更新的故事：新增 US-J18；更新 US-J14 与覆盖索引，同步 `docs/usage/worktrees.md`、`docs/usage/shell-control.md` 与 `docs/protocol/http-workspace.md`。
- 依据：
  - `pnpm --dir web check`、`check:test`、`check:storybook`、`lint`、`check:arch` 与 `go test ./internal/docguard` 通过；`branch-actions`、`wire-contract`、`wire-arrays` 单测通过；
  - 浏览器用例 `BranchOperations`、`RepositoryPanel`、`RepositoryGroup`、`WorkbenchTree`、`SwitchBranchDialog` 等通过；Storybook `overflow.spec.ts` 的仓库面板与分支操作场景通过；
  - Playwright E2E `branch-operations.spec.ts`、`worktree-switch.spec.ts` 与 `repository-maintenance.spec.ts` 在桌面与 iPhone SE 项目通过。
  - 集成：`make format`、`make check`（含 `golangci-lint` v2.15.0）通过；`make test-go` 全部通过；
  - Vitest Node 套件只有 `base64url.test.ts` 因本机 Node 24 失败；浏览器套件中 `TerminalGraphics`、`TerminalSyncOutput` 各 1 项在整套并发运行时失败，单独重跑 24 项全部通过；
  - E2E `branch-operations`、`repository-maintenance`、`worktree-switch`、`worktrees` 共 36 项通过；在隔离演示仓库上采集桌面、平板、手机明暗主题截图与发布、清理分支流程 GIF。
- 限制：未在真机上验证；无头浏览器无法验证悬停与触屏常驻；全量 E2E 与 Storybook 构建交给 CI。

## 2026-11-11：Handoff 01 持久身份与事务基础（US-Q12）

- 范围：新旧混合记录的 Conversation / ProviderThread 身份、Run ordinal、handoff / delivery / receipt 原子提交、迁移与关联删除；本层不开放交接或保存完整 Activity。
- 依据：新增存储与迁移测试的 Go race 检查、子进程事务提交前崩溃回滚、ID 冲突、并发与重启、淘汰与 worktree 释放/重建保留测试通过。
- 集成：`make check` 通过，含文档、持久化、架构、前端检查、vet 与 golangci-lint；前端生产构建通过。
- 兼容：Go 1.26.10 / modernc.org/sqlite v1.62.0；Linux amd64 与 macOS arm64 均以 `CGO_ENABLED=0` 交叉编译通过。
- 体积：相同前端与 `-trimpath -ldflags='-s -w'` 下，Linux amd64 从 50,610,338 到 54,853,794 字节，增加 4,243,456 字节（约 4.05 MiB）。
- 限制：未在原生 macOS 运行 SQLite；完整端到端交接尚未实现；本机继承的 `CODEX_PATH` 会覆盖 fake agent，回归测试需清除此变量。

本次局部核对（2026-11-10）：Kimi 2.5.2 从 ACP 聊天切到 TUI 时恢复本地工具注册，对应更新 US-Q04、US-Q12，不新增故事或改变覆盖索引。

- 范围：新增 agent 的 TUI 恢复准备钩子；Kimi 在切换、归档恢复和回滚启动 TUI 前，按会话环境定位 `wire.jsonl`，只把当前会话的 `acp:<会话ID>` 环境绑定改为 `local`，首次修改保留备份并原子替换，其他事件保持不变；没有事件日志的旧实现不变，补丁失败取消终端启动并回滚。
- 依据：
  - `go test -race ./internal/agentprofile/... ./internal/hybrid` 通过，覆盖精确绑定修改、备份、幂等、损坏日志、重复日志、切换、重复切换、归档恢复和失败回滚；
  - `make test-fast` 通过，包含 Go race 全仓测试、前端单元测试与 78 个浏览器测试文件共 648 项；
  - `make persistence-check`、`make docs-check` 与 `git diff --check` 通过；
  - 使用真实 Kimi 2.5.2 和隔离 WebMux 实例，经 API 创建 ACP 会话、启动后台任务并切到 TUI，确认绑定改为 `local` 且真实 Bash 工具调用成功输出标记。
- 限制：真实端到端覆盖聊天切终端，未单独点击归档恢复；未运行完整 Storybook、全量 E2E 与性能套件，未重新核验其余故事能力。

本次局部核对（2026-11-11）：默认 STUN 列表增加小米与 Bilibili，保留 Cloudflare 与 Google，对应更新 US-H12，不新增故事或改变覆盖索引。

- 范围：同步配置示例、部署说明与 P2P 变更规格；已保存的 TOML 或网页自定义列表不变。
- 依据：`go test -race ./internal/config ./internal/p2psettings ./cmd/webmux -run 'TestP2P|Test.*Store|TestConfiguredViewCorruptP2P'` 通过；`go test ./internal/api -run TestWireSnapshotsP2P -update` 通过并更新 P2P 默认列表快照。
- 网络探测：当前环境通过 IPv4 UDP 向四个节点各发送 3 次 STUN Binding 请求，Google 与 Bilibili 均成功 3 次，小米与 Cloudflare 均成功 2 次、超时 1 次；成功响应包含映射地址。
- 限制：探测结果仅代表当前出站网络，不保证各地运营商可达性；未重新核验全部 P2P 行为或其他故事。

本次局部核对（2026-11-11）：修复正式版与 dev 发版安装 NSIS 时依赖 runner 默认 Ubuntu 镜像列表的问题，对应更新 US-L04，不新增故事或改变覆盖索引。

- 范围：两条发版流程共用 `.github/scripts/install-nsis.sh`，只为该次安装使用独立的 kernel.org HTTPS 源与临时索引目录，仍以 Ubuntu 归档密钥验证签名，索引失败停止安装。
- 依据：Ubuntu 26.04 干净容器内真实安装 NSIS 3.10 并编译 Windows 安装程序成功，验证全局 `ubuntu.sources` 摘要不变；`bash -n`、受影响文件 Oxfmt、文档治理及 `TestAgents` 检查通过。
- 限制：本地容器通过宿主代理与显式 CA 包配置联网；不能代替 GitHub runner 的实际发版验收，未重新核验其他故事。

## 2026-11-11：Handoff 02 完整 Activity 与 replay 对账（US-Q05、US-Q12）

- 范围：UI 截断前的规范化消息、命令输入/退出码/输出、文件修改、计划、压缩、权限结果、错误、回合与切换；附件只存元数据和来源引用。
- 存储：64 KiB 分块追加，快照保留相同前缀；完成后事务发布 position；长项按 UTF-8 字节分页；半条崩溃尾部保持未提交并标 gap。
- 对账：ProviderThread + provider item ID，以及无 ID 的完整项有序前缀；native ID 改变、乱序、缺失或未知无 ID 内容保持 reconcile_required，TUI 缺口不被猜测性删除。
- 验证：存储与 recorder 的针对性 Go race 测试通过，含大于 256 KiB 的消息/工具、重复文本、快照尾部、后台工具跨回合归属、v1 升级和实际 fake-agent 冷 load 去重。
- 基准：Linux amd64 / Go 1.26.10，128 次 4 KiB 追加约 67.9 μs/op、60.31 MB/s、7,279 B/op；这是临时目录中的单次样本，不代表生产磁盘或全套性能验收。
- 限制：当前提供服务层读取，REST 与浏览器历史查询在下一层接入；未在原生 macOS 执行本层测试，未验证真实 provider 的完整回放能力。
- 评审补充：工具标题、命令输入与退出码独立分块保存，metadata-only patch 不改写累计输出；省略初始工具状态按 pending 处理，跨回合完成保持原 Run。新增回归覆盖这两种情况和输出零改写。
- 插话补充：接受插话后切断无 native ID 的消息连续段，显式 failed 丢弃未消费草稿；实际 ACP 调用、关闭和重开数据库的回归测试验证顺序正确且不产生伪崩溃缺口。

## 2026-11-12：移动网络 P2P 候选交换与切网恢复（US-H12）

- 范围：候选串行批量交换及空拉取、30 秒后台协商窗口、取消与迟到响应清理；切网后重建 PeerConnection，隔离旧探测；区分直连失败与已连通但收益不足。
- 定向验证：P2P session、传输与切换单测共 22 项通过；浏览器生命周期与设置组件共 8 项通过，覆盖晚到候选、12 秒握手、取消、连续切网、旧请求迟到、回到前台及停止 A/B 测速后的状态隔离。
- 静态检查：Svelte/TypeScript 生产与测试类型检查、Oxlint、前端架构守卫与文档治理通过。
- 实际浏览器：隔离实例 Chromium 与真实 Pion 建立 `prflx/host` IPv6 回环直连；关闭后退回 WebSocket，重新启用后再次直连；受控信令失败显示未建立直连。
- 限制：本次环境没有三大运营商的蜂窝接入或物理手机，未实测跨运营商成功率、真实 Wi-Fi/蜂窝切换及双方严格 NAT；未重新核验其他故事。部署页保留真机验收步骤，不以本机测试替代。
- 评审修复：分离只读状态更新与配置变更后的连接重建；打开设置页、端口映射轮询不再拆除活动直连或取消慢协商、A/B 测速。合并 main 的前端目录迁移，并同步新增测试的导入路径。
- 修复验证：22 项 P2P 单测与 11 项浏览器测试通过，新增回归覆盖页面加载保留连接与诊断更新、6 秒协商跨越多次轮询、轮询期间 A/B 测速；生产与测试类型、Oxlint、架构守卫及源码格式检查通过。
- 实例核验：隔离 Chromium 与真实 Pion 的 IPv6 回环直连在进入设置页后保持同一 peer ID，双路径测速完成；显式关闭后 peer 数为 0，重新开启后建立新 peer。没有修改布局或文案，仍未验证物理触屏、鼠标悬停及真实蜂窝网络。

## 2026-11-12：CI 暴露的 screen 硬截止判定（US-S10）

- 范围：持续输出到 1 秒硬截止时，按绝对截止时间判定快照不稳定，不再依赖 WebSocket 返回错误是否包装 `context.DeadlineExceeded`；静默窗口、退出码和已退出会话语义不变。
- 依据：PR #538 首次完整 CI 的 `TestScreenKeepsChangingPrintsUnstableSnapshot` 在 1 秒返回 `stable=true`；未通过重跑掩盖失败，也未放宽测试。
- 修复验证：静默、持续重绘、退出三项 screen 定向测试在 race 与跨包原子覆盖配置下以 `-count=20` 通过；未重新核验其他 CLI 行为。
