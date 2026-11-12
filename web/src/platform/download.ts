// Saves blob as filename through a temporary link in owner, the document of
// the control that asked.
export function saveBlob(owner: Document, blob: Blob, filename: string) {
  const href = URL.createObjectURL(blob);
  const anchor = owner.createElement("a");
  anchor.href = href;
  anchor.download = filename;
  anchor.hidden = true;
  owner.body.append(anchor);
  anchor.click();
  anchor.remove();
  (owner.defaultView ?? window).requestAnimationFrame(() =>
    URL.revokeObjectURL(href),
  );
}
