<script lang="ts">
  import IconLayoutDashboard from "~icons/tabler/layout-dashboard";
  import IconMessageCircle from "~icons/tabler/message-circle";
  import { Button } from "$lib/components/ui/button";
  import type { ShellMode } from "../../hooks/shared/shell.svelte";

  // The two mode buttons at the top of the sidebar. Each keeps its variant
  // and only toggles aria-pressed; focus stays on the pressed button.
  let {
    mode,
    onchange,
  }: {
    mode: ShellMode;
    onchange: (mode: ShellMode) => void;
  } = $props();
</script>

<div class="flex shrink-0 items-center" role="group" aria-label="模式">
  <Button
    variant="ghost"
    size="icon-touch"
    aria-label="聊天模式"
    aria-pressed={mode === "chat"}
    title="聊天模式：快速开始与聊天历史"
    onclick={() => onchange("chat")}
    ><IconMessageCircle aria-hidden="true" /></Button
  >
  <Button
    variant="ghost"
    size="icon-touch"
    aria-label="工作台模式"
    aria-pressed={mode === "workbench"}
    title="工作台模式：仓库、worktree 与全部会话"
    onclick={() => onchange("workbench")}
    ><IconLayoutDashboard aria-hidden="true" /></Button
  >
</div>
