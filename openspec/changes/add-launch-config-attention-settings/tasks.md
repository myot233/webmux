# Tasks

## 1. 启动配置数据模型、契约与持久化

- [x] 1.1 在 `internal/launch`（`types.go`、`validate.go`、`store.go`）中新增 `AttentionMode` 与 `AttentionSettings` 定义、克隆和输入校验逻辑（`mode` 为 `all`/`urgent`/`off`，`minTurnSeconds` `0..300`，`doneDelaySeconds` `0..60`），并通过 `go test -race ./internal/launch` 验证单元测试通过
- [x] 1.2 在 `internal/contract`、`internal/wire`、`internal/api` 和 `web/src/contracts/schemas.ts` 中同步 `attention` 可选字段的 JSON 契约与转换逻辑，运行 `go test ./internal/api -run TestWireSnapshots -update` 与 `go test -race ./internal/api ./internal/wire` 验证契约快照与接口测试通过

## 2. 终端会话信号源开关、子代理过滤与防误报状态机

- [x] 2.1 在 `internal/session` 中引入会话级 `AttentionPolicy`（支持按会话覆盖 `Mode`、`ProgramStatus`、`AgentProtocol`、`Notify`、`Bell`）并接入信号解析与环境变量构建，运行 `go test -race ./internal/session` 验证优先级回退与范围过滤
- [x] 2.2 在 `internal/session/attention.go` 中实现 Warp `cli-agent` 事件的主会话 `session_id` 跟踪与子代理 `stop`/`idle_prompt`/`stop_failure`/`subagent_start` 过滤（保留 `question_asked` 与 `permission_request`），运行 `go test -race ./internal/session` 验证子代理完成不会误触发主会话 `done`
- [x] 2.3 在 `internal/session` 中实现非紧急完成信号（`done`、`notify`、`bell`）的 `minTurnSeconds` 最短回合耗时校验与 `doneDelaySeconds` 延迟确认定时器（恢复 `working`、用户输入、收到紧急提醒或会话退出时取消），运行 `go test -race ./internal/session` 验证防抖逻辑

## 3. 混合会话（ACP 聊天端）与工作区编排接入

- [x] 3.1 在 `internal/hybrid` 中为聊天回合结束（`end_turn`）的 `done` 提醒接入会话级 `AttentionPolicy`（遵守 `mode`、`minTurnSeconds` 与 `doneDelaySeconds`，在新回合、切换终端、新后台任务或会话关闭时取消挂起定时器），并通过 `go test -race ./internal/hybrid` 验证单元测试通过
- [x] 3.2 在 `internal/workspace` 创建会话流程中将启动配置的 `AttentionSettings` 转换为会话与混合服务的提醒策略，并通过 `go test -race ./internal/workspace` 验证预设配置能正确传递至新建会话

## 4. 前端启动配置弹窗 UI 与信号源优先级提示

- [x] 4.1 在 `web/src/domains/settings/launch-configs.ts` 与 `web/src/components/settings/LaunchConfigDialog.svelte` 中实现“完成与介入提醒”分组。
  - 包含“跟随全局默认”、提醒范围 `SegmentedControl`、带优先级说明的信号源开关、阈值输入框及摘要标签。
  - 运行 `pnpm --dir web exec vitest run --config vitest.config.ts` 验证通过。
- [x] 4.2 更新用户文档 `docs/usage/attention.md`、`docs/usage/sessions.md` 及受影响的 `docs/user-stories/` 条目，明确记录启动配置提醒设置与四层信号源优先级说明，并通过 `make docs-check` 验证文档守卫通过

## 5. 集成检查与全量门禁验证

- [x] 5.1 运行 `pnpm --dir web check:arch` 与 `make check` 验证前后端架构守卫、类型检查、格式化与静态检查全部通过
- [x] 5.2 运行 `make test-fast` 验证所有 Go 竞态测试与前端单元测试全部通过
