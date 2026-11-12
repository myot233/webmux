<script lang="ts">
  import { onDestroy, untrack } from "svelte";
  import IconChevronDown from "~icons/tabler/chevron-down";
  import IconDots from "~icons/tabler/dots";
  import { Badge } from "$lib/components/ui/badge";
  import { Button } from "$lib/components/ui/button";
  import * as Collapsible from "$lib/components/ui/collapsible";
  import * as DropdownMenu from "$lib/components/ui/dropdown-menu";
  import { ListButton } from "$lib/components/ui/list-button";
  import {
    branchActions,
    cleanupCandidates,
    cleanupOperation,
    divergedText,
    type BranchOperation,
  } from "../../domains/workspace/branch-actions";
  import { remoteLocalName } from "../../domains/workspace/branch-name";
  import { relativeTime } from "../../domains/workspace/commit-choices";
  import { errorMessage } from "../../platform/error";
  import { LatestRequest } from "../../hooks/shared/remote-request.svelte";
  import type { WorktreePreset } from "../../domains/sessions/new-session-defaults";
  import type { Branches } from "../../contracts/schemas";
  import { upstreamState } from "../../domains/workspace/worktree-sync";
  import BranchMenuItems from "./BranchMenuItems.svelte";
  import InlineNotice from "../shared/InlineNotice.svelte";
  import ListSearchInput from "../shared/ListSearchInput.svelte";
  import UpstreamStatus from "./UpstreamStatus.svelte";

  // The repository panel's branches: local branches with their upstream and
  // the worktree that has each checked out, then the remote-tracking
  // branches in a fold. Read when the section is first shown; both groups
  // filter by name. A branch no worktree has checked out, and every remote
  // one, can be opened in a new worktree. Each local branch's "⋯" offers
  // its sync actions and the title row "清理分支（N）"; both open in the
  // panel (`onoperation`), which bumps `version` afterwards to read the
  // branches again.
  let {
    active,
    load,
    worktreePaths,
    onopen,
    onnewworktree,
    onoperation,
    version = 0,
  }: {
    /** The section is shown; the first time starts the read. */
    active: boolean;
    load: (signal: AbortSignal) => Promise<Branches>;
    /** Paths of the repository's worktrees; others are shown as text. */
    worktreePaths: ReadonlySet<string>;
    /** Opens the details of the worktree at a path. */
    onopen: (path: string) => void;
    /** Opens the new-worktree dialog prefilled for a branch. */
    onnewworktree: (preset: WorktreePreset) => void;
    onoperation?: (operation: BranchOperation) => void;
    /** A change makes a section that was read read again. */
    version?: number;
  } = $props();

  let data = $state<Branches | null>(null);
  let error = $state("");
  let loading = $state(false);
  let query = $state("");
  let remoteOpen = $state(false);
  const requests = new LatestRequest();
  onDestroy(() => requests.dispose());

  async function read() {
    const request = requests.begin();
    loading = true;
    error = "";
    try {
      const result = await load(request.signal);
      if (request.current()) data = result;
    } catch (err) {
      if (request.reportable()) error = errorMessage(err);
    } finally {
      if (request.current()) loading = false;
    }
  }
  $effect(() => {
    if (active && !data && !loading && !error) void read();
  });
  let readVersion = untrack(() => version);
  $effect(() => {
    if (version === readVersion) return;
    readVersion = version;
    if (untrack(() => data || error)) untrack(() => void read());
  });
  const candidates = $derived(cleanupCandidates(data?.branches ?? []));

  const remotePrefix = "refs/remotes/";
  const term = $derived(query.trim().toLowerCase());
  const local = $derived(data?.branches ?? []);
  const remote = $derived(
    (data?.bases ?? [])
      .filter((base) => base.kind === "remote")
      .map((base) => ({
        ...base,
        label: base.name.startsWith(remotePrefix)
          ? base.name.slice(remotePrefix.length)
          : base.name,
      })),
  );
  const localShown = $derived(
    term
      ? local.filter((branch) => branch.name.toLowerCase().includes(term))
      : local,
  );
  const remoteShown = $derived(
    term
      ? remote.filter((branch) => branch.label.toLowerCase().includes(term))
      : remote,
  );
  // A search shows the matching remote branches without another click.
  const remoteExpanded = $derived(remoteOpen || !!term);
  const countText = (shown: number, total: number) =>
    term ? `匹配 ${shown} / ${total}` : String(total);
</script>

