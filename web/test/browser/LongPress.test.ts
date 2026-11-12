import { afterEach, expect, test, vi } from "vitest";
import { longPress } from "../../src/hooks/shared/long-press";

const touch = (type: string) =>
  new PointerEvent(type, {
    pointerType: "touch",
    isPrimary: true,
    bubbles: true,
    clientX: 10,
    clientY: 10,
  });

afterEach(() => {
  document.body.replaceChildren();
});

// On a slow phone the sheet a long press opens slides under the finger;
// the compatibility click of the lift must not choose one of its actions.
test("the click ending a long press never reaches what the press opened", async () => {
  const row = document.createElement("button");
  const action = document.createElement("button");
  const chosen = vi.fn();
  action.addEventListener("click", chosen);
  document.body.append(row);
  const detach = longPress(() => document.body.append(action))(row);

  row.dispatchEvent(touch("pointerdown"));
  await expect.poll(() => action.isConnected).toBe(true);
  action.dispatchEvent(touch("pointerup"));
  action.click();
  expect(chosen).not.toHaveBeenCalled();
  action.click();
  expect(chosen).toHaveBeenCalledOnce();
  if (typeof detach === "function") detach();
});

test("a tap still clicks and a delegated press resolves the held row", async () => {
  const list = document.createElement("ul");
  const row = document.createElement("button");
  row.dataset.row = "";
  const label = document.createElement("span");
  row.append(label);
  list.append(row);
  document.body.append(list);
  const pressed = vi.fn<(element: HTMLElement) => void>();
  const tapped = vi.fn();
  row.addEventListener("click", tapped);
  const detach = longPress(pressed, "[data-row]")(list);

  label.dispatchEvent(touch("pointerdown"));
  label.dispatchEvent(touch("pointerup"));
  row.click();
  expect(tapped).toHaveBeenCalledOnce();

  label.dispatchEvent(touch("pointerdown"));
  await expect.poll(() => pressed.mock.calls.length).toBe(1);
  expect(pressed).toHaveBeenCalledWith(row);
  if (typeof detach === "function") detach();
});
