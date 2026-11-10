# 核对记录：2026-11-01 至 2026-11-05

本文收录 2026-11-01 至 2026-11-05 各次局部核对的记录：对应的故事、已运行的验证、范围限制与未核对的部分。

条目由[用户故事索引](../user-stories.md)原文迁入，按日期升序排列，措辞保持当时的口径；故事的当前内容以各分组文档为准。

其他时间段见[verification-log-2026-10-23-to-26.md](verification-log-2026-10-23-to-26.md)、[verification-log-2026-10-27-to-31.md](verification-log-2026-10-27-to-31.md)与[verification-log-2026-11-06](verification-log-2026-11-06.md)。

本次局部核对（2026-11-01）：基于上游 `76d82fd` 整合九款 CLI 入口，恢复 Gemini 默认启动配置，并新增 Cursor、Copilot、Qwen、Kimi、CodeBuddy，对应 US-B01、US-B08。

- 通过：53 项启动配置单元、21 项浏览器组件、6 项桌面/Pixel 7 E2E，相关 Go race、固定版本 Go lint、前端类型、架构、文档治理和生产构建检查。
- 隔离生产实例以替身 CLI 验证九款配置的命令、参数边界与退出码，并重拍桌面/393px 手机的浅色和深色对比图；未使用真实账号或调用模型。
- Gemini 认证方式见 [接入说明](../cli-agents.md)，保留入口不承诺账号套餐或额度；既有启动配置仍不补写或覆盖。
- 核对了 174 个故事 ID、两份索引及 B 组来源路径；未重新验收其余功能组或真实手机键盘。

本次局部核对（2026-11-01）：#220 代码工作区资源管理器的右键文件操作（上传、下载、目录 ZIP、新建文件夹、重命名、复制、剪切、粘贴、删除）对应新增 US-I19，并更新 US-I18（临时文件夹改为可写）与 US-A05（代码接口含文件操作）；覆盖索引同步。未重新核验其余故事能力。

本次局部核对（2026-11-01）：#220 补充移动端文件操作（长按、底部操作面板、触控尺寸、剪贴板提示条、重命名预选文件名），对应更新 US-I19；Playwright 桌面与 Pixel 7 用例覆盖长按与操作面板。未重新核验其余故事能力。

本次局部核对（2026-11-01）：独立代码窗口改为在窗口内用同源 iframe 运行独立的代码工作区，并新增代码工作区拖放上传，对应更新 US-I13、US-I19。未重新核验其余故事能力。

- 行为：弹出、收回与直接关闭窗口保留完整阅读状态；窗口内右键菜单、对话框与底部面板不再打开在主窗口。
- 依据：`CodePanel`、各阅读组件与导航的浏览器测试，以及 `code-popout.spec.ts` 的桌面 E2E（画中画、普通弹窗、拖放上传）。

本次局部核对（2026-11-01）：#208 的 Codex CLI 混合模式与第三方 ACP 连接器安装检查对应新增 US-Q16，并更新 US-Q01、US-Q03、US-Q08，覆盖索引同步。

- 依据：`internal/agentprofile`、`internal/hybrid`（含 `codex_test.go` 的往返、连接器缺失与安装后重检、启动文件检查、外部占用、插话退回队首、启动静默期）与 `internal/session` 的 Go race 测试，线上快照与 wire 契约用例，浏览器组件测试，以及桌面/Pixel 7 的 Codex E2E 用例。
- 真实工具实测：Linux 上 Codex CLI 0.166.0 与 codex-acp 2.5.0 的写锁、`session/load` 配置重置、`thread_active_writer`、插话 `idleBehavior` 与权限选项（结论记录在 OpenSpec 变更的 design.md）；未在 macOS 上用真实 Codex 验收。
- 一致性：175 个故事 ID 唯一；未重新核验其余故事能力。

本次局部核对（2026-11-01）：#222 把 agent 差异收拢到 `internal/agentprofile` 的适配器接口与每个 agent 一个子包，对应更新 US-Q01（不支持原因与勾选框说明列出服务端返回的受支持 agent）、US-Q03（强制切换损失说明由服务端下发）与 US-L05（agent 名称守卫）。未重新核验其余故事能力。

- 依据：`internal/agentprofile/...` 的适配器契约测试与命令解析测试、`internal/archtest`、`internal/hybrid` 与 `internal/api` 的 Go 测试、重录的线上快照，以及前端单元与浏览器组件测试。

本次局部核对（2026-11-01）：#227 重连期间恢复“正在恢复…”提示，对应更新 US-D03 与 `terminal-stream-resume` 规格。可续接终端在重连时显示不遮挡内容的提示，连接恢复后消失，已有内容保持可见。

- 通过：定向 `TerminalReplay` 与 `TerminalViewport` 浏览器用例、类型检查、测试类型检查、Oxlint、Oxfmt、前端架构守卫与文档治理。
- 未重新核验其余故事能力。

本次局部核对（2026-11-01）：`AGENTS.md` 体积预算对应新增 US-L11；`AGENTS.md` 由约 22 KB 压缩到 10 KiB 以内，频率较低的规则原文迁入 `docs/agent-rules/`。

- 依据：`go test -count=1 ./internal/archtest -run TestAgents` 与 `go test -count=1 ./internal/docguard`。
- 一致性：177 个故事 ID 唯一，两份索引已同步 US-L11；未重新核验其余故事能力。

本次局部核对（2026-11-01）：调整尺寸重绘风暴对应 US-D01；新增文字与 Kitty 图片混合的整屏重绘模拟程序与 4x CPU 性能用例。

- 依据：`go test -race ./internal/hub ./internal/api`（新增突发输出合并回归：200 块由 200 帧降为约 16 帧）、`kitty-graphics` 单元与 `TerminalGraphics` 浏览器用例。
- 本机性能用例（DOM 渲染器）：每次风暴 WebSocket 消息 113→13，最大帧间隔 1.7 s→0.18 s，时长 4.6 s→约 2.5 s；未测 WebGL；未重新核验其余故事能力。

本次局部核对（2026-11-01）：#239 恢复时闪屏对应 US-D01、US-D03（同步输出与回放期间文字与 Kitty 图片同帧切换）。

