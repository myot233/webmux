# terminal-notice-overlay Specification

## Purpose

终端页面顶部的状态提示悬浮显示，不改变终端区域尺寸，避免显示或关闭提示造成 PTY resize 与全屏程序重绘。

## Requirements

### Requirement: 顶部提示不改变终端尺寸

终端页面顶栏下方的状态提示（包括观察中、上传进度、待复制、暂存更新、混合会话提示、错误、进程已退出与连接器安装提示）SHALL 悬浮在终端区域上方显示，SHALL NOT 占用文档流高度。

提示出现、消失或被用户关闭时，终端宿主尺寸 SHALL NOT 变化，系统 SHALL NOT 因此向 PTY 发送尺寸变化。

#### Scenario: 关闭提示

- **WHEN** 终端顶部显示“知道了”类提示，用户点击关闭
- **THEN** 终端行列与宿主尺寸不变，没有发出尺寸变化，全屏程序不重绘

#### Scenario: 提示出现

- **WHEN** 终端控制端变为观察端或上传开始，顶部出现提示
- **THEN** 提示盖在终端上方，终端行列与宿主尺寸不变

#### Scenario: 无提示

- **WHEN** 没有任何提示
- **THEN** 提示容器不占高度，也不遮挡终端
