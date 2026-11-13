# 常用流程

跨多个命令的完整例子。单个命令的选项见 `webmux <命令> --help`。

## 在新 worktree 中启动 agent 并取回结果

```sh
webmux launch-config ls --json
# 从列表选择获用户许可且支持 chat 的真实名称，设置 CONFIG_NAME；CLI 按名称匹配，不按配置 ID。
: "${CONFIG_NAME:?先设置为列表中已选定的真实配置名称}"
webmux launch-config options "$CONFIG_NAME" --json
# 仅当需要自定义设置且已发现真实设置 ID/允许值时，按 run --help 添加 --set。
webmux run --new-worktree --view chat --launch-config "$CONFIG_NAME" --label run=42 \
  --prompt "修复所有 lint 并汇报改动" --reply --timeout 30m
webmux ls --parent self --label run=42
```

## 继续对话，处理授权请求

```sh
id=$(webmux ls --parent self --label run=42 --json | jq -r '.[0].id')
webmux send "$id" "再补上测试" --reply --timeout 20m
webmux chat status "$id" --json
# 无论成功或失败都以真实状态为准。没有 pending request 就不回答。
# 区分提问与授权，核对当前请求和用户授权范围；从当前请求复制 requestId/optionId。
# 获得范围内授权后才运行：webmux chat answer "$id" <真实请求ID> --option <真实optionId>
```

## 中断并改变方向

```sh
webmux interrupt "$id"
webmux send "$id" "改用较小的修复" --reply
```

## 在终端与聊天之间切换

```sh
webmux view "$id" chat && webmux send "$id" "继续"
webmux view "$id" terminal --force && webmux attach "$id"
```

## 恢复并清理已归档的 agent 会话

```sh
webmux ls --archived --repo .
webmux resume <ID> --view chat --detach
webmux rm <ID> --forget --yes
```

## 为并行任务准备 worktree 并共享开发服务器

```sh
dir=$(webmux worktree create . --branch exp/a --refresh-base)
id=$(webmux run --detach --cwd "$dir" --label task=exp-a -- bash -lc 'npm ci && npm run dev')
webmux forward add 5173 --session "$id" --label exp-a
```

## 长任务结束时提醒用户

```sh
make release && webmux notify --kind done "release 构建完成" || webmux notify --kind error "release 构建失败"
```

## 在持久会话里跑测试并拿到退出码

```sh
webmux run --follow --strip --name tests -- go test ./... > test.log
echo "exit $?"
```

## 驱动一个 shell 会话并读结果

```sh
id=$(webmux run --detach --name build -- bash)
webmux send "$id" 'make build' --enter --wait waiting,idle --timeout 20m
webmux interrupt "$id"           # 卡住时按 Ctrl-C
webmux logs "$id" --strip --tail 30
```

这里只观察状态变化，不证明 make build 已完成或成功；需另外检查命令完成证据和构建产物。

## 看终端里的 agent 是否在等你

```sh
webmux wait "$id" --until attention --changed --timeout 30m
webmux screen "$id"            # 读取当前请求内容和真实选项
# 先确认用户授权范围与当前选中项；没有授权请求或不在授权范围就不要按 Enter。
# 仅确认允许当前操作后，才可用 webmux send "$id" --key enter
```

## 找到并清理自己创建的会话

```sh
webmux ls --parent self --json | jq -r '.[] | select(.status=="exited") | .id' | xargs -n1 webmux rm
```

## 了解自己所在的环境

```sh
webmux get                       # 仓库、分支、启动配置、视图
webmux get --field worktree.path
```

## 找到并汇报交付物

```sh
dir=$(webmux dir self)
cp report.html "$dir/"
echo "报告在临时文件夹的 report.html"
```

## 查看命令的完整用法

```sh
webmux help                      # 全部命令
webmux run --help                # 一个命令的选项、退出码与示例
webmux help --json | jq -r '.commands[].usage'
```
