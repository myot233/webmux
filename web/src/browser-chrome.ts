// Tab title and favicon state shared by every page. The favicon is drawn from
// the bundled icon; frames are local data URLs, so animation never hits the
// network. No DOM access at module scope keeps `chromeTitle` unit-testable.

const staticIcon = "/icon.svg";
const runningMarker = "▶ ";
const frameInterval = 500;
const frameCount = 6;
const iconSize = 64;

let running = false;
let title = "";
let frames: string[] | undefined;
let frameIndex = 0;
let timer: ReturnType<typeof setInterval> | undefined;
let loading: Promise<string[]> | undefined;
let motionQuery: MediaQueryList | undefined;
let watchingVisibility = false;

export function chromeTitle(title: string, active: boolean) {
  return active ? `${runningMarker}${title}` : title;
}

export function setBrowserChrome(next: string, active: boolean) {
  title = next;
  running = active;
  document.title = chromeTitle(next, active);
  if (!active) {
    stopTimer();
    restoreIcon();
    return;
  }
  watchMotion();
  watchVisibility();
  if (frames) renderRunningIcon();
  else
    void loadFrames()
      .then((result) => {
        // The page may have left the running state while the icon decoded.
        if (running && result.length) renderRunningIcon();
      })
      .catch(() => undefined);
}

// Stops the running indicator. The title marker is removed only while the
// document still holds the exact marked title this module set, so a page that
// already claimed the title keeps it.
export function resetBrowserChrome() {
  running = false;
  stopTimer();
  restoreIcon();
  if (document.title === chromeTitle(title, true))
    document.title = chromeTitle(title, false);
}

function prefersReducedMotion() {
  return matchMedia("(prefers-reduced-motion: reduce)").matches;
}

function watchMotion() {
  if (motionQuery) return;
  motionQuery = matchMedia("(prefers-reduced-motion: reduce)");
  motionQuery.addEventListener("change", () => {
    if (running) renderRunningIcon();
  });
}

// Both listeners live for the page: this module has one state owner, so they
// only need to be installed once.
function watchVisibility() {
  if (watchingVisibility) return;
  watchingVisibility = true;
  document.addEventListener("visibilitychange", () => {
    if (!running) return;
    if (document.hidden) stopTimer();
    else if (!prefersReducedMotion()) {
      advanceFrame();
      startTimer();
    }
  });
}

function renderRunningIcon() {
  if (!running) return;
  if (prefersReducedMotion()) {
    stopTimer();
    showFrame(0);
    return;
  }
  showFrame(frameIndex);
  startTimer();
}

function startTimer() {
  // Repeated status frames refresh the title but must not postpone the next
  // frame; the interval keeps its own cadence.
  if (timer !== undefined) return;
  if (!running || document.hidden || prefersReducedMotion()) return;
  timer = setInterval(advanceFrame, frameInterval);
}

function stopTimer() {
  if (timer === undefined) return;
  clearInterval(timer);
  timer = undefined;
}

function advanceFrame() {
  frameIndex = (frameIndex + 1) % frameCount;
  showFrame(frameIndex);
}

function iconLink() {
  let link = document.querySelector<HTMLLinkElement>('link[rel="icon"]');
  if (!link) {
    link = document.createElement("link");
    link.rel = "icon";
    document.head.append(link);
  }
  return link;
}

function showFrame(index: number) {
  const frame = frames?.[index % frameCount];
  if (!frame) return;
  const link = iconLink();
  link.href = frame;
  link.type = "image/png";
}

function restoreIcon() {
  const link = iconLink();
  link.href = staticIcon;
  link.type = "image/svg+xml";
}

function loadFrames() {
  if (frames) return Promise.resolve(frames);
  if (loading) return loading;
  loading = drawFrames()
    .then((result) => (frames = result))
    .finally(() => {
      loading = undefined;
    });
  return loading;
}

async function drawFrames() {
  const image = await loadIcon();
  const canvas = document.createElement("canvas");
  canvas.width = iconSize;
  canvas.height = iconSize;
  const context = canvas.getContext("2d");
  if (!context) return [];
  const result: string[] = [];
  for (let frame = 0; frame < frameCount; frame++) {
    context.clearRect(0, 0, iconSize, iconSize);
    context.drawImage(image, 0, 0, iconSize, iconSize);
    drawPulse(context, frame / frameCount);
    result.push(canvas.toDataURL("image/png"));
  }
  return result;
}

function drawPulse(context: CanvasRenderingContext2D, phase: number) {
  const pulse = 0.5 + 0.5 * Math.cos(phase * Math.PI * 2);
  const radius = 3 + 3 * pulse;
  context.save();
  context.globalAlpha = 0.35 + 0.65 * pulse;
  context.fillStyle = "#3fb950";
  context.beginPath();
  context.arc(iconSize - radius - 4, radius + 4, radius, 0, Math.PI * 2);
  context.fill();
  context.restore();
}

function loadIcon() {
  return new Promise<HTMLImageElement>((resolve, reject) => {
    const image = new Image();
    image.addEventListener("load", () => resolve(image), { once: true });
    image.addEventListener("error", () => reject(new Error("icon")), {
      once: true,
    });
    image.src = staticIcon;
  });
}
