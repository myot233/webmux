import { onMount, tick } from "svelte";
import { animate } from "motion/mini";

// Animate the actual card height, not a scaled screenshot of its contents:
// text, the caret and touch targets keep their normal size throughout.
export function useComposerMorph(
  getCard: () => HTMLElement | null,
  isCompact: () => boolean,
) {
  let previous: boolean | undefined;
  let revision = 0;
  let cleanup: (() => void) | undefined;

  function cancel() {
    revision += 1;
    cleanup?.();
    cleanup = undefined;
  }

  $effect.pre(() => {
    const compact = isCompact();
    const card = getCard();
    if (!card) return;
    const changed = previous !== undefined && previous !== compact;
    previous = compact;
    if (!changed) return;

    // Measure before Svelte changes the compact classes. On interruption,
    // continue from the current visual height instead of the old endpoint.
    const before = card.getBoundingClientRect();
    const action = card.querySelector<HTMLElement>("[data-composer-action]");
    const actionBefore = action?.getBoundingClientRect();
    const start = before.height;
    cancel();
    const current = revision;
    void tick().then(() => {
      if (
        current !== revision ||
        !card.isConnected ||
        window.matchMedia("(prefers-reduced-motion: reduce)").matches
      )
        return;
      const after = card.getBoundingClientRect();
      const end = after.height;
      if (Math.abs(start - end) < 1) return;
      const height = card.style.height;
      const overflow = card.style.overflow;
      const transform = action?.style.transform ?? "";
      const actionAfter = action?.getBoundingClientRect();
      card.style.overflow = "clip";
      const animation = animate(
        card,
        { height: [`${start}px`, `${end}px`] },
        { duration: 0.26, ease: [0.22, 1, 0.36, 1] },
      );
      const actionAnimation =
        action && actionBefore && actionAfter
          ? animate(
              action,
              {
                transform: [
                  `translate(${actionBefore.left - before.left - (actionAfter.left - after.left)}px, ${actionBefore.top - before.top - (actionAfter.top - after.top)}px)`,
                  "translate(0px, 0px)",
                ],
              },
              { duration: 0.26, ease: [0.22, 1, 0.36, 1] },
            )
          : undefined;
      cleanup = () => {
        animation.cancel();
        actionAnimation?.cancel();
        card.style.height = height;
        card.style.overflow = overflow;
        if (action) action.style.transform = transform;
      };
      void animation.then(() => {
        if (revision === current) cancel();
      });
    });
  });

  onMount(() => {
    const reduced = window.matchMedia("(prefers-reduced-motion: reduce)");
    window.addEventListener("resize", cancel);
    reduced.addEventListener("change", cancel);
    return () => {
      cancel();
      window.removeEventListener("resize", cancel);
      reduced.removeEventListener("change", cancel);
    };
  });
}
