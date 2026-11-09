import { onMount, tick } from "svelte";
import { animate } from "motion/mini";

// Watch only the open state and confirmed item IDs, not streaming text.
export function useDockMotion(
  element: () => HTMLElement | null,
  expanded: () => boolean,
  items: () => string = () => "",
) {
  let last: string | undefined;
  let generation = 0;
  let finish: (() => void) | undefined;
  const settle = () => {
    generation++;
    finish?.();
    finish = undefined;
  };
  $effect.pre(() => {
    const node = element();
    const open = expanded();
    const key = `${open}:${items()}`;
    if (!node) return;
    const changed = last !== undefined && last !== key;
    last = key;
    if (!changed) return;
    const height = node.getBoundingClientRect().height;
    const positions = new WeakMap(
      Array.from(node.children, (child) => [
        child,
        child.getBoundingClientRect(),
      ]),
    );
    settle();
    const version = generation;
    void tick().then(() => {
      if (
        version !== generation ||
        !node.isConnected ||
        window.matchMedia("(prefers-reduced-motion: reduce)").matches
      )
        return;
      const target = open ? node.getBoundingClientRect().height : 0;
      node.hidden = false;
      node.style.overflow = "clip";
      const controls = [
        animate(
          node,
          { height: [`${height}px`, `${target}px`] },
          { duration: 0.24, ease: [0.22, 1, 0.36, 1] },
        ),
      ];
      const moved: HTMLElement[] = [];
      if (open && height > 0) {
        for (const child of node.children) {
          const before = positions.get(child);
          if (!(child instanceof HTMLElement) || !before) continue;
          const after = child.getBoundingClientRect();
          const y = before.top - after.top;
          if (Math.abs(y) < 1) continue;
          moved.push(child);
          controls.push(
            animate(
              child,
              { transform: [`translateY(${y}px)`, "translateY(0px)"] },
              { duration: 0.24, ease: [0.22, 1, 0.36, 1] },
            ),
          );
        }
      }
      finish = () => {
        controls.forEach((control) => control.cancel());
        node.style.removeProperty("height");
        node.style.removeProperty("overflow");
        moved.forEach((child) => child.style.removeProperty("transform"));
        node.hidden = !expanded();
      };
      void Promise.all(controls).then(() => {
        if (version === generation) settle();
      });
    });
  });
  onMount(() => {
    const media = matchMedia("(prefers-reduced-motion: reduce)");
    window.addEventListener("resize", settle);
    media.addEventListener("change", settle);
    return () => {
      settle();
      window.removeEventListener("resize", settle);
      media.removeEventListener("change", settle);
    };
  });
}
