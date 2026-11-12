import { describe, expect, test } from "vitest";
import {
  deviceLabel,
  encodeSubmission,
  newSubmissionId,
  preparePaste,
  resolveClientId,
} from "../src/domains/terminal/terminal-protocol";
import { SubmissionIdSchema } from "../src/contracts/schemas";

describe("submission wire format", () => {
  test("prepares text exactly like xterm paste", () => {
    expect(preparePaste("a\nb\r\nc", false)).toBe("a\rb\rc");
    expect(preparePaste("中文🙂\n", true)).toBe("\x1b[200~中文🙂\r\x1b[201~");
  });

  test("keeps terminal bytes out of the JSON header", () => {
    const payload = encodeSubmission(
      "s-0001",
      "0123456789abcdef",
      "中文\r",
      "\r",
    );
    const newline = payload.indexOf(10);
    const header = JSON.parse(
      new TextDecoder().decode(payload.subarray(0, newline)),
    );
    expect(header).toEqual({
      id: "s-0001",
      generation: "0123456789abcdef",
      bodyBytes: 7,
    });
    const rest = payload.subarray(newline + 1);
    expect(new TextDecoder().decode(rest.subarray(0, 7))).toBe("中文\r");
    expect(new TextDecoder().decode(rest.subarray(7))).toBe("\r");
  });

  test("submission IDs satisfy the server contract and do not repeat", () => {
    const ids = new Set(Array.from({ length: 100 }, newSubmissionId));
    expect(ids.size).toBe(100);
    for (const id of ids) expect(SubmissionIdSchema.parse(id)).toBe(id);
  });
});

test("device labels name the device and browser without the full agent", () => {
  expect(
    deviceLabel(
      "Mozilla/5.0 (iPhone; CPU iPhone OS 18_0 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/18.0 Mobile/15E148 Safari/604.1",
    ),
  ).toBe("iPhone · Safari");
  expect(
    deviceLabel(
      "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/18.0 Safari/605.1.15",
      5,
    ),
  ).toBe("iPad · Safari");
  expect(
    deviceLabel(
      "Mozilla/5.0 (Linux; Android 15; Pixel 7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/140.0 Mobile Safari/537.36",
    ),
  ).toBe("Android · Chrome");
  expect(
    deviceLabel(
      "Mozilla/5.0 (Windows NT 10.0; Win64; x64; rv:140.0) Gecko/20100101 Firefox/140.0",
    ),
  ).toBe("Windows · Firefox");
});

test("a reload keeps the tab identity, while a duplicated tab takes a new one", () => {
  const data = new Map<string, string>();
  const storage = {
    getItem: (key: string) => data.get(key) ?? null,
    setItem: (key: string, value: string) => void data.set(key, value),
    removeItem: (key: string) => void data.delete(key),
  } as Storage;
  let next = 0;
  const generate = () => `c-generated-${++next}`;
  const first = resolveClientId(storage, generate);
  expect(first).toBe("c-generated-1");
  // pagehide clears the live marker before a reload.
  storage.removeItem("webmux.clientLive");
  expect(resolveClientId(storage, generate)).toBe(first);
  // A duplicated tab copies session storage while the original is still live.
  const copy = new Map(data);
  const duplicate = {
    getItem: (key: string) => copy.get(key) ?? null,
    setItem: (key: string, value: string) => void copy.set(key, value),
    removeItem: (key: string) => void copy.delete(key),
  } as Storage;
  expect(resolveClientId(duplicate, generate)).toBe("c-generated-2");
  expect(data.get("webmux.clientId")).toBe(first);
  data.set("webmux.clientId", "bad id");
  data.delete("webmux.clientLive");
  expect(resolveClientId(storage, generate)).toBe("c-generated-3");
});
