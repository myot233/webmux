// Deterministic test PDFs for the code document preview. Each page shows
// "第 N 页 中文测试" in the Adobe-GB1 STSong-Light font (not embedded, so the
// reader must load the UniGB-UCS2-H CMap) and "Page N" in Helvetica.

export type PdfFixtureOptions = {
  pages?: number;
  // Page 1 links to `internalTarget` (1-based) and to `externalUrl`.
  internalTarget?: number;
  externalUrl?: string;
  // An OpenAction script that sets `window.pdfScript` if it ever runs.
  script?: boolean;
  // A Standard security handler whose empty user password does not match.
  encrypted?: boolean;
};

// Page size in PDF points (A5 portrait keeps tests fast).
const pdfPageSize = { width: 420, height: 595 };

const hexUcs2 = (text: string) =>
  [...text]
    .map((char) => char.charCodeAt(0).toString(16).padStart(4, "0"))
    .join("")
    .toUpperCase();

export function pdfFixture(options: PdfFixtureOptions = {}): Uint8Array {
  const pages = options.pages ?? 1;
  const objects: string[] = [];
  // Object numbers are 1-based; reserve fixed ones first.
  const catalog = 1,
    tree = 2,
    cjk = 3,
    cid = 4,
    descriptor = 5,
    latin = 6,
    firstPage = 7;
  const pageRef = (index: number) => firstPage + index * 2;
  const encrypt = firstPage + pages * 2;
  const openAction = options.script
    ? " /OpenAction << /S /JavaScript /JS (window.pdfScript = true) >>"
    : "";
  objects[catalog] = `<< /Type /Catalog /Pages ${tree} 0 R${openAction} >>`;
  objects[tree] =
    `<< /Type /Pages /Count ${pages} /Kids [${Array.from({ length: pages }, (_, i) => `${pageRef(i)} 0 R`).join(" ")}] >>`;
  objects[cjk] =
    `<< /Type /Font /Subtype /Type0 /BaseFont /STSong-Light /Encoding /UniGB-UCS2-H /DescendantFonts [${cid} 0 R] >>`;
  objects[cid] =
    `<< /Type /Font /Subtype /CIDFontType0 /BaseFont /STSong-Light /CIDSystemInfo << /Registry (Adobe) /Ordering (GB1) /Supplement 2 >> /FontDescriptor ${descriptor} 0 R /DW 1000 >>`;
  objects[descriptor] =
    "<< /Type /FontDescriptor /FontName /STSong-Light /Flags 6 /FontBBox [-25 -254 1000 880] /ItalicAngle 0 /Ascent 880 /Descent -120 /CapHeight 880 /StemV 93 >>";
  objects[latin] =
    "<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica /Encoding /WinAnsiEncoding >>";
  for (let i = 0; i < pages; i++) {
    const content = [
      "BT /F1 28 Tf 40 520 Td",
      `<${hexUcs2(`第 ${i + 1} 页 中文测试`)}> Tj ET`,
      `BT /F2 20 Tf 40 470 Td (Page ${i + 1}) Tj ET`,
    ];
    const annots: string[] = [];
    if (i === 0 && options.internalTarget) {
      content.push(
        `BT /F2 16 Tf 40 400 Td (Go to page ${options.internalTarget}) Tj ET`,
      );
      annots.push(
        `<< /Type /Annot /Subtype /Link /Rect [36 392 236 420] /Border [0 0 0] /Dest [${pageRef(options.internalTarget - 1)} 0 R /XYZ null null null] >>`,
      );
    }
    if (i === 0 && options.externalUrl) {
      content.push("BT /F2 16 Tf 40 350 Td (External link) Tj ET");
      annots.push(
        `<< /Type /Annot /Subtype /Link /Rect [36 342 236 370] /Border [0 0 0] /A << /S /URI /URI (${options.externalUrl}) >> >>`,
      );
      content.push("BT /F2 16 Tf 40 300 Td (Script link) Tj ET");
      annots.push(
        "<< /Type /Annot /Subtype /Link /Rect [36 292 236 320] /Border [0 0 0] /A << /S /JavaScript /JS (window.pdfScript = true) >> >>",
      );
    }
    const stream = content.join("\n");
    objects[pageRef(i)] =
      `<< /Type /Page /Parent ${tree} 0 R /MediaBox [0 0 ${pdfPageSize.width} ${pdfPageSize.height}] /Resources << /Font << /F1 ${cjk} 0 R /F2 ${latin} 0 R >> >> /Contents ${pageRef(i) + 1} 0 R${annots.length ? ` /Annots [${annots.join(" ")}]` : ""} >>`;
    objects[pageRef(i) + 1] =
      `<< /Length ${stream.length} >>\nstream\n${stream}\nendstream`;
  }
  const filler = "0123456789ABCDEF".repeat(4);
  if (options.encrypted)
    objects[encrypt] =
      `<< /Filter /Standard /V 1 /R 2 /Length 40 /P -4 /O <${filler}> /U <${filler}> >>`;
  let body = "%PDF-1.7\n";
  const offsets: number[] = [];
  for (let n = 1; n < objects.length; n++) {
    offsets[n] = body.length;
    body += `${n} 0 obj\n${objects[n]}\nendobj\n`;
  }
  const xref = body.length;
  body += `xref\n0 ${objects.length}\n0000000000 65535 f \n`;
  for (let n = 1; n < objects.length; n++)
    body += `${String(offsets[n]).padStart(10, "0")} 00000 n \n`;
  const id = `<${filler.slice(0, 32)}>`;
  body += `trailer\n<< /Size ${objects.length} /Root ${catalog} 0 R /ID [${id} ${id}]${options.encrypted ? ` /Encrypt ${encrypt} 0 R` : ""} >>\nstartxref\n${xref}\n%%EOF\n`;
  return new TextEncoder().encode(body);
}

// A PDF cut off before any object: the header matches but nothing parses.
export const brokenPdf = () =>
  new TextEncoder().encode("%PDF-1.7\n%\n1 0 obj\n<<");
