// Let ordinary mouse wheels reach overflowing toolbar options. Keep native
// horizontal gestures and browser zoom, and release scrolling at either end.
export function horizontalWheel(node: HTMLElement) {
  function wheel(event: WheelEvent) {
    if (event.ctrlKey || event.deltaX || !event.deltaY) return;
    const unit =
      event.deltaMode === WheelEvent.DOM_DELTA_LINE
        ? 16
        : event.deltaMode === WheelEvent.DOM_DELTA_PAGE
          ? node.clientWidth
          : 1;
    const before = node.scrollLeft;
    node.scrollLeft += event.deltaY * unit;
    if (node.scrollLeft !== before) event.preventDefault();
  }
  node.addEventListener("wheel", wheel, { passive: false });
  return {
    destroy() {
      node.removeEventListener("wheel", wheel);
    },
  };
}
