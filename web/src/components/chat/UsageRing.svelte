<script lang="ts">
  import { Button } from "$lib/components/ui/button";
  import * as Popover from "$lib/components/ui/popover";
  import type { ChatUsage } from "../../schemas";

  // Context-window use from the agent's usage updates, with its cost; a
  // click opens the exact token counts.
  let { usage }: { usage: ChatUsage } = $props();
  const ratio = $derived(
    usage.size > 0 ? Math.min(1, usage.used / usage.size) : 0,
  );
  const percent = $derived(Math.round(ratio * 100));
  const circumference = 2 * Math.PI * 15;
  const kilo = (value: number) =>
    value >= 1000 ? `${(value / 1000).toFixed(1)}k` : String(value);
  const exact = (value: number) => value.toLocaleString("zh-CN");
  const money = $derived(
    usage.cost === undefined
      ? ""
      : `${usage.currency && usage.currency !== "USD" ? `${usage.currency} ` : "$"}${usage.cost.toFixed(2)}`,
  );
  const label = $derived(
    `上下文已用 ${percent}%（${kilo(usage.used)} / ${kilo(usage.size)}）${money ? ` · ${money}` : ""}`,
  );
  const remaining = $derived(Math.max(0, usage.size - usage.used));
  const rows = $derived<Array<[string, string]>>([
    ["已用", `${exact(usage.used)} tokens`],
    ["上下文窗口", usage.size > 0 ? `${exact(usage.size)} tokens` : "未知"],
    ["剩余", usage.size > 0 ? `${exact(remaining)} tokens` : "未知"],
    ...(money ? ([["本会话费用", money]] as Array<[string, string]>) : []),
  ]);
</script>

<Popover.Root>
  <Popover.Trigger>
    {#snippet child({ props })}<Button
        {...props}
        variant="quiet"
        size="xs"
        class="shrink-0"
        aria-label={label}
        title={label}
      >
        <svg
          viewBox="0 0 36 36"
          class="size-4 shrink-0 -rotate-90"
          aria-hidden="true"
        >
          <circle
            cx="18"
            cy="18"
            r="15"
            fill="none"
            class="stroke-muted"
            stroke-width="4"
          />
          <circle
            cx="18"
            cy="18"
            r="15"
            fill="none"
            class={ratio > 0.8 ? "stroke-warning" : "stroke-primary"}
            stroke-width="4"
            stroke-linecap="round"
            stroke-dasharray={`${ratio * circumference} ${circumference}`}
          />
        </svg>
        <span class="tabular-nums max-sm:hidden">{percent}%</span>
      </Button>{/snippet}
  </Popover.Trigger>
  <Popover.Content
    role="dialog"
    aria-label="Token 使用情况"
    align="end"
    side="top"
    collisionPadding={16}
    class="w-64 gap-3"
  >
    <Popover.Header>
      <Popover.Title>Token 使用情况</Popover.Title>
      <Popover.Description>来自 agent 最近一次用量报告</Popover.Description>
    </Popover.Header>
    <div class="space-y-1.5">
      <div class="flex items-baseline justify-between text-xs">
        <span class="text-muted-foreground">上下文</span>
        <span class="font-medium tabular-nums">{percent}%</span>
      </div>
      <div
        class="h-1.5 overflow-hidden rounded-full bg-muted"
        role="progressbar"
        aria-label="上下文已用"
        aria-valuemin={0}
        aria-valuemax={100}
        aria-valuenow={percent}
      >
        <div
          class={[
            "h-full w-(--usage) rounded-full",
            ratio > 0.8 ? "bg-warning" : "bg-primary",
          ]}
          style:--usage={`${percent}%`}
        ></div>
      </div>
    </div>
    <dl class="grid grid-cols-2 gap-x-3 gap-y-1.5 text-xs">
      {#each rows as [name, value] (name)}
        <dt class="text-muted-foreground">{name}</dt>
        <dd class="text-right font-medium tabular-nums">{value}</dd>
      {/each}
    </dl>
  </Popover.Content>
</Popover.Root>
