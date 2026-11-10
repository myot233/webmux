# 功能覆盖核对与后续维护

## 功能覆盖核对

以下索引把可见页面、服务入口与配套工具映射到故事，补充各组故事。底层锁、队列和解析实现归入其支撑的用户行为，不各自包装成独立产品功能。

| 功能入口 / 能力                                                                                                                                  | 对应故事                                            |
| ------------------------------------------------------------------------------------------------------------------------------------------------ | --------------------------------------------------- |
| 登录页与统一品牌标识、浏览器图标、失效认证、连接状态、主机与能力提示、设置页按需加载                                                             | US-A01–US-A04                                       |
| REST API、列表和终端 WebSocket                                                                                                                   | US-A05；US-C02；US-D01–US-D06                       |
| 统一上下文新建会话、十一款 CLI 启动配置、目录浏览与创建、名称和命令                                                                              | US-B01–US-B07                                       |
| 设置页自定义启动配置与图标、启动配置 API 与按配置启动                                                                                            | US-B01；US-B08                                      |
| Claude Code / Codex / Copilot / Antigravity / Qoder / QoderCN 启动前预写文件夹信任记录与 `agent_pretrust` 开关                                   | US-B09；US-K04                                      |
| 混合模式勾选（`chat`）、Claude Code 与 Antigravity CLI 默认混合、`chat-support` 判定、起始视图选择                                               | US-Q01–US-Q02；US-B01；US-B08                       |
| 终端 ↔ 聊天切换、Claude 注册记录身份/候选/外部占用、中断等待、草稿退回与回滚                                                                     | US-Q03–US-Q04                                       |
| 聊天视图时间线、手机浏览器标签页文档级滚动阅读与输入聚焦回退、代码链接打开代码浏览器、权限与表单、权限模式、配置项、命令菜单及 Claude 配置重应用 | US-Q05–US-Q07；US-H06                               |
| 混合聊天输入框找回已发送的提示词文字                                                                                                             | US-Q07                                              |
| 聊天端服务端队列、去重、排队位置、发送模式（auto/steer/refuse/now）与 omp/Codex/Claude 插话                                                      | US-Q08                                              |
| Kimi 默认混合、候选识别与进程级目录信任                                                                                                          | US-B01；US-B08–US-B09；US-Q01；US-Q03–US-Q04        |
| Kimi 配置恢复与通用输入队列                                                                                                                      | US-Q07–US-Q08                                       |
| 聊天附件能力块构造、降级、队列、路径安全与已发送图片小图预览                                                                                     | US-Q15                                              |
| 第三方 ACP 连接器（Codex 的 codex-acp、Claude 的 claude-agent-acp）启动前检查与安装提示                                                          | US-Q16；US-Q01；US-Q03                              |
| 聊天端关注、推送、首条消息临时标题与首页混合卡片                                                                                                 | US-Q09；US-Q02；US-C02                              |
| 聊天端 agent 标识：卡片与会话页头部的品牌图标和显示名                                                                                            | US-Q17                                              |
| 快速开始聊天：新建 worktree、启动聊天端、预选配置、首条消息与快捷指令、窄屏空闲单行收起，幂等与失败报告                                          | US-Q18；US-J12；US-G02；US-G06                      |
| 聊天历史：稳定排序的运行中与已归档聊天、状态、切换与恢复                                                                                         | US-Q19；US-Q12                                      |
| 混合会话异常、自动重连、Claude 现有认证复用与登录引导（US-Q11 已退役）                                                                           | US-Q10；US-D09                                      |
| 混合会话记录、工作树内已归档记录、恢复、Claude 配置重应用，以及删除记录与删除记录和工作树两种操作                                                | US-Q12；US-D10；US-L10                              |
| `run --launch-config`、`attach` 与 `ls` 对混合会话的行为（视图字段与 4409 取值）                                                                 | US-Q13；US-M01；US-M03；US-M05                      |
| 会话内连回所属实例：`WEBMUX_SOCKET`、`WEBMUX_BIN`、`--socket`、`self` 与版本过旧提示                                                             | US-S01                                              |
| 会话详情接口与 `webmux get`、父会话、会话标签与 `ls` 筛选                                                                                        | US-S02–US-S03                                       |
| 命令表、完整 `--help`、`help --json` 与用法错误约定                                                                                              | US-S04；US-M01–US-M07                               |
| llms.txt 形式的 WebMux 工具手册、`webmux guide` 与 `webmux dir`                                                                                  | US-S05                                              |
| `{{webmux}}` 说明接口、初始化指令与快捷指令中的 `{{webmux}}`、预设“WebMux 工具”                                                                  | US-S06；US-G06；US-J12                              |
| `run` 的位置、`--new-worktree` 与 `--view`                                                                                                       | US-S07；US-M01                                      |
| `run` 的 `--json/--wait/--reply/--dry-run/--label`、自动分离与 `--follow`                                                                        | US-S07；US-M01                                      |
| `run --cols/--rows` 的初始终端尺寸（请求字段 `cols`、`rows`）                                                                                    | US-S07；US-D06                                      |
| `run --new-worktree --view chat --prompt --reply` 分派任务与 `launch-config options`                                                             | US-S08；US-Q18                                      |
| 按视图分派的 `send` 与 `interrupt`、`wait`、`logs`、`screen`（含 `--scrollback`）与会话事件                                                      | US-S09–US-S11；US-D05                               |
| `resize` 命令与 `POST /api/sessions/{id}/resize`（脚本改终端尺寸，`--take` 接管控制权）                                                          | US-S19；US-D06                                      |
| `api`、`kill`、`rm`（含 `--forget`）、`rename` 与 shell 补全                                                                                     | US-S11–US-S12                                       |
| `notify` 提醒（来源 `cli`）与清除                                                                                                                | US-S13；US-C06、US-C12                              |
| 聊天视图：`send --reply`、发送模式与排队提示、`chat status/watch/log/answer/set/permission/queue`                                                | US-S14；US-Q01–US-Q19                               |
| `view` 切换视图、`ls --archived` 列出、`resume` 恢复与 `rm --forget` 删除记录                                                                    | US-S15                                              |
| `worktree` 命令与主页显示（`homeWorktrees`）                                                                                                     | US-S16；US-J01–US-J12                               |
| `repo` 命令与仓库设置（初始化指令）                                                                                                              | US-S17；US-J12                                      |
| `launch-config` 增删改与 `forward` 命令                                                                                                          | US-S18；US-R01–US-R04                               |
| 聊天端选择、拖放或粘贴文件，上传进度、取消、待发送附件与图片小图预览                                                                             | US-Q14；US-E04–US-E07                               |
| 首页导航、紧凑/预览、详情、分组、按需表单、增量更新、排序、搜索、筛选与空状态                                                                    | US-C01–US-C05                                       |
| 快速切换会话、跨仓库搜索、激活现有窗格                                                                                                           | US-C18                                              |
| OSC 7501 程序状态、结构化 agent 信号、OSC 通知、BEL 与集成配置                                                                                   | US-C05–US-C06；US-K04                               |
| 页面内右上角弹窗提醒（默认开启）与 Web Push 后台提醒、按设备开关、测试通知与推送文案                                                             | US-C12；US-C06                                      |
| 设置页的预览、草稿与暂存存储、快捷指令                                                                                                           | US-C07；US-F07；US-G01–US-G04                       |
| 设置页桌面侧栏、手机吸顶分区导航、锚点与滚动高亮                                                                                                 | US-K10                                              |
| Git 外部变化、前台恢复与手动刷新                                                                                                                 | US-C08；US-J02                                      |
| 主题切换、跨设备显示偏好与终端/代码区 `Maple Mono NF CN` 中英文等宽字体                                                                          | US-C09；US-D08；US-H04                              |
| 终端输入输出、历史、重连、同步、清屏、全屏、截断后的安全恢复与重绘                                                                               | US-D01–US-D04；US-D07–US-D08                        |
| 终端嵌套分屏、统一新建/加入会话、焦点、放大、比例恢复与关闭窗格                                                                                  | US-D13–US-D14；US-H08                               |
| 终端右上角快捷键帮助、分组键位、生效范围与焦点恢复                                                                                               | US-D15                                              |
| 当前终端缓冲查找、匹配导航、复制、范围与回放提示                                                                                                 | US-D03；US-D11                                      |
| Kitty 终端图片、图形查询、滚动/局部擦除/回放与 omp 预置环境                                                                                      | US-D12；US-B08                                      |
| 控制端/观察端、接管、尺寸与设备应答                                                                                                              | US-D05–US-D06                                       |
| 会话退出、关闭、移除与进程清理                                                                                                                   | US-D09–US-D10                                       |
| 文字复制粘贴、OSC 52、后台待处理复制、权限降级文本框                                                                                             | US-E01–US-E03                                       |
| 文件选择、拖放、粘贴上传、进度、取消与会话临时文件夹存放                                                                                         | US-E04–US-E07                                       |
| 草稿编辑（含完整字符）、输入路由、隐藏、清空与离线保留                                                                                           | US-F01–US-F03；US-F08                               |
| 草稿发送、结果确认、查询、原提交重发和恢复                                                                                                       | US-F04–US-F07                                       |
| 多条暂存编辑、排序、取用与清理                                                                                                                   | US-F09                                              |
| 快捷指令菜单、编辑、排序、删除、系统预设、跨设备同步与临时文件夹                                                                                 | US-G01–US-G06                                       |
| 可插拔 `STDIN/STDOUT` 语音识别（`[voice]`、`/ws/voice`）、聊天与终端双模式录音、实时音量与无回车插入                                             | US-G07；US-K03                                      |
| 软键（含回车与软键盘摇杆模式）、主动键盘、键盘弹出时保持尺寸、触摸/笔、选词、滚动、双指防误触、视口、主屏幕与裁切图标、窄屏紧凑顶栏              | US-H01–US-H08                                       |
| 安装应用入口、安装状态、非安全上下文提示与 Web App Manifest                                                                                      | US-H09；US-K10                                      |
| 实时连接 permessage-deflate 按设备开关（手机默认开启）、静态资源构建期 brotli/gzip 预压缩与编码协商                                              | US-H10；US-H11                                      |
| 代码根目录、边界、文件导航、路径搜索与分页、文件标签焦点激活                                                                                     | US-I01–US-I04；US-I11                               |
| 启动目录、Shell 报告目录、远端标记与复制目录                                                                                                     | US-C04；US-C10                                      |
| 会话链接当前标签页打开（US-C11 已退役）                                                                                                          | US-C14                                              |
| 浏览器标签页运行指示（标题标记、favicon 动画、减少动效）                                                                                         | US-C13                                              |
| 会话卡片菜单与侧栏行手动修改标题、清空恢复自动标题                                                                                               | US-C15                                              |
| 侧栏外壳、聊天/工作台模式切换与记忆、底部全局区、桌面收起与手机抽屉                                                                              | US-C16                                              |
| 工作台导航树：层级、“已不可用工作区”下的已归档记录、跳转、计数、仓库新建/管理菜单与键盘导航                                                      | US-C17；US-J04；US-J14；US-Q12                      |
| 按路径打开文件或目录                                                                                                                             | US-I14                                              |
| 文件内查找、跳转行、行号、Diff 换行与代码字号缩放                                                                                                | US-I05                                              |
| 修改分组/筛选、统一/并排 Diff、上下文与完整文件                                                                                                  | US-I06–US-I08                                       |
| 文件/单行/行范围引用、版本来源与转义                                                                                                             | US-I09                                              |
| 图片识别、预览缩放、SVG 安全、图片 Diff 与下载                                                                                                   | US-I15                                              |
| Markdown 预览、源码切换、安全渲染、工作区链接与图片、标题目录与代码块高亮                                                                        | US-I16                                              |
| HTML 静态预览、CSV/TSV 表格、JSON 树、PDF 识别与预览                                                                                             | US-I17；US-I11                                      |
| 代码栏临时文件夹根、按需创建、浏览、文件操作与随会话清理                                                                                         | US-I18；US-E05                                      |
| 资源管理器右键菜单、上传、拖放上传、下载、目录 ZIP、新建、重命名、复制、移动、删除与重名后缀                                                     | US-I19；US-I18                                      |
| 外部更新、手动刷新、只读限制、快照隔离与过期                                                                                                     | US-I04；US-I10–US-I12                               |
| 仓库登记、工作树状态与主页显示选择                                                                                                               | US-J01–US-J03                                       |
| 侧栏仓库菜单的工作树创建与显式启动、新分支/已有分支、起始提交标题、建议名与路径                                                                  | US-J04–US-J06                                       |
| 工作树会话关联、共享文件与生命周期分离                                                                                                           | US-B06；US-J06–US-J07                               |
| 删除预览、安全删除、强制确认与过期预览；删除后保留已归档记录                                                                                     | US-J08–US-J09；US-Q12                               |
| 工作树批量管理弹窗、未锁定 prunable 失效登记清理、批量预览与部分失败隔离                                                                         | US-J14；US-J03；US-J08–US-J09；US-C17               |
| 已归档工作树释放空间：预览、保留提交与记录、推送检查警告、重建确认与私有引用清理                                                                 | US-J13；US-Q12；US-Q19                              |
| 移除登记、Git/存储/仓库故障降级                                                                                                                  | US-J10–US-J11；US-K08                               |
| 仓库初始化指令：仓库设置中编辑或清空、引用快捷指令、长度上限与快速开始/新建会话勾选                                                              | US-J12；US-Q18；US-B01                              |
| 前台启动、内嵌资源与平台支持                                                                                                                     | US-K01                                              |
| 无服务管理器后台启动、日志与客户端停止                                                                                                           | US-K12                                              |
| 一键安装脚本、平台识别、同源校验与镜像回退                                                                                                       | US-K11                                              |
| Linux deb / rpm 安装包、包内用户单元与服务命令识别                                                                                               | US-K13                                              |
| deb / rpm 自动更新定时器、root 安装、保留停用与不触碰用户服务                                                                                    | US-K14                                              |
| `update check`、签名清单与手动验证发布签名                                                                                                       | US-K15                                              |
| 运行中服务的待生效版本：状态、本机 API、配置响应与 Web UI 提示                                                                                   | US-K16                                              |
| 更新通道：`update channel`、`/etc/webmux/update-channel`、开发版通道与切回不降级                                                                 | US-K17                                              |
| `install.sh` 开发版安装：`WEBMUX_CHANNEL`、当前开发版 `WEBMUX_VERSION`                                                                           | US-K11                                              |
| config init/path/show/validate/check 与启动参数覆盖                                                                                              | US-K02–US-K04                                       |
| 所有 TOML 配置项：访问、会话、提醒、存储                                                                                                         | US-A03；US-K03–US-K04；US-K08–US-K09；US-N01–US-N04 |
| service install/--dry-run/--force/start/stop/restart/status/logs/uninstall                                                                       | US-K05–US-K07                                       |
| version/version --json/help、VPN/HTTPS 代理与升级                                                                                                | US-K09                                              |
| 持久化文件、锁、写入入口与清单防漂移                                                                                                             | US-L10                                              |
| 本地开发、构建/安装、测试、覆盖率报告、格式/静态检查与 Go 规范门禁                                                                               | US-L01–US-L02                                       |
| 文档与 spec 行数、单行长度、体积、目录树治理                                                                                                     | US-L08                                              |
| `AGENTS.md` 体积预算与规则下沉到 `docs/agent-rules/`                                                                                             | US-L11                                              |
| 文档可达性、相对链接与锚点、README 首页预算                                                                                                      | US-L12                                              |
| Storybook、多视口 PR 截图与交互 GIF、baseline、CI 与 release                                                                                     | US-L03–US-L04                                       |
| 发布签名、签名密钥检查、发布前验证与密钥轮换                                                                                                     | US-L13                                              |
| 开发版发布：`dev` Release、版本推导、跳过规则、只保留当前构建与独立签名密钥                                                                      | US-L14                                              |
| 源码文件命名约定、Go 文件名守卫与前端文件名守卫                                                                                                  | US-L15                                              |
| 分层架构测试、线上 JSON 快照、必填数组 Zod 契约与重新录制                                                                                        | US-L05                                              |
| agent 适配器接口、注册表、Claude 进程注册记录与隐私边界、契约测试与 agent 名称守卫                                                               | US-L05；US-Q01；US-Q03                              |
| 前端架构守卫：设计系统 lint、分层、重复代码、未使用代码与基线                                                                                    | US-L06                                              |
| Pullfrog 指定订阅模型与 GitHub Actions 密钥                                                                                                      | US-L07                                              |
| 前端质量基线、类型感知与 Svelte lint、测试类型、wire 契约和失败路径回归                                                                          | US-L09                                              |
| URL token 片段消费与 WebSocket 子协议认证                                                                                                        | US-A06                                              |
| 桌面代码独立窗口、状态保留与收回                                                                                                                 | US-I13                                              |
| run（含 Web 端保存启动配置）、显式环境、attach/observe、输入取消/等待与信号中断重试、分离和控制交接                                              | US-M01–US-M04                                       |
| ls/open/status、版本化诊断、停止实例和 stdio bridge                                                                                              | US-M05–US-M08                                       |
| Windows CLI、发行版优先级和路径/控制台适配                                                                                                       | US-M01–US-M06；US-O05                               |
| Quick/Named Tunnel、token、二维码与依赖下载                                                                                                      | US-N01–US-N04                                       |
| 隧道故障隔离、Web/本机持久开关、运行时应用与紧急关闭                                                                                             | US-N05–US-N06；US-M08                               |
| Web Quick/Named 配置、写入式凭据、恢复默认与配置指南                                                                                             | US-N01–US-N03；US-N07                               |
| 隧道期望/运行状态、动态模式/主机名、存储错误与设置能力                                                                                           | US-N06；US-N08；US-O09；US-O11                      |
| 托盘安装升级（含自动升级较旧服务）、启动方式、管理归属、自动恢复                                                                                 | US-O01–US-O03                                       |
| 托盘登录启动、单实例、发行版、设置、状态与日志                                                                                                   | US-O04–US-O07                                       |
| Windows 远程转发、敏感链接复制、退出托盘                                                                                                         | US-O08–US-O10                                       |
| 托盘与 WSL 服务的兼容、版本对应、降级运行与更新入口                                                                                              | US-O11                                              |
| Windows 安装程序的安装、升级、卸载、静默模式与托盘退出请求                                                                                       | US-O04；US-O12                                      |
| 托盘自动更新：检查调度、安装版识别、静默安装、便携版提示与“检查更新”                                                                             | US-O13                                              |
| 托盘更新通道：菜单与 `updateChannel`、开发版服务端下载、切换到开发版通道并更新托盘                                                               | US-O01；US-O11；US-O14                              |
| Tailscale 检测、节点 DNS、HTTPS 与现有入口只读状态                                                                                               | US-P01                                              |
| Tailscale Serve/Funnel 配置、回读确认、精确移除与公网风险                                                                                        | US-P02–US-P03                                       |
| WSL NAT 阻塞、Windows provider 选择与 operator 提示                                                                                              | US-P04–US-P05                                       |
| Tailscale 所有权、启动对账、失败关闭与来源并存                                                                                                   | US-P06                                              |
| Tailscale 网页设置、立即生效、Funnel 确认与恢复配置文件设置                                                                                      | US-P07                                              |
| 托盘“公网访问”子菜单、一键 Tailscale Serve、未就绪引导与 Windows 节点确认                                                                        | US-P08；US-N06                                      |
| 端口转发网关 `/forward/{port}/`、转发凭据 Cookie 与兑换、上游状态                                                                                | US-R01；US-A02                                      |
| 转发兼容改写（Host、Origin、`X-Forwarded-*`、重定向与 Cookie）和路径前缀模式                                                                     | US-R02                                              |
| 首页端口转发按钮与对话框、转发管理 API、`/ws/sessions` 转发事件                                                                                  | US-R03；US-A05                                      |
| 会话监听端口检测（Linux/macOS）、会话页（终端/聊天视图）端口提示与 30000 及以上高端口默认过滤设置                                                | US-R04；US-R03；US-C02                              |

## 后续维护

每次新增或修改功能时，在对应分组补充或更新故事、验收要点和代码依据，并更新覆盖索引。代码、使用指南与故事不一致时，先核对实际行为，再明确这是实现缺陷、文档错误还是待实现需求；不要把规划中的故事直接标作现有能力。
