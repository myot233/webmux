export function sessionReference(title: string, id: string) {
  const label = title.replace(/[\[\]\\\r\n]/g, " ").trim() || id;
  return `[${label}](${new URL(`/s/${id}`, location.origin).href})`;
}

/** Only same-origin session URLs become app navigation in plain user text. */
export function sessionReferenceParts(text: string) {
  const parts: { text: string; href?: string }[] = [];
  let start = 0;
  for (const match of text.matchAll(
    /\[([^\]\n]+)\]\((https?:\/\/[^)\s]+)\)/g,
  )) {
    let url: URL;
    try {
      url = new URL(match[2] ?? "");
    } catch {
      continue;
    }
    if (
      url.origin !== location.origin ||
      url.username ||
      url.password ||
      url.search ||
      url.hash ||
      !/^\/s\/[a-z2-7]{8}$/.test(url.pathname)
    )
      continue;
    parts.push({ text: text.slice(start, match.index) });
    parts.push({ text: match[1] ?? "", href: url.pathname });
    start = match.index + match[0].length;
  }
  parts.push({ text: text.slice(start) });
  return parts;
}
