## 1. 配置与数据契约

- [x] 1.1 在 `internal/config/config.go` 中新增 `VoiceConfig`（`cmd` 与 `env`）及其 `Validate()` 规则，并在 `deploy/config.example.toml` 中添加示例注释
- [x] 1.2 在 `internal/contract/config.go`、`internal/contract/voice.go` 与 `web/src/contracts/schemas.ts` 中新增 `ConfigResponse.Voice` 布尔字段及 `/ws/voice` 客户端/服务端消息契约

## 2. 后端语音识别服务与 WebSocket 端点

- [x] 2.1 创建 `internal/voice` 包并在 `internal/archtest/layers_test.go` 中登记为 `service` 层，实现基于 `STDIN / STDOUT` 的子进程启动、PCM 写入、EOF 刷出、NDJSON 与纯文本回退解析及超时清理
- [x] 2.2 在 `internal/api/voice.go` 与 `internal/api/router.go` 中实现受认证与 Origin 检查保护的 `GET /ws/voice` 端点，并在 `cmd/internal/assemble` 中完成 `voice.Service` 组装注入
- [x] 2.3 为 `internal/config`、`internal/voice` 与 `internal/api` 编写单元测试并更新 `internal/api/testdata/wire` 快照

## 3. 前端音频采集与双模式录音组件

- [x] 3.1 实现 `web/src/hooks/shared/voice.svelte.ts`（`16kHz Mono s16le` 音频采集、实时音量 `level` 计算、`/ws/voice` 协议通信与四阶段状态机）与 `web/src/components/shared/VoiceButton.svelte`
- [x] 3.2 在聊天模式（`ChatComposer.svelte`、`QuickStart.svelte`）和终端模式（`SoftKeys.svelte`、`TerminalViewport.svelte`、`TerminalHeader.svelte`、`TerminalInputComposer.svelte`、`SessionTerminal.svelte`）挂载语音按钮，实现识别完成后写入文本且不自动回车
- [x] 3.3 在 `web/test/` 中添加语音按钮状态机与文本插入的单元测试，并运行前端架构守卫与类型检查

## 4. 外部单文件测试脚本与文档校验

- [x] 4.1 将 `/home/rainchan/voice/` 下的各提供商脚本改造为内联公共逻辑、支持 `STDIN / STDOUT` 流式协议与本地文件双模式的独立单文件（`node <file.js>` 即可运行，且不放入 WebMux 仓库）
- [x] 4.2 更新 `docs/deployment/configuration.md`（`[voice]` 配置与 `STDIN / STDOUT` 协议规范）、`docs/protocol/websocket.md`、`docs/user-stories/`，并运行 `make check` 验证全部守卫通过
