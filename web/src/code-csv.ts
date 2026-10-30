// RFC 4180 delimited-text parsing for the read-only table preview.

export const maxTableRows = 2000;

type DelimitedTable =
  | {
      ok: true;
      header: string[];
      // At most `limit` data rows, each padded to `columns` cells.
      rows: string[][];
      // All data rows in the file, excluding the header.
      total: number;
      columns: number;
    }
  | { ok: false; line: number; message: string };

// Parses `text` with a state machine: quoted fields may contain the
// delimiter, doubled quotes and line breaks. The first record is the header;
// blank lines are skipped. Scanning continues past `limit` only to count rows.
export function parseDelimited(
  text: string,
  delimiter: "," | "\t",
  limit = maxTableRows,
): DelimitedTable {
  const source = text.startsWith("\uFEFF") ? text.slice(1) : text;
  const records: string[][] = [];
  let total = -1;
  let record: string[] = [];
  let field = "";
  let quoted = false;
  // A field was opened by a quote and closed by its matching quote.
  let closed = false;
  let line = 1;
  let quoteLine = 0;
  // The current record has content (a blank line yields none).
  let started = false;
  const endField = () => {
    record.push(field);
    field = "";
    closed = false;
  };
  const endRecord = () => {
    if (started) {
      endField();
      total++;
      if (records.length <= limit) records.push(record);
    }
    record = [];
    field = "";
    closed = false;
    started = false;
  };
  for (let i = 0; i < source.length; i++) {
    const char = source[i];
    if (quoted) {
      if (char === '"') {
        if (source[i + 1] === '"') {
          field += '"';
          i++;
        } else {
          quoted = false;
          closed = true;
        }
      } else {
        if (char === "\n") line++;
        field += char;
      }
      continue;
    }
    if (char === delimiter) {
      started = true;
      endField();
    } else if (char === "\n" || char === "\r") {
      if (char === "\r" && source[i + 1] === "\n") i++;
      endRecord();
      line++;
    } else if (char === '"' && field === "" && !closed) {
      started = true;
      quoted = true;
      quoteLine = line;
    } else {
      started = true;
      field += char;
    }
  }
  if (quoted)
    return {
      ok: false,
      line: quoteLine,
      message: `第 ${quoteLine} 行引号未闭合`,
    };
  endRecord();
  const header = records[0] ?? [];
  const rows = records.slice(1);
  const columns = Math.max(header.length, ...rows.map((row) => row.length));
  const pad = (row: string[]) =>
    row.length < columns
      ? [...row, ...Array<string>(columns - row.length).fill("")]
      : row;
  return {
    ok: true,
    header: pad(header),
    rows: rows.map(pad),
    total: Math.max(total, 0),
    columns,
  };
}
