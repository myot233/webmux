<script lang="ts">
  import type { Snippet } from "svelte";
  import { Button } from "$lib/components/ui/button";

  // The form of a dialog that previews a worktree operation (removal,
  // release, rebuild) before running it: what it targets, a busy notice, the
  // preview, the last error, then cancel beside the operation's actions.
  let {
    busy,
    error,
    onsubmit,
    onclose,
    summary,
    children,
    actions,
  }: {
    busy: boolean;
    error: string;
    onsubmit: () => void;
    onclose: () => void;
    summary: Snippet;
    children: Snippet;
    actions: Snippet;
  } = $props();
</script>

<form
  class="space-y-4"
  onsubmit={(event) => {
    event.preventDefault();
    onsubmit();
  }}
>
  {@render summary()}
  {#if busy}<p role="status" class="text-sm">正在处理，请勿重复提交…</p>{/if}
  {@render children()}
  {#if error}<p role="alert" class="break-all text-sm text-destructive">
      {error}
    </p>{/if}
  <div class="flex flex-wrap justify-end gap-2">
    <Button type="button" variant="outline" disabled={busy} onclick={onclose}
      >取消</Button
    >
    {@render actions()}
  </div>
</form>
