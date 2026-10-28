<script lang="ts">
  import IconCircle from "~icons/tabler/circle";
  import IconCircleCheck from "~icons/tabler/circle-check";
  import IconLoader2 from "~icons/tabler/loader-2";
  import { Button } from "$lib/components/ui/button";
  import type { SwitchStep, SwitchTarget } from "../../hybrid";

  // A switch between the chat and the terminal: the side being left must
  // exit before the other starts, and input stays disabled throughout.
  let {
    target,
    steps,
    awaitingChoice,
    onchoose,
  }: {
    target: SwitchTarget | null;
    steps: SwitchStep[];
    /** The interrupt timed out: the user picks how to go on (D21). */
    awaitingChoice: boolean;
    /** Reopens the choice after its dialog was closed. */
    onchoose: () => void;
  } = $props();
</script>

<div
  class="flex h-full flex-col items-center justify-center gap-4 overflow-y-auto bg-background p-6"
  role="status"
  aria-live="polite"
>
  <IconLoader2
    class="size-8 shrink-0 animate-spin text-muted-foreground"
    aria-hidden="true"
  />
  <h2 class="text-center text-base font-medium">
    {target === "tui"
      ? "正在切换到终端…"
      : target === "acp"
        ? "正在切换到聊天…"
        : "正在切换…"}
  </h2>
  <ol class="w-full max-w-xs space-y-2 text-sm">
    {#each steps as step (step.label)}
      <li
        class={step.state === "pending"
          ? "flex items-center gap-2 text-muted-foreground"
          : "flex items-center gap-2"}
      >
        {#if step.state === "done"}
          <IconCircleCheck
            class="size-4 shrink-0 text-success"
            aria-label="完成"
          />
        {:else if step.state === "current"}
          <IconLoader2
            class="size-4 shrink-0 animate-spin text-info"
            aria-label="进行中"
          />
        {:else}
          <IconCircle class="size-4 shrink-0" aria-label="等待" />
        {/if}
        <span class="min-w-0 break-words">{step.label}</span>
      </li>
    {/each}
  </ol>
  {#if awaitingChoice}
    <p class="max-w-xs text-center text-sm text-warning">
      TUI 仍未停下，需要你决定如何继续。
    </p>
    <Button variant="outline" size="sm" onclick={onchoose}>选择…</Button>
  {/if}
  <p class="max-w-xs text-center text-xs text-muted-foreground">
    切换期间暂停输入；失败时停留在原来的一端并显示原因。
  </p>
</div>
