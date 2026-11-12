# 语音输入用户故事

本页延续终端功能组的 US-G07，描述麦克风按钮、服务端识别程序与设置页的语音输入区域；US-G01–US-G06 在 [终端、输入与移动端](terminal.md) 的 G 组。

- **US-G07 点击录音并转文字插入**：作为在浏览器中向 agent 或终端输入长句子的使用者，我希望点击麦克风开始录音、看到实时音量、再次点击结束并把识别文字插入当前输入位置而不自动回车，以便补充或检查后再发送。
  验收要点：
  - 服务端在 `config.toml` 配置非空 `voice.cmd`（可选 `voice.env`）时，`GET /api/config` 返回 `voice: true`，但从不暴露 `cmd` 或 `env` 明文；未配置时返回 `voice: false`，界面不显示语音按钮，`/ws/voice` 返回 404；
  - 设置页“语音输入”区域显示服务端是否已配置识别程序，并可编辑识别命令与参数（每行一个，WebMux 不提供识别服务，命令由使用者自己编写）和环境变量（值默认遮挡）：
    - 保存经 `PUT /api/voice/settings`，不重启即对下一次录音生效，覆盖 `config.toml` 的 `[voice]` 并保存在 `voice-settings.json`；
    - “恢复为配置文件设置”经 `DELETE` 删除覆盖；
    - 校验与 `[voice]` 相同，设置文件损坏时语音识别保持关闭；
    - 环境变量值只有持登录 Token 的请求能读取，`/api/config` 不返回；
  - 同一区域提供“在本设备启用语音输入”开关（只保存在本机浏览器，默认开启；服务端未配置时禁用），并链接到 `STDIN / STDOUT` 外部识别协议文档；关闭后聊天、快速开始页与终端都不显示语音按钮；
  - 启用后，聊天模式在消息输入框与快速开始页 `/new` 输入框底部、快捷指令按钮旁显示语音按钮；终端模式只在草稿模式开启时显示，放在草稿框的操作行（暂存按钮左侧），关闭草稿时软键快捷栏与顶栏都不显示语音按钮；
  - 采用点击开始、再次点击结束的切换手势，依次经过空闲、连接准备中（加载态）、录音中（根据麦克风实时音量展示动态音柱且 `aria-pressed="true"`）、完成收尾中（加载态）四个阶段；
  - 浏览器将麦克风音频重采样为 `16000 Hz`、单声道、`s16le` PCM 二进制帧发往 `/ws/voice`，再次点击时发送 `{"type":"stop"}` 由服务端关闭子进程 `STDIN`（`EOF`）并等待 `final` 结果；
  - 收到最终文本后：聊天视图与快速开始页插入输入框光标处（或替换选区），终端视图插入草稿框，均不追加回车（`\r`）且不自动提交；
  - 麦克风权限被拒绝或子进程报错时提示具体原因并恢复为空闲状态。

依据：

- [语音识别服务](../../internal/voice/voice.go)、[语音 WebSocket](../../internal/hub/voice.go)、[语音按钮](../../web/src/components/shared/VoiceButton.svelte)、[录音状态机](../../web/src/hooks/shared/voice.svelte.ts)、[音频重采样](../../web/src/platform/voice.ts)
- [语音后端用例](../../internal/voice/voice_test.go)、[语音接口用例](../../internal/api/voice_test.go)、[音频单元用例](../../web/test/voice.test.ts)、[语音按钮浏览器用例](../../web/test/browser/VoiceButton.test.ts)
- [聊天视图插入验收](../../web/test/e2e/hybrid.spec.ts)
- [识别设置存储](../../internal/voicesettings/store.go)、[设置接口](../../internal/api/voice_settings.go)、[运行时切换](../../cmd/webmux/voicectl.go)、[设置区域](../../web/src/components/settings/VoiceSettings.svelte)、[接口文档](../protocol/http-voice.md)
