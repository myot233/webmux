import { afterEach, expect, test, vi } from "vitest";
import { page, userEvent } from "vitest/browser";
import { cleanup, render } from "vitest-browser-svelte/pure";
import SessionViewControls from "../../src/components/terminal/SessionViewControls.svelte";

afterEach(() => cleanup());

const choice = (name: string) => page.getByRole("radio", { name, exact: true });

test("the display choice cannot be cleared and moves with the arrow keys", async () => {
  const onchange = vi.fn();
  render(SessionViewControls, { compact: false, onchange });
  await expect
    .element(choice("输出预览"))
    .toHaveAttribute("aria-checked", "true");

  // Pressing the chosen option again keeps it chosen and reports nothing.
  await choice("输出预览").click();
  await expect
    .element(choice("输出预览"))
    .toHaveAttribute("aria-checked", "true");
  expect(onchange).not.toHaveBeenCalled();

  await userEvent.keyboard("{ArrowLeft}{Enter}");
  expect(onchange).toHaveBeenLastCalledWith(true);
  await expect
    .element(choice("紧凑列表"))
    .toHaveAttribute("aria-checked", "true");
});
