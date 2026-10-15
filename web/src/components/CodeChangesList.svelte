<script lang="ts">
  import { onMount, tick } from "svelte";
  import type { CodeChange } from "../schemas";
  import { Button } from "$lib/components/ui/button";
  let {
    entries,
    total,
    nextCursor,
    version,
    pending,
    enabled,
    onmore,
    onopen,
    group = "",
  }: {
    entries: CodeChange[];
    total: number;
    nextCursor: string;
    version: string;
    pending: boolean;
    enabled: boolean;
    onmore: () => void;
    onopen: (change: CodeChange) => void;
    group?: string;
  } = $props();
  const rowHeight = 32,
    overscan = 6;
  const groups = [
    ["unstaged", "未暂存"],
    ["staged", "已暂存"],
    ["untracked", "未跟踪"],
  ];
  type Row = { key: string; label?: string; change?: CodeChange };
  const rows = $derived(
    groups
      .filter(([name]) => !group || name === group)
      .flatMap(([group, label]): Row[] => [
        { key: `group:${group}`, label },
        ...entries
          .filter((c) => c.group === group)
          .map((change) => ({ key: change.changeId, change })),
      ]),
  );
  let host: HTMLDivElement;
  let top = $state(0),
    height = $state(400);
  const start = $derived(Math.max(0, Math.floor(top / rowHeight) - overscan));
  const end = $derived(
    Math.min(rows.length, Math.ceil((top + height) / rowHeight) + overscan),
  );
  $effect(() => {
    version;
    top = 0;
    if (host) host.scrollTop = 0;
  });
  onMount(() => {
    const observer = new ResizeObserver(() => {
      height = host.clientHeight;
    });
    observer.observe(host);
    height = host.clientHeight;
    return () => observer.disconnect();
  });
  function scroll() {
    top = host.scrollTop;
    if (
      enabled &&
      !pending &&
      nextCursor &&
      !document.hidden &&
      top + height >= rows.length * rowHeight - 4 * rowHeight
    )
      onmore();
  }
  async function keyboard(event: KeyboardEvent, index: number) {
    let target = index;
    if (event.key === "ArrowDown") target++;
    else if (event.key === "ArrowUp") target--;
    else if (event.key === "Home") target = 0;
    else if (event.key === "End") target = rows.length - 1;
    else return;
    event.preventDefault();
    const direction = event.key === "ArrowUp" || event.key === "End" ? -1 : 1;
    while (target >= 0 && target < rows.length && !rows[target].change)
      target += direction;
    if (target < 0 || target >= rows.length) {
      if (target >= rows.length && nextCursor && !pending && enabled) onmore();
      return;
    }
    if (target * rowHeight < top) host.scrollTop = target * rowHeight;
    else if ((target + 1) * rowHeight > top + height)
      host.scrollTop = (target + 1) * rowHeight - height;
    top = host.scrollTop;
    await tick();
    host
      .querySelector<HTMLButtonElement>(`[data-row-index="${target}"] button`)
      ?.focus({ preventScroll: true });
  }
</script>

<div
  class="min-h-0 flex-1 overflow-auto"
  bind:this={host}
  onscroll={scroll}
  role="list"
  aria-label="修改列表"
>
  <div
    class="relative"
    style:height={`${rows.length * rowHeight}px`}
    role="presentation"
  >
    {#each rows.slice(start, end) as row, offset (row.key)}
      <div
        class="absolute left-0 right-0"
        style:top={`${(start + offset) * rowHeight}px`}
        style:height={`${rowHeight}px`}
        data-row-index={start + offset}
        role="listitem"
      >
        {#if row.change}
          <button
            class="block h-full w-full truncate rounded px-2 text-left text-xs hover:bg-muted"
            title={`${row.change.status} ${row.change.oldPath === row.change.newPath ? row.change.newPath : `${row.change.oldPath} → ${row.change.newPath}`}`}
            onclick={() => onopen(row.change!)}
            onkeydown={(event) => void keyboard(event, start + offset)}
            >{row.change.status} {row.change.newPath}</button
          >
        {:else}<h3 class="px-2 py-1 text-xs font-medium">{row.label}</h3>{/if}
      </div>
    {/each}
  </div>
</div>
<div class="shrink-0 border-t p-2 text-xs">
  <p>已加载 {entries.length} / {total} 项</p>
  {#if nextCursor}<Button
      variant="outline"
      size="sm"
      disabled={pending || !enabled}
      onclick={onmore}>{pending ? "加载中…" : "加载更多修改"}</Button
    >
  {:else if !pending && total === 0}<p>没有匹配的修改</p>{/if}
</div>
