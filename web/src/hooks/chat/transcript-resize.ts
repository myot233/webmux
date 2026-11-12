import { untrack } from "svelte";

type TranscriptResize = {
  viewport: HTMLElement;
  transcript: HTMLElement | null;
  dock: HTMLElement | null;
  scrollRoot: () => HTMLElement | null;
  following: () => boolean;
  jumping: () => boolean;
  scrollToBottom: () => void;
  onResize: () => void;
  onManualScroll: () => void;
};

// Layout scroll events can arrive between ResizeObserver and the next frame.
// Keep follow intent across coalesced composer/keyboard resizes, but never
// undo a reader's input or an explicit jump into history.
export function observeTranscriptResize(options: TranscriptResize) {
  let frame = 0;
  let followEnd = untrack(options.following);
  let scrollOwner = untrack(options.scrollRoot);
  const manualEvents = ["wheel", "touchmove", "pointerdown", "keydown"];
  const interrupt = (event?: Event) => {
    if (event) options.onManualScroll();
    followEnd = false;
    cancelAnimationFrame(frame);
  };
  for (const type of manualEvents)
    options.viewport.addEventListener(type, interrupt, { passive: true });
  const observer = new ResizeObserver(() => {
    const target = options.scrollRoot();
    // Document/container handoffs restore their own offset, not the old root's
    // pending follow intent.
    if (target !== scrollOwner) {
      scrollOwner = target;
      followEnd = options.following();
    } else {
      followEnd ||= options.following();
    }
    const top = target?.scrollTop ?? 0;
    const scrollHeight = target?.scrollHeight;
    const clientHeight = target?.clientHeight;
    cancelAnimationFrame(frame);
    // Scrolling inside ResizeObserver can loop; perform it in the next frame.
    frame = requestAnimationFrame(() => {
      const movedUp =
        target &&
        target.scrollTop < top &&
        target.scrollHeight === scrollHeight &&
        target.clientHeight === clientHeight;
      if (
        followEnd &&
        !movedUp &&
        !options.jumping() &&
        target === options.scrollRoot()
      )
        options.scrollToBottom();
      followEnd = false;
      options.onResize();
    });
  });
  observer.observe(options.viewport);
  if (options.transcript) observer.observe(options.transcript);
  if (options.dock) observer.observe(options.dock);
  return {
    interrupt,
    dispose() {
      cancelAnimationFrame(frame);
      observer.disconnect();
      for (const type of manualEvents)
        options.viewport.removeEventListener(type, interrupt);
    },
  };
}
