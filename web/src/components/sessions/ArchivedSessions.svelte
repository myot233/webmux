<script lang="ts">
  import IconChevronDown from "~icons/tabler/chevron-down";
  import IconDots from "~icons/tabler/dots";
  import IconMessageCircle from "~icons/tabler/message-circle";
  import IconTerminal2 from "~icons/tabler/terminal-2";
  import { Badge } from "$lib/components/ui/badge";
  import { Button } from "$lib/components/ui/button";
  import * as Collapsible from "$lib/components/ui/collapsible";
  import * as DropdownMenu from "$lib/components/ui/dropdown-menu";
  import * as Item from "$lib/components/ui/item";
  import ConfirmDialog from "../shared/ConfirmDialog.svelte";
  import HybridRecordHints from "./HybridRecordHints.svelte";
  import HybridResumeActions from "./HybridResumeActions.svelte";
  import InlineNotice from "../shared/InlineNotice.svelte";
  import ReleasedBadge from "../workspace/ReleasedBadge.svelte";
  import { errorMessage } from "../../platform/error";
  import type { ArchiveActions } from "../../domains/sessions/archived";
  import type { ArchivedSession, Worktree } from "../../contracts/schemas";

  // The archived (closed or interrupted) hybrid sessions of one workspace
  // group, below its live sessions: collapsed to a count by default, newest
  // first, the first few at a time.
  const PAGE = 5;

  let {
    title,
    records,
    groupPath,
    worktree,
    archive,
  }: {
    // The workspace group's title, naming the section for assistive tech.
    title: string;
    records: ArchivedSession[];
    groupPath?: string;
    /**
     * The group's linked worktree when it can be removed or released; a
     * released record never targets it, deleting only itself.
     */
    worktree?: Worktree;
    archive: ArchiveActions;
  } = $props();

  let expanded = $state(false);
  // While filters reveal the section, a toggle only overrides that until the
  // reveal ends; the user's own choice stays.
  let override = $state<boolean | undefined>();
  let showAll = $state(false);
  let busy = $state("");
  let error = $state("");
  let removing = $state<ArchivedSession | null>(null);
  let removeOpen = $state(false);

  const reveal = $derived(archive.filtering && records.length > 0);
  $effect(() => {
    void reveal;
    override = undefined;
  });
  const open = $derived(override ?? (reveal || expanded));
  const shown = $derived(
    showAll || archive.filtering ? records : records.slice(0, PAGE),
  );
  const interrupted = $derived(
    records.filter((record) => record.state === "interrupted").length,
  );

  function recordTitle(record: ArchivedSession) {
    return record.title;
  }

  function since(record: ArchivedSession) {
    const at = Date.parse(
      record.archive.interruptedAt ?? record.archive.updatedAt,
    );
    const seconds = Math.max(0, Math.floor((archive.now - at) / 1000));
    if (seconds < 60) return "刚刚";
    if (seconds < 3600) return `${Math.floor(seconds / 60)} 分钟前`;
    if (seconds < 86400) return `${Math.floor(seconds / 3600)} 小时前`;
    return `${Math.floor(seconds / 86400)} 天前`;
  }

  async function confirmRemove() {
    const record = removing;
    removeOpen = false;
    if (!record) return;
    busy = record.id;
    error = "";
    try {
      await archive.remove(record.id);
    } catch (err) {
      error = `无法删除“${recordTitle(record)}”的记录：${errorMessage(err)}`;
    } finally {
      busy = "";
    }
  }
</script>

<section
  class="min-w-0"
  aria-label={`${title} 的已归档会话`}
  hidden={!records.length}
