const terminalFontFamily = "Maple Mono NF CN";

// The generic is sans-serif so a missing glyph cannot fall through to a serif
// CJK face such as SimSun. The two spellings resolve to the same faces; xterm
// only remeasures when the fontFamily string changes.
export const terminalFontFamilyCss = `"${terminalFontFamily}", sans-serif`;
export const terminalFontFamilyAlt = `"${terminalFontFamily}",sans-serif`;

export const terminalLineHeight = 1.15;

export async function loadTerminalFont(size: number): Promise<void> {
  const fonts = document.fonts;
  if (!fonts?.load) return;
  const family = `"${terminalFontFamily}"`;
  const pending = Promise.all(
    [`${size}px ${family}`, `700 ${size}px ${family}`].map((spec) =>
      fonts.load(spec, "A").catch(() => []),
    ),
  );
  await Promise.race([
    pending.then(() => undefined),
    new Promise<void>((resolve) => {
      window.setTimeout(resolve, 3000);
    }),
  ]);
}

// Icon, box-drawing and CJK slices load on first use, after xterm has already
// rasterized those cells. WebKit has no FontFaceSetLoadEvent and may never
// dispatch loadingdone, so each pending terminal face reports its own load.
// Faces are watched once per page and notify the open terminals, so a slice
// that never loads does not retain a closed terminal.
const fontLoadListeners = new Set<() => void>();
const watchedFaces = new WeakSet<FontFace>();
const notifyFontLoad = () => {
  for (const listener of fontLoadListeners) listener();
};

// Calls `listener` whenever another terminal face finishes loading; returns
// the unsubscribe function.
export function onTerminalFontLoad(listener: () => void): () => void {
  document.fonts.forEach((face) => {
    if (face.family !== terminalFontFamily || watchedFaces.has(face)) return;
    watchedFaces.add(face);
    if (face.status === "loaded" || face.status === "error") return;
    face.loaded.then(notifyFontLoad, () => {});
  });
  fontLoadListeners.add(listener);
  return () => {
    fontLoadListeners.delete(listener);
  };
}
