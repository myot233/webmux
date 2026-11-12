import { describe, expect, it } from "vitest";
import { createArchiveSignal } from "../src/domains/sessions/archive-signal";

describe("archive signal", () => {
  it("reaches only listeners that are still subscribed", () => {
    const signal = createArchiveSignal();
    const first: string[] = [];
    const second: string[] = [];
    const stop = signal.subscribe((id) => first.push(id));
    signal.subscribe((id) => second.push(id));

    signal.emit("abc");
    stop();
    signal.emit("def");

    expect(first).toEqual(["abc"]);
    expect(second).toEqual(["abc", "def"]);
  });

  it("is not remembered for a listener that subscribes later", () => {
    const signal = createArchiveSignal();
    signal.emit("abc");
    const seen: string[] = [];
    signal.subscribe((id) => seen.push(id));
    expect(seen).toEqual([]);
  });

  it("lets a listener unsubscribe while the signal is being delivered", () => {
    const signal = createArchiveSignal();
    const seen: string[] = [];
    const stop = signal.subscribe(() => stop());
    signal.subscribe((id) => seen.push(id));
    signal.emit("abc");
    expect(seen).toEqual(["abc"]);
  });
});
