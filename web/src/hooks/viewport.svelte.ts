import { onMount } from "svelte";

export function useViewport() {
  let height = $state(
    Math.round(window.visualViewport?.height ?? window.innerHeight),
  );
  onMount(() => {
    const viewport = window.visualViewport;
    const root = document.documentElement;
    const previousOverflow = root.style.overflow;
    // A scrollable document lets iOS pan the page to chase the focused caret.
    root.style.overflow = "hidden";
    let composing = false;
    let frame = 0;
    const apply = () => {
      cancelAnimationFrame(frame);
      frame = requestAnimationFrame(() => {
        // Composition updates move the caret every key. Snapping the scroll
        // offset or the page height on each of those events is the shake.
        if (composing) return;
        if (window.scrollX || window.scrollY) window.scrollTo(0, 0);
        const next = Math.round(viewport?.height ?? window.innerHeight);
        if (next !== height) height = next;
      });
    };
    const start = () => {
      composing = true;
    };
    const end = () => {
      composing = false;
      apply();
    };
    viewport?.addEventListener("resize", apply);
    viewport?.addEventListener("scroll", apply);
    window.addEventListener("resize", apply);
    document.addEventListener("compositionstart", start, true);
    document.addEventListener("compositionend", end, true);
    return () => {
      cancelAnimationFrame(frame);
      root.style.overflow = previousOverflow;
      viewport?.removeEventListener("resize", apply);
      viewport?.removeEventListener("scroll", apply);
      window.removeEventListener("resize", apply);
      document.removeEventListener("compositionstart", start, true);
      document.removeEventListener("compositionend", end, true);
    };
  });
  return {
    get height() {
      return height;
    },
  };
}
