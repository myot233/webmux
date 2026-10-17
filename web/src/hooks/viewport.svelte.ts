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
        // Composition updates move the caret every key; snapping the scroll
        // offset back on each of them is the shake. Height still follows the
        // keyboard: Android keyboards compose every word and may hide without
        // ending the composition.
        if (!composing && (window.scrollX || window.scrollY))
          window.scrollTo(0, 0);
        const next = Math.round(viewport?.height ?? window.innerHeight);
        if (next !== height) height = next;
      });
    };
    const start = () => {
      composing = true;
    };
    // A blur or a hidden page can abandon a composition without compositionend.
    const end = () => {
      if (!composing) return;
      composing = false;
      apply();
    };
    const hidden = () => {
      if (document.hidden) end();
    };
    viewport?.addEventListener("resize", apply);
    viewport?.addEventListener("scroll", apply);
    window.addEventListener("resize", apply);
    document.addEventListener("compositionstart", start, true);
    document.addEventListener("compositionend", end, true);
    document.addEventListener("focusout", end, true);
    document.addEventListener("visibilitychange", hidden);
    return () => {
      cancelAnimationFrame(frame);
      root.style.overflow = previousOverflow;
      viewport?.removeEventListener("resize", apply);
      viewport?.removeEventListener("scroll", apply);
      window.removeEventListener("resize", apply);
      document.removeEventListener("compositionstart", start, true);
      document.removeEventListener("compositionend", end, true);
      document.removeEventListener("focusout", end, true);
      document.removeEventListener("visibilitychange", hidden);
    };
  });
  return {
    get height() {
      return height;
    },
  };
}
