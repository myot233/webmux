import { describe, expect, it } from "vitest";
import { createOutputBudget } from "../src/terminal-output";

describe("terminal output budget", () => {
  it("waits for parsed output before requesting one fresh replay", () => {
    const callbacks: (() => void)[] = [];
    let replays = 0;
    let writes = 0;
    const output = createOutputBudget(
      (_data, done) => {
        writes++;
        callbacks.push(done);
      },
      () => replays++,
    );
    const chunk = new Uint8Array(1024 * 1024);
    for (let i = 0; i < 4; i++) expect(output.write(chunk)).toBe(true);
    expect(output.write(chunk)).toBe(false);
    expect(writes).toBe(4);
    expect(replays).toBe(0);
    while (callbacks.length) callbacks.shift()!();
    expect(replays).toBe(1);
    expect(output.write(chunk)).toBe(false);
    expect(replays).toBe(1);
    output.replay();
    expect(output.write(chunk)).toBe(true);
  });

  it("releases the budget only when the terminal consumes bytes", () => {
    const callbacks: (() => void)[] = [];
    const output = createOutputBudget(
      (_data, done) => callbacks.push(done),
      () => {},
    );
    const chunk = new Uint8Array(1024 * 1024);
    for (let i = 0; i < 4; i++) output.write(chunk);
    callbacks.shift()!();
    expect(output.write(chunk)).toBe(true);
  });

  it("handles immediate parsing and cancels recovery after disposal", () => {
    let replays = 0;
    const fast = createOutputBudget(
      (_data, done) => done(),
      () => replays++,
    );
    const chunk = new Uint8Array(1024 * 1024);
    for (let i = 0; i < 10; i++) expect(fast.write(chunk)).toBe(true);
    expect(replays).toBe(0);
    const callbacks: (() => void)[] = [];
    const slow = createOutputBudget(
      (_data, done) => callbacks.push(done),
      () => replays++,
    );
    for (let i = 0; i < 5; i++) slow.write(chunk);
    slow.dispose();
    while (callbacks.length) callbacks.shift()!();
    expect(replays).toBe(0);
    expect(slow.write(chunk)).toBe(false);
  });
});
