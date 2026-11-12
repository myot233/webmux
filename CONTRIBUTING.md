# 参与贡献

感谢你对 WebMux 的关注！欢迎通过 Issue 报告问题、提出建议，或直接提交 Pull Request。

## 开发环境

| 工具     | 版本           |
| -------- | -------------- |
| Go       | 1.27+          |
| Node.js  | 26.13.0（LTS） |
| pnpm     | 12.18.0        |
| GNU Make | 任意           |

Node.js 版本固定在根目录 `.nvmrc`，CI 使用同一文件；使用 nvm 时可运行 `nvm install && nvm use`。使用 [pnpm 官方独立安装脚本](https://pnpm.io/installation#using-a-standalone-script)安装指定版本：

```sh
curl -fsSL https://get.pnpm.io/install.sh | env PNPM_VERSION=12.18.0 sh -
```

安装完成后重新打开终端，使 `pnpm` 的 PATH 配置生效，再运行：

```sh
pnpm --version  # 应为 12.18.0
pnpm --dir web install --frozen-lockfile
make test-browsers-install   # 安装 Playwright Chromium（Linux 上同时安装系统依赖，可能需要 sudo）
```

前端依赖由 `web/pnpm-lock.yaml` 锁定。请使用 pnpm，不要使用 npm、Yarn 或 Bun 管理依赖。

### 从源码构建

只想自己编译、不参与开发时，装好上述工具后执行：

```sh
git clone https://github.com/mzdluo123/webmux.git
cd webmux
make build          # 产物：bin/webmux
make install        # 安装到 ~/.local/bin/webmux
```

## 本地开发

前后端分开运行，Vite 会把 `/api` 和 `/ws` 代理到 `127.0.0.1:7681`：

```sh
# 终端 1：后端（配置中需加入 allowed_origins = ["http://127.0.0.1:5173"]）
make build && ./bin/webmux --config dev.toml

# 终端 2：前端开发服务器
pnpm --dir web dev    # http://127.0.0.1:5173
```

不添加开发 Origin 时，浏览器的写请求会被 Origin 检查拒绝。

## 组件与布局回归

运行 `pnpm --dir web storybook` 浏览真实业务组件；`pnpm --dir web test:storybook` 检查窄屏、矮屏、长文本和 200% 字号。测试命令及新增场景约定见 [Storybook 与防溢出测试](docs/storybook.md)。

## 更新 README 截图

`make build && pnpm --dir web screenshots` 会启动临时后端和演示仓库，用 Playwright 重新生成 `docs/images/` 下的截图。README 与 [项目介绍](docs/overview.md) 引用这些截图；README“更多功能截图”折叠区的 `terminal-splits`、`kitty-graphics`、`file-explorer`、`cli-agents`（`after-desktop-{light,dark}`）也由该脚本生成；用 `WEBMUX_SCREENSHOTS_ONLY=features` 可只重拍这八张，不需要 omp。其余 `docs/images/<主题>/` 是各 PR 的界面证据，不再更新。`make docs-check` 会校验 README 的图片与链接。

omp 的对话默认从 `web/scripts/screenshots/omp-session.jsonl` 回放，不调用模型、不需要 API key：

- 脚本把录制的会话放进临时的 omp 目录（`PI_CODING_AGENT_DIR`），让预置的 omp 启动配置以 `--resume` 打开它，再切到聊天视图。
- 回放时注入占位的 `ANTHROPIC_API_KEY`：omp 加载会话需要已配置的模型，但不会发出请求；占位值也保证不会误用调用方环境中的真实 key。
- 仍需要本机能运行 `omp`，且进程可执行文件名为 `omp`（官方发布的单文件二进制即可）。经 npm/Bun 安装时进程名是 `bun`，WebMux 不会把它识别为 omp，聊天切换不可用。

需要换一段对话时设置 `WEBMUX_SCREENSHOTS_RECORD=1`：omp 以已登录的模型真实回答两轮提问，结束后把新会话导出到上述文件。提交前检查文件里只有演示仓库的内容。

README 顶部的合成主图 `docs/images/hero.jpg`（浏览器窗口 + 手机外框）由 `pnpm --dir web screenshots:hero` 从 `desktop-omp.png` 与 `mobile-chat.png` 合成，不需要后端或 agent；重新生成截图后请一并运行。两个脚本都可以用 `PLAYWRIGHT_CHROMIUM_EXECUTABLE` 指定预装的 Chromium。

## 常用命令

| 命令                        | 说明                                                                              |
| --------------------------- | --------------------------------------------------------------------------------- |
| `make build`                | 构建前端并编译内嵌前端的 `bin/webmux`                                             |
| `make release`              | 交叉编译 Linux / macOS 的 amd64 与 arm64 二进制                                   |
| `make test`                 | 完整门禁：Go race 测试 + 前端全部测试（Storybook、E2E 与性能）                    |
| `make test-fast`            | 快速迭代：Go race 测试 + Vitest 单元与组件测试，不含 Storybook、E2E 与性能        |
| `make test-unit`            | Vitest 单元测试（Node）                                                           |
| `make test-browser`         | Vitest 组件测试（无头 Chromium）                                                  |
| `make test-e2e`             | Playwright 端到端测试（桌面 + Pixel 7）                                           |
| `make format`               | gofmt + Oxfmt 格式化                                                              |
| `pnpm --dir web lint`       | Oxlint 前端代码检查（含类型感知 Promise 规则）                                    |
| `pnpm --dir web check:test` | `web/src` 与顶层 Node 测试的 Svelte/TypeScript 严格检查                           |
| `pnpm --dir web check:arch` | 前端架构守卫：Svelte 推荐 lint、设计系统、分层、重复与未使用代码、架构测试        |
| `make docs-check`           | 文档与 spec 的行数、单行长度、体积、目录扇出与树状组织守卫                        |
| `make test-install`         | 安装脚本语法检查与本地 Release fixture 测试                                       |
| `make check`                | 类型检查、构建、格式检查、文档守卫、Oxlint、架构守卫、`go vet` 与 `golangci-lint` |

运行单个测试：

```sh
go test -race ./internal/session
pnpm --dir web exec vitest run --config vitest.config.ts test/input.test.ts
pnpm --dir web exec playwright test test/e2e/terminal.spec.ts
```

## 测试

`make test` 始终是完整门禁；日常迭代可用 `make test-fast`，或在需要时单独运行某一层。本地 `make test` 先并行运行 Go 与 Vitest，再并行运行 Storybook 与 E2E，避免原生手势用例与 E2E 争用资源；性能测试仍在其他套件结束后单独执行。`make test-fast` 并行运行 Go 与 Vitest。

- **Go**：标准 `testing` 包，始终使用 `-race`；默认 `-parallel=4`，多核本机可提高 `GO_TEST_PARALLEL`（例如 `make test GO_TEST_PARALLEL=8`）。
- **单元测试**：Vitest 在 Node 中运行。
- **组件测试**：真实 Svelte / Tailwind / xterm 渲染，不需要后端；本地按可用并行度最多使用 4 个 worker，避免与其他并行套件争用资源。
- **Storybook**：本地按可用并行度使用最多 6 个 worker，CI 固定为 6 个。
- **端到端测试**：`make test` 与 `make test-e2e` 先构建一次真实的生产前端和 `.cache/webmux-e2e/webmux`，并在本次调用内复用；直接运行 `pnpm --dir web test:e2e` 时仍在私有临时目录构建。各 worker 使用独立后端、临时 HOME、配置、工作目录与动态端口，结束后自动清理进程。本地按可用并行度使用最多 8 个 worker，可用 `WEBMUX_E2E_WORKERS=1` 回退串行；直接运行 Playwright 时可加 `--workers=N`。CI 在 Ubuntu 上分四片运行（每片 2 个 worker）。测试不会连接你正在使用的 WebMux 服务。

E2E 失败时的截图、trace 和日志位于 `web/playwright-report/` 与 `web/test-results/`，可通过 `pnpm --dir web test:e2e:report` 查看。

CI 先由 `Classify changes` job 按变更路径裁剪测试：
仅文档变更只跑格式、文档、持久化与 agent/web 静态守卫；仅 Go 变更跳过 Vitest、Storybook 与性能，
仅前端变更跳过 Go race job、Windows 交叉编译与 Windows 安装程序冒烟，E2E 在两类变更下都保留；
未知路径、`.github/**`、`Makefile`、模块/锁文件与安装脚本变更一律全量运行。
分类 job 会自行校验 base，`.github/` 变更独立触发全量，并执行 base 版本的分类脚本，
避免 PR 修改分类器后跳过自身的测试。

CI 将各套件拆成并行 job：格式检查与前端构建、Go race（含 `GOOS=darwin go vet` 交叉检查）、Vitest、Storybook、性能回归
以及分四片的 E2E（桌面与手机项目）均在 Ubuntu 上运行。`E2E runtime` job
按冻结锁文件安装依赖，一次构建前端、Linux E2E 程序并打包 Playwright
的三个运行包，Go race 与所有 E2E 分片依赖该 job 复用同一产物。
分片不再各自安装 pnpm、前端依赖或 Go，也不重复构建；`Test result` 汇总全部 job，
接受被有意跳过的 `skipped`，并按分类结果交叉校验应运行的 job 均为 `success`。
同一 PR 的新推送会取消仍在运行的旧检查。

速度测量、用例一致性和整体 CI 的验收方法见 [CI 耗时优化](docs/ci-performance.md)。

## Pullfrog 模型配置

`.github/workflows/pullfrog.yml` 使用 OpenCode Go 订阅通道的
`opencode-go/deepseek-v4.1-flash`，并通过 `effort: high` 固定使用 High 推理等级。
在仓库的 GitHub Actions Secrets 中设置
`OPENCODE_API_KEY`，值为订阅所在 OpenCode workspace 的 API Key；不要使用
DeepSeek 直连的 `DEEPSEEK_API_KEY`。

在 OpenCode workspace 的 **Settings → Privacy → Regions** 中启用 **Global**，
以允许 DeepSeek 请求。在 Pullfrog 控制台选择 **BYOK**，并将仓库模型设为
**Custom (set in workflow)**，避免覆盖 workflow 中的模型选择。
配置完成后，可从 GitHub Actions 的 Pullfrog workflow 手动输入 prompt 发起任务；
模型调用受订阅额度限制。

参考：[OpenCode Go](https://opencode.ai/v2/docs/console/go/)、
[Pullfrog 模型设置](https://docs.pullfrog.com/models#custom-providers)。

## 代码规范

- Go 使用 gofmt；前端使用 Oxfmt（80 列、两空格缩进、分号、尾随逗号）。提交前运行 `make format`。
- 使用 `pnpm --dir web lint` 运行 Oxlint，`pnpm --dir web lint:fix` 应用自动修复。Oxlint 通过 `oxlint-tsgolint` 对 `.ts` 与 `.svelte.ts` 启用类型感知 Promise 规则，并检查 Svelte 的脚本部分；Oxlint 没有 Svelte 模板规则，也没有 `.svelte` 脚本的类型感知，模板事件处理器仍需按 [前端代码质量基线](docs/frontend-style.md) 第 6 条处理 rejection。`check:arch` 额外运行 Svelte 推荐模板规则；`check`、`build`、`check:test`、`check:storybook` 均拒绝 warning。
- 使用 `pnpm --dir web check:arch` 检查组件复用、设计令牌与分层；基线已清空，任何违规都会失败；只有引入带存量违规的新规则时才记录基线，并在修复后运行 `pnpm --dir web arch:ratchet` 收紧。规则见 [前端架构守卫](docs/frontend-architecture.md)。
- 文档与 OpenSpec 规格遵循 [文档治理](docs/doc-governance.md)：单个文件 20–400 行、单行不超过 300 显示列（汉字与全角标点算 2 列，仅围栏代码块除外）且不超过 48 KiB，`docs/` 与 `openspec/` 每一级直接子目录不超过 20 个；`make docs-check` 失败时会给出具体指令，超长行按语义改写而非机械折行，并更新引用。
- 前端代码遵循 [前端代码质量基线](docs/frontend-style.md)：`$state` 只用于驱动界面更新，派生值用 `$derived`，`$effect` 只做副作用并清理，列表用稳定业务 ID 作 key，props 必须带类型，事件使用 Svelte 5 属性写法。
- TypeScript 开启严格模式；HTTP 响应、WebSocket JSON 和本地设置必须经过 Zod 校验。
- 修改后端 JSON 字段时，同步更新 `web/src/contracts/schemas.ts` 及其使用方；`web/test/wire-contract.test.ts` 消费 Go wire 快照并检查必填字段，新增或变更快照映射时一并更新。
- 命令以 argv 执行，不经 shell 展开。
- 不要在持有会话 / 管理器锁时执行阻塞的网络或 PTY 操作。
- 不在日志中记录终端输入输出，也不在配置响应中暴露 token。
- 基础 UI 控件使用 `web/src/lib/components/ui/` 中的 shadcn-svelte 组件，新增组件请使用 `pnpm --dir web exec shadcn-svelte add <组件名>`。
- 新增或调整 UI 图标统一使用 Iconify，优先 `tabler` 图标集；图标数据随应用本地打包，纯图标按钮需有可访问名称和提示。详细约定见 [图标规范](docs/design-system.md#icons)。

更完整的约定见 [AGENTS.md](AGENTS.md)，设计背景见 [架构设计](docs/architecture.md)。

## 提交 Pull Request

1. 从 `main` 创建分支。
2. 保持改动聚焦，并为行为变化补充测试。
3. 确认 `make format-check` 与 `make test` 通过。
4. 提交信息遵循 [Conventional Commits](https://www.conventionalcommits.org/)，例如 `feat(web): ...`、`fix: ...`。
5. 在 PR 描述中说明动机、改动内容和验证方式。

## Go 代码规范

Go 代码以 [Go 质量基线](docs/go-style.md) 为准：采纳 Uber Go Style Guide，仓库架构约定优先，例外集中登记。提交前运行 `make check`；本机与 CI 固定使用 golangci-lint v2.15.0，安装方法与 review checklist 见该文档。新增代码不得增加告警，测试 errcheck 的存量豁免只能收紧。

## OpenSpec 工作流（可选）

较大的功能可以使用 [OpenSpec](https://openspec.dev) 先撰写提案、设计与任务清单：

```sh
npm install -g @fission-ai/openspec@1.13.0
openspec list
openspec validate --all --strict --no-interactive
```

规格位于 `openspec/specs/`，在途变更位于 `openspec/changes/`。规格文档使用简体中文，结构标题与 SHALL / MUST 关键字保持英文；文件规模与目录结构遵循 [文档治理](docs/doc-governance.md)，归档变更不参与检查。在 AI 工具中通过 `/opsx-propose`（omp）、`/opsx:propose`（Claude Code）或 `$openspec-propose`（Codex）发起提案。

## 发布

维护者发布新版本：

```sh
make release VERSION=x.y.z
cd bin && sha256sum webmux-* > SHA256SUMS
gh release create vx.y.z bin/webmux-* bin/SHA256SUMS
```
