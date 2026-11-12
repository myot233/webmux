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
};

// Layout scroll events can arrive between ResizeObserver and the next frame.
// Keep follow intent across coalesced composer/keyboard resizes, but never
// undo a reader's input or an explicit jump into history.
export function observeTranscriptResize(options: TranscriptResize) {
  let frame = 0;
  let followEnd = untrack(options.following);
  const manualEvents = ["wheel", "touchmove", "pointerdown", "keydown"];
  const interrupt = () => {
    followEnd = false;
    cancelAnimationFrame(frame);
  };
  for (const type of manualEvents)
    options.viewport.addEventListener(type, interrupt, { passive: true });
  const observer = new ResizeObserver(() => {
    followEnd ||= options.following();
    const target = options.scrollRoot();
    cancelAnimationFrame(frame);
    // Scrolling inside ResizeObserver can loop; perform it in the next frame.
    frame = requestAnimationFrame(() => {
      if (followEnd && !options.jumping() && target === options.scrollRoot())
        options.scrollToBottom();
      followEnd = false;
      options.onResize();
    });
  });
  observer.observe(options.viewport);
  if (options.transcript) observer.observe(options.transcript);
  if (options.dock) observer.observe(options.dock);
  return () => {
    cancelAnimationFrame(frame);
    observer.disconnect();
    for (const type of manualEvents)
      options.viewport.removeEventListener(type, interrupt);
  };
}
