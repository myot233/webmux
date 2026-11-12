import { afterEach, expect, test, vi } from "vitest";
import {
  sessionReference,
  sessionReferenceParts,
} from "../src/domains/sessions/session-reference";

afterEach(() => vi.unstubAllGlobals());

test("relative references resolve across service entry origins", () => {
  vi.stubGlobal("location", new URL("https://webmux.example.test"));
  const id = "abcd2345";
  const reference = sessionReference("Demo [work]", id);
  expect(reference).toBe(`[Demo  work](/s/${id})`);
  vi.stubGlobal("location", new URL("https://alternate.example.test"));
  const external = `[external](https://example.test/s/${id})`;
  const query = `[query](https://alternate.example.test/s/${id}?token=example)`;
  expect(
    sessionReferenceParts(`前文 ${reference} 后文 ${external} ${query}`),
  ).toEqual([
    { text: "前文 " },
    { text: "Demo  work", href: `/s/${id}` },
    { text: ` 后文 ${external} ${query}` },
  ]);
});

test("legacy absolute references only link exact same-origin session paths", () => {
  vi.stubGlobal("location", new URL("https://webmux.example.test"));
  const valid = "[Local](https://webmux.example.test/s/abcd2345)";
  const invalid = [
    "[external](https://other.example.test/s/abcd2345)",
    "[userinfo](https://user@webmux.example.test/s/abcd2345)",
    "[password](https://user:pass@webmux.example.test/s/abcd2345)",
    "[query](https://webmux.example.test/s/abcd2345?token=secret)",
    "[hash](https://webmux.example.test/s/abcd2345#message)",
    "[long path](https://webmux.example.test/s/abcd2345/extra)",
    "[short id](/s/abc)",
    "[invalid id](/s/abcd0189)",
    "[query path](/s/abcd2345?token=secret)",
    "[fragment path](/s/abcd2345#message)",
  ].join(" ");
  expect(sessionReferenceParts(`${valid} ${invalid}`)).toEqual([
    { text: "" },
    { text: "Local", href: "/s/abcd2345" },
    { text: ` ${invalid}` },
  ]);
});

test("unmatched opening brackets remain text before a valid reference", () => {
  vi.stubGlobal("location", new URL("https://webmux.example.test"));
  const brackets = "[".repeat(65_536);
  expect(sessionReferenceParts(`${brackets}[Session](/s/abcd2345)`)).toEqual([
    { text: brackets },
    { text: "Session", href: "/s/abcd2345" },
    { text: "" },
  ]);
});
