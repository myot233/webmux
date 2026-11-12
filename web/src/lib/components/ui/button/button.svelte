<script lang="ts" module>
  import { type VariantProps, tv } from "tailwind-variants";
  import { cn, type WithElementRef } from "$lib/utils.js";
  import type {
    HTMLAnchorAttributes,
    HTMLButtonAttributes,
  } from "svelte/elements";

  // Open, pressed and current buttons gain a line instead of a fill, matching
  // the line tabs; `dark:` repeats it over the outline's dark border. Tailwind
  // scans this file, so the classes stay literal.
  const active =
    "aria-expanded:border-foreground/50 aria-expanded:text-foreground aria-pressed:border-foreground/50 aria-pressed:text-foreground aria-[current=true]:border-foreground/50 aria-[current=true]:text-foreground dark:aria-expanded:border-foreground/50 dark:aria-pressed:border-foreground/50 dark:aria-[current=true]:border-foreground/50";

  export const buttonVariants = tv({
    base: "wm-control focus-visible:border-ring focus-visible:ring-ring/50 aria-invalid:ring-destructive/20 dark:aria-invalid:ring-destructive/40 aria-invalid:border-destructive dark:aria-invalid:border-destructive/50 rounded-md border border-transparent bg-clip-padding text-sm font-medium focus-visible:ring-2 aria-invalid:ring-1 active:not-aria-[haspopup]:translate-y-px [&_svg:not([class*='size-'])]:size-4 group/button inline-flex shrink-0 items-center justify-center whitespace-nowrap transition-colors outline-none select-none disabled:pointer-events-none disabled:opacity-50 [&_svg]:pointer-events-none [&_svg]:shrink-0",
    variants: {
      variant: {
        default: "bg-primary text-primary-foreground hover:bg-primary/80",
        outline: `border-border bg-background hover:bg-muted hover:text-foreground dark:bg-input/30 dark:border-input dark:hover:bg-input/50 ${active}`,
        ghost: `hover:bg-muted hover:text-foreground dark:hover:bg-muted/50 ${active}`,
        destructive:
          "bg-destructive/10 hover:bg-destructive/20 focus-visible:ring-destructive/20 dark:focus-visible:ring-destructive/40 dark:bg-destructive/20 text-destructive focus-visible:border-destructive/40 dark:hover:bg-destructive/30",
        // A destructive action drawn as an outlined text button.
        "destructive-outline":
          "border-destructive/50 bg-background text-destructive hover:bg-destructive/10 focus-visible:ring-destructive/20 dark:border-destructive/50 dark:bg-transparent dark:hover:bg-destructive/20",
        success: "bg-success text-background hover:bg-success/90",
        link: "text-primary underline-offset-4 hover:underline",
        // A ghost button whose resting text recedes until hovered.
        quiet: `text-muted-foreground hover:bg-muted hover:text-foreground dark:hover:bg-muted/50 ${active}`,
        // High-contrast virtual key in the soft-key gamepad.
        hud: `border-foreground/30 bg-card/75 text-foreground font-semibold shadow-sm backdrop-blur-md hover:border-foreground/50 hover:bg-card/90 active:bg-accent/90 dark:border-foreground/35 dark:bg-card/75 dark:hover:bg-card/90 ${active}`,
      },
      size: {
        default:
          "h-(--control-default) gap-2 px-3 has-data-[icon=inline-end]:pr-2 has-data-[icon=inline-start]:pl-2",
        xs: "h-(--control-xs) gap-1 rounded-md px-2 text-xs has-data-[icon=inline-end]:pr-1.5 has-data-[icon=inline-start]:pl-1.5 [&_svg:not([class*='size-'])]:size-3",
        sm: "h-(--control-sm) gap-1.5 rounded-md px-2.5 text-xs has-data-[icon=inline-end]:pr-1.5 has-data-[icon=inline-start]:pl-1.5 [&_svg:not([class*='size-'])]:size-3.5",
        lg: "h-(--control-lg) gap-2 px-4 has-data-[icon=inline-end]:pr-2 has-data-[icon=inline-start]:pl-2",
        icon: "size-(--control-default)",
        "icon-xs":
          "size-(--control-xs) rounded-md [&_svg:not([class*='size-'])]:size-3",
        "icon-sm": "size-(--control-sm) rounded-md",
        "icon-lg": "size-(--control-lg)",
        // Narrow layouts: the touch size below `sm`, the regular size above.
        "icon-touch": "size-(--control-touch) sm:size-(--control-default)",
        // Compact toolbars that keep a full touch target on phones only.
        "icon-touch-sm": "size-(--control-touch) sm:size-(--control-sm)",
        "sm-touch":
          "h-(--control-touch) sm:h-(--control-default) gap-1.5 rounded-md px-4 text-xs has-data-[icon=inline-end]:pr-1.5 has-data-[icon=inline-start]:pl-1.5 [&_svg:not([class*='size-'])]:size-3.5",
      },
      // Floats above content, e.g. an overlay action on a terminal or code.
      raised: {
        true: "border-border bg-background shadow-md dark:bg-background dark:hover:bg-muted",
      },
      // A long text label that wraps instead of overflowing a narrow row
      // (large text on a phone); it keeps its height until it has to wrap.
      wrap: {
        true: "h-auto min-h-(--control-default) max-w-full py-1.5 text-center whitespace-normal",
      },
    },
    defaultVariants: {
      variant: "default",
      size: "default",
    },
  });

  export type ButtonVariant = VariantProps<typeof buttonVariants>["variant"];
  export type ButtonSize = VariantProps<typeof buttonVariants>["size"];

  export type ButtonProps = WithElementRef<HTMLButtonAttributes> &
    WithElementRef<HTMLAnchorAttributes> & {
      variant?: ButtonVariant;
      size?: ButtonSize;
      raised?: boolean;
      wrap?: boolean;
    };
</script>

<script lang="ts">
  let {
    class: className,
    variant = "default",
    size = "default",
    raised = false,
    wrap = false,
    ref = $bindable(null),
    href = undefined,
    type = "button",
    disabled,
    children,
    ...restProps
  }: ButtonProps = $props();
</script>

{#if href}
  <a
    bind:this={ref}
    data-slot="button"
    class={cn(buttonVariants({ variant, size, raised, wrap }), className)}
    href={disabled ? undefined : href}
    aria-disabled={disabled}
    role={disabled ? "link" : undefined}
    tabindex={disabled ? -1 : undefined}
    {...restProps}
  >
    {@render children?.()}
  </a>
{:else}
  <button
    bind:this={ref}
    data-slot="button"
    class={cn(buttonVariants({ variant, size, raised, wrap }), className)}
    {type}
    {disabled}
    {...restProps}
  >
    {@render children?.()}
  </button>
{/if}
