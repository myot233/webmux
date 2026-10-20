# WebMux UI 优化记录

## Method

使用 [uditgoenka/autoresearch](https://github.com/uditgoenka/autoresearch) 的 Codex skill，版本 2.2.2，读取的源码提交为 `050e30dc4ba0974b03f2873111b9901ec3211390`。采用建立基线、单项实验、提交、测量、保留的流程；两轮实验后达到本次代理指标目标，独立场景验证又发现并修复一项大字号手机布局问题。

工作分支：`codex/ui-autoresearch`。基线：`2a81223`。

提交 PR 前，将分支移回 `9b492ca`，去掉基线中无关的 Pullfrog 工作流提交。应用源码与测试文件保持一致；本记录及原始测量 JSON 保留实验当时的提交号。对应的最终分支提交为：`c277dfc` → `7e1161b`、`2020ddc` → `4567b70`、`07b4e57` → `818bf5a`。原基线 `2a81223` 与 `9b492ca` 仅有该工作流文件差异。

## Metric

固定场景：320、390、1280px，分别使用浅色和深色主题，三个真实后端会话，包含长名称。

每个场景统计页面横向溢出、手机可见操作入口小于 44 × 44px、缺少搜索、缺少状态筛选的次数。这是可操作性代理指标，不是审美评分，也不是完整无障碍审计。

| 轮次             | 提交      | 检查缺口 | 决策                          |
| ---------------- | --------- | -------: | ----------------------------- |
| 基线             | `2a81223` |       48 | 记录                          |
| 触控与会话排版   | `c277dfc` |       12 | 保留                          |
| 搜索与状态筛选   | `2020ddc` |        0 | 保留                          |
| 大字号操作栏换行 | `07b4e57` |        0 | 保留；另修复 1 项独立场景失败 |

之后去掉卡片间重复分隔线，补充图标提示和空预览设置说明，稳定主题截图与测试进程。最终指标仍为 0。原页面及最终页面在这些普通字号场景中都没有横向溢出。

## Changes

- 首页操作入口至少 44 × 44px；标题、状态、设备信息分层显示。
- 使用现有 shadcn-svelte 组件，调整背景、边框、留白与状态色条。
- 无可见内容的预览收起；有内容时仍使用用户设定的行数。
- 会话搜索支持名称、目录、仓库名称和分支，忽略首尾空白与大小写。
- 全部、运行中、等待中、已退出筛选可与搜索叠加，并响应 WebSocket 更新。
- 搜索匹配的仓库默认展开，退出搜索后恢复原来的折叠状态。
- 空结果提供清除筛选操作，图标按钮有可访问名称与提示。

## Verification

- Svelte/TypeScript 检查、生产构建和修改文件的 Prettier 检查通过。
- 83 个 Node 单元测试通过。
- 最终新增 E2E 集合：7 项通过，1 项跳过（UI 测量由桌面项目统一遍历三种宽度，手机项目避免重复执行）。
- 24 个相关 Storybook 用例通过，覆盖两个组件、六种尺寸或字号设置、两种主题。
- 新增搜索、组合筛选、中文与大小写匹配、目录匹配、退出事件实时更新、菜单、键盘导航、分支搜索及折叠恢复的真实后端测试，在桌面与 Pixel 7 模拟环境通过。
- 320px、200% 字号的完整首页操作可达性，在桌面和手机项目中单独验证通过。手机场景最初发现顶栏操作区会向左挤出屏幕；顶栏、页面操作栏和标题允许换行后通过，同时补充按钮左右边界检查。
- 原有 lifecycle 测试中 9 项通过；1 项手机滚动用例失败。
- 完整浏览器组件测试中 55 项通过，3 项终端渲染用例失败。

以下四项失败均在未修改的 `2a81223` 源码副本、同一套依赖和浏览器环境中复现，断言一致，未计为本次 UI 改动引入的回归：

1. `lifecycle.spec.ts`：手机端 replay / return-to-latest 的向上滚动断言。
2. `TerminalRendering.test.ts`：native Chrome swipes。
3. `TerminalRendering.test.ts`：native alternate-buffer wheel fallback。
4. `TerminalRendering.test.ts`：pending viewport reset after a theme change。

首次新增筛选测试使用空闲 Shell，较慢执行时会自然进入 waiting，导致运行中数量断言不稳定。测试改为持续输出的临时子进程后通过，产品状态判定保持原样。

## Environment and limits

测试使用 Node 24.22.0、Go 1.26.1，以及原工作目录已安装的依赖。此机器的 pnpm 12.x 可执行文件不可运行；现有 pnpm 10.32.1 也不能读取仓库的新格式 lockfile，因此 frozen install 未完成。没有改动 manifest 或 lockfile；工作树的被忽略 node_modules 目录链接到原安装，并用 `--config.manage-package-manager-versions=false` 调用本地工具。这不能替代干净环境的锁定依赖安装验证。

浏览器检查使用项目现有 Playwright/Vitest 与隔离的临时后端，没有连接开发者正在使用的 WebMux 服务。未进行真实 iOS/Android 设备验证。构建仍有现存的大 chunk 提示。

## Reproduce

在具备仓库要求的 pnpm 版本且已完成 frozen install 的环境中：

```sh
pnpm --dir web build
pnpm --dir web exec playwright test test/e2e/workspace-ui.spec.ts test/e2e/workspace-discovery.spec.ts
pnpm --dir web test:unit
pnpm --dir web test:browser
pnpm --dir web build-storybook
pnpm --dir web exec playwright test --config playwright.storybook.config.ts -g 'session-row|workspace-group'
```

完整测量值见 `baseline.json`、`iteration-1.json`、`final.json`；实验记录见 `loop-results.tsv`。截图放在本次聊天的本地可视化目录，避免将测试图片提交到应用仓库。

验证调度中曾将生产构建与 E2E 并行运行，Vite 清理 dist 的瞬间导致一个 Go embed 构建失败；构建结束后按顺序重跑上述 E2E 集合全部通过。