>
  <Collapsible.Root
    {open}
    onOpenChange={(value) => {
      if (reveal) override = value;
      else expanded = value;
    }}
  >
    <!-- The interrupted count sits beside the trigger, so large text on a
         narrow screen wraps it under the button instead of widening it. -->
    <div class="flex min-w-0 flex-wrap items-center gap-x-2 gap-y-1">
      <Collapsible.Trigger>
        {#snippet child({ props })}
          <Button {...props} variant="ghost" size="sm" class="-ml-2">
            <IconChevronDown
              aria-hidden="true"
              class={open ? "" : "-rotate-90"}
            />
            <span class="font-medium text-muted-foreground">已归档</span>
            <Badge variant="secondary">{records.length}</Badge>
          </Button>
        {/snippet}
      </Collapsible.Trigger>
      {#if interrupted}<Badge variant="warning">{interrupted} 条已中断</Badge
        >{/if}
    </div>
    <!-- Mounted while closed and hidden by hand, like the repository groups:
         a section opened by a reveal right after it mounted otherwise kept
         bits-ui's `hidden` on its content although its state was open. -->
    <Collapsible.Content
      forceMount
      hidden={!open}
      class="mt-2 grid min-w-0 grid-cols-1 gap-2"
    >
      {#if error}<InlineNotice tone="danger">{error}</InlineNotice>{/if}
      {#each shown as record (record.id)}
        {@const removableWorktree = record.released ? undefined : worktree}
        <Item.Root
          variant="muted"
          class="min-w-0 flex-col flex-nowrap items-stretch"
          data-archived-id={record.id}
        >
          <Item.Content class="min-w-0">
            <div class="flex min-w-0 flex-wrap items-center gap-x-2 gap-y-1">
              <Item.Title
                class="min-w-0 max-w-full"
                title={recordTitle(record)}
              >
                <span class="truncate">{recordTitle(record)}</span>
              </Item.Title>
              <Badge
                variant={record.state === "interrupted" ? "warning" : "outline"}
                >{record.state === "closed" ? "已关闭" : "已中断"}</Badge
              >
              {#if record.released}<ReleasedBadge
                  released={record.released}
                />{/if}
            </div>
            <Item.Description>
              <span class="flex flex-wrap gap-x-3 gap-y-1">
                <span>{record.agent.name}</span>
                <span class="inline-flex items-center gap-1"
                  >{#if record.view === "chat"}<IconMessageCircle
                      aria-hidden="true"
                    />聊天{:else}<IconTerminal2
                      aria-hidden="true"
                    />终端{/if}</span
                >
                <span>{since(record)}</span>
              </span>
            </Item.Description>
            {#if record.cwd !== groupPath}<p
                class="font-mono text-xs break-all text-muted-foreground"
              >
                {record.cwd}
              </p>{/if}
            <HybridRecordHints {record} />
          </Item.Content>
          <Item.Actions class="flex flex-wrap gap-2">
            <HybridResumeActions
              {record}
              resume={archive.resume}
              onresumed={archive.onresumed}
              onrebuilt={archive.onrebuilt}
              disabled={!!busy}
              onbusy={(active) => (busy = active ? record.id : "")}
              onerror={(message) => (error = message)}
            />
            <DropdownMenu.Root>
              <DropdownMenu.Trigger disabled={!!busy}>
                {#snippet child({ props })}
                  <Button
                    {...props}
                    size="icon-sm"
                    variant="ghost"
                    class="size-9 sm:size-8"
                    title={`更多操作 ${recordTitle(record)}`}
                    aria-label={`更多操作 ${recordTitle(record)}`}
                    ><IconDots class="size-4" aria-hidden="true" /></Button
                  >
                {/snippet}
              </DropdownMenu.Trigger>
              <DropdownMenu.Content align="end" class="min-w-48">
                {#if removableWorktree && archive.release}
                  {@const release = archive.release}
                  <DropdownMenu.Item onSelect={() => release(removableWorktree)}
                    >释放空间</DropdownMenu.Item
                  >
                  <DropdownMenu.Separator />
                {/if}
                <DropdownMenu.Item
                  variant="destructive"
                  onSelect={() => {
                    removing = record;
                    removeOpen = true;
                  }}>删除记录</DropdownMenu.Item
                >
                {#if removableWorktree && archive.removeRecordAndWorktree}
                  {@const removeRecordAndWorktree =
                    archive.removeRecordAndWorktree}
                  <DropdownMenu.Item
                    variant="destructive"
                    onSelect={() =>
                      removeRecordAndWorktree(record, removableWorktree)}
                    >删除记录和工作树</DropdownMenu.Item
                  >
                {/if}
              </DropdownMenu.Content>
            </DropdownMenu.Root>
          </Item.Actions>
        </Item.Root>
      {/each}
      {#if shown.length < records.length}<Button
          variant="link"
          size="sm"
          class="justify-self-start"
          onclick={() => (showAll = true)}>显示全部 {records.length} 条</Button
        >{/if}
    </Collapsible.Content>
  </Collapsible.Root>
</section>

<ConfirmDialog
  bind:open={removeOpen}
  title={`删除“${removing ? recordTitle(removing) : ""}”的记录？`}
  onconfirm={confirmRemove}
>
  只删除 WebMux 的记录与本机草稿；工作树与 agent 自己保存的会话不受影响。
</ConfirmDialog>
