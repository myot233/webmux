<script lang="ts">
  import * as Command from "$lib/components/ui/command";

  // The searchable list of a repository's local and optional remote-tracking
  // branches, shared by the new-worktree form and quick start's base branch.
  // Rows are keyed by name (several branches can share a commit); a row with
  // a `reason` stays listed, disabled, with the reason under it.
  type BranchItem = { name: string; reason?: string };

  let {
    label,
    branches,
    remoteBranches = [],
    selected,
    loading = false,
    failed = false,
    disabled = false,
    search = $bindable(""),
    onselect,
  }: {
    /** The list's accessible name. */
    label: string;
    branches: readonly BranchItem[];
    remoteBranches?: readonly BranchItem[];
    selected: string;
    loading?: boolean;
    /** The branch list could not be read. */
    failed?: boolean;
    disabled?: boolean;
    search?: string;
    onselect: (name: string) => void;
  } = $props();
</script>

{#snippet branchOption(item: BranchItem)}
  <Command.Item
    value={item.name}
    disabled={disabled || !!item.reason}
    data-checked={selected === item.name}
    onSelect={() => onselect(item.name)}
  >
    <span class="min-w-0 flex-1 break-all">
      {item.name}
      {#if item.reason}
        <span class="block text-muted-foreground">{item.reason}</span>
      {/if}
    </span>
  </Command.Item>
{/snippet}

<Command.Root
  {label}
  class="h-auto min-w-0"
  filter={(value, query) =>
    value.toLowerCase().includes(query.trim().toLowerCase()) ? 1 : 0}
  disableInitialScroll
>
  <Command.Input
    bind:value={search}
    placeholder={remoteBranches.length ? "搜索分支…" : "搜索本地分支…"}
    aria-label={remoteBranches.length ? "搜索分支" : "搜索本地分支"}
    autocomplete="off"
    autocapitalize="off"
    autocorrect="off"
    spellcheck={false}
  />
  <Command.List aria-label="本地分支列表" class="max-h-[min(14rem,50dvh)] p-1">
    <Command.Empty>
      {loading
        ? "正在读取本地分支…"
        : failed
          ? "无法读取本地分支"
          : "没有匹配的本地分支"}
    </Command.Empty>
    {#if remoteBranches.length}
      {#if branches.length}
        <Command.Group heading="本地分支">
          {#each branches as item (item.name)}
            {@render branchOption(item)}
          {/each}
        </Command.Group>
      {/if}
      <Command.Group heading="远程分支">
        {#each remoteBranches as item (item.name)}
          {@render branchOption(item)}
        {/each}
      </Command.Group>
    {:else}
      {#each branches as item (item.name)}
        {@render branchOption(item)}
      {/each}
    {/if}
  </Command.List>
</Command.Root>