- 依据：新增 `TerminalSyncOutput` 浏览器用例（同步重绘、滚动重绘、快照回放、同步期间 resize 与焦点变化、快照停在图片上传中），修复前均失败；全部浏览器与单元用例、终端相关 E2E 通过。
- 本机重绘风暴性能用例（DOM 渲染器、4x CPU、同步输出）：可见重绘帧 37→2–3，长任务 2.3 s→0.33 s；未测 WebGL；未重新核验其余故事能力。

本次局部核对（2026-11-02）：聊天回合由服务端记录开始与完成时间，进行中和完成后的耗时跨刷新、重连与设备保持一致，对应更新 US-Q05；不新增或重编号故事；文档守卫不再豁免表格行，对应更新 US-L08，故事表格改为列表，ID 与验收内容不变；超限整理新增“重构而不是压缩”原则。

本次局部核对（2026-11-02）：单文件阅读迁移到 `@pierre/diffs` 只读 `File`/`CodeView`，与 Diff 共用渲染器、主题和高亮 worker；复核 US-I04、US-I05、US-I09、US-I13，并更新 US-I04、US-I05 的验收与来源。

- 已运行：svelte-check、前端单元与浏览器测试、`check:arch`、`codeview`/`code-popout`/`code-documents`/`compact-chrome` E2E 及大型代码文件性能用例。
- 范围限制：未重新核对其余功能组。

本次局部核对（2026-11-02）：统一会话创建入口、当前仓库默认目标、工作树独立创建及分屏完整表单，对应 US-B01、US-B03、US-B06、US-D13、US-J03、US-J06；验证范围见本次 PR，不代表重新核对其余故事。

评审补充（2026-11-02）：同步使用指南与工作区创建规范；分屏中新建工作树后启动失败，切换仓库仍可重选并重试，对应 US-D13。定向验证见 PR，未重新核对其余故事。

测试补充（2026-11-02）：修复代码根识别被取消后缓存普通目录的问题，对应 US-I13；同时完善弹出窗口与终端输入用例的同步条件。验证范围见 PR，未重新核对其余故事。

本次局部核对（2026-11-02）：`add-claude-code-hybrid-profile` 的 Claude Code 混合档案对应更新 US-Q01、US-Q03、US-Q04、US-Q06–US-Q08、US-Q10、US-Q12、US-Q14–US-Q16；不新增或重编号故事，覆盖索引同步。

- 内容：新安装默认混合、Node 22+ 与 `claude-agent-acp`、现有认证复用、进程注册身份与候选、外部占用、配置恢复、插话/附件及后台进程边界。
- 依据：变更 design/delta specs 中固定的 Linux 原生 Claude Code 2.1.316、连接器 0.99.1 实测基线，以及当前适配器与身份实现；本次未重新运行真实双向验收。
- 限制：npm 启动器、macOS 与订阅登录不属于该实测矩阵；未重新核对其余故事能力。

本次局部核对（2026-11-02）：选择 Bash / Zsh 启动文件与只读 `defaultShell` 对应 US-B01、US-B08；后端定向 Go race 测试覆盖 Bash/Zsh 包装、恢复、ACP fd 3 与连接器检查，前端由 `launchConfigs` 单元测试与启动配置对话框浏览器测试覆盖；未重新核对其余故事。

本次局部核对（2026-11-02）：可恢复混合会话持久化并展示当前会话标题，旧记录回退到创建名称，对应 US-Q12，不新增故事或改变覆盖索引。

- 通过：标题持久化与恢复、API/wire Go race、线上快照、Zod 契约及可恢复分组浏览器测试。
- 其余混合会话 Go race 在排除 `TestClaudeRoundTrip` 后通过；该用例在未修改的 HEAD 同样失败。未重新核对其余故事。

本次局部核对（2026-11-02）：Windows 安装程序对应新增 US-O12，并更新 US-O01（本地缓存与预置服务端离线安装）与 US-O04（安装程序登记登录启动、`webmux-tray.exe quit`）；覆盖索引同步。
核对依据为 `internal/tray` 的 Go race 测试与在 Windows 11 上实际运行的 `windows-installer-smoke.ps1`（静默安装、`quit`、运行中升级、拒绝其他位置的托盘、架构不符、卸载）；178 个故事 ID 唯一，O 组来源路径有效；未用真实 WSL 发行版验收托盘菜单离线安装，未重新核验其余故事能力。

本次局部核对（2026-11-02）：Linux deb / rpm 安装包对应新增 US-K13，覆盖索引同步。

- 依据：`internal/service` 的 Go 测试（包内单元的状态、卸载与无单元错误），以及 `test-linux-packages.sh` 在 Debian 与 Fedora 容器中的安装、检查与卸载。
- 实测：在本机 systemd 用户管理器上把同内容单元放在非 `~/.config` 的用户单元目录，`webmux service status|restart|stop|start|uninstall` 均作用于该单元，卸载后文件保留。
- 实测：在以 systemd 为 PID 1 的 Debian 与 Fedora 容器中为普通用户启用 linger 后安装 0.0.1 包。
  - 安装后服务保持未启用；`systemctl --user enable --now webmux` 启动并生成配置。
  - 升级到 0.0.2 时服务进程不变，`webmux service restart` 后运行新版本。
  - `webmux service uninstall` 禁用并停止且保留单元文件；卸载包后两项文件删除、用户配置保留。
- 限制：未验证 arm64 包的安装；未重新核验其余故事能力。

本次局部核对（2026-11-02）：文档整理，不改变任何故事的内容，故事 ID 与验收内容不变，未重新核对其余功能组。

- 迁移：`docs/user-stories.md` 中的核对记录原文迁入本目录的三个核对记录文件，索引只保留产品边界、功能索引与分组表；`docs/deployment.md` 按主题拆到 `docs/deployment/`，章节标题与锚点不变，外部链接已更新。
- 依据：对应 US-L08 新增的“超限时重构而不是压缩”整理原则。
- 验证：已运行 `make docs-check`、Oxfmt 检查、迁移前后的逐行对账与相对链接/锚点检查。

本次局部核对（2026-11-02）：文档导航治理与 README 重写，对应新增 US-L12，覆盖索引与功能索引同步。

- 迁移：README 中的设计理念、完整特性、混合会话说明、截图生成说明与技术栈原文迁入 `docs/overview.md`，常见问题迁入 `docs/faq.md`。
  - 手动下载步骤迁入 `docs/deployment/install-and-upgrade.md#手动下载`，从源码构建迁入 `CONTRIBUTING.md#从源码构建`。
  - 配置项表、服务命令、Windows 托盘步骤与安装脚本变量已由 `docs/deployment/` 完整覆盖，README 中只保留链接。
