# 参与贡献

感谢你对 WebMux 的关注！欢迎通过 Issue 报告问题、提出建议，或直接提交 Pull Request。

## 开发环境

| 工具     | 版本    |
| -------- | ------- |
| Go       | 1.26+   |
| Node.js  | 24 LTS  |
| pnpm     | 12.11.2 |
| GNU Make | 任意    |

```sh
corepack enable && corepack prepare pnpm@12.11.2 --activate
pnpm --dir web install --frozen-lockfile
make test-browsers-install   # 安装 Playwright Chromium（Linux 上同时安装系统依赖，可能需要 sudo）
```

前端依赖由 `web/pnpm-lock.yaml` 锁定。请使用 pnpm，不要使用 npm、Yarn 或 Bun 管理依赖。

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

## 常用命令

| 命令                | 说明                                                  |
| ------------------- | ----------------------------------------------------- |
| `make build`        | 构建前端并编译内嵌前端的 `bin/webmux`                 |
| `make release`      | 交叉编译 Linux / macOS 的 amd64 与 arm64 二进制       |
| `make test`         | Go race 测试 + 前端全部测试                           |
| `make test-unit`    | Vitest 单元测试（Node）                               |
| `make test-browser` | Vitest 组件测试（无头 Chromium）                      |
| `make test-e2e`     | Playwright 端到端测试（桌面 + Pixel 7）               |
| `make format`       | gofmt + Prettier 格式化                               |
| `make check`        | 类型检查、构建、格式检查、`go vet` 与 `golangci-lint` |

运行单个测试：

```sh
go test -race ./internal/session
pnpm --dir web exec vitest run --config vitest.config.ts test/input.test.ts
pnpm --dir web exec playwright test test/e2e/terminal.spec.ts
```

## 测试

- **Go**：标准 `testing` 包，始终使用 `-race`。
- **单元测试**：Vitest 在 Node 中运行。
- **组件测试**：真实 Svelte / Tailwind / xterm 渲染，不需要后端。
- **端到端测试**：每次启动真实 Go 后端和 Vite，使用临时 HOME、配置、工作目录与动态端口，结束后自动清理进程。测试不会连接你正在使用的 WebMux 服务。

E2E 失败时的截图、trace 和日志位于 `web/playwright-report/` 与 `web/test-results/`，可通过 `pnpm --dir web test:e2e:report` 查看。

CI 在 Ubuntu 与 macOS 上运行格式检查和全部测试套件。

## 代码规范

- Go 使用 gofmt；前端使用 Prettier（两空格缩进、分号、尾随逗号）。提交前运行 `make format`。
- TypeScript 开启严格模式；HTTP 响应、WebSocket JSON 和本地设置必须经过 Zod 校验。
- 修改后端 JSON 字段时，同步更新 `web/src/schemas.ts` 及其使用方。
- 命令以 argv 执行，不经 shell 展开。
- 不要在持有会话 / 管理器锁时执行阻塞的网络或 PTY 操作。
- 不在日志中记录终端输入输出，也不在配置响应中暴露 token。
- 基础 UI 控件使用 `web/src/lib/components/ui/` 中的 shadcn-svelte 组件，新增组件请使用 `pnpm --dir web exec shadcn-svelte add <组件名>`。

更完整的约定见 [AGENTS.md](AGENTS.md)，设计背景见 [架构设计](docs/architecture.md)。

## 提交 Pull Request

1. 从 `main` 创建分支。
2. 保持改动聚焦，并为行为变化补充测试。
3. 确认 `make format-check` 与 `make test` 通过。
4. 提交信息遵循 [Conventional Commits](https://www.conventionalcommits.org/)，例如 `feat(web): ...`、`fix: ...`。
5. 在 PR 描述中说明动机、改动内容和验证方式。

## OpenSpec 工作流（可选）

较大的功能可以使用 [OpenSpec](https://openspec.dev) 先撰写提案、设计与任务清单：

```sh
npm install -g @fission-ai/openspec@1.13.0
openspec list
openspec validate --all --strict --no-interactive
```

规格位于 `openspec/specs/`，在途变更位于 `openspec/changes/`。规格文档使用简体中文，结构标题与 SHALL / MUST 关键字保持英文。在 AI 工具中通过 `/opsx-propose`（omp）、`/opsx:propose`（Claude Code）或 `$openspec-propose`（Codex）发起提案。

## 发布

维护者发布新版本：

```sh
make release VERSION=x.y.z
cd bin && sha256sum webmux-* > SHA256SUMS
gh release create vx.y.z bin/webmux-* bin/SHA256SUMS
```
