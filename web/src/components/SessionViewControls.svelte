<script lang="ts">
  import IconList from "~icons/tabler/list";
  import IconTerminal2 from "~icons/tabler/terminal-2";
  import { Button } from "$lib/components/ui/button";
  let {
    compact,
    onchange,
  }: { compact: boolean; onchange: (compact: boolean) => void } = $props();
</script>

<div
  role="group"
  aria-label="会话显示方式"
  class="flex min-w-0 items-center gap-1"
>
  <Button
    variant={compact ? "secondary" : "ghost"}
    size="sm-touch"
    aria-label="紧凑列表"
    title="紧凑列表"
    aria-pressed={compact}
    onclick={() => onchange(true)}
  >
    <IconList aria-hidden="true" /><span class="hidden sm:inline">紧凑</span>
  </Button>
  <Button
    variant={compact ? "ghost" : "secondary"}
    size="sm-touch"
    aria-label="输出预览"
    title="输出预览"
    aria-pressed={!compact}
    onclick={() => onchange(false)}
  >
    <IconTerminal2 aria-hidden="true" /><span class="hidden sm:inline"
      >预览</span
    >
  </Button>
</div>
