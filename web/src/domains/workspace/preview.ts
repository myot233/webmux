import type { PreviewSettings } from "../../contracts/schemas";

// The backend supplies nonempty screen rows in their original order.
export function selectPreview(text: string, settings: PreviewSettings): string {
  if (!text) return "";
  const rows = text.split("\n");
  const end = Math.max(0, rows.length - settings.skip);
  return rows.slice(Math.max(0, end - settings.lines), end).join("\n");
}
