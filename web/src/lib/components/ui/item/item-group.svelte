<script lang="ts">
  import { cn, type WithElementRef } from "$lib/utils.js";
  import type { HTMLAttributes } from "svelte/elements";

  let {
    ref = $bindable(null),
    class: className,
    children,
    variant = "default",
    ...restProps
  }: WithElementRef<HTMLAttributes<HTMLDivElement>> & {
    variant?: "default" | "panel";
  } = $props();
</script>

<div
  bind:this={ref}
  role="list"
  data-slot="item-group"
  class={cn(
    "group/item-group flex w-full min-w-0 flex-col",
    variant === "panel"
      ? "gap-0 rounded-xl border bg-card p-1 [&>[data-slot=item]:not([hidden])~[data-slot=item]:not([hidden])]:border-t"
      : "gap-4 has-data-[size=sm]:gap-2.5 has-data-[size=xs]:gap-2",
    className,
  )}
  {...restProps}
>
  {@render children?.()}
</div>
