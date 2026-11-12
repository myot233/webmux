<script lang="ts">
  import { Button } from "$lib/components/ui/button";

  // The buttons ending a branch flow: cancel and confirm, then "完成" once
  // the operation ran.
  let {
    done,
    busy,
    ready,
    label,
    destructive = false,
    onconfirm,
    onclose,
  }: {
    done: boolean;
    busy: boolean;
    /** The shown preview can be confirmed. */
    ready: boolean;
    /** The confirm button's words, for the current state. */
    label: string;
    destructive?: boolean;
    onconfirm: () => void;
    onclose: () => void;
  } = $props();
</script>

<div class="flex flex-wrap justify-end gap-2">
  {#if done}
    <Button onclick={onclose}>完成</Button>
  {:else}
    <Button variant="outline" disabled={busy} onclick={onclose}>取消</Button>
    <Button
      variant={destructive ? "destructive" : "default"}
      disabled={!ready}
      onclick={onconfirm}>{label}</Button
    >
  {/if}
</div>