- 新增：`docs/README.md`（文档中心）与 `docs/specs.md`（64 个 capability 的规格索引），原先不可达的 `docs/performance.md`、`docs/page-performance.md`、`docs/binary-size.md` 与全部主规格由此可达。
- 修正：`docs/persistence.md` 指向已归档变更的失效链接；`docs/cli-agents.md` 与 `docs/usage.md` 中“只有 OpenCode / omp 默认混合”的过时描述，按 `internal/launch/presets.go` 改为 Claude Code、Codex CLI、OpenCode 与 omp。
- 验证：已运行 `make docs-check`（含新增的 `TestDocsNavigation`、`TestNavigationRules`、`TestSlug`）与 Oxfmt 检查；未重新核对其余故事。

本次局部核对（2026-11-02）：README 首屏改版，对应 US-L03（新增 `screenshots:hero` 合成主图）与 US-L12（README 预算不变）。

- 首屏改为合成主图、一句话主张、使用场景叙述、“三步看懂”截图流程与六项亮点；README 仍为 170 行以内。
- 验证：以预装 Chromium 运行 `pnpm --dir web screenshots:hero` 生成 `docs/images/hero.jpg`，并用本地 Markdown 渲染截图目视检查首屏；已运行 `make docs-check` 与 Oxfmt 检查。未在 GitHub 页面上实际渲染。

本次局部核对（2026-11-02）：重新生成 README 截图与主图，截图脚本新增回放模式，对应 US-L03。

- 录制：以真实 omp 18.15.0（Opus 5.5）运行 `WEBMUX_SCREENSHOTS_RECORD=1` 流程，导出两轮对话到 `web/scripts/screenshots/omp-session.jsonl`；文件只含演示仓库内容。
- 回放：在未设置任何 API key 的环境中运行 `pnpm --dir web screenshots`，十张截图全部生成；再运行 `screenshots:hero` 更新 `docs/images/hero.jpg`。
- 发现：经 npm/Bun 安装的 omp 进程名为 `bun`，混合会话判定为 `not_root`，聊天切换不可用；本次用以 `omp` 命名的 Bun 运行时启动同一安装包绕过。产品侧的识别规则未改动。
- 限制：截图在 Linux 容器中以预装 Chromium 生成，未在 macOS 上运行脚本。

本次局部核对（2026-11-03）：自动更新新增 US-K14–US-K16、US-L13、US-O13，并更新 US-K13、US-O01、US-O11、US-O12；功能索引与覆盖索引已同步。发布签名故事最初编号为 US-L12，合并 main 时与先合入的“文档导航与首页”冲突，改为 US-L13；合并后 187 个故事 ID 唯一。

- 依据：`internal/update`、`cmd/webmux`、`cmd/internal/releasesign`、`internal/api`、`internal/tray` 的 Go race 测试与前端 `pendingUpdate` 用例。
- 安装包：`test-linux-packages.sh` 在 Debian/Fedora 容器中覆盖定时器的首次启用、带标记与无标记升级、卸载；systemd 容器中定时器服务把 0.0.1 升到 0.0.2，用户服务 PID 与会话不变。
- 签名：minisign 0.12 验签通过；签名步骤在空模块缓存且 `GOPROXY=off` 时可完成。
- 限制：托盘自更新与 NSIS 静默升级只在 Linux 上测试与编译（makensis 3.11），未在 Windows 上运行；未重新核验其余故事能力。

本次局部核对（2026-11-03）：PR #260 第二轮审查修复，更新 US-K14、US-K15、US-K16、US-O04、US-O12、US-O13，不新增故事，覆盖索引不变。

- 依据：`internal/update`、`internal/fetch`、`internal/tray`、`cmd/webmux` 的 Go race 测试，`internal/tray` 与 `cmd/webmux-tray` 的 Windows amd64/arm64 交叉 vet，`CodeReader` 浏览器测试。
- 覆盖：安装期间取消不终止包管理器、锁文件判定与无法确认时跳过、已安装版本读取失败后重试、镜像取最高版本、托盘失败退避与等待期间关闭自动更新、`quit` 无托盘时退出码 3。
- 限制：未在真实 systemd/SELinux 主机上运行更新服务，未编译或在 Windows 上运行 NSIS 脚本，`windows-installer-smoke.ps1` 未运行；未重新核验其余故事能力。

本次局部核对（2026-11-03）：PR #260 端到端验证，对应 US-K13、US-K14、US-K16、US-O01、US-O12、US-O13，故事内容与覆盖索引不变。均用临时测试密钥签名、本地 HTTP 提供的测试 Release。

- Linux：以 systemd 为 PID 1 的 Debian 13 与 Fedora 44 容器中，更新服务把 deb/rpm 从 0.0.1 升到 0.0.2，另一用户的 `webmux.service` PID 不变。
  - 安装中 `systemctl stop` 等 dpkg 完成后才返回，`dpkg --audit` 为空。
  - `apt-mark hold` 与 dnf5 `versionlock.toml`（仓库不可达）时跳过且不发起请求，锁文件无法解析时按锁定跳过。
- Windows 11（build 26300，NSIS 3.11）：
  - 删除登录启动项后 `/S` 升级仍不登记；安装中止时托盘运行则退出码 4 并恢复、重启 0.0.1 托盘，未运行则退出码 2 且不启动托盘。
  - 安装版 0.0.1 托盘 2 分钟后自更新到 0.0.2，无 SmartScreen/UAC 提示；新托盘把 WSL 中的 webmux 升到 0.0.2，运行中的服务端 PID 不变并提示重启后生效。
  - 托盘触发的安装失败后，重新启动的托盘保留 `updateAttempt` 并提示失败，此后约 3 分钟内的自动检查不再运行安装程序。
- 限制：SELinux enforcing 无法在容器中启用，未验证；未配置托盘转发，转发恢复未单独验证；未重新核验其余故事能力。

本次局部核对（2026-11-03）：补充上一条记录中未验证的两项，对应 US-K14、US-O13，故事内容与覆盖索引不变。

