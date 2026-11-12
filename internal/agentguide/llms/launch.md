# 启动会话

`webmux run` 是唯一的创建命令，与 Web 端新建会话对话框一样，一次请求完成全部步骤。选项与示例用 `webmux run --help` 查看。

## 命令

- 自定义命令写在 `--` 之后，原样作为 argv，不经 shell 展开；需要 shell 特性时显式写 `bash -lc '...'`。没有 `--` 时出现的位置参数是用法错误。
- 自定义命令继承调用方的环境（终端身份变量与 `WEBMUX_*` 除外）。`--launch-config` 使用 Web 端保存的启动配置，环境由服务端展开，不会经过 CLI。
- `webmux run -- <argv…>` 总是创建普通会话（只有终端视图），即使命令是 agent；要聊天视图必须用开启了聊天视图的启动配置。

## 位置

与 Web 端对话框中的“位置”相同：

- 当前目录：位于已登记 worktree 内时自动关联该 worktree；
- 已有 worktree：`--worktree`；
- 新建 worktree：`--new-worktree` 是开关，分支名用 `--branch` 给出，省略时由服务端自动生成（`wt/<日期>-<时间>`）。
  会话从新 worktree 的根目录开始，新 worktree 像 Web 端一样加入主页显示。

## 视图与首条消息

- 启动配置开启了聊天视图时，可以直接以聊天视图启动（`--view chat`，没有终端可附着，因此总是后台运行）。
- `--prompt` 是首条消息：聊天视图上是第一条提示，终端视图上是输入并回车。`--init` 在它前面加上仓库的初始化指令（见 [仓库与工作区](/llms/workspace.md)），需要关联 worktree。
- 未给 `--name` 时，终端视图的会话以 `--prompt` 的文本（折叠空白，最多 80 个字符，不含初始化指令）为名；没有 `--prompt` 时跟随终端标题，再回退到目录名。
- 聊天视图的 agent 设置、权限模式与附件也在创建时给出；可用的设置项与取值用 `webmux launch-config options` 查询。

## 之后怎样跟进

- 后台运行：输出会话 ID；没有终端时（agent 的工具调用、管道）`run` 自动后台运行。
- 等就绪：`--wait` 等会话第一次进入某个状态；`--reply` 跟踪首条提示的回合并输出 agent 的最终回复，规则与 `send` 的 `--reply` 相同（见 [会话](/llms/sessions.md)）。
- 跟随：`--follow` 把输出写到标准输出并以程序的退出码退出；Ctrl+C 只停止跟随，会话继续运行。
- 失败：按步骤（precheck、worktree、session、config、prompt）报告，已创建的 worktree 与会话保留，不回滚。

启动配置名称可以用 `webmux launch-config ls` 查看。