<div class="space-y-3">
  <ListSearchInput
    bind:value={query}
    label="搜索分支"
    placeholder="按分支名称搜索…"
  />

  {#if error}
    <InlineNotice tone="danger">无法读取分支：{error}</InlineNotice>
    <Button size="sm" variant="outline" onclick={() => void read()}
      >重新读取</Button
    >
  {:else if !data}
    <p role="status" class="text-xs text-muted-foreground">正在读取分支…</p>
  {:else}
    <section class="space-y-2" aria-label="本地分支">
      <div class="flex flex-wrap items-center justify-between gap-2">
        <h3 class="flex items-center gap-2 text-sm font-medium">
          本地分支
          <Badge variant="secondary"
            >{countText(localShown.length, local.length)}</Badge
          >
        </h3>
        {#if onoperation && candidates.length}
          <Button
            size="sm"
            variant="outline"
            title="删除未被检出、且上游已删除或已合入默认分支的本地分支"
            onclick={() => onoperation(cleanupOperation(candidates))}
            >清理分支（{candidates.length}）</Button
          >
        {/if}
      </div>
      {#if localShown.length === 0}
        <p class="text-xs text-muted-foreground">
          {local.length ? "没有匹配的本地分支。" : "没有本地分支。"}
        </p>
      {:else}
        <ul class="space-y-2">
          {#each localShown as branch (branch.name)}
            {@const offered = branchActions(branch)}
            <li class="space-y-1 rounded-lg border p-3 text-xs">
              <div class="flex min-w-0 items-start gap-1.5">
                <div class="flex min-w-0 flex-1 flex-wrap items-center gap-1.5">
                  <span class="font-mono text-sm font-medium break-all"
                    >{branch.name}</span
                  >
                  <span class="font-mono text-muted-foreground"
                    >{branch.head.slice(0, 8)}</span
                  >
                  {#if branch.name === data.defaultBranch}<Badge
                      variant="secondary">默认</Badge
                    >{:else if branch.mergedIntoDefault}<Badge variant="outline"
                      >已合入默认分支</Badge
                    >{/if}
                  {#if !branch.worktreePaths.length}<Button
                      size="xs"
                      variant="outline"
                      class="ml-auto"
                      aria-haspopup="dialog"
                      title={`在新工作树中检出 ${branch.name}`}
                      onclick={() =>
                        onnewworktree({
                          mode: "existing",
                          branch: branch.name,
                        })}>在新工作树打开…</Button
                    >{/if}
                </div>
                {#if onoperation && offered.actions.length}
                  <DropdownMenu.Root>
                    <DropdownMenu.Trigger>
                      {#snippet child({ props })}
                        <Button
                          {...props}
                          variant="ghost"
                          size="icon-sm"
                          class="-my-1"
                          aria-label={`分支操作：${branch.name}`}
                          title={`分支操作：${branch.name}`}
                          ><IconDots aria-hidden="true" /></Button
                        >
                      {/snippet}
                    </DropdownMenu.Trigger>
                    <DropdownMenu.Content align="end" class="min-w-44">
                      <BranchMenuItems target={branch} {onoperation} />
                    </DropdownMenu.Content>
                  </DropdownMenu.Root>
                {/if}
              </div>
              <p class="break-all text-muted-foreground">
                {branch.subject || "无标题提交"}{branch.committedAt
                  ? ` · ${relativeTime(branch.committedAt)}`
                  : ""}
              </p>
              <div class="flex min-w-0 flex-wrap items-center gap-1.5">
                <UpstreamStatus state={upstreamState(branch)} />
              </div>
              {#if offered.diverged}
                <p class="text-muted-foreground">{divergedText}。</p>
              {/if}
              {#each branch.worktreePaths as path (path)}
                <p class="flex min-w-0 flex-wrap items-baseline gap-1">
                  <span class="text-muted-foreground">检出于</span>
                  {#if worktreePaths.has(path)}
                    <ListButton
                      variant="code-link"
                      aria-haspopup="dialog"
                      title={`打开工作树详情：${path}`}
                      onclick={() => onopen(path)}>{path}</ListButton
                    >
                  {:else}<span class="font-mono break-all">{path}</span>{/if}
                </p>
              {/each}
            </li>
          {/each}
        </ul>
      {/if}
    </section>

    <Collapsible.Root
      open={remoteExpanded}
      onOpenChange={(value) => (remoteOpen = value)}
    >
      <section class="space-y-2" aria-label="远程分支">
        <Collapsible.Trigger disabled={!!term}>
          {#snippet child({ props })}
            <Button {...props} variant="ghost" size="sm" class="-ml-2">
              <IconChevronDown
                aria-hidden="true"
                class={remoteExpanded ? "" : "-rotate-90"}
              />
              <span class="font-medium">远程分支</span>
              <Badge variant="secondary"
                >{countText(remoteShown.length, remote.length)}</Badge
              >
            </Button>
          {/snippet}
        </Collapsible.Trigger>
        <Collapsible.Content forceMount hidden={!remoteExpanded}>
          {#if remoteShown.length === 0}
            <p class="text-xs text-muted-foreground">
              {remote.length ? "没有匹配的远程分支。" : "没有远程跟踪分支。"}
            </p>
          {:else}
            <ul class="space-y-1">
              {#each remoteShown as branch (branch.name)}
                <li
                  class="flex min-w-0 flex-wrap items-baseline gap-x-2 gap-y-0.5 border-b py-1.5 text-xs last:border-b-0"
                >
                  <span class="font-mono break-all">{branch.label}</span>
                  <span class="font-mono text-muted-foreground"
                    >{branch.head.slice(0, 8)}</span
                  >
                  <span class="min-w-0 break-all text-muted-foreground"
                    >{branch.subject || "无标题提交"}</span
                  >
                  {#if !branch.label.endsWith("/HEAD")}<Button
                      size="xs"
                      variant="outline"
                      class="ml-auto"
                      aria-haspopup="dialog"
                      title={`从 ${branch.label} 新建分支并在新工作树中打开`}
                      onclick={() =>
                        onnewworktree({
                          mode: "new",
                          startBranch: branch.label,
                          name: remoteLocalName(branch.label),
                        })}>在新工作树打开…</Button
                    >{/if}
                </li>
              {/each}
            </ul>
          {/if}
        </Collapsible.Content>
      </section>
    </Collapsible.Root>
  {/if}
</div>