- SELinux：Fedora 44 Cloud 虚拟机（QEMU，enforcing，selinux-policy-targeted 43.3）中更新服务连续把 rpm 从 0.0.1 升到 0.0.4，scriptlet 进入 `rpm_script_t`，没有 AVC 拒绝；加回 `NoNewPrivileges=yes` 时 `nnp_transition` 被拒绝、scriptlet 失败。
- 转发：本机 WSL 为 mirrored 模式，在测试发行版中模拟 NAT 后，托盘自更新期间转发中断约 1.75 秒，新托盘在同一地址与端口恢复转发，服务端 PID 不变。
- 限制：NAT 网络路径由测试替身代替，未在真实 NAT 模式主机上验证；未重新核验其余故事能力。

本次局部核对（2026-11-03）：安装程序可选添加托盘转发的防火墙规则，更新 US-O08、US-O12，不新增故事，覆盖索引不变。

- 依据：`internal/tray` 的规则名称、netsh 参数与 `Firewall.Apply` 单元测试，Windows amd64/arm64 交叉 vet，makensis 3.11 `-WX` 构建。
- 真机（Windows 11 build 26300，非管理员会话）：安装程序冒烟脚本全部通过，静默安装不加规则；交互安装经 UAC 确认后添加入站 TCP 允许规则（域、专用），再次运行不弹窗，静默覆盖后规则保留；交互卸载经 UAC 确认后删除规则。
- 对账（记录托盘路径之后的版本）：首次交互安装经 UAC 添加规则并记录路径；再次安装选项默认勾选、不弹窗；取消勾选后经 UAC 删除规则并清除记录；之后静默卸载无 UAC。
- CI：Windows installer smoke 以管理员运行，`firewall allow` 添加的规则为入站、允许、域与专用网络，重复运行幂等，`remove` 删除规则。
- 限制：未重新核验其余故事能力。

本次局部核对（2026-11-03）：合并 main 后发现 Fedora 45（rpm 6.1，`%_pkgverify_level` 为 `all`）拒绝未签名的 rpm，自动更新改用 `rpm -U --nosignature`，对应 US-K14，故事内容不变。

- 复现：Fedora 45 容器中直接 `rpm -U` 测试包报 “does not verify: no signature”，版本不变。
- 验证：同一容器中 `webmux update apply` 从本地签名的测试 Release 把 0.0.1 升到 0.0.2，`rpm -Va webmux` 无差异；`dnf install` 本地包照常可用。
- 限制：未在 Fedora 45 的 systemd 环境中经定时器运行；未重新核验其余故事能力。

本次局部核对（2026-11-03）：聊天端展示当前 agent 新增 US-Q17，覆盖索引同步，不改变其他故事。

- 依据：`internal/launch` 的档案图标一致性测试、`internal/hybrid` 与 `internal/wire` 的 Go 测试、线上快照与 Zod 契约，以及 `SessionRow` 与 `TerminalHeader` 的浏览器测试。
- 变更作者运行了 `make check`、`make test-fast`（Go race、Vitest 单元与浏览器）、Storybook 与 `hybrid.spec.ts`；本次修复后重跑 `SessionRow` 浏览器用例（14 项通过）与文档治理检查。
- 覆盖：聊天端、休眠与切换中的卡片保留品牌图标和名称，终端端不变，未知图标键只显示名称；聊天页头部的可访问名称、窄屏仅图标与标题截断。
- 限制：截图与手动冒烟使用 fake agent，未连接真实 agent 账号；未重新核验其余故事能力。

本次局部核对（2026-11-03）：新增开发版更新通道（OpenSpec `add-dev-update-channel`），新增 US-K17、US-L14、US-O14，更新 US-K11、US-K15、US-O01、US-O11 与覆盖索引。

- 依据：变更的规格与设计、`internal/update`、`cmd/webmux`、`internal/tray`、发布脚本的实现，以及部署文档。
- 已运行：`make check`、`make test-fast`、`make test-install` 通过；本地 `make release-build VERSION=0.10.3-dev.2` 产出带版本号的全部产物，deb/rpm 包内版本为 `0.10.3~dev.2-1`。
- 已运行（US-K15、US-K17）：Debian 容器中以 root 和本地签名的测试 Release（正式版 0.10.2、开发版 0.10.3-dev.4）执行 `webmux update channel` 与 `update check --json`：切到 dev 后发现 0.10.3-dev.4，未知通道退出码 2，文件 0666 时按 stable 并警告，非 root 写入被拒。
- 已运行（US-K17）：同一测试 Release 上 deb 0.10.2 切到 dev 后 `update apply` 安装 `webmux_0.10.3-dev.4-1_amd64.deb`（`dpkg-query` 报告 `0.10.3~dev.4-1`）；切回 stable 不降级并提示；发布 0.10.3 后升级到 `0.10.3-1`。
- 待验证：未经 systemd 定时器运行；`dev-release.yml` 只经 actionlint 与模拟验证，未在 GitHub 上运行（US-L14）；Windows 托盘未在真实 Windows 上验证（US-O01、US-O11、US-O14）。

本次局部核对（2026-11-03）：默认监听地址改回 `127.0.0.1:7681`、托盘新增“允许局域网访问”开关，非回环监听与托盘转发器接受任意合法请求 `Host`（保留回环监听的 `Host` 白名单防 DNS 重绑定），对应更新 US-A03、US-K02、US-O08，不新增故事，覆盖索引同步。

- 依据：
  - `internal/config`、`internal/client`、`internal/api`（含任意主机名/MagicDNS 同源放行与跨源拦截）的 Go race 测试；
  - `internal/tray/...`（含 `LANEnabled`、`LANListenAddress`、转发器同源校验）与 `cmd/webmux`（含 `TestConfigListen`）的 Go race 测试；
  - `cmd/webmux-tray` 与 `internal/tray/...` 的 Windows amd64 交叉 vet 与编译。
- 限制：未在真实 Windows 11 主机上运行新版托盘菜单点击流程；未重新核验其余故事能力。

本次局部核对（2026-11-03）：`add-antigravity-acp-hybrid` 新增 Antigravity CLI（`agy`）混合会话适配器与默认启动配置，对应更新 US-B01、US-B08、US-Q01、US-Q03–US-Q05、US-Q08、US-Q11；不新增或重编号故事，覆盖索引同步。

