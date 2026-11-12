// Feature-detect the ES2025 byte APIs without raising the browser build target.
type Base64Bytes = Uint8Array & {
  toBase64?(options: { alphabet: "base64url"; omitPadding: true }): string;
};
const byteArray = Uint8Array as typeof Uint8Array & {
  fromBase64?(value: string): Uint8Array<ArrayBuffer>;
};

export function encodeBase64Url(text: string): string {
  const bytes: Base64Bytes = new TextEncoder().encode(text);
  if (bytes.toBase64)
    return bytes.toBase64({ alphabet: "base64url", omitPadding: true });

  let binary = "";
  for (const byte of bytes) binary += String.fromCharCode(byte);
  return btoa(binary)
    .replaceAll("+", "-")
    .replaceAll("/", "_")
    .replace(/=+$/, "");
}

export function decodeBase64Url(value: string): Uint8Array<ArrayBuffer> {
  // VAPID keys have also been accepted with the standard alphabet.
  const base64 = value.replaceAll("-", "+").replaceAll("_", "/");
  const padded = base64 + "=".repeat((4 - (base64.length % 4)) % 4);
  return (
    byteArray.fromBase64?.(padded) ??
    Uint8Array.from(atob(padded), (character) => character.charCodeAt(0))
  );
}
