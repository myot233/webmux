<script lang="ts" module>
  import { type VariantProps, tv } from "tailwind-variants";

  // Clickable regions inside lists, tab strips and cards. Unlike Button they
  // take no control sizing, so virtualized rows keep their fixed height; use
  // Button for standalone controls.
  // A file-tree row without its end padding; the variants below add it.
  const fileTree =
    "flex min-h-7 w-full touch-manipulation items-center gap-1.5 rounded-sm py-1 pl-1.5 text-left text-xs outline-none select-none hover:bg-muted focus-visible:ring-1 focus-visible:ring-inset focus-visible:ring-ring aria-[current=page]:bg-accent aria-[current=page]:text-accent-foreground aria-[current=page]:ring-1 aria-[current=page]:ring-inset aria-[current=page]:ring-ring/50 disabled:opacity-50 data-[drop-target=true]:bg-primary/10 data-[drop-target=true]:ring-1 data-[drop-target=true]:ring-inset data-[drop-target=true]:ring-primary max-sm:min-h-(--control-touch-compact) max-sm:text-sm any-pointer-coarse:min-h-(--control-touch-compact) [-webkit-touch-callout:none]";
  const navigation =
    "relative flex min-h-11 w-full min-w-0 items-center gap-2 rounded-md px-3 py-2.5 text-left text-sm font-medium outline-none hover:bg-muted focus-visible:ring-2 focus-visible:ring-ring aria-pressed:bg-primary/10 aria-pressed:text-primary aria-pressed:before:absolute aria-pressed:before:inset-y-3 aria-pressed:before:left-0 aria-pressed:before:w-0.5 aria-pressed:before:rounded-full aria-pressed:before:bg-primary aria-[current=page]:bg-primary/10 aria-[current=page]:text-primary aria-[current=page]:before:absolute aria-[current=page]:before:inset-y-3 aria-[current=page]:before:left-0 aria-[current=page]:before:w-0.5 aria-[current=page]:before:rounded-full aria-[current=page]:before:bg-primary";
  export const listButtonVariants = tv({
    base: "",
    variants: {
      variant: {
        // Inherits the surrounding text, e.g. a breadcrumb segment.
        text: "",
        // A file or change in a navigation list.
        row: "gap-1.5 rounded-sm px-2 py-1 text-left text-xs hover:bg-muted",
        // aria-pressed marks a chosen filter, aria-current the page shown.
        navigation,
        // A navigation row with a file-tree-action button over its end
        // (sidebar chat rows). With a mouse the button appears over the
        // row's trailing mark on hover; touch screens keep room for the
        // always-shown button, and rows without one use it too so their
        // ends line up.
        "navigation-action": `${navigation} any-pointer-coarse:pr-(--control-touch)`,
        // A compact conversation-message tick. Role colors are paired with
        // the accessible label and native tooltip, never color alone.
        turn: "flex h-3 min-h-1.5 w-9 items-center rounded-sm pl-1 outline-none focus-visible:ring-2 focus-visible:ring-ring [&_span]:h-0.5 [&_span]:w-6 [&_span]:origin-left [&_span]:scale-x-50 [&_span]:rounded-full [&_span]:bg-muted-foreground [&_span]:opacity-40 [&_span]:transition-[transform,opacity,background-color] [&_span]:duration-150 [&_span]:ease-out hover:[&_span]:scale-x-75 hover:[&_span]:opacity-80 aria-[current=location]:[&_span]:scale-x-75 aria-[current=location]:[&_span]:opacity-100 data-[turn-role=user]:[&_span]:bg-foreground data-[turn-role=agent]:[&_span]:bg-info motion-reduce:[&_span]:transition-none",
        // Tree rows follow the code panel's compact touch target on touch
        // screens and narrow windows; a long press must not select text or
        // raise the iOS link callout. data-drop-target marks the folder that
        // receives files dragged over the tree.
        "file-tree": `${fileTree} pr-7 max-sm:pr-(--control-touch-compact) any-pointer-coarse:pr-(--control-touch-compact)`,
        // A file-tree row whose action button appears over its end on
        // hover with a mouse (the sidebar tree), so its trailing mark sits
        // at the edge; touch screens keep room for the always-shown button.
        "file-tree-overlay": `${fileTree} pr-1.5 any-pointer-coarse:pr-(--control-touch-compact)`,
        "file-tree-root":
          "flex min-h-8 w-full touch-manipulation items-center gap-1.5 rounded-sm py-1 pr-7 pl-1.5 text-left text-xs font-semibold outline-none select-none hover:bg-muted focus-visible:ring-1 focus-visible:ring-inset focus-visible:ring-ring data-[drop-target=true]:bg-primary/10 data-[drop-target=true]:ring-1 data-[drop-target=true]:ring-inset data-[drop-target=true]:ring-primary max-sm:min-h-(--control-touch-compact) max-sm:pr-(--control-touch-compact) max-sm:text-sm any-pointer-coarse:min-h-(--control-touch-compact) any-pointer-coarse:pr-(--control-touch-compact) [-webkit-touch-callout:none]",
        // The actions button over the end of a file-tree row: shown while
        // its row is hovered or focused or its menu is open, always on
        // touch screens.
        "file-tree-action":
          "absolute inset-y-0 right-0 flex w-7 items-center justify-center rounded-sm text-muted-foreground opacity-0 outline-none group-focus-within/row:opacity-100 group-hover/row:opacity-100 hover:bg-muted hover:text-foreground focus-visible:opacity-100 focus-visible:ring-1 focus-visible:ring-inset focus-visible:ring-ring aria-expanded:bg-muted aria-expanded:opacity-100 max-sm:w-(--control-touch-compact) any-pointer-coarse:w-(--control-touch-compact) any-pointer-coarse:opacity-100 [&_svg]:size-4",
        // One action of a bottom action sheet; data-destructive marks
        // actions that discard data.
        "action-sheet":
          "flex min-h-(--control-touch) w-full items-center gap-3 rounded-md px-3 text-left text-base outline-none hover:bg-muted focus-visible:ring-2 focus-visible:ring-ring disabled:opacity-50 data-[destructive=true]:text-destructive [&_svg]:size-5 [&_svg]:shrink-0",
        // The body of a list item whose actions sit beside it.
        content:
          "gap-1 text-left outline-none focus-visible:ring-3 focus-visible:ring-ring/50",
        // A selectable card; aria-pressed marks the chosen one.
        option:
          "gap-0.5 rounded-md border px-3 py-2 text-left outline-none hover:bg-accent focus-visible:ring-3 focus-visible:ring-ring/50 aria-pressed:border-primary aria-pressed:bg-accent",
        // A suggestion under an input; aria-selected marks the highlighted one.
        suggestion:
          "gap-2 rounded-sm px-2 py-1.5 text-left text-xs hover:bg-muted aria-selected:bg-muted",
        // A document tab and its close button.
        tab: "px-2 py-1.5 text-xs whitespace-nowrap aria-selected:font-medium aria-selected:text-foreground",
        "tab-close": "px-2",
        // Inline-flowing link and monospace code-location link inside rows.
        link: "rounded-sm text-left break-all text-primary underline underline-offset-2 outline-none hover:opacity-80 focus-visible:ring-2 focus-visible:ring-ring",
        "code-link":
          "rounded-sm text-left font-mono text-xs break-all text-primary underline underline-offset-2 outline-none hover:opacity-80 focus-visible:ring-2 focus-visible:ring-ring",
      },
    },
    defaultVariants: {
      variant: "text",
    },
  });

  export type ListButtonVariant = VariantProps<
    typeof listButtonVariants
  >["variant"];
</script>

<script lang="ts">
  import { cn, type WithElementRef } from "$lib/utils.js";
  import type {
    HTMLAnchorAttributes,
    HTMLButtonAttributes,
  } from "svelte/elements";

  let {
    ref = $bindable(null),
    class: className,
    variant = "text",
    href = undefined,
    type = "button",
    children,
    ...restProps
  }: WithElementRef<HTMLButtonAttributes> &
    WithElementRef<HTMLAnchorAttributes> & {
      variant?: ListButtonVariant;
    } = $props();
</script>

{#if href}
  <a
    bind:this={ref}
    data-slot="list-button"
    class={cn(listButtonVariants({ variant }), className)}
    {href}
    {...restProps}
  >
    {@render children?.()}
  </a>
{:else}
  <button
    bind:this={ref}
    data-slot="list-button"
    class={cn(listButtonVariants({ variant }), className)}
    {type}
    {...restProps}
  >
    {@render children?.()}
  </button>
{/if}
