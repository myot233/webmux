// Random identifiers for client requests. getRandomValues works in insecure
// contexts (plain-HTTP LAN access); randomUUID does not.
export function randomHex(bytes: number) {
  return Array.from(crypto.getRandomValues(new Uint8Array(bytes)), (byte) =>
    byte.toString(16).padStart(2, "0"),
  ).join("");
}

/** A clientRequestId: the server runs each ID at most once. */
export function newRequestId() {
  return "r-" + randomHex(16);
}
