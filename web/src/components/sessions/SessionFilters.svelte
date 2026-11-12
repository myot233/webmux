<script lang="ts">
  import { MediaQuery } from "svelte/reactivity";
  import IconSearch from "~icons/tabler/search";
  import { Button } from "$lib/components/ui/button";
  import * as InputGroup from "$lib/components/ui/input-group";
  import * as SegmentedControl from "$lib/components/ui/segmented-control";
  import type { SessionInfo } from "../../contracts/schemas";
  import {
    countSessions,
    type StatusFilter,
  } from "../../domains/sessions/session-status";

  let {
    sessions,
    archivedCount,
    query = $bindable(),
    statusFilter = $bindable(),
    filtered,
    shortcutDisabled,
    onclear,
  }: {
    // Unfiltered list: the filter counts never depend on the active filter.
    sessions: readonly SessionInfo[];
    // Archived hybrid records, also unfiltered.
    archivedCount: number;
    query: string;
    statusFilter: StatusFilter;
    // Filters are active and at least one session still matches.
    filtered: boolean;
    // A dialog or sheet owns the keyboard.
    shortcutDisabled: boolean;
    onclear: () => void;
  } = $props();

  let searchInput = $state<HTMLInputElement | null>(null);
  const wide = new MediaQuery("min-width: 768px");
  let group = $state<HTMLElement | null>(null);
  // The single-row filter scrolls sideways on phones; keep the choice visible.
  $effect(() => {
    void statusFilter;
    group
      ?.querySelector('[role="radio"][aria-checked="true"]')
      ?.scrollIntoView({ inline: "nearest", block: "nearest" });
  });
  const filters = $derived.by(() => {
    const counts = countSessions(sessions);
    return [
      { id: "all", label: "全部", count: sessions.length },
      { id: "active", label: "运行中", count: counts.active },
      { id: "waiting", label: "等待中", count: counts.waiting },
      { id: "exited", label: "已退出", count: counts.exited },
      { id: "archived", label: "已归档", count: archivedCount },
    ] as const;
  });

  function searchShortcut(event: KeyboardEvent) {
    if (
      event.defaultPrevented ||
      event.isComposing ||
      event.metaKey ||
      event.ctrlKey ||
      event.altKey ||
      shortcutDisabled
    )
      return;
    const target = event.target;
    if (
      event.key !== "/" ||
      (target instanceof HTMLElement &&
        (target.isContentEditable ||
          target.closest('input, textarea, select, [role="dialog"]')))
    )
      return;
    event.preventDefault();
    searchInput?.focus();
  }
</script>

<svelte:window onkeydown={searchShortcut} />

<div class="flex flex-col gap-2 md:flex-row md:items-center md:gap-4">
  <InputGroup.Root class="min-w-0 md:flex-1">
    <InputGroup.Addon>
      <IconSearch aria-hidden="true" />
    </InputGroup.Addon>
    <InputGroup.Input
      type="search"
      aria-label="搜索会话"
      aria-keyshortcuts="/"
      title="按 / 搜索，Esc 清除筛选"
      placeholder={wide.current ? "搜索会话、仓库、目录或分支" : "搜索会话"}
      bind:ref={searchInput}
      bind:value={query}
      onkeydown={(event) => {
        if (event.key === "Escape" && !event.isComposing) {
          event.preventDefault();
          onclear();
        }
      }}
    />
    <InputGroup.Addon align="inline-end" class="hidden md:flex">
      <kbd class="border px-1" aria-hidden="true">/</kbd>
    </InputGroup.Addon>
  </InputGroup.Root>
  <SegmentedControl.Root
    bind:ref={group}
    value={statusFilter}
    onValueChange={(value) => {
      const next = filters.find((filter) => filter.id === value);
      if (next) statusFilter = next.id;
    }}
    size="form"
    aria-label="按会话状态筛选"
    class="no-scrollbar w-full justify-start overflow-x-auto overflow-y-hidden md:w-fit md:overflow-visible"
  >
    {#each filters as filter (filter.id)}
      <SegmentedControl.Item value={filter.id} class="min-h-9 shrink-0">
        {filter.label}
        <span
          class="inline-block w-[3ch] text-right font-mono text-xs tabular-nums opacity-60"
          >{filter.count}</span
        >
      </SegmentedControl.Item>
    {/each}
  </SegmentedControl.Root>
</div>
{#if filtered}
  <div class="flex items-center gap-2 text-xs text-muted-foreground">
    <span>已筛选会话</span>
    <Button
      variant="link"
      size="sm"
      class="min-h-9 sm:min-h-8"
      onclick={onclear}>清除筛选</Button
    >
  </div>
{/if}
