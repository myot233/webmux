<script lang="ts">
  import { Drawer as DrawerPrimitive } from "vaul-svelte";
  import { cn } from "$lib/utils.js";
  import type { WithoutChildrenOrChild } from "$lib/utils.js";
  import DrawerOverlay from "./drawer-overlay.svelte";
  import DrawerPortal from "./drawer-portal.svelte";
  import type { ComponentProps } from "svelte";
  import { onDestroy, tick } from "svelte";
  import { animate } from "motion/mini";
  import { tv } from "tailwind-variants";

  const motionVariants = tv({
    variants: {
      motion: {
        slide: "",
        origin:
          "data-open:animate-none! motion-reduce:animate-none! motion-reduce:transition-none!",
      },
    },
  });

  let {
    ref = $bindable(null),
    class: className,
    portalProps,
    children,
    motion = "slide",
    origin = null,
    onOpenAutoFocus,
    onCloseAutoFocus,
    ...restProps
  }: DrawerPrimitive.ContentProps & {
    portalProps?: WithoutChildrenOrChild<ComponentProps<typeof DrawerPortal>>;
    /** Reveal the sheet from its trigger without scaling its text. */
    motion?: "slide" | "origin";
    origin?: DOMRectReadOnly | null;
  } = $props();

  let reveal: ReturnType<typeof animate> | undefined;
  let contentReveal: ReturnType<typeof animate> | undefined;
  let restoreClip: (() => void) | undefined;
  let revealVersion = 0;

  function cancelReveal() {
    revealVersion += 1;
    reveal?.cancel();
    reveal = undefined;
    contentReveal?.cancel();
    contentReveal = undefined;
    restoreClip?.();
    restoreClip = undefined;
  }

  async function revealFromOrigin() {
    cancelReveal();
    const version = revealVersion;
    // The focus scope mounts before its node ref is propagated through the
    // Drawer wrapper. Wait for that flush, but never revive a dismissed sheet.
    await tick();
    if (
      version !== revealVersion ||
      motion !== "origin" ||
      !ref ||
      ref.dataset.state !== "open" ||
      !origin ||
      window.matchMedia("(prefers-reduced-motion: reduce)").matches
    )
      return;

    const sheet = ref.getBoundingClientRect();
    const button = origin;
    if (!sheet.width || !sheet.height || !button.width || !button.height)
      return;
    // Clamp the source to the sheet: the keyboard or a short viewport can
    // put the trigger outside its final bounds. Never scale form controls.
    const left = Math.max(
      0,
      Math.min(sheet.width - button.width, button.left - sheet.left),
    );
    const top = Math.max(
      0,
      Math.min(sheet.height - button.height, button.top - sheet.top),
    );
    const right = Math.max(0, sheet.width - left - button.width);
    const bottom = Math.max(0, sheet.height - top - button.height);
    const radius = Math.min(button.width, button.height) / 2;
    const corner = getComputedStyle(ref).borderTopLeftRadius;
    const element = ref;
    const previousClip = element.style.clipPath;
    const contents = Array.from(element.children).filter(
      (child): child is HTMLElement => child instanceof HTMLElement,
    );
    const opacity = contents.map((child) => child.style.opacity);
    restoreClip = () => {
      // Motion commits the final keyframe inline. Restore the unclipped
      // surface so later content, focus rings and swipe gestures stay intact.
      element.style.clipPath = previousClip;
      contents.forEach((child, index) => {
        child.style.opacity = opacity[index] ?? "";
      });
    };
    reveal = animate(
      ref,
      {
        clipPath: [
          `inset(${top}px ${right}px ${bottom}px ${left}px round ${radius}px)`,
          `inset(0px 0px 0px 0px round ${corner} ${corner} 0px 0px)`,
        ],
      },
      { duration: 0.32, ease: [0.22, 1, 0.36, 1] },
    );
    contentReveal = animate(
      contents,
      { opacity: [0, 1] },
      { delay: 0.06, duration: 0.18 },
    );
    const current = reveal;
    void current.then(() => {
      if (reveal === current) cancelReveal();
    });
  }

  onDestroy(cancelReveal);
</script>

<DrawerPortal {...portalProps}>
  <DrawerOverlay />
  <DrawerPrimitive.Content
    bind:ref
    data-slot="drawer-content"
    class={cn(
      "bg-popover text-popover-foreground flex min-w-0 h-auto flex-col [overflow-wrap:anywhere] text-sm/relaxed data-[vaul-drawer-direction=bottom]:rounded-t-xl data-[vaul-drawer-direction=bottom]:border-t data-[vaul-drawer-direction=left]:rounded-none data-[vaul-drawer-direction=left]:border-r data-[vaul-drawer-direction=right]:rounded-none data-[vaul-drawer-direction=right]:border-l data-[vaul-drawer-direction=top]:rounded-none data-[vaul-drawer-direction=top]:border-b group/drawer-content fixed z-50 data-[vaul-drawer-direction=bottom]:inset-x-0 data-[vaul-drawer-direction=bottom]:bottom-0 data-[vaul-drawer-direction=bottom]:mt-24 data-[vaul-drawer-direction=bottom]:max-h-[80vh] data-[vaul-drawer-direction=left]:inset-y-0 data-[vaul-drawer-direction=left]:start-0 data-[vaul-drawer-direction=left]:w-3/4 data-[vaul-drawer-direction=right]:inset-y-0 data-[vaul-drawer-direction=right]:end-0 data-[vaul-drawer-direction=right]:w-3/4 data-[vaul-drawer-direction=top]:inset-x-0 data-[vaul-drawer-direction=top]:top-0 data-[vaul-drawer-direction=top]:mb-24 data-[vaul-drawer-direction=top]:max-h-[80vh] data-[vaul-drawer-direction=left]:sm:max-w-sm data-[vaul-drawer-direction=right]:sm:max-w-sm",
      motionVariants({ motion }),
      className,
    )}
    {...restProps}
    onOpenAutoFocus={(event) => {
      void revealFromOrigin();
      onOpenAutoFocus?.(event);
    }}
    onCloseAutoFocus={(event) => {
      cancelReveal();
      onCloseAutoFocus?.(event);
    }}
  >
    <div
      class="bg-muted mx-auto mt-4 hidden h-1 w-10 shrink-0 rounded-none mx-auto hidden shrink-0 group-data-[vaul-drawer-direction=bottom]/drawer-content:block"
    ></div>
    {@render children?.()}
  </DrawerPrimitive.Content>
</DrawerPortal>
