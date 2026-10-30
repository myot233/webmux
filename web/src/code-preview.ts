export const pdfMediaType = "application/pdf";

// Text formats with a read-only preview beside the source view.
type PreviewFormat = "markdown" | "html" | "csv" | "tsv" | "json";

const formats: [RegExp, PreviewFormat][] = [
  [/\.(?:md|markdown|mdown|mkd)$/i, "markdown"],
  [/\.html?$/i, "html"],
  [/\.csv$/i, "csv"],
  [/\.tsv$/i, "tsv"],
  [/\.json$/i, "json"],
];

export const previewFormat = (path: string): PreviewFormat | undefined =>
  formats.find(([pattern]) => pattern.test(path))?.[1];

// Names the preview in the narrow code menu.
const labels: Record<PreviewFormat, string> = {
  markdown: "预览 Markdown",
  html: "预览 HTML",
  csv: "预览表格",
  tsv: "预览表格",
  json: "预览 JSON",
};

export const previewFormatLabel = (format: PreviewFormat) => labels[format];
