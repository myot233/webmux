export interface Modifiers {
  ctrl: boolean;
  alt: boolean;
}
export function modifiedInput(input: string, modifiers: Modifiers): string {
  let result = input;
  if (modifiers.ctrl && input.length === 1 && /^[a-z@[\]\\^_]$/i.test(input)) {
    result = String.fromCharCode(input.toUpperCase().charCodeAt(0) & 0x1f);
  }
  return modifiers.alt ? "\x1b" + result : result;
}
