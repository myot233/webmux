<script lang="ts">
  import type { Snippet } from "svelte";

  let {
    label,
    alignEnd = false,
    children,
  }: { label: string; alignEnd?: boolean; children: Snippet } = $props();
</script>

<div
  class={[
    "-my-1 flex min-w-0 items-center gap-0.5 overflow-x-auto overscroll-x-contain px-1 py-1 *:shrink-0 sm:[scrollbar-width:none]",
    "max-sm:[&>button]:max-w-[min(12rem,100%)] max-sm:[&>button]:px-1 max-sm:[&_[data-option-chevron]]:hidden",
    alignEnd && "sm:[&>:first-child]:ms-auto",
  ]}
  role="toolbar"
  aria-label={label}
  data-chat-options
>
  {@render children()}
</div>
