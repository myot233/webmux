// Rows for the start-commit dropdown. A row shows the commit title first and
// its short hash, tip tags and age second, so users recognise a commit by what
// it did and how recent it is instead of typing a hash.
import { api } from "../../platform/api";
import { errorMessage } from "../../platform/error";
import type { WorktreePreset } from "../sessions/new-session-defaults";
import {
  BranchesSchema,
  CommitsSchema,
  RefreshBranchBaseRequestSchema,
  RefreshBranchBaseResponseSchema,
} from "../../contracts/schemas";
import type { z } from "zod";

export type Commit = z.infer<typeof CommitsSchema>["commits"][number];
/** A branch tip the history was read from, e.g. the local or upstream tip. */
export type CommitTip = { hash: string; tag: string };
export type CommitChoice = {
  value: string;
  subject: string;
  shortHash: string;
  tags: string[];
  age: string;
};

const units: Array<[Intl.RelativeTimeFormatUnit, number]> = [
  ["year", 365 * 24 * 3600],
  ["month", 30 * 24 * 3600],
  ["week", 7 * 24 * 3600],
  ["day", 24 * 3600],
  ["hour", 3600],
  ["minute", 60],
];
const formatter = new Intl.RelativeTimeFormat("zh-CN", { numeric: "auto" });

export function relativeTime(iso: string, now = Date.now()): string {
  const seconds = (Date.parse(iso) - now) / 1000;
  if (Number.isNaN(seconds)) return "";
  for (const [unit, size] of units) {
    if (Math.abs(seconds) >= size)
      return formatter.format(Math.trunc(seconds / size), unit);
  }
  return "刚刚";
}

const subjectOf = (subject: string) => subject || "无标题提交";

/** Text naming a row for screen readers, typeahead and tests. */
export function commitChoiceLabel(choice: CommitChoice): string {
  return [
    choice.subject,
    choice.shortHash,
    ...choice.tags,
    ...(choice.age ? [choice.age] : []),
  ].join(" · ");
}

/**
 * Dropdown rows for `commits` (newest first). Each commit that is one of the
 * `tips` carries that tip's tag. A selected `base` outside the list (for
 * example while the history is still loading) stays selectable as its own
 * leading row.
 */
export function commitChoices(
  commits: Commit[],
  options: {
    tips: CommitTip[];
    base: string;
    baseSubject: string;
    now?: number;
  },
): CommitChoice[] {
  const tagsOf = (hash: string) =>
    options.tips.filter((tip) => tip.hash === hash).map((tip) => tip.tag);
  const rows = commits.map((commit) => ({
    value: commit.hash,
    subject: subjectOf(commit.subject),
    shortHash: commit.hash.slice(0, 7),
    tags: tagsOf(commit.hash),
    age: relativeTime(commit.committedAt, options.now),
  }));
  if (!options.base || rows.some((row) => row.value === options.base))
    return rows;
  return [
    {
      value: options.base,
      subject: subjectOf(options.baseSubject),
      shortHash: options.base.slice(0, 7),
      tags: tagsOf(options.base),
      age: "",
    },
    ...rows,
  ];
}

// Tags for a branch's local tip and, after a fetch, its upstream tip.
export function branchTips(local: string, upstreamHead: string): CommitTip[] {
  if (!local && upstreamHead) return [{ hash: upstreamHead, tag: "远程最新" }];
  if (!upstreamHead) return [{ hash: local, tag: "本地最新" }];
  if (upstreamHead === local) return [{ hash: local, tag: "本地与远程最新" }];
  return [
    { hash: upstreamHead, tag: "远程最新" },
    { hash: local, tag: "本地最新" },
  ];
}

export function fetchRepositoryCommits(
  repositoryId: string,
  tips: CommitTip[],
  signal: AbortSignal,
) {
  const query = new URLSearchParams(
    [...new Set(tips.map((tip) => tip.hash))].map((hash) => ["from", hash]),
  );
  return api(
    `/repositories/${repositoryId}/commits?${query}`,
    CommitsSchema,
    "GET",
    undefined,
    signal,
  );
}

