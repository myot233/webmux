import { afterEach, expect, test, vi } from "vitest";
import { copyText } from "../../src/clipboard";

afterEach(() => {
  vi.restoreAllMocks();
  for (const area of document.querySelectorAll("textarea")) area.remove();
});

test("legacy copy works without the Clipboard API and restores focus", async () => {
  const previous = document.activeElement;
  const button = document.createElement("button");
  button.textContent = "copy";
  document.body.append(button);
  button.focus();

  // Plain-HTTP origins expose no navigator.clipboard; a rejected write models
  // the same fallback in a secure context.
  if (navigator.clipboard?.writeText)
    vi.spyOn(navigator.clipboard, "writeText").mockRejectedValue(
      new Error("denied"),
    );
  const exec = vi
    .spyOn(document, "execCommand")
    .mockImplementation((command) => command === "copy");

  try {
    await expect(copyText("copyable")).resolves.toBe(true);
    expect(exec).toHaveBeenCalledWith("copy");
    expect(document.activeElement).toBe(button);
  } finally {
    button.remove();
    if (previous instanceof HTMLElement) previous.focus();
  }
});
