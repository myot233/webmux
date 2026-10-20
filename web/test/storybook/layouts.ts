const viewports = [
  { name: "narrow", width: 320, height: 568 },
  { name: "mobile", width: 390, height: 844 },
  { name: "tablet", width: 768, height: 800 },
  { name: "desktop", width: 1280, height: 900 },
  { name: "short", width: 667, height: 320 },
  { name: "large-text", width: 320, height: 568, textScale: 2 },
];

export const layouts = viewports.flatMap(({ name, width, height, textScale }) =>
  (["light", "dark"] as const).map((theme) => ({
    name: `${name}-${theme}`,
    viewport: { width, height },
    theme,
    textScale: textScale ?? 1,
  })),
);
