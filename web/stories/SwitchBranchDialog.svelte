<script lang="ts">
  import { untrack } from "svelte";
  import type { SwitchSource } from "../src/components/workspace/SwitchBranch.svelte";
  import SwitchBranchDialog from "../src/components/workspace/SwitchBranchDialog.svelte";
  import { Button } from "../src/lib/components/ui/button";
  import type {
    Branches,
    Repository,
    SwitchPreview,
    Worktree,
  } from "../src/contracts/schemas";
  import { longPath, longToken, session, worktree } from "./fixtures";

  // The switch of a worktree's branch with a preview that is blocked, one
  // that lists running sessions to acknowledge, and long names that must
  // wrap. A preview appears once a target branch is chosen.
  let {
    scenario = "blockers",
  }: { scenario?: "blockers" | "sessions" | "long-names" } = $props();
  let open = $state(true);
  const shown = untrack(() => scenario);

  const at = "2026-01-01T09:00:00Z";
  const head = "1234567890abcdef1234567890abcdef12345678";
  const target = "abcdef1234567890abcdef1234567890abcdef12";
  const long = shown === "long-names";
  const current: Worktree = {
    ...worktree,
    kind: "linked",
    path: `${longPath}/current`,
    branch: long ? `feature/current-${longToken}` : "feature/current",
    head,
    locked: false,
    lockReason: "",
    prunable: false,
    available: true,
    error: "",
    operation: null,
  };
  const other: Worktree = {
    ...current,
    id: "storybook-other",
    path: `${longPath}/other-${longToken}`,
    branch: long ? `feature/taken-${longToken}` : "feature/taken",
  };
  const repository: Repository = {
    id: "storybook-repository",
    name: `仓库 ${longToken}`,
    commonDir: `${longPath}/.git`,
    anchorPath: longPath,
    availability: "available",
    error: "",
    checkedAt: at,
    lastFetchedAt: at,
    initInstruction: "",
    worktrees: [current, other],
  };
  const chosen = long ? `feature/target-${longToken}` : "feature/target";
  const branch = (name: string, paths: string[] = []) => ({
    name,
    head: name === chosen ? target : head,
    worktreePaths: paths,
    subject: `提交 ${name}`,
    committedAt: at,
    upstream: null,
    ahead: null,
    behind: null,
    upstreamGone: false,
    mergedIntoDefault: false,
  });
  const branches: Branches = {
    branches: [
      branch(current.branch!, [current.path]),
      branch(other.branch!, [other.path]),
      branch(chosen),
    ],
    bases: [
      {
        name: `refs/remotes/origin/fix/${long ? longToken : "remote"}`,
        head: target,
        subject: `远程提交 ${longToken}`,
        kind: "remote",
        committedAt: at,
      },
    ],
    defaultBase: head,
    defaultBaseSubject: "",
    defaultBranch: "main",
    defaultParent: longPath,
  };
  const previews: Record<typeof scenario, Partial<SwitchPreview>> = {
    blockers: {
      blockers: [
        {
          code: "changes",
          message: `有未提交的修改（2 个未暂存），请先提交或贮藏：${longToken}`,
        },
        {
          code: "operation",
          message: "工作树正在变基，请先在终端完成或中止 rebase",
        },
      ],
      warnings: [
        { code: "untracked", message: "有未跟踪的文件；切换不会删除它们" },
        { code: "submodules", message: "仓库包含子模块，切换后不会自动更新" },
      ],
    },
    sessions: {
      confirmationId: "storybook-confirmation",
      expiresAt: "2026-01-01T12:05:00Z",
      sessions: [
        { ...session, id: "abcdefga", status: "idle" },
        {
          ...session,
          id: "abcdefgb",
          title: `Claude Code · ${longToken}`,
          status: "running",
          agent: {
            name: "claude",
            title: "Claude Code",
            icon: "claudecode",
            sessionId: "01a",
          },
        },
      ],
      warnings: [
        {
          code: "sessions",
          message: "有 2 个运行中的会话；它们不会被关闭，会看到切换后的文件",
        },
        { code: "locked", message: `工作树已锁定：${longToken}` },
      ],
    },
    "long-names": {
      blockers: [
        {
          code: "occupied",
          message: `分支 ${chosen} 已被另一个工作树检出：${other.path}`,
          path: other.path,
        },
      ],
    },
  };
  const source: SwitchSource = {
    branches: async () => branches,
    commits: async () => ({
      commits: [
        { hash: head, subject: "当前提交", author: "dev", committedAt: at },
      ],
    }),
    preview: async (tree, next) => ({
      confirmationId: "",
      expiresAt: null,
      worktree: tree,
      target: {
        kind: next.kind,
        branch: next.kind === "local" ? next.branch : chosen,
        commit: target,
        subject: `目标提交 ${longToken}`,
        createsBranch: next.kind !== "local",
        upstream: null,
      },
      sessions: [],
      blockers: [],
      warnings: [],
      ...previews[shown],
    }),
    switch: async (tree) => ({ ...tree, branch: chosen }),
  };
</script>

<main class="mx-auto w-full max-w-5xl p-4" data-testid="switch-story">
  <Button onclick={() => (open = true)}>打开切换分支</Button>
  {#if open}
    <SwitchBranchDialog
      {repository}
      worktree={current}
      {source}
      onchange={() => {}}
      onnewworktree={() => {}}
      ongoto={() => {}}
      onclose={() => (open = false)}
    />
  {/if}
</main>
