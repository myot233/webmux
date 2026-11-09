import { onMount } from "svelte";
import { animate } from "motion/mini";

// Interpolate presentation only. Counts and ARIA always use the latest report.
export function useUsageMotion(
  element: () => SVGCircleElement | HTMLDivElement | null,
  property: "strokeDasharray" | "width",
  value: () => string,
) {
  let previousNode: Element | null = null;
  let previousValue: string | undefined;
  let stop: (() => void) | undefined;
  function cancel() {
    stop?.();
    stop = undefined;
  }
  $effect.pre(() => {
    const node = element();
    const next = value();
    const changed =
      node === previousNode &&
      previousValue !== undefined &&
      next !== previousValue;
    previousNode = node;
    previousValue = next;
    if (!node || !changed) return;
    const from = getComputedStyle(node)[property];
    cancel();
    if (matchMedia("(prefers-reduced-motion: reduce)").matches) return;
    const original = node.style[property];
    const animation = animate(
      node,
      { [property]: [from, next] },
      { duration: 0.3, ease: [0.22, 1, 0.36, 1] },
    );
    const restore = () => {
      animation.cancel();
      node.style[property] = original;
    };
    stop = restore;
    void animation.then(() => {
      if (stop === restore) cancel();
    });
  });
  onMount(() => {
    const reduced = matchMedia("(prefers-reduced-motion: reduce)");
    reduced.addEventListener("change", cancel);
    window.addEventListener("resize", cancel);
    return () => {
      cancel();
      reduced.removeEventListener("change", cancel);
      window.removeEventListener("resize", cancel);
    };
  });
}
