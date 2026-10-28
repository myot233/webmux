<script lang="ts">
  import type { ChatUsage } from "../../schemas";

  // Context-window use from the agent's usage updates, with its cost.
  let { usage }: { usage: ChatUsage } = $props();
  const ratio = $derived(
    usage.size > 0 ? Math.min(1, usage.used / usage.size) : 0,
  );
  const percent = $derived(Math.round(ratio * 100));
  const circumference = 2 * Math.PI * 15;
  const kilo = (value: number) =>
    value >= 1000 ? `${(value / 1000).toFixed(1)}k` : String(value);
  const cost = $derived(
    usage.cost === undefined
      ? ""
      : ` · ${usage.currency && usage.currency !== "USD" ? `${usage.currency} ` : "$"}${usage.cost.toFixed(2)}`,
  );
  const label = $derived(
    `上下文已用 ${percent}%（${kilo(usage.used)} / ${kilo(usage.size)}）${cost}`,
  );
</script>

<span
  class="flex shrink-0 items-center gap-1 px-1 text-xs text-muted-foreground tabular-nums"
  role="img"
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
  <span class="max-sm:hidden">{percent}%</span>
</span>