- 内容：新安装默认预置 `Antigravity CLI`（`agy --dangerously-skip-permissions`，混合模式，图标 `antigravity`，保留旧 `geminicli` 兼容），`OSC 7501` 子记录段长放宽到 64 字节以提取 36 字符 UUID 会话 ID，跨进程加载后补齐 `model`/`thought_level`，活动回合忽略回显的 `user_message_chunk`。
- 依据：`internal/session`、`internal/agentprofile/...`、`internal/acp`、`internal/hybrid`、`internal/launch`、`internal/api` 的 Go race 测试与线上快照，`launchConfigs` 单元与 `LaunchConfigDialog` 浏览器测试，以及 `baseline.md` 中的 `agy 1.4.12` 实测基线。
- 限制：自动化测试使用 `fakeagent --profile agy` 替身验证；未重新核验其余故事能力。

本次局部核对（2026-11-03）：PR #273 审查回归修复，对应 US-B01、US-B08 与 US-Q03，更新 US-Q03 的首次切换验收，其他故事内容和覆盖索引不变。

- `agy` 首次从终端切到聊天时，将原命令的 `--model` 与 `--effort` 分别继承为 `model` 与 `thought_level`，不再静默回到 ACP 默认值；`--mode` 仍明确列入不支持参数。
- 旧 `geminicli` 图标在编辑对话框中显示“Gemini CLI（旧配置）”，可保存并重新选择；同步补齐 README、概览、部署与索引中的 Antigravity CLI 混合能力列表。
- 依据：`go test -race ./internal/agentprofile/...`；`LaunchConfigDialog` 浏览器测试 22 项、`launchConfigs` 单元测试 57 项；前端类型与架构检查、OpenSpec strict 校验、文档与格式检查均通过。
- 限制：未重新运行完整产品套件，未重新核验其余故事能力。

本次局部核对（2026-11-03）：聊天端会话卡片工作细节对应 US-C02、US-Q09。

- 依据：`internal/hybrid/work.go` 与 `actor.go` 的回合内 `tool.upsert`/`plan.set` 跟踪（回合结束丢弃未完成调用）、`Overlay` 覆盖层与 `session-get-hybrid-work` wire 快照；`SessionRow.svelte` 新增一行“当前工作”（进行中的工具调用或计划步骤，一行截断），移除“等待 Agent 回复…”占位；紧凑列表不显示。
- 验证：`internal/hybrid`（含 fake agent 回合内工作行断言）、`internal/wire`、`internal/api`、`internal/docguard` 的 Go 测试通过，wire 快照已重录；`SessionRow` 浏览器用例（15 项）、svelte-check、Oxlint、前端架构守卫、类型检查与格式检查通过。
- 限制：未运行全量 E2E 与 Storybook Playwright；未重新核验其余故事能力；未做真机验收。

本次局部核对（2026-11-07）：终端视图启动、未命名且带用户指令时以首条 prompt 为会话名（含普通 Shell 命令），聊天端勾选初始化指令时临时标题只取用户输入；更新 US-B01、US-B07、US-Q02、US-Q18、US-S07、US-C15，覆盖索引与故事数量不变。

- 依据：`internal/workspace` 的 `TestCreateSessionTerminalPromptTitle`、`TestCreateSessionChatTitleIgnoresInstruction`，`internal/hybrid` 的 `TestPromptTitle`，使用 fake agent 与 `/bin/cat`。
- 限制：未用真实 agent 验收；未重新核验其余故事能力。

本次局部核对（2026-11-03）：聊天端首条消息临时标题对应 US-Q02，覆盖索引同步，不改变其他故事。

- 依据：`internal/hybrid` 的 `TestPromptTitle`、`TestFirstPromptTitle` 与 `TestFirstPromptTitleSkipped`（已命名、只有附件、由终端切来的会话不设置），使用 fake agent；`hybrid.spec.ts` 新增的临时标题用例通过。
- 限制：未用真实 agent 验收；未重新核验其余故事能力。

本次局部核对（2026-11-03）：`hold-terminal-size-on-keyboard-button` 的软键栏键盘按钮暂时保持终端尺寸对应更新 US-H06。键盘经按钮或系统方式收起且视口高度稳定后才结束保持，保持期间宽度与字号变化照常适配；终端菜单新增“键盘弹出时保持尺寸”开关（默认开启）。

- 通过：`keyboard-size-hold` 单元用例、`TerminalTransport` 浏览器用例、桌面与 Pixel 7 软键 E2E、svelte-check、Oxlint、前端架构守卫与文档治理。
- 限制：Chromium 模拟以缩小视口代替软键盘，未进行 iOS/Android 真机键盘验收；未重新核对其余功能组。

本次局部核对（2026-11-03）：`cache-workspace-on-return` 的返回首页缓存对应更新 US-C01。从终端返回首页时立即显示上次的会话列表、服务端设置与仓库分组，并在后台刷新；缓存仅在当前标签页内存中，令牌失效时清空。

- 通过：`frontend-quality`、`repository-scope`、`workspace-discovery`、`page-stability` 共 40 项 E2E（含挂起 `/api/config` 与 `/api/repositories` 时返回首页仍显示列表且无骨架屏的新用例）、svelte-check、前端架构守卫。
- 限制：用请求挂起模拟慢速链接，未做真机或真实限速网络验收；未重新核对其余功能组。

本次局部核对（2026-11-03）：`default-mobile-ws-compression` 使实时连接压缩在手机与平板（主指针为粗指针）上默认开启，对应更新 US-H10；显式选择优先，桌面默认仍关闭，服务端协商不变。

- 通过：`ws.test.ts` 默认值与显式选择用例，以及前端检查（见 PR）。
- 限制：未做真机移动网络测速；未重新核验其余故事能力。

本次局部核对（2026-11-03）：设置页「新建启动配置」旁新增「添加系统预设」下拉菜单，支持单独补回或一键补回全部缺失的系统预设，对应更新 US-B08，不新增故事或改变覆盖索引。

- 依据：`internal/launch`、`internal/wire`、`internal/api` 的 Go race 测试与线上快照，`wireArrays`、`wire-contract` 单元用例，以及 `LaunchConfigsSettings` 浏览器组件测试。
- 限制：未重新核验其余故事能力。

本次局部核对（2026-11-03）：终端顶部状态提示改为悬浮显示，显示或关闭不再改变终端尺寸，对应更新 US-D05，不新增故事或改变覆盖索引。

- 依据：`pnpm --dir web check`、`check:arch`、`make docs-check`，以及 `openspec validate float-terminal-notices --strict`。
- 限制：未在真机上核对，未重新核验其余故事能力。

