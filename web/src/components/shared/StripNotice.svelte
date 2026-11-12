<script lang="ts">
  import type { Snippet } from "svelte";
  import { Button } from "$lib/components/ui/button";

  // A one-line status strip above the terminal or chat: a message, optional
  // action buttons and an optional "知道了" dismiss.
  let {
    tone = "info",
    label,
    error = "",
    ondismiss,
    actions,
    children,
  }: {
    tone?: "info" | "warning" | "danger";
    label?: string;
    error?: string;
    ondismiss?: () => void;
    actions?: Snippet;
    children: Snippet;
  } = $props();
</script>

<div
  class={[
    "flex shrink-0 flex-wrap items-center gap-x-2 gap-y-1 px-4 py-1.5 text-sm",
    tone === "danger"
      ? "bg-destructive/10 text-destructive"
      : "border-b bg-muted",
    tone === "warning" && "text-warning",
  ]}
  role={tone === "danger" ? "alert" : "status"}
  aria-label={label}
>
  <div class="min-w-0 flex-1 wrap-anywhere max-sm:basis-full">
    <p>{@render children()}</p>
    {#if error}<p class="text-xs text-destructive" role="alert">{error}</p>{/if}
  </div>
  {@render actions?.()}
  {#if ondismiss}
    <Button variant="ghost" size="sm" onclick={ondismiss}>知道了</Button>
  {/if}
</div>
