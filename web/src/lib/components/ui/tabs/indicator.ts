import { animate } from "motion/mini";

// One indicator moves between selected tabs; it never changes selection.
export function indicator(node: HTMLElement) {
  const list = node.parentElement!;
  const reduced = matchMedia("(prefers-reduced-motion: reduce)");
  let previous = "";
  let cancel: (() => void) | undefined;
  function stop() {
    cancel?.();
    cancel = undefined;
  }
  function update(animateChange = true) {
    const selected = list.querySelector<HTMLElement>(
      "[role=tab][aria-selected=true]",
    );
    if (!selected) {
      stop();
      node.hidden = true;
      previous = "";
      return;
    }
    const parent = list.getBoundingClientRect();
    const box = selected.getBoundingClientRect();
    const left = `${box.left - parent.left}px`;
    const width = `${box.width}px`;
    const next = `${left}:${width}`;
    if (next === previous) return;
    const style = getComputedStyle(node);
    const from = { left: style.left, width: style.width };
    const hadSelection = !!previous;
    stop();
    node.style.setProperty("--indicator-x", left);
    node.style.setProperty("--indicator-width", width);
    node.hidden = false;
    previous = next;
    if (!hadSelection || !animateChange || reduced.matches) return;
    const animation = animate(
      node,
      { left: [from.left, left], width: [from.width, width] },
      { duration: 0.22, ease: [0.22, 1, 0.36, 1] },
    );
    const restore = () => {
      animation.cancel();
      node.style.removeProperty("left");
      node.style.removeProperty("width");
    };
    cancel = restore;
    void animation.then(() => {
      if (cancel === restore) stop();
    });
  }
  const mutation = new MutationObserver(() => update());
  mutation.observe(list, {
    subtree: true,
    attributes: true,
    attributeFilter: ["aria-selected"],
  });
  const resize = new ResizeObserver(() => update(false));
  resize.observe(list);
  update(false);
  reduced.addEventListener("change", stop);
  return {
    destroy() {
      stop();
      mutation.disconnect();
      resize.disconnect();
      reduced.removeEventListener("change", stop);
    },
  };
}
