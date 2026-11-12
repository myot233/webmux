// Any touch screen raises a virtual keyboard when an input gains focus, even
// when a stylus, keyboard cover or desktop mode makes the primary pointer fine.
// On such devices only the explicit keyboard soft key may focus text input.
// The soft-key bar in styles.css uses the same media query.
export function hasTouchInput(): boolean {
  return matchMedia("(any-pointer: coarse)").matches;
}

// iPhone/iPad, including iPadOS desktop mode (Macintosh UA, touch points).
// Used to stop WebKit from panning the page while an IME composition caret moves.
export function isIOS(): boolean {
  const ua = navigator.userAgent;
  return (
    /iPad|iPhone|iPod/.test(ua) ||
    (navigator.platform === "MacIntel" && navigator.maxTouchPoints > 1)
  );
}
