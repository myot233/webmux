<script lang="ts">
  import IconTerminal2 from "~icons/tabler/terminal-2";
  import IconX from "~icons/tabler/x";
  import { z } from "zod";
  import { Button } from "$lib/components/ui/button";
  import * as Command from "$lib/components/ui/command";
  import * as Dialog from "$lib/components/ui/dialog";
  import { api } from "../api";
  import { errorMessage } from "../error";
  import type { Repositories } from "../hooks/repositories.svelte";
  import type { SessionList } from "../hooks/session-list.svelte";
  import {
    RepositoriesSchema,
    SessionInfoSchema,
    type Repository,
    type SessionInfo,
  } from "../schemas";
  import { sessionPins } from "../session-pins.svelte";
  import SessionStatus from "./SessionStatus.svelte";

  let {
    open = $bindable(false),
    source,
    onselect,
    oncancel,
  }: {
    open: boolean;
    source?: { sessions: SessionList; repositories: Repositories };
    onselect: (id: string) => void;
    oncancel: () => void;
  } = $props();

  let input = $state<HTMLInputElement | null>(null);
  let snapshot = $state.raw<SessionInfo[] | null>(null);
  let repositories = $state.raw<Repository[]>([]);
  let loadingRepositories = $state(false);
  let error = $state("");
  let selected: string | undefined;
  const sessions = $derived(source?.sessions.sessions ?? snapshot);
  const loading = $derived(source?.sessions.loading ?? snapshot === null);
  const loadError = $derived(source?.sessions.error ?? error);
  const loadingMetadata = $derived(
    source ? !source.repositories.settled : loadingRepositories,
  );
  const metadata = $derived(source?.repositories.list ?? repositories);
  const repositoryById = $derived(
    new Map(metadata.map((repository) => [repository.id, repository])),
  );
  const pinned = $derived(sessionPins.ordered(sessions ?? []));
  const resumable = $derived(
    sessions?.filter(
      (session) => session.status !== "exited" && !sessionPins.has(session.id),
    ) ?? [],
  );
  const exited = $derived(
    sessions?.filter((session) => session.status === "exited") ?? [],
  );

  $effect(() => {
    snapshot = null;
    repositories = [];
    loadingRepositories = false;
    error = "";
    if (!open || source) return;
    const controller = new AbortController();
    const { signal } = controller;
    void api("/sessions", z.array(SessionInfoSchema), "GET", undefined, signal)
      .then((result) => {
        if (signal.aborted) return;
        snapshot = result;
        if (result.some((session) => session.workspace)) {
          // A slow or unavailable Git registry should not block session access.
          loadingRepositories = true;
          void api(
            "/repositories",
            RepositoriesSchema,
            "GET",
            undefined,
            signal,
          )
            .then((result) => {
              if (!signal.aborted) repositories = result;
            })
            .catch(() => {})
            .finally(() => {
              if (!signal.aborted) loadingRepositories = false;
            });
        }
      })
      .catch((err: unknown) => {
        if (!signal.aborted) error = errorMessage(err);
      });
    return () => controller.abort();
  });
</script>

{#snippet sessionOption(session: SessionInfo)}
  {@const repository = repositoryById.get(
    session.workspace?.repositoryId ?? "",
  )}
  {@const branch = repository?.worktrees.find(
    (tree) => tree.id === session.workspace?.worktreeId,
  )?.branch}
  {@const detail = [repository?.name, branch, session.cwd]
    .filter(Boolean)
    .join(" · ")}
  <Command.Item
    value={`${session.title} ${detail} ${session.id}`}
    onSelect={() => {
      selected = session.id;
      open = false;
    }}
  >
    <IconTerminal2 class="size-4 text-muted-foreground" aria-hidden="true" />
    <span class="min-w-0 flex-1">
      <span class="block truncate text-sm font-medium" title={session.title}>
        {session.title}
      </span>
      <span class="block truncate text-xs text-muted-foreground" title={detail}>
        {detail}
      </span>
    </span>
    <SessionStatus {session} />
  </Command.Item>
{/snippet}

<Dialog.Root bind:open>
  <Dialog.Content
    class="flex flex-col sm:max-w-lg"
    showCloseButton={false}
    onOpenAutoFocus={(event) => {
      event.preventDefault();
      input?.focus();
    }}
    onCloseAutoFocus={(event) => {
      event.preventDefault();
      const id = selected;
      selected = undefined;
      if (id) onselect(id);
      else oncancel();
    }}
  >
    <Dialog.Header class="shrink-0 pr-8 text-left">
      <Dialog.Title>快速切换会话</Dialog.Title>
      <Dialog.Description
        >查找所有仓库和直接会话，回到手头的工作。</Dialog.Description
      >
    </Dialog.Header>
    <Dialog.Close>
      {#snippet child({ props })}
        <Button
          {...props}
          variant="ghost"
          size="icon-sm"
          class="absolute top-2 right-2"
          aria-label="关闭会话切换"
          title="关闭会话切换"
        >
          <IconX class="size-4" aria-hidden="true" />
        </Button>
      {/snippet}
    </Dialog.Close>
    <Command.Root
      label="搜索所有会话"
      class="min-h-0"
      filter={(value, search) =>
        value.toLocaleLowerCase().includes(search.trim().toLocaleLowerCase())
          ? 1
          : 0}
    >
      <Command.Input
        bind:ref={input}
        placeholder="搜索会话、目录、仓库或分支"
        autocomplete="off"
        autocapitalize="off"
        spellcheck={false}
      />
      <Command.List
        aria-label="匹配的会话"
        class="max-h-[min(25rem,50dvh)] min-h-0 overscroll-contain"
      >
        {#if sessions?.length}
          <Command.Empty>
            {loadingMetadata
              ? "正在读取仓库和分支…"
              : "找不到匹配的会话。试试名称或目录的一部分。"}
          </Command.Empty>
          {#if pinned.length}
            <Command.Group heading="已置顶">
              {#each pinned as session (session.id)}
                {@render sessionOption(session)}
              {/each}
            </Command.Group>
          {/if}
          {#if resumable.length}
            <Command.Group heading="可续接">
              {#each resumable as session (session.id)}
                {@render sessionOption(session)}
              {/each}
            </Command.Group>
          {/if}
          {#if exited.length}
            <Command.Group heading="已退出">
              {#each exited as session (session.id)}
                {@render sessionOption(session)}
              {/each}
            </Command.Group>
          {/if}
        {/if}
      </Command.List>
      {#if loadError && !sessions?.length}
        <p role="alert" class="py-3 text-sm text-destructive">
          读取会话失败：{loadError}
        </p>
      {:else if loading}
        <p role="status" class="py-3 text-sm text-muted-foreground">
          正在读取会话…
        </p>
      {:else if !sessions?.length}
        <p role="status" class="py-3 text-sm text-muted-foreground">
          暂无会话。可在工作区新建会话。
        </p>
      {/if}
    </Command.Root>
    <div
      class="hidden shrink-0 items-center gap-4 text-xs text-muted-foreground sm:flex"
    >
      <span><kbd>↑ ↓</kbd> 选择</span>
      <span><kbd>Enter</kbd> 打开</span>
      <span class="ml-auto"><kbd>Esc</kbd> 关闭</span>
    </div>
  </Dialog.Content>
</Dialog.Root>
