<script lang="ts">
  import {
    commitChoiceLabel,
    type CommitChoice,
  } from "../../domains/workspace/commit-choices";
  import { Badge } from "$lib/components/ui/badge";
  import { Input } from "$lib/components/ui/input";
  import { Label } from "$lib/components/ui/label";
  import * as Select from "$lib/components/ui/select";
  import AdvancedSection from "../shared/AdvancedSection.svelte";

  let {
    mode,
    path = $bindable(""),
    base = $bindable(""),
    disabled = false,
    loading = false,
    hasData = false,
    commitRows,
    selectedCommit,
    commitsCount,
    commitsLoading = false,
    commitsError = "",
    onpathedited,
    onbaseedited,
  }: {
    mode: "new" | "existing";
    path: string;
    base: string;
    disabled?: boolean;
    loading?: boolean;
    hasData?: boolean;
    commitRows: readonly CommitChoice[];
    selectedCommit: CommitChoice | undefined;
    commitsCount: number;
    commitsLoading?: boolean;
    commitsError?: string;
    onpathedited: () => void;
    onbaseedited: () => void;
  } = $props();
</script>

{#snippet commitDetails(item: CommitChoice)}
  <span class="grid min-w-0 flex-1 gap-0.5">
    <span class="truncate text-sm font-medium" title={item.subject}
      >{item.subject}</span
    >
    <span
      class="flex min-w-0 flex-wrap items-center gap-1.5 text-xs text-muted-foreground"
    >
      <span class="font-mono">{item.shortHash}</span>
      {#each item.tags as tag (tag)}
        <Badge variant={tag === "远程最新" ? "info" : "secondary"}>{tag}</Badge>
      {/each}
      {#if item.age}<span>{item.age}</span>{/if}
    </span>
  </span>
{/snippet}

{#if mode === "new" && hasData && !commitRows.length && !commitsLoading}<p
    class="text-sm text-muted-foreground"
  >
    尚无可用提交。请先在终端创建提交；仍可在主工作区启动会话。
  </p>{/if}
<AdvancedSection
  title={mode === "new" ? "高级：起始提交与存放目录" : "高级：存放目录"}
  summary={[mode === "new" ? selectedCommit?.shortHash : "", path]
    .filter(Boolean)
    .join(" · ")}
>
  {#if mode === "new"}
    <div class="space-y-2">
      <Label for="worktree-base">起始提交</Label>
      <Select.Root
        type="single"
        bind:value={base}
        disabled={disabled || loading || !hasData || !commitRows.length}
        onValueChange={onbaseedited}
      >
        <Select.Trigger
          id="worktree-base"
          aria-describedby="worktree-base-help"
        >
          {#if selectedCommit}
            {@render commitDetails(selectedCommit)}
          {:else}
            <span class="text-muted-foreground">
              {commitsLoading ? "正在读取最近提交…" : "选择提交"}
            </span>
          {/if}
        </Select.Trigger>
        <Select.Content
          class="max-h-[min(24rem,var(--bits-select-content-available-height))]"
        >
          {#each commitRows as item (item.value)}
            <Select.Item
              value={item.value}
              label={item.subject}
              aria-label={commitChoiceLabel(item)}
            >
              {@render commitDetails(item)}
            </Select.Item>
          {/each}
        </Select.Content>
      </Select.Root>
      <p id="worktree-base-help" class="text-xs text-muted-foreground">
        {#if commitsLoading}正在读取最近提交…{:else}所选分支最近 {commitsCount}
          条提交；“刷新上游”会加入远程最新提交。{/if}
      </p>
      {#if commitsError}<p role="status" class="text-xs text-warning">
          {commitsError}
        </p>{/if}
    </div>
  {/if}
  <div class="space-y-2">
    <Label for="worktree-path">新工作树完整路径</Label>
    <Input
      id="worktree-path"
      bind:value={path}
      oninput={onpathedited}
      mono
      required
      autocomplete="off"
      autocapitalize="off"
      autocorrect="off"
      spellcheck={false}
    />
    <p class="text-xs text-muted-foreground">
      目录名默认取分支最后一段，不会覆盖已有目录。
    </p>
  </div>
</AdvancedSection>
