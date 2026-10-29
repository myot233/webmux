<script lang="ts">
  import IconKeyboard from "~icons/tabler/keyboard";
  import { Button } from "$lib/components/ui/button";
  import * as Dialog from "$lib/components/ui/dialog";

  const groups = [
    {
      title: "窗格管理",
      scope:
        "终端获得焦点时，先按 Ctrl + B，松开后再按下一个键；字母键无需 Shift。",
      shortcuts: [
        { action: "左右分屏 · 新建 Shell", keys: ["Ctrl + B", "%"] },
        { action: "上下分屏 · 新建 Shell", keys: ["Ctrl + B", '"'] },
        { action: "切换到相邻窗格", keys: ["Ctrl + B", "↑ ↓ ← →"] },
        { action: "切换到下一个窗格", keys: ["Ctrl + B", "O"] },
        { action: "放大或恢复当前窗格", keys: ["Ctrl + B", "Z"] },
        { action: "关闭窗格，保留会话", keys: ["Ctrl + B", "X"] },
        { action: "向终端发送 Ctrl + B", keys: ["Ctrl + B", "Ctrl + B"] },
      ],
    },
    {
      title: "终端输入",
      scope:
        "在终端中使用。⌘ 是 macOS 的 Command，Alt 对应 Option；macOS 的 Ctrl+C 始终中断程序。",
      shortcuts: [
        { action: "复制选中文字（Windows / Linux）", keys: ["Ctrl + C"] },
        { action: "复制选中文字（macOS）", keys: ["⌘ + C"] },
        { action: "粘贴文字或文件", keys: ["Ctrl / ⌘ + V"] },
        { action: "通过剪贴板入口粘贴", keys: ["Ctrl / ⌘ + Shift + V"] },
        { action: "向前或向后移动一个单词", keys: ["Alt + ← / →"] },
        {
          action: "向上或向下翻页查看输出",
          keys: ["Shift + PageUp / PageDown"],
        },
      ],
    },
    {
      title: "查找终端输出",
      scope: "在终端或草稿区域打开查找；Enter 和 Esc 在查找栏内生效。",
      shortcuts: [
        { action: "打开查找", keys: ["Ctrl + Shift + F 或 ⌘ + F"] },
        { action: "下一个 / 上一个匹配项", keys: ["Enter / Shift + Enter"] },
        { action: "关闭查找栏", keys: ["Esc"] },
      ],
    },
    {
      title: "代码阅读",
      scope: "仅在代码阅读区或其查找框获得焦点时生效。",
      shortcuts: [
        { action: "在文件内查找", keys: ["Ctrl / ⌘ + F"] },
        { action: "下一个匹配项", keys: ["Ctrl / ⌘ + G 或 F3"] },
        {
          action: "上一个匹配项",
          keys: ["Ctrl / ⌘ + Shift + G 或 Shift + F3"],
        },
        {
          action: "查找框内的下一个 / 上一个",
          keys: ["Enter / Shift + Enter"],
        },
        { action: "跳转到行", keys: ["Ctrl / ⌘ + Alt + G"] },
        { action: "选中下一个相同文本", keys: ["Ctrl / ⌘ + D"] },
        { action: "选中所有相同文本", keys: ["Ctrl / ⌘ + Shift + L"] },
        { action: "关闭查找或跳转面板", keys: ["Esc"] },
        { action: "文件标签：上一个 / 下一个", keys: ["← / →"] },
        { action: "文件标签：第一个 / 最后一个", keys: ["Home / End"] },
        {
          action: "文件树 / 修改列表：移动焦点",
          keys: ["↑ / ↓ 或 Home / End"],
        },
        { action: "文件树：展开 / 折叠或定位父目录", keys: ["→ / ←"] },
      ],
    },
    {
      title: "调整布局",
      scope: "先用 Tab 聚焦对应的分隔条，再按以下键。",
      shortcuts: [
        { action: "窗格分隔条：调整比例 5%", keys: ["← → 或 ↑ ↓"] },
        { action: "窗格分隔条：最小 / 最大比例", keys: ["Home / End"] },
        { action: "窗格分隔条：恢复等分", keys: ["Enter"] },
        { action: "草稿 / 代码面板：调整比例 10%", keys: ["← → 或 ↑ ↓"] },
        {
          action: "草稿 / 代码面板：最小 / 最大比例",
          keys: ["Home / End 或 Shift + 方向键"],
        },
        { action: "草稿 / 代码面板：切换分隔条", keys: ["F6 / Shift + F6"] },
        { action: "代码导航栏：调整宽度 16px", keys: ["← / →"] },
        { action: "代码导航栏：调整宽度 64px", keys: ["Shift + ← / →"] },
        { action: "代码导航栏：最窄 / 最宽", keys: ["Home / End"] },
      ],
    },
    {
      title: "聊天输入",
      scope: "混合会话的聊天输入框；斜杠菜单打开时，Enter 优先选择指令。",
      shortcuts: [
        { action: "发送消息（桌面）", keys: ["Enter"] },
        { action: "换行（触屏上 Enter 也换行）", keys: ["Shift + Enter"] },
        { action: "发送；运行中支持时插入提示", keys: ["Ctrl / ⌘ + Enter"] },
        { action: "斜杠菜单：选择指令", keys: ["↑ / ↓"] },
        { action: "斜杠菜单：填入 / 关闭", keys: ["Enter 或 Tab / Esc"] },
      ],
    },
    {
      title: "列表与弹窗",
      scope: "在会话列表或打开的弹窗中使用。",
      shortcuts: [
        { action: "会话列表：聚焦搜索框", keys: ["/"] },
        { action: "搜索框：清除筛选", keys: ["Esc"] },
        { action: "关闭弹窗或菜单", keys: ["Esc"] },
        { action: "目录选择：返回上级目录", keys: ["Alt + ↑"] },
        { action: "目录选择：填入补全建议", keys: ["Tab"] },
        { action: "聊天授权：输入框外选择对应选项", keys: ["1–9"] },
      ],
    },
  ];
