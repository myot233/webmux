<script lang="ts">
  import type {
    ArchivedSession,
    SessionInfo,
    Worktree,
  } from "../../contracts/schemas";
  import type { ArchiveActions } from "../../domains/sessions/archived";
  import type { BranchOperation } from "../../domains/workspace/branch-actions";
  import IconFolder from "~icons/tabler/folder";
  import IconGitBranch from "~icons/tabler/git-branch";
  import IconPlus from "~icons/tabler/plus";
  import { Button } from "$lib/components/ui/button";
  import * as Item from "$lib/components/ui/item";
  import WorktreeBadges from "./WorktreeBadges.svelte";
  import WorktreeControls from "./WorktreeControls.svelte";
  import SessionRow from "../sessions/SessionRow.svelte";
  import ArchivedSessions from "../sessions/ArchivedSessions.svelte";
  import InlineNotice from "../shared/InlineNotice.svelte";
  import LazyDialog from "../shared/LazyDialog.svelte";
  let {
    title,
    path,
    worktree,
    sessions,
    visibleIds,
    hidden = false,
    separated = false,
    focused = false,
    compact = false,
    now,
    onclose,
    onstart,
    onremove,
    onswitch,
    pinned,
    ontogglepin,
    archived = [],
    archive,
    detailsOpen,
    ondetailschange,
    lastFetchedAt,
    onoperation,
  }: {
    title: string;
    path?: string;
    worktree?: Worktree;
    sessions: SessionInfo[];
    visibleIds?: ReadonlySet<string>;
    hidden?: boolean;
    /** Adds a divider before this worktree when it follows another group. */
    separated?: boolean;
    /** The page header already names this group: omit the repeated title. */
    focused?: boolean;
    compact?: boolean;
    now: number;
    onclose: (session: SessionInfo) => void;
    onstart?: () => void;
    onremove?: () => void;
    /** Opens the switch of the worktree's branch. */
    onswitch?: () => void;
    // Undefined outside curated repository lists.
    pinned?: boolean;
    ontogglepin?: () => void;
    // Closed and interrupted hybrid sessions placed in this group; already
    // narrowed by the home page's filters.
    archived?: ArchivedSession[];
    archive?: ArchiveActions;
    /** Set by the page header while focused; otherwise the group owns it. */
    detailsOpen?: boolean;
    ondetailschange?: (open: boolean) => void;
    /** The repository's last fetch, shown in the worktree details. */
    lastFetchedAt?: string | null;
    /** Starts a sync action of the worktree's branch (menu and details). */
    onoperation?: (operation: BranchOperation) => void;
  } = $props();
  let localOpen = $state(false);
  const visible = $derived(
    visibleIds
      ? sessions.filter((session) => visibleIds.has(session.id))
      : sessions,
  );
  const groupPath = $derived(path ?? worktree?.path);
  const panelOpen = $derived(detailsOpen ?? localOpen);
  function setDetails(open: boolean) {
    localOpen = open;
    ondetailschange?.(open);
  }
</script>

{#snippet notices()}
  {#if worktree?.lockReason}<p
      class="w-full text-xs break-all whitespace-pre-wrap text-muted-foreground"
    >
      锁定原因：{worktree.lockReason}
    </p>{/if}
  {#if worktree?.error}<InlineNotice tone="danger" class="w-full">
      {worktree.error}
    </InlineNotice>{/if}
{/snippet}

<section
  class={separated
    ? "mt-8 min-w-0 space-y-3 border-t border-border pt-8"
    : "min-w-0 space-y-3"}
  data-worktree-id={worktree?.id}
  {hidden}
  aria-label={title}
>
  {#if !focused}
    <div class="flex min-w-0 flex-wrap items-center gap-x-4 gap-y-2">
      <div
        class="flex w-full min-w-0 flex-none flex-wrap items-center gap-2 sm:w-auto sm:flex-1"
      >
        {#if worktree}
          <IconGitBranch
            class="size-4 shrink-0 text-muted-foreground"
            aria-hidden="true"
          />
        {:else}
          <IconFolder
            class="size-4 shrink-0 text-muted-foreground"
            aria-hidden="true"
          />
        {/if}
        <h3 class="min-w-0 break-all text-sm font-semibold">{title}</h3>
        {#if visible.length > 1}<span
            class="shrink-0 text-xs text-muted-foreground"
            >{visible.length}</span
          >{/if}
        {#if worktree}<WorktreeBadges {worktree} {pinned} />{/if}
      </div>
      <div
        class="ml-auto flex max-w-full flex-wrap items-center justify-end gap-1"
      >
        {#if onstart}<Button
            size="sm"
            variant="outline"
            class="min-h-9 sm:min-h-8"
            disabled={worktree &&
              (!worktree.available || worktree.kind === "bare")}
            aria-label="新建会话"
            title="新建会话"
            onclick={onstart}
            ><IconPlus class="size-4" aria-hidden="true" /><span
              class="hidden sm:inline">新建会话</span
            ></Button
          >{/if}
        <WorktreeControls
          {title}
          kind={worktree ? "worktree" : "directory"}
          ondetails={() => setDetails(true)}
          {hidden}
          {pinned}
          {ontogglepin}
          {onswitch}
          {onremove}
          {worktree}
          {onoperation}
        />
      </div>
      {@render notices()}
    </div>
  {:else if worktree?.lockReason || worktree?.error}
    <div class="flex min-w-0 flex-wrap gap-2">{@render notices()}</div>
  {/if}
  {#if sessions.length}
    <Item.Group
      variant={compact ? "panel" : "default"}
      class={compact ? undefined : "gap-3"}
      aria-label={`${title} 的会话`}
    >
      {#each sessions as session (session.id)}
        <SessionRow
          {session}
          {compact}
          {now}
          {groupPath}
          {onclose}
          hidden={hidden || Boolean(visibleIds && !visibleIds.has(session.id))}
        />
      {/each}
    </Item.Group>
  {:else if !archived.length}
    <p class={["py-4 text-xs text-muted-foreground", !focused && "border-t"]}>
      暂无会话。工作树独立保留，可以随时启动。
    </p>
  {/if}
  {#if archive && archived.length}<ArchivedSessions
      {title}
      records={archived}
      {groupPath}
      worktree={worktree &&
      worktree.kind === "linked" &&
      worktree.available &&
      !worktree.prunable
        ? worktree
        : undefined}
      {archive}
    />{/if}
</section>
{#if panelOpen && !hidden}
  <LazyDialog
    load={() => import("./WorktreeDetailsDialog.svelte")}
    label="详情"
    onclose={() => setDetails(false)}
  >
    {#snippet children(page)}<page.default
        {title}
        path={groupPath}
        {lastFetchedAt}
        {worktree}
        sessions={visible}
        {onoperation}
        onclose={() => setDetails(false)}
      />{/snippet}
  </LazyDialog>
{/if}
