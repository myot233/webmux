import { expect, test } from "vitest";
import { decodeBase64Url, encodeBase64Url } from "../src/platform/base64url";

test("legacy Base64URL handling matches the typed-array methods", () => {
  const text = "~~~???a终";
  const encoded = "fn5-Pz8_Yee7iA";
  const bytes = new TextEncoder().encode(text);
  const standard = "fn5+Pz8/Yee7iA==";
  const verify = () => {
    expect(encodeBase64Url(text)).toBe(encoded);
    expect(decodeBase64Url(encoded)).toEqual(bytes);
    expect(decodeBase64Url(standard)).toEqual(bytes);
    expect(() => decodeBase64Url("!")).toThrow();
  };
  verify();

  const encoder = Object.getOwnPropertyDescriptor(
    Uint8Array.prototype,
    "toBase64",
  )!;
  const decoder = Object.getOwnPropertyDescriptor(Uint8Array, "fromBase64")!;
  try {
    Object.defineProperty(Uint8Array.prototype, "toBase64", {
      value: undefined,
    });
    Object.defineProperty(Uint8Array, "fromBase64", { value: undefined });
    verify();
  } finally {
    Object.defineProperty(Uint8Array.prototype, "toBase64", encoder);
    Object.defineProperty(Uint8Array, "fromBase64", decoder);
  }
});
