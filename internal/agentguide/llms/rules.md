# 操作守则

这些命令以服务用户的身份运行，不是沙箱。请遵守：

- 对全屏 TUI，优先使用聊天视图（`webmux view`、`send` 的 `--reply`、`webmux chat`）；需要看画面时用 `webmux screen`，不要依赖 `logs` 去除控制序列后的文本。
- 交付给用户的文件放进会话临时文件夹（见 [临时文件夹](/llms/artifacts.md)），不要放进项目仓库。
