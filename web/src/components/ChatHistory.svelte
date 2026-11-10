<script lang="ts" module>
  import type {
    ArchivedSession,
    LaunchConfigSummary,
    Repository,
    SessionInfo,
    Worktree,
  } from "../schemas";

  /**
   * Live sessions with a chat view and archived sessions in a stable order.
   * Live chats run newest created first, ties by ID, as the workbench tree
   * and the session list do: output, status changes and renames never move
   * a row, and a new chat appears on top. Archived records run newest
   * `archive.updatedAt` first, ties by ID; that time only moves when the
   * user archives, resumes or edits the record, never from background
   * activity. Terminal-only sessions are left out, and a live session wins
   * over a record with its ID.
   */
  export function chatHistorySections(
    sessions: readonly SessionInfo[],
    records: readonly ArchivedSession[],
  ): { live: SessionInfo[]; archived: ArchivedSession[] } {
    const live = sessions
      .filter(
        (session) =>
          session.views.includes("chat") && session.state === "active",
      )
      .sort(
        (a, b) =>
          Date.parse(b.createdAt) - Date.parse(a.createdAt) ||
          a.id.localeCompare(b.id),
      );
    const ids = new Set(live.map((session) => session.id));
    const archived = records
      .filter((record) => !ids.has(record.id))
      .sort(
        (a, b) =>
          Date.parse(b.archive.updatedAt) - Date.parse(a.archive.updatedAt) ||
          a.id.localeCompare(b.id),
      );
    return { live, archived };
  }
</script>

<script lang="ts">
  import IconChevronDown from "~icons/tabler/chevron-down";
  import IconLoader2 from "~icons/tabler/loader-2";
  import { Badge } from "$lib/components/ui/badge";
  import { Button } from "$lib/components/ui/button";
  import * as Collapsible from "$lib/components/ui/collapsible";
  import { ListButton } from "$lib/components/ui/list-button";
  import { releasableRecordWorktree } from "../archived";
  import LaunchConfigIcon from "./LaunchConfigIcon.svelte";
  import RowStatus from "./RowStatus.svelte";
  import SidebarLiveSessionRow from "./SidebarLiveSessionRow.svelte";
  import SidebarSessionMenu from "./SidebarSessionMenu.svelte";

  // The chat-mode sidebar: live chats flat in a stable order (newest created
  // first, as the workbench tree), with archived
  // records folded below them. The current row carries aria-current and every
  // row ends in a status dot; the "⋯" menu offers a live chat's rename and
  // archive, and an archived one's resume, space release, record deletion
  // and record-plus-worktree deletion. Archived rows also resume when
  // clicked; a released one is marked so.
  let {
    sessions,
    records,
    configs,
    repositories,
    current,
    resuming = "",
    onopen,
    onresume,
    onrename,
    onclose,
    onrelease,
    onremove,
    onremoveworktree,
  }: {
    sessions: readonly SessionInfo[];
    records: readonly ArchivedSession[];
    configs: readonly LaunchConfigSummary[];
    /** Resolve which archived records' worktrees can be released. */
    repositories: readonly Repository[];
    /** The session page shown, or "" on a home page. */
    current: string;
    /** The record being resumed; its row shows the progress. */
    resuming?: string;
    onopen: (session: SessionInfo) => void;
    onresume: (record: ArchivedSession) => void;
    onrename: (session: SessionInfo) => void;
    onclose: (session: SessionInfo) => void;
    /** Asks to release the disk space of an archived record's worktree. */
    onrelease: (worktree: Worktree) => void;
    /** Asks to delete an archived record; its worktree stays. */
    onremove: (record: ArchivedSession) => void;
    /** Asks to delete an archived record and its linked worktree. */
    onremoveworktree?: (record: ArchivedSession, worktree: Worktree) => void;
  } = $props();
  // The archived list mounts a page at a time while the section is open: a
  // long history then costs nothing behind a closed section and never builds
  // every row at once.
  const PAGE = 20;
  const { live, archived } = $derived(chatHistorySections(sessions, records));
  let archivedExpanded = $state(false);
  let archivedOverride = $state<boolean | undefined>();
  let archivedLimit = $state(PAGE);
  const currentInArchived = $derived(
    archived.some((record) => record.id === current || record.id === resuming),
  );
  $effect(() => {
    void currentInArchived;
    archivedOverride = undefined;
  });
  const archivedOpen = $derived(
    archivedOverride ?? (currentInArchived || archivedExpanded),
  );
  $effect(() => {
    if (!archivedOpen) archivedLimit = PAGE;
  });
  const archivedShown = $derived(archived.slice(0, archivedLimit));
  const archivedRemaining = $derived(archived.length - archivedShown.length);