本次局部核对（2026-11-04）：PR #291 手机“会话视图”切换统一为同一标签页控件，对应更新 US-H08，不新增故事或改变覆盖索引。普通会话“终端 | 代码”，混合会话“聊天 | 终端 | 代码”；页面加载时不再先显示按钮样式再换成标签页。

- 通过：`make check`；桌面与 Pixel 7 的 compact-chrome、hybrid（含挂起终端流时同一控件随会话信息补上“聊天”的新用例）、composer、lifecycle、quick-commands、split-quick-commands、splits、terminal 与代码相关 E2E；CI 全部测试任务。
- 限制：截图用 Pixel 7 模拟与 fake agent，未做真机验收；未重新核验其余故事能力。

本次局部核对（2026-11-04）：iOS 键盘扩展（如 Typeless 语音输入）一次提交的整段文字完整写入终端一次，不再只剩首字，对应更新 US-H06，不新增故事或改变覆盖索引。

- 依据：`TerminalInput` 浏览器用例重放 WebKit 整段键事件序列（修复前只收到首字、修复后完整一次），前端单元与浏览器全量、`make check`、`check:arch`。
- 限制：事件序列取自公开的 iOS Simulator 实录，未在真机上用 Typeless 核对，未重新核验其余故事能力。

本次局部核对（2026-11-04）：混合会话“恢复到终端”后首页卡片不再丢失 agent 标识；终端端卡片同样显示 agent 品牌小图标与名称，对应更新 US-Q17，不新增故事或改变覆盖索引。

- 通过：`SessionRow` 与 `SessionListRecoverable` 浏览器测试（18 项）、`pnpm --dir web check`、`check:arch`、`make docs-check`。
- 截图：Storybook “Workspace/Hybrid home” 示例数据，桌面 1280px 与手机 390px、明暗主题前后对比。
- 限制：未在真实服务中手动执行“恢复到终端”；未重新核验其余故事能力。

本次局部核对（2026-11-04）：修复聊天端会话卡片无最新消息预览时的底部边距，并使 `HybridInfo.Work` 参与会话显示比对与回合内工具调用跟踪，对应 US-C02、US-Q09，不新增故事或改变覆盖索引。

- 依据：`internal/session`（`TestHybridMetadataParticipatesInDisplayComparison`）与 `internal/hybrid`（`TestWork*`、`TestHybridWorkDetailDuringTurn` 会话列表事件断言）的 Go race 测试；`SessionRow` 浏览器用例（16 项，含无预览卡片底边距断言）、`acp-chat.spec.ts` Storybook 用例、`make check` 通过。
- 限制：未重新核验其余故事能力。

本次局部核对（2026-11-04）：PR #300 会话卡片菜单手动修改标题，新增 US-C15 并登记覆盖索引；普通会话改名不再依赖混合会话记录存储。

- 依据：`internal/api`（`TestSessionRename`、`TestSessionRenameIgnoresUnavailableHybridStore`，修复前 503、修复后 200）、`internal/session`、`internal/hybrid` 的 Go race 测试；`make format-check`、`make docs-check`。
- 限制：未运行完整 `make test`，未在真机上核对；未重新核验其余故事能力。

本次局部核对（2026-11-04）：服务端不再接受 `webmux_token` Cookie 认证，HTTP 只认 Bearer 头，WebSocket 另认 Token 子协议；Origin 检查不变。对应更新 US-A02，不新增故事或改变覆盖索引。

- 依据：`internal/api` Go race 测试（`TestAuthenticationAndOriginBoundaries`、`TestWebSocketHandshakeBoundaries` 的 Cookie 用例由放行改为 401）、`token-link.spec.ts` E2E、`make format-check`、`make docs-check`。
- 限制：未运行完整 `make test`；手动设置 Cookie 的第三方客户端需改用 Bearer 头；未重新核验其余故事能力。

本次局部核对（2026-11-04）：PR #304 新建会话位置列表可删除 linked 工作树，对应更新 US-J08；不新增故事，覆盖索引不变。

- 依据：`NewSessionDialog` 浏览器测试（删除后回退到主工作区，并断言 `onworktreeremoved` 回调携带被删工作树；分屏页面 `Terminal.svelte` 据此剪除本地列表，该剪除未单独测试）、`svelte-check`、`check:arch`、`make docs-check`；隔离演示数据下的桌面/手机、明暗截图见 PR。
- 限制：未运行完整 `make test`；未重新核验其余故事能力。

本次局部核对（2026-11-04）：统一线条设计语言，对应更新 US-L06 的设计系统约定。具体改动：

- 带面板的标签页统一使用 `Tabs variant="line"`。
- 分屏方向、主题、起始端、工作树分支方式、状态筛选、显示方式与 SVG 预览/源码改为基于 RadioGroup 的 `SegmentedControl`（`radiogroup`/`radio`）。
- 开关按钮改为固定 variant 加 `aria-pressed` 描边，Button 移除 `secondary`。

各选项的行为不变，不新增故事，也不改变覆盖索引。

- 依据：
  - `pnpm --dir web check`、`check:test`、`check:storybook`、`lint`、`check:arch`。
  - Vitest 浏览器套件，含新增 `SessionViewControls` 单选不可清空用例；去掉清空保护后该用例失败。
  - Storybook 套件（452 项）。
  - 受影响的 E2E：splits、worktrees、settings-sync、repository-scope、session-creation、workspace-discovery、workspace-ui、hybrid、code-documents。
  - 隔离演示服务在 1280/390/320px 与明暗主题下的截图检查。
- 限制：未运行完整 `make test`，未在真机上核对；未重新核验其余故事能力。

本次局部核对（2026-11-05）：紧急关注提示统一改用 `destructive` 红色语义，对应更新 US-C01、US-C06、US-Q06，不新增故事或改变覆盖索引。

- 内容：`question`（需要回答）、`permission`（等待授权）与 `error`（出错停止）的会话徽章、会话页头部状态圆点、仓库侧栏包含上述等待会话时的 `待 N` 徽章，以及聊天端权限卡片、表单提问卡片与工具调用行“等待授权”均显示红色；`done`、`notify`、`bell` 与启发式 `waiting` 保持琥珀色。
- 依据：`make check`、前端单元全量（53 个文件）、Vitest 浏览器套件全量（60 个文件共 473 项）、Storybook 验证套件（452 项），以及桌面与 Pixel 7 的 `attention`、`hybrid`、`repository-scope` E2E（57 项通过）；隔离演示服务在 1280×720 与 390×844、浅色与深色主题下的前后对比截图。
- 限制：未运行其余未受影响的 E2E 与性能套件，未做 iOS/Android 真机核对；未重新核验其余故事能力。