</script>

<Dialog.Root>
  <div class="shrink-0">
    <Dialog.Trigger>
      {#snippet child({ props })}
        <Button
          {...props}
          variant="ghost"
          size="icon-sm"
          aria-label="快捷键"
          title="查看所有快捷键"
        >
          <IconKeyboard class="size-4" aria-hidden="true" />
        </Button>
      {/snippet}
    </Dialog.Trigger>
  </div>
  <Dialog.Content class="flex flex-col gap-0 overflow-hidden p-0 sm:max-w-2xl">
    <div class="shrink-0 border-b p-5 pr-12">
      <Dialog.Header>
        <Dialog.Title>快捷键</Dialog.Title>
        <Dialog.Description
          >WebMux 的键盘操作，按使用场景查阅。</Dialog.Description
        >
      </Dialog.Header>
    </div>
    <!-- svelte-ignore a11y_no_noninteractive_tabindex (Scrollable help must be reachable for keyboard scrolling.) -->
    <div
      class="min-h-0 max-h-[60dvh] overflow-y-auto overscroll-contain px-5"
      tabindex="0"
      role="region"
      aria-label="所有快捷键"
    >
      {#each groups as group (group.title)}
        <section
          class="space-y-3 border-b py-5 last:border-0"
          aria-label={group.title}
        >
          <div class="space-y-1">
            <h3 class="text-sm font-medium">{group.title}</h3>
            <p class="text-xs text-muted-foreground">{group.scope}</p>
          </div>
          <dl class="space-y-3">
            {#each group.shortcuts as shortcut (shortcut.action)}
              <div class="flex items-start justify-between gap-3">
                <dt class="min-w-0 flex-1 text-sm">{shortcut.action}</dt>
                <dd
                  class="flex max-w-[60%] flex-wrap items-center justify-end gap-1 text-right"
                >
                  {#each shortcut.keys as key, index (index)}
                    {#if index > 0}<span class="text-xs text-muted-foreground"
                        >再按</span
                      >{/if}
                    <kbd
                      class="rounded border bg-muted px-1.5 py-0.5 font-mono text-xs"
                      >{key}</kbd
                    >
                  {/each}
                </dd>
              </div>
            {/each}
          </dl>
        </section>
      {/each}
    </div>
    <div
      class="flex shrink-0 flex-col-reverse items-center gap-2 border-t bg-muted/30 px-5 py-3 sm:flex-row sm:justify-between"
    >
      <p class="text-xs text-muted-foreground">
        Shell 和 CLI 的其他键位由当前程序决定；草稿需点击发送。
      </p>
      <Dialog.Close>
        {#snippet child({ props })}
          <Button {...props} variant="outline" size="sm">关闭</Button>
        {/snippet}
      </Dialog.Close>
    </div>
  </Dialog.Content>
</Dialog.Root>