export function refreshRepositoryBranchBase(
  repositoryId: string,
  branchName: string,
  signal: AbortSignal,
) {
  return api(
    `/repositories/${repositoryId}/branches/refresh`,
    RefreshBranchBaseResponseSchema,
    "POST",
    { branch: branchName },
    signal,
    RefreshBranchBaseRequestSchema,
  );
}

export function fetchRepositoryBranches(
  repositoryId: string,
  signal?: AbortSignal,
) {
  return api(
    `/repositories/${repositoryId}/branches`,
    BranchesSchema,
    "GET",
    undefined,
    signal,
  );
}

export function formatCommitLoadError(err: unknown) {
  return `无法读取最近提交：${errorMessage(err)}`;
}

export function resolveRefreshedLocalHead(
  value: z.infer<typeof RefreshBranchBaseResponseSchema>,
  data: z.infer<typeof BranchesSchema> | null,
  branchName: string,
): string {
  if (value.baseSource === "local") return value.base;
  return data?.branches.find((item) => item.name === branchName)?.head ?? "";
}

export function isAbortError(err: unknown): boolean {
  return err instanceof DOMException && err.name === "AbortError";
}

export function resolveSelectedStartBranchState(
  name: string,
  data: z.infer<typeof BranchesSchema> | null,
  remoteBranches: readonly { name: string; head: string; subject: string }[],
):
  | {
      kind: "local";
      base: string;
      baseSubject: string;
      tips: CommitTip[];
    }
  | {
      kind: "remote";
      base: string;
      upstream: string;
      baseSubject: string;
      tips: CommitTip[];
    }
  | {
      kind: "default";
      base: string;
      baseSubject: string;
      tips: CommitTip[];
    } {
  const selected = data?.branches.find((item) => item.name === name);
  if (selected) {
    const baseSubject =
      data?.bases.find((item) => item.name === `refs/heads/${name}`)?.subject ||
      "";
    return {
      kind: "local",
      base: selected.head,
      baseSubject,
      tips: branchTips(selected.head, ""),
    };
  }
  const remote = remoteBranches.find((item) => item.name === name);
  if (remote) {
    return {
      kind: "remote",
      base: remote.head,
      upstream: remote.name,
      baseSubject: remote.subject,
      tips: branchTips("", remote.head),
    };
  }
  const base = data?.defaultBase || "";
  return {
    kind: "default",
    base,
    baseSubject: data?.defaultBaseSubject || "",
    tips: base ? [{ hash: base, tag: "当前主工作区" }] : [],
  };
}
export function resolveWorktreePresetSelection(
  initial: WorktreePreset,
  value: z.infer<typeof BranchesSchema>,
): { branch: string; branchEdited: boolean } | null {
  if (initial.mode === "existing") {
    const chosen = value.branches.find((item) => item.name === initial.branch);
    if (chosen && !chosen.worktreePaths.length) {
      return { branch: chosen.name, branchEdited: false };
    }
    return null;
  }
  if (initial.name) {
    return { branch: initial.name, branchEdited: true };
  }
  return null;
}

export function resolveInitialStartBranch(
  value: z.infer<typeof BranchesSchema>,
  remoteBranches: readonly { name: string }[],
  initial: WorktreePreset | undefined,
  preferredStartBranch: string | undefined,
): string {
  const start = initial?.mode === "new" ? initial.startBranch : "";
  if (
    start &&
    (value.branches.some((item) => item.name === start) ||
      remoteBranches.some((item) => item.name === start))
  ) {
    return start;
  }
  if (
    preferredStartBranch &&
    value.branches.some((item) => item.name === preferredStartBranch)
  ) {
    return preferredStartBranch;
  }
  return value.defaultBranch || "";
}