本次局部核对（2026-11-05）：移除混合会话聊天端空闲休眠能力，使聊天端 ACP 进程在空闲期间保持存活。具体调整：

- 聊天端 ACP 进程仅在切换到终端、显式归档（关闭）会话或服务停止时结束，避免误杀 agent 后台命令、子任务或定时器。
- 将 US-Q11 标记为已退役（保留编号），同步更新 US-Q06、US-Q09、US-Q10、US-Q13、US-Q14、US-Q16、US-Q17、US-M03 以及总目录与覆盖索引。
- 保留旧 `settings.json` 中遗留 `chatIdleSleepMinutes` 字段的向后兼容读取与重写清理。

- 依据：
  - `openspec validate --specs` 以及 `add-antigravity-acp-hybrid`、`show-chat-agent-identity`、`quick-start-chat` 的 `--strict` 校验。
  - `make check`（前端生产构建、格式检查、文档门禁、持久化门禁、测试类型检查、Oxlint、前端架构门禁、`go vet`、`golangci-lint`）。
  - `make test-fast`（全量 Go `-race` 含 `TestChatSideStaysAliveWhileIdleUntilArchived`、`internal/settings` 遗留字段兼容、`internal/hybrid` 崩溃重连恢复与 `internal/api` wire 快照，以及 Vitest 单元与浏览器套件）。
  - 受影响的 Playwright E2E（`hybrid.spec.ts` 含聊天端空闲保活与显式归档清理用例、`settings-sync.spec.ts`）与 Storybook 套件（`contracts.spec.ts`、`acp-chat.spec.ts`）。
  - 隔离演示服务下设置页“启动配置”分区的桌面（1280×800）与手机（390×844）、明暗主题前后对比截图。
- 限制：未运行其余未受影响的 E2E 与性能基准用例，未在真机上核对；未重新核验其余故事能力。

本次局部核对（2026-11-05）：聊天视图支持点击代码位置链接直达代码浏览器，对应更新 US-Q05 与覆盖索引，不新增故事。

- 范围：agent 消息中的本机 `file://` URI、绝对或相对路径（含行范围与标题锚点），以及工具调用与“已浏览 N 项”中的文件位置、`resource_link`，点击后在当前会话的代码浏览器（含独立弹出窗口）中打开文件源码并定位行范围或标题。
- 依据：
  - `make check`（生产构建、类型检查、测试类型检查、Oxlint、前端架构守卫、文档与持久化守卫、`go vet`、`golangci-lint`）与 `make test-frontend`（Vitest 单元与浏览器全量）通过。
  - 新增 `parseChatCodeLink` 单元测试、`CodeMarkdownPreview` / `ChatView` / `CodePanel` 浏览器组件测试（含 `file://` URI、相对路径回退、字面冒号文件名回退与弹出窗口转发）。
  - Storybook `acp-chat.spec.ts`（241 项）与 `design-system.spec.ts`（84 项），以及 Playwright `hybrid.spec.ts` 全量（45 项通过，含桌面 / Pixel 7 代码链接跳转用例）。
  - 隔离演示服务下桌面 1280×720 与手机 390×844、浅色与深色主题前后对比截图。
- 限制：未运行完整 `make test`；截图与 E2E 使用 fake agent，未在 iOS/Android 真机浏览器核对；未重新核验其余故事能力。

本次局部核对（2026-11-05）：实现 OpenSpec 变更 `quick-start-chat`，新增 US-C16、US-C17、US-J12、US-Q18、US-Q19，更新 US-C14、US-C15、US-R 端口转发入口位置与覆盖索引。

- 范围：侧栏外壳与聊天/工作台模式、工作台导航树、聊天历史、侧栏改名、快速开始页与 `POST /api/quick-starts`、仓库初始化指令、按启动配置记忆的 agent 配置项清单。
- 依据：
  - Go：`go test -race` 覆盖 `internal/workspace`、`internal/hybrid`、`internal/statefile`、`internal/api`（含 wire 快照重录）、`internal/archtest`、`./cmd/...`；`go vet` 与 `golangci-lint` 通过。
  - 前端：`svelte-check`、测试类型检查、Oxlint、前端架构守卫通过；Vitest 单元与浏览器全量通过，新增 AppShell、WorkbenchTree、ChatHistory、QuickStart、仓库初始化指令组件测试。
  - Playwright：`quick-start.spec.ts` 桌面（1280×720）与 Pixel 7（390×844）通过；其余 E2E 交由 CI 运行。
- 限制：未在真机上核对，未截取 PR 前后对比截图；E2E 使用 fake agent。

本次局部核对（2026-11-05）：混合会话聊天视图的快捷指令入口从顶栏移到输入框发送按钮旁，对应更新 US-G02。Playwright `hybrid.spec.ts` 快捷指令用例桌面与 Pixel 7 通过；未在真机核对，未重新核验其余故事。

本次局部核对（2026-11-05）：调整混合会话归档文案、聊天历史归档折叠与记录清理联动，对应更新 US-C15、US-G04、US-Q12、US-Q19，不新增故事或改变覆盖索引。

- 内容：
  - 混合会话归档操作文案明确为“停止会话并归档”（确认框为“停止并归档”）。
  - 聊天模式侧栏的已归档记录改为默认折叠的“已归档”分组。
  - 已归档记录的“删除记录”在关联 linked worktree 仍存在时联动打开删除 worktree 确认流程一并清理；工作树不存在或位于主工作区/直接目录时仍确认删除记录。
- 依据：
  - `make check`（生产构建、格式检查、文档与持久化门禁、类型检查、Oxlint、前端架构守卫、`go vet`、`golangci-lint`）通过。
  - `archived.test.ts` 单元用例，以及 `AppShell`、`ArchivedSessions`、`ChatHistory`、`CloseSessionDialog` Vitest 浏览器组件测试通过。
  - Playwright `quick-start.spec.ts` 与 `splits.spec.ts` 受影响 E2E 用例通过，覆盖桌面与 Pixel 7。
  - 隔离演示数据下桌面（1280×720）与手机（390×844）、浅色与深色主题前后对比截图。
