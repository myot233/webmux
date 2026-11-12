// Browser tabs scroll the page header away; installed apps keep its real
// height, including touch-target padding and safe-area insets, above content.
export function observeStickyHeaderOffset(update: (height: number) => void) {
  const header = document.querySelector<HTMLElement>(
    ".safe-area-page > .app-header",
  );
  const standalone = matchMedia("(display-mode: standalone)");
  function sync() {
    update(
      standalone.matches ? (header?.getBoundingClientRect().height ?? 0) : 0,
    );
  }
  const observer = new ResizeObserver(sync);
  if (header) observer.observe(header, { box: "border-box" });
  standalone.addEventListener("change", sync);
  sync();
  return () => {
    observer.disconnect();
    standalone.removeEventListener("change", sync);
  };
}
