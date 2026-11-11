<script lang="ts">
  import { Label } from "$lib/components/ui/label";
  import * as Select from "$lib/components/ui/select";
  import {
    commitChoiceLabel,
    type CommitChoice,
  } from "../../domains/workspace/commit-choices";

  let {
    commit = $bindable(""),
    commitRows,
    commitsRead = false,
    commitsError = "",
    busy = false,
  }: {
    commit: string;
    commitRows: readonly CommitChoice[];
    commitsRead?: boolean;
    commitsError?: string;
    busy?: boolean;
  } = $props();

  const chosenCommit = $derived(commitRows.find((row) => row.value === commit));
</script>

<div class="space-y-2">
  <Label for="switch-commit">提交</Label>
  <Select.Root
    type="single"
    bind:value={commit}
    disabled={busy || !commitRows.length}
  >
    <Select.Trigger id="switch-commit" class="w-full">
      {#if chosenCommit}
        <span class="min-w-0 flex-1 truncate" title={chosenCommit.subject}
          >{chosenCommit.subject} · {chosenCommit.shortHash}</span
        >
      {:else}
        <span class="text-muted-foreground">
          {commitsRead ? "选择提交" : "正在读取最近提交…"}
        </span>
      {/if}
    </Select.Trigger>
    <Select.Content
      class="max-h-[min(24rem,var(--bits-select-content-available-height))]"
    >
      {#each commitRows as row (row.value)}
        <Select.Item
          value={row.value}
          label={row.subject}
          aria-label={commitChoiceLabel(row)}
          >{commitChoiceLabel(row)}</Select.Item
        >
      {/each}
    </Select.Content>
  </Select.Root>
  {#if commitsError}<p role="alert" class="text-xs text-destructive">
      无法读取最近提交：{commitsError}
    </p>{:else if commitsRead && !commitRows.length}<p
      class="text-xs text-muted-foreground"
    >
      该工作树还没有提交。
    </p>{/if}
  <p class="text-xs text-muted-foreground">
    分离后工作树不在任何分支上，新提交需要另建分支保留。
  </p>
</div>
