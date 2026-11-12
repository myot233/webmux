import { ADJECTIVES, NOUNS } from "./branch-words";

export const worktreeBranchAdjectives = ADJECTIVES;
export const worktreeBranchNouns = NOUNS;

const PAIR_COUNT = ADJECTIVES.length * NOUNS.length;
const RANDOM_ATTEMPTS = 64;
const MAX_PAIR_SUFFIX = 20;

export interface BranchNameInput {
  branchNames: readonly string[];
  worktreePaths: readonly string[];
  avoid?: string;
}

export interface BranchNameOptions {
  random?: () => number;
  now?: Date;
}

function randomIndex(limit: number, random: () => number): number {
  const raw = random();
  const scaled = Math.floor((Number.isFinite(raw) ? raw : 0) * limit);
  return scaled <= 0 ? 0 : scaled >= limit ? limit - 1 : scaled;
}

function pairSegment(index: number): string {
  const adjective = ADJECTIVES[Math.floor(index / NOUNS.length)]!;
  const noun = NOUNS[index % NOUNS.length]!;
  return `${adjective}-${noun}`;
}

function free(
  name: string,
  segment: string,
  refs: ReadonlySet<string>,
  segments: ReadonlySet<string>,
  avoid?: string,
): boolean {
  return name !== avoid && !refs.has(name) && !segments.has(segment);
}

function timestampBranch(
  now: Date,
  refs: ReadonlySet<string>,
  segments: ReadonlySet<string>,
  avoid?: string,
): string {
  const stamp = `${now.getFullYear()}${String(now.getMonth() + 1).padStart(2, "0")}${String(now.getDate()).padStart(2, "0")}-${String(now.getHours()).padStart(2, "0")}${String(now.getMinutes()).padStart(2, "0")}${String(now.getSeconds()).padStart(2, "0")}`;
  for (let suffix = 1; suffix <= 1000; suffix++) {
    const segment = suffix === 1 ? stamp : `${stamp}-${suffix}`;
    const name = `wt/${segment}`;
    if (free(name, segment, refs, segments, avoid)) return name;
  }
  return `wt/${stamp}-${now.getMilliseconds()}`;
}

export function suggestWorktreeBranch(
  input: BranchNameInput,
  options: BranchNameOptions = {},
): string {
  const random = options.random ?? Math.random;
  const refs = new Set(input.branchNames);
  const segments = new Set(
    input.worktreePaths
      .map(
        (path) =>
          path
            .split("/")
            .filter((part) => part !== "")
            .at(-1) ?? "",
      )
      .filter((segment) => segment !== ""),
  );
  for (let attempt = 0; attempt < RANDOM_ATTEMPTS; attempt++) {
    const segment = pairSegment(randomIndex(PAIR_COUNT, random));
    const name = `wt/${segment}`;
    if (free(name, segment, refs, segments, input.avoid)) return name;
  }
  const start = randomIndex(PAIR_COUNT, random);
  for (let offset = 0; offset < PAIR_COUNT; offset++) {
    const segment = pairSegment((start + offset) % PAIR_COUNT);
    const name = `wt/${segment}`;
    if (free(name, segment, refs, segments, input.avoid)) return name;
  }
  for (let offset = 0; offset < PAIR_COUNT; offset++) {
    const base = pairSegment((start + offset) % PAIR_COUNT);
    for (let suffix = 2; suffix <= MAX_PAIR_SUFFIX; suffix++) {
      const segment = `${base}-${suffix}`;
      const name = `wt/${segment}`;
      if (free(name, segment, refs, segments, input.avoid)) return name;
    }
  }
  return timestampBranch(
    options.now ?? new Date(),
    refs,
    segments,
    input.avoid,
  );
}

const REMOTE_REF_PREFIX = "refs/remotes/";

export interface RemoteBranchBase {
  name: string;
  head: string;
  subject: string;
}

/**
 * Extracts remote-tracking branches (`origin/main`, `origin/feature/x`, etc.)
 * from `/api/repositories/{id}/branches` bases, skipping symbolic `/HEAD` refs
 * and any short name that collides with an existing local branch name.
 */
export function remoteBranchBases(
  bases: readonly RemoteBranchBase[],
  localNames: Iterable<string> = [],
): RemoteBranchBase[] {
  const locals = new Set(localNames);
  const result: RemoteBranchBase[] = [];
  for (const base of bases) {
    if (
      !base.name.startsWith(REMOTE_REF_PREFIX) ||
      base.name.endsWith("/HEAD")
    ) {
      continue;
    }
    const short = base.name.slice(REMOTE_REF_PREFIX.length);
    if (!short || !short.includes("/") || locals.has(short)) continue;
    result.push({ name: short, head: base.head, subject: base.subject });
  }
  return result;
}

/**
 * The local branch a remote-tracking branch's short name (`origin/fix/b`)
 * checks out as: the name without its remote (`fix/b`), as `git switch
 * --track` derives it.
 */
export function remoteLocalName(ref: string): string {
  const slash = ref.indexOf("/");
  return slash < 0 ? ref : ref.slice(slash + 1);
}
