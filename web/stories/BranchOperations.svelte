<script lang="ts">
  import type { BranchOperation } from "../src/domains/workspace/branch-actions";
  import type { BranchOperations } from "../src/domains/workspace/branch-operations";
  import BranchOperationDialog from "../src/components/workspace/BranchOperationDialog.svelte";
  import { Button } from "../src/lib/components/ui/button";
  import type {
    BranchDeleteItem,
    Commit,
    Repository,
  } from "../src/contracts/schemas";
  import { longPath, longToken, session, worktree } from "./fixtures";

  // The branch flows in their dialog (a drawer on phones) with long branch
  // names, a long push URL, blockers, sessions and both delete groups.
  let {
    flow = "fast-forward",
  }: {
    flow?:
      | "fast-forward"
      | "fast-forward-blocked"
      | "publish"
      | "push-blocked"
      | "delete";
  } = $props();
  let open = $state(true);

  const at = "2026-01-01T09:00:00Z";
  const branch = `feature/${longToken}`;
  const commits = (count: number, total = count) => ({
    total,
    items: Array.from({ length: count }, (_, i): Commit => ({
      hash: (i + 1).toString(16).padStart(40, "0"),
      subject: `第 ${i + 1} 个提交：修复 ${longToken} 的换行`,
      author: `开发者 ${longToken}`,
      committedAt: at,
    })),
  });
  const repository: Repository = {
    id: "storybook-repository",
    name: "仓库",
    commonDir: `${longPath}/.git`,
    anchorPath: longPath,
    availability: "available",
    error: "",
    checkedAt: at,
    lastFetchedAt: at,
    initInstruction: "",
    worktrees: [],
  };
  const tree = { ...worktree, available: true, prunable: false, error: "" };
  const confirm = { confirmationId: "story", expiresAt: at };
  const blocked = { confirmationId: "", expiresAt: null };
  const contained = (name: string): BranchDeleteItem => ({
    branch: name,
    commit: "abcdef1234567890abcdef1234567890abcdef12",
    group: "contained",
    unmerged: { total: 0, items: [] },
    blockers: [],
  });
  const source: BranchOperations = {
    fastForwardPreview: async () => ({
      ...(flow === "fast-forward-blocked" ? blocked : confirm),
      branch,
      from: "1234567890abcdef1234567890abcdef12345678",
      to: "abcdef1234567890abcdef1234567890abcdef12",
      upstream: `origin/${branch}`,
      commits: commits(20, 42),
      worktree: tree,
      sessions: flow === "fast-forward" ? [session] : [],
      blockers:
        flow === "fast-forward-blocked"
          ? [
              { code: "changes", message: "有未提交的修改，请先提交或贮藏" },
              {
                code: "operation",
                message: "正在变基，完成或中止后再快进",
                path: longPath,
              },
            ]
          : [],
      warnings: [
        {
          code: "untracked",
          message:
            "存在未跟踪文件；上游跟踪了同名文件时快进会失败，文件不会被覆盖",
        },
        {
          code: "sessions",
          message: "有 1 个运行中的会话；它们不会被关闭，会看到快进后的文件",
        },
      ],
    }),
    fastForward: async () => ({ repository }),
    pushPreview: async () => ({
      ...(flow === "push-blocked" ? blocked : confirm),
      branch,
      commit: "1234567890abcdef1234567890abcdef12345678",
      remote: "origin",
      pushUrl: `https://***@github.example.invalid/${longToken}/${longToken}.git`,
      target: branch,
      publish: flow === "publish",
      commits: commits(flow === "push-blocked" ? 0 : 3),
      blockers:
        flow === "push-blocked"
          ? [
              {
                code: "behind",
                message: "分支落后上游 3 个提交，请先快进",
              },
            ]
          : [],
      warnings:
        flow === "publish"
          ? [
              {
                code: "gone",
                message: `上游 origin/${branch} 已删除；发布会在远端重新创建该分支`,
              },
            ]
          : [],
    }),
    push: async () => ({ status: "ok", message: "", repository }),
    deletePreview: async () => ({
      ...confirm,
      items: [
        contained(`merged/${longToken}`),
        contained("merged/old"),
        {
          branch: `squashed/${longToken}`,
          commit: "fedcba0987654321fedcba0987654321fedcba09",
          group: "unmerged",
          unmerged: commits(3),
          blockers: [],
        },
        {
          ...contained(branch),
          blockers: [
            {
              code: "occupied",
              message: `分支 ${branch} 已被工作树检出，不能删除`,
              path: longPath,
            },
          ],
        },
      ],
    }),
    deleteBranches: async () => ({ results: [], repository }),
    remotes: async () => ({
      remotes: [
        { name: "origin", fetchUrl: "", pushUrl: "" },
        { name: `fork-${longToken}`, fetchUrl: "", pushUrl: "" },
      ],
    }),
  };
  const operation = $derived<BranchOperation>(
    flow === "delete"
      ? { kind: "delete", branches: [branch], cleanup: true }
      : flow === "publish" || flow === "push-blocked"
        ? { kind: "push", branch, publish: flow === "publish" }
        : { kind: "fast-forward", branch },
  );
</script>

<main class="mx-auto w-full max-w-5xl p-4" data-testid="branch-story">
  <Button onclick={() => (open = true)}>打开分支操作</Button>
  {#if open}
    <BranchOperationDialog
      repositoryId={repository.id}
      {operation}
      {source}
      onchange={() => {}}
      onclose={() => (open = false)}
    />
  {/if}
</main>
