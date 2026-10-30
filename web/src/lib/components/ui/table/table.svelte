<script lang="ts" module>
  import { type VariantProps, tv } from "tailwind-variants";

  // `sheet` fills its flex parent and scrolls both ways with a sticky header;
  // cells keep the text's own line breaks (data previews such as CSV).
  export const tableVariants = tv({
    slots: {
      container: "relative w-full",
      table: "w-full caption-bottom text-xs",
    },
    variants: {
      variant: {
        default: { container: "overflow-x-auto" },
        sheet: {
          container: "min-h-0 flex-1 overflow-auto",
          table:
            "[&_td]:whitespace-pre [&_thead]:sticky [&_thead]:top-0 [&_thead]:z-10 [&_thead]:bg-background",
        },
      },
    },
    defaultVariants: { variant: "default" },
  });

  export type TableVariant = VariantProps<typeof tableVariants>["variant"];
</script>

<script lang="ts">
  import { cn, type WithElementRef } from "$lib/utils.js";
  import type { HTMLTableAttributes } from "svelte/elements";

  let {
    ref = $bindable(null),
    class: className,
    variant = "default",
    children,
    ...restProps
  }: WithElementRef<HTMLTableAttributes> & {
    variant?: TableVariant;
  } = $props();
  const styles = $derived(tableVariants({ variant }));
</script>

<div data-slot="table-container" class={styles.container()}>
  <table
    bind:this={ref}
    data-slot="table"
    class={cn(styles.table(), className)}
    {...restProps}
  >
    {@render children?.()}
  </table>
</div>