- 限制：未运行其余未受影响的 E2E 与性能套件，未在真机上核对；未重新核验其余故事能力。

本次局部核对（2026-11-05）：聊天与快速开始待发送附件栏及对话流用户消息中的图片附件支持小图（缩略图）预览，并新增 `GET /api/sessions/{id}/upload?path=...` 供跨刷新/多设备读取会话上传图片，对应更新 US-Q14、US-Q15 与覆盖索引，不新增故事。

- 范围：上传中的本地图片附件直接复用内存对象 URL 预览；刷新后或其他设备通过会话上传读取接口按路径安全校验（仅限当前会话上传目录内且小于 10 MiB 的普通文件）加载图片，加载中或不可读时平稳降级为图片图标。
- 依据：
  - Go：`go test -race ./internal/artifacts ./internal/api`（新增 `TestReadUpload` 与 `TestReadSessionUpload` 覆盖正常读取、路径逃逸、符号链接、空文件与会话不存在）通过。
  - 前端：`make check`（生产构建、`svelte-check`、测试类型检查、Oxlint、`check:arch`、`docs-check`、`persistence-check`、`go vet`、`golangci-lint`）通过；`ChatUpload`、`ChatView`、`QuickStart` 浏览器测试（37 项）通过。
  - Storybook 与 E2E：`acp-chat.spec.ts`（241 项）与 `hybrid.spec.ts` 图片附件用例（桌面与 Pixel 7）通过；Storybook 隔离示例数据下桌面（1280×720）与手机（390×844）、浅色与深色主题前后对比截图。
- 限制：未运行完整 `make test` 全量套件，未在 iOS/Android 真机浏览器核对；未重新核验其余故事能力。

本次局部核对（2026-11-05）：新增页面内右上角弹窗提醒（Toast）作为默认开启的通知渠道，对应更新 US-C06、US-C12 与覆盖索引，不新增故事。

- 范围：用户使用网站时，新的会话关注信号默认在右上角弹出提醒（状态徽章、会话标题与操作说明），点击直达对应会话 `/s/<ID>`，支持手动关闭、悬停/聚焦暂停、超时自动收起、关注升级替换及关注解除/会话移除时自动清理；设置页“通知推送”卡片提供本机开关（默认开启，无需浏览器通知授权或 HTTPS）。
- 依据：`make check`、`attention-toast` 单元测试、`AttentionToasts`/`AppShell`/`PushSettings` 浏览器组件测试，以及 `attention.spec.ts` E2E 与隔离演示环境下的桌面（1280×720）/手机（390×844）明暗主题截图。
- 限制：未在 iOS/Android 真机核对；未重新核验其余故事能力。

本次局部核对（2026-11-05）：支持 ACP 表单提问的多选 `array` 字段（`items.anyOf` 与 `items.enum`），修复 `agy` 等 agent 发起含多选题的 `elicitation/create` 时在聊天端退化为“该表单需要在终端中完成”的问题，对应更新 US-Q06，不新增故事或改变覆盖索引。

- 依据：
  - `internal/hybrid`（`TestFormRequest` 与 `TestAgyFormElicitationMultiSelect`）、`internal/wire`、`internal/api` 的 Go race 测试。
  - `ChatView` 浏览器组件测试（18 项通过，含多选复选框默认值、必填校验与提交字符串数组）。
  - `make check`（生产构建、类型检查、测试类型检查、Oxlint、前端架构守卫、文档与持久化守卫、`go vet`、`golangci-lint`）通过。
  - 隔离演示服务下桌面（1280×800）与手机（390×844）、浅色与深色主题前后对比截图及浏览器端完整表单提交验证。
- 限制：未运行全量 E2E 套件，未在真机上核对；未重新核验其余故事能力。

本次局部核对（2026-11-05）：代码查看器（`CodeReader`）与 Diff 查看器（`CodeDiffReader`）底部补充常驻横向滚动条，对应更新 US-I05，不新增故事或改变覆盖索引。

- 行为：当源码或未开启自动换行的 Diff 行宽超出可视区域时，在阅读器视口底部显示横向滚动条，并与代码列双向同步 `scrollLeft`；无需滚到文件末尾即可横向拖动，切换到自动换行或无横向溢出时自动隐藏。
- 依据：`CodeReader` 与 `CodeDocument` 浏览器组件测试（含 `N:列` 横向跳转同步、Diff 横向滚动同步与自动换行隐藏断言）、`make check`、`make test-fast`，以及桌面与移动视口明暗主题截图核对。
- 限制：未在 iOS/Android 真机浏览器核对；未重新核验其余故事能力。

本次局部核对（2026-11-05）：新增 `webmux-docs` 文档质量与由浅入深写作技能（`.agents/skills/webmux-docs/SKILL.md`），重组用户文档并同步近期提交能力，对应更新 US-L12 与覆盖索引（不新增故事编号）。

- 范围：
  - 新增 `.agents/skills/webmux-docs/SKILL.md`（及 `.claude/skills/webmux-docs` 软链接）并在 `AGENTS.md` 登记，规定面向网站读者的三层渐进式披露、信息守恒原则、页面模板与提交同步核对清单。
  - 将逼近 400 行上限的 `docs/usage.md` 按由浅入深重构为新手两步上手、侧栏双模式（含快速开始 `/new` 分支搜索、附件、初始化指令、侧栏拖宽）、新建会话四步说明（含单选起始端记忆、首条用户指令与初始化指令），并将 `OSC 7501` 协议与旧版钩子独立拆分为 `docs/usage/attention.md`（保留原锚点与全部细节）。
  - 同步更新 `docs/README.md`、`docs/overview.md`、`docs/hybrid-sessions.md`、`docs/faq.md`、`docs/website.md` 及 `docs/website/*.md`（修正十一款 CLI agent 与七款混合会话 agent 数量）、`CONTRIBUTING.md`（同步移除 macOS CI runner 后的实际流水线描述）。
- 依据：`make format-check`、`make docs-check`（`go test -count=1 ./internal/docguard`）、`go test ./internal/archtest -run TestAgentsGuideSize`、`make check` 通过。
- 限制：本次仅涉及文档与技能说明，未改动运行时代码，未运行完整 `make test` 或真机测试。
