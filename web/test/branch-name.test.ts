import { describe, expect, test } from "vitest";
import {
  suggestWorktreeBranch,
  worktreeBranchAdjectives,
  worktreeBranchNouns,
} from "../src/branch-name";

const random = () => 0;
const bases = worktreeBranchAdjectives.flatMap((adjective) =>
  worktreeBranchNouns.map((noun) => `wt/${adjective}-${noun}`),
);

describe("worktree branch suggestions", () => {
  test("provides a large pool of distinct lowercase ref-safe word pairs", () => {
    expect(new Set(worktreeBranchAdjectives).size).toBe(
      worktreeBranchAdjectives.length,
    );
    expect(new Set(worktreeBranchNouns).size).toBe(worktreeBranchNouns.length);
    const overlap = worktreeBranchNouns.filter((noun) =>
      (worktreeBranchAdjectives as readonly string[]).includes(noun),
    );
    expect(overlap).toEqual([]);
    expect(
      worktreeBranchAdjectives.length * worktreeBranchNouns.length,
    ).toBeGreaterThanOrEqual(20_000);
    for (const word of [...worktreeBranchAdjectives, ...worktreeBranchNouns]) {
      expect(word).toMatch(/^[a-z]+$/);
    }
  });

  test("suggests a ref-safe wt/adjective-noun name", () => {
    expect(
      suggestWorktreeBranch({ branchNames: [], worktreePaths: [] }, { random }),
    ).toMatch(/^wt\/[a-z]+-[a-z]+$/);
  });

  test("switches pairs when the chosen ref or directory segment is taken", () => {
    const first = suggestWorktreeBranch(
      { branchNames: [], worktreePaths: [] },
      { random },
    );
    const segment = first.slice("wt/".length);
    expect(
      suggestWorktreeBranch(
        { branchNames: [first], worktreePaths: [] },
        { random },
      ),
    ).toMatch(/^wt\/[a-z]+-[a-z]+$/);
    expect(
      suggestWorktreeBranch(
        { branchNames: [], worktreePaths: [`/tmp/repo.worktrees/${segment}/`] },
        { random },
      )
        .split("/")
        .at(-1),
    ).not.toBe(segment);
  });

  test("does not treat another branch's last segment as a directory collision", () => {
    const first = suggestWorktreeBranch(
      { branchNames: [], worktreePaths: [] },
      { random },
    );
    expect(
      suggestWorktreeBranch(
        {
          branchNames: [`feature/${first.slice("wt/".length)}`],
          worktreePaths: [],
        },
        { random },
      ),
    ).toBe(first);
  });

  test("avoid selects a different free pair", () => {
    const first = suggestWorktreeBranch(
      { branchNames: [], worktreePaths: [] },
      { random },
    );
    const next = suggestWorktreeBranch(
      { branchNames: [], worktreePaths: [], avoid: first },
      { random },
    );
    expect(next).not.toBe(first);
    expect(next).toMatch(/^wt\/[a-z]+-[a-z]+$/);
  });

  test("suffixes a pair only after every base name is taken", () => {
    const first = suggestWorktreeBranch(
      { branchNames: bases, worktreePaths: [] },
      { random },
    );
    expect(first).toMatch(/^wt\/[a-z]+-[a-z]+-2$/);
    expect(
      suggestWorktreeBranch(
        { branchNames: bases, worktreePaths: [], avoid: first },
        { random },
      ),
    ).not.toBe(first);
  });

  test("uses a local timestamp after pair suffixes are exhausted", () => {
    const branchNames = bases.flatMap((name) => [
      name,
      ...Array.from({ length: 19 }, (_, index) => `${name}-${index + 2}`),
    ]);
    const now = new Date(2026, 9, 14, 15, 30, 45);
    expect(
      suggestWorktreeBranch(
        { branchNames, worktreePaths: [] },
        { random, now },
      ),
    ).toBe("wt/20261014-153045");
    expect(
      suggestWorktreeBranch(
        {
          branchNames: [...branchNames, "wt/20261014-153045"],
          worktreePaths: [],
        },
        { random, now },
      ),
    ).toBe("wt/20261014-153045-2");
  });
});