</script>

<h2 class="px-3 pt-1 pb-1 text-xs text-muted-foreground">最近</h2>
{#if live.length || archived.length}
  {#if live.length}
    <ul class="space-y-0.5" aria-label="聊天历史">
      {#each live as session (session.id)}
        <SidebarLiveSessionRow
          {session}
          {current}
          icon={session.agent?.icon ?? ""}
          {onopen}
          {onrename}
          {onclose}
        />
      {/each}
    </ul>
  {/if}
  {#if archived.length}
    <Collapsible.Root
      open={archivedOpen}
      onOpenChange={(value) => {
        if (currentInArchived) archivedOverride = value;
        else archivedExpanded = value;
      }}
      class={live.length ? "mt-2" : ""}
    >
      <Collapsible.Trigger>
        {#snippet child({ props })}
          <Button
            {...props}
            variant="ghost"
            size="sm"
            class="w-full justify-start"
          >
            <IconChevronDown
              aria-hidden="true"
              class={archivedOpen ? "" : "-rotate-90"}
            />
            <span class="font-medium text-muted-foreground">已归档</span>
            <Badge variant="secondary" class="ml-auto">{archived.length}</Badge>
          </Button>
        {/snippet}
      </Collapsible.Trigger>
      <Collapsible.Content forceMount hidden={!archivedOpen} class="mt-0.5">
        {#if archivedOpen}
          <ul class="space-y-0.5" aria-label="已归档聊天">
            {#each archivedShown as record (record.id)}
              {@const title = record.title || record.agent.name}
              {@const releasable = releasableRecordWorktree(
                record,
                repositories,
              )}
              {@const state = record.released ? "已释放空间，已结束" : "已结束"}
              <li class="group/row relative">
                <!-- The same end padding as live rows keeps every status in
                     one column. -->
                <ListButton
                  variant="navigation-action"
                  aria-current={record.id === current ? "page" : undefined}
                  aria-label={`${title}（${resuming === record.id ? "正在恢复" : state}）`}
                  title={record.released
                    ? `${title}（${record.agent.name} · 已释放空间：${record.released.path} · ${record.released.commit.slice(0, 12)}，点击重建并恢复）`
                    : `${title}（${record.agent.name} · 已结束，点击恢复）`}
                  aria-busy={resuming === record.id}
                  disabled={!!resuming}
                  onclick={() => onresume(record)}
                >
                  <LaunchConfigIcon
                    icon={configs.find(
                      (config) => config.id === record.configId,
                    )?.icon ?? ""}
                    size="sm"
                  />
                  <span class="min-w-0 flex-1 truncate text-muted-foreground"
                    >{title}</span
                  >
                  {#if resuming === record.id}<span
                      class="flex shrink-0 items-center gap-1 text-xs font-normal text-muted-foreground"
                      ><IconLoader2
                        class="size-3.5 animate-spin motion-reduce:animate-none"
                        aria-hidden="true"
                      />恢复中…</span
                    >{:else}<RowStatus
                      status={record.released ? "released" : "ended"}
                    />{/if}
                </ListButton>
                <SidebarSessionMenu
                  label={title}
                  disabled={!!resuming}
                  target={{
                    kind: "archived",
                    onresume: () => onresume(record),
                    onrelease: releasable
                      ? () => onrelease(releasable)
                      : undefined,
                    onremove: () => onremove(record),
                    onremoveworktree:
                      releasable && onremoveworktree
                        ? () => onremoveworktree(record, releasable)
                        : undefined,
                  }}
                />
              </li>
            {/each}
          </ul>
          {#if archivedRemaining > 0}
            <Button
              variant="link"
              size="sm"
              class="w-full justify-start"
              onclick={() => (archivedLimit += PAGE)}
            >
              显示更多（还有 {archivedRemaining} 条）
            </Button>
          {/if}
        {/if}
      </Collapsible.Content>
    </Collapsible.Root>
  {/if}
{:else}
  <p class="px-3 py-2 text-sm text-muted-foreground">
    还没有聊天。写下任务，发送后就会出现在这里。
  </p>
{/if}
