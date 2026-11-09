import { afterEach, expect, test, vi } from "vitest";
import {
  sessionReference,
  sessionReferenceParts,
} from "../src/session-reference";

afterEach(() => vi.unstubAllGlobals());

test("only exact same-origin session references become links", () => {
  vi.stubGlobal("location", new URL("https://webmux.example.test"));
  const id = "abcd2345";
  const reference = sessionReference("Demo [work]", id);
  const external = `[external](https://example.test/s/${id})`;
  const query = `[query](https://webmux.example.test/s/${id}?token=example)`;
  expect(
    sessionReferenceParts(`前文 ${reference} 后文 ${external} ${query}`),
  ).toEqual([
    { text: "前文 " },
    { text: "Demo  work", href: `/s/${id}` },
    { text: ` 后文 ${external} ${query}` },
  ]);
});
