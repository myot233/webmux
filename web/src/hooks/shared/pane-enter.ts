import { animate } from "motion/mini";

// The caller supplies actual readiness; never use animation time as progress.
export function paneEnter(node: HTMLElement, shown: boolean) {
  const reduced = matchMedia("(prefers-reduced-motion: reduce)");
  let cleanup: (() => void) | undefined;
  let visible = false;
  function cancel() {
    cleanup?.();
    cleanup = undefined;
  }
  function update(next: boolean) {
    if (next === visible) return;
    visible = next;
    cancel();
    if (!next || reduced.matches) return;
    const original = node.style.opacity;
    const animation = animate(node, { opacity: [0, 1] }, { duration: 0.2 });
    const restore = () => {
      animation.cancel();
      node.style.opacity = original;
    };
    cleanup = restore;
    void animation.then(() => {
      if (cleanup === restore) cancel();
    });
  }
  update(shown);
  reduced.addEventListener("change", cancel);
  return {
    update,
    destroy() {
      cancel();
      reduced.removeEventListener("change", cancel);
    },
  };
}
