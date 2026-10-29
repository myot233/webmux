<script lang="ts">
  import { onMount } from "svelte";
  import IconChevronDown from "~icons/tabler/chevron-down";
  import IconFolder from "~icons/tabler/folder";
  import IconList from "~icons/tabler/list";
  import IconTerminal2 from "~icons/tabler/terminal-2";
  import { Button } from "$lib/components/ui/button";
  import { Badge } from "$lib/components/ui/badge";
  import { ListButton } from "$lib/components/ui/list-button";
  import type { Repository, SessionInfo } from "../schemas";
  import { observeStickyHeaderOffset } from "../hooks/sticky-header";

  let {
    repositories,
    sessions,
    directSessions,
    selected,
    label,
    onselect,
  }: {
    repositories: Repository[];
    sessions: SessionInfo[];
    directSessions: SessionInfo[];
    selected: string;
    label: string;
    onselect: (id: string) => void;
  } = $props();
  const navigationId = $props.id();
  let open = $state(false);
  let trigger = $state<HTMLButtonElement | null>(null);
  let sidebar: HTMLElement;
  onMount(() =>
    observeStickyHeaderOffset((height) => {
      sidebar.style.setProperty("--sticky-header-offset", `${height}px`);
    }),
  );
  const entries = $derived([
    {
      id: "all",
      name: "全部仓库",
      icon: IconList,
      count: sessions.length,
      waiting: sessions.filter((session) => session.status === "waiting")
        .length,
      path: "",
    },
    ...repositories.map((repo) => {
      const items = sessions.filter(
        (session) => session.workspace?.repositoryId === repo.id,
      );
      return {
        id: repo.id,
        name: repo.name,
        icon: IconFolder,
        count: items.length,
        waiting: items.filter((session) => session.status === "waiting").length,
        path: repo.anchorPath,
      };
    }),
    {
      id: "direct",
      name: "直接会话",
      icon: IconTerminal2,
      count: directSessions.length,
      waiting: directSessions.filter((session) => session.status === "waiting")
        .length,
      path: "",
    },
  ]);
  function select(id: string) {
    onselect(id);
    open = false;
    if (trigger?.getClientRects().length) trigger.focus();
  }
  function parentPath(path: string) {
    const parent = path.slice(0, path.lastIndexOf("/")) || "/";
    const segments = parent.split("/").filter(Boolean);
    return segments.length > 2 ? `…/${segments.slice(-2).join("/")}` : parent;
  }
</script>

<aside
  bind:this={sidebar}
  class="min-w-0 lg:sticky lg:top-[calc(var(--sticky-header-offset,0px)+1.5rem)] lg:self-start lg:rounded-xl lg:border lg:bg-card lg:p-2"
>
  <Button
    variant="outline"
    size="sm-touch"
    class="w-full min-w-0 justify-between lg:hidden"
    aria-label={`仓库导航：${label}`}
    aria-expanded={open}
    aria-controls={navigationId}
    bind:ref={trigger}
    onclick={() => (open = !open)}
  >
    <IconFolder aria-hidden="true" />
    <span class="min-w-0 flex-1 truncate text-left">{label}</span>
    <IconChevronDown class={open ? "rotate-180" : ""} aria-hidden="true" />
  </Button>
  <nav
    id={navigationId}
    aria-label="仓库导航"
    class={open ? "mt-3 lg:mt-0" : "hidden lg:block"}
  >
    <div
      class="mb-2 flex items-center justify-between px-3 pt-2 text-xs font-medium text-muted-foreground"
    >
      <span>仓库</span><span class="tabular-nums">{repositories.length}</span>
    </div>
    <div
      data-slot="repository-list"
      class="max-h-[50dvh] space-y-1 overflow-y-auto overscroll-contain p-1 lg:max-h-[calc(100dvh-var(--sticky-header-offset,0px)-7rem)]"
    >
      {#each entries as entry (entry.id)}
        <ListButton
          variant="navigation"
          aria-label={entry.path ? `仓库 ${entry.name}` : entry.name}
          aria-describedby={entry.path
            ? `${navigationId}-${entry.id}-path`
            : undefined}
          aria-pressed={selected === entry.id}
          data-repository-scope={entry.id}
          onclick={() => select(entry.id)}
        >
          <entry.icon class="size-4 shrink-0" aria-hidden="true" />
          <span class="min-w-0 flex-1">
            <span class="block truncate">{entry.name}</span>
            {#if entry.path}
              <span id={`${navigationId}-${entry.id}-path`} class="sr-only"
                >{entry.path}</span
              >
              <span
                class="block truncate text-xs font-normal text-muted-foreground"
                title={entry.path}>{parentPath(entry.path)}</span
              >
            {/if}
          </span>
          {#if entry.waiting}<Badge
              variant="warning"
              class="shrink-0"
              title={`等待 ${entry.waiting} 个会话`}>待 {entry.waiting}</Badge
            >{/if}
          <Badge variant="secondary" class="min-w-6 shrink-0 justify-center"
            ><span class="tabular-nums">{entry.count}</span></Badge
          >
        </ListButton>
      {/each}
    </div>
  </nav>
</aside>
