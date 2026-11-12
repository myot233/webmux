<script lang="ts">
  import type { Snippet } from "svelte";
  import * as AlertDialog from "$lib/components/ui/alert-dialog";
  import { Button } from "$lib/components/ui/button";
  import { errorMessage } from "../../platform/error";

  // A cancel/confirm alert. `onconfirm` may be async: the dialog then stays
  // open and busy until it resolves, shows a rejection inline and closes on
  // success.
  let {
    open = $bindable(false),
    title,
    confirmLabel = "确认",
    busyLabel = "正在处理…",
    destructive = false,
    onconfirm,
    children,
  }: {
    open?: boolean;
    title: string;
    confirmLabel?: string;
    busyLabel?: string;
    destructive?: boolean;
    onconfirm: () => void | Promise<void>;
    children: Snippet;
  } = $props();

  let busy = $state(false);
  let error = $state("");

  $effect(() => {
    if (open) error = "";
  });

  async function confirm() {
    if (busy) return;
    busy = true;
    error = "";
    try {
      await onconfirm();
      open = false;
    } catch (cause) {
      error = errorMessage(cause);
    } finally {
      busy = false;
    }
  }
</script>

<AlertDialog.Root
  bind:open
  onOpenChange={(value) => {
    if (!value && busy) open = true;
  }}
>
  <AlertDialog.Content>
    <AlertDialog.Header>
      <AlertDialog.Title>{title}</AlertDialog.Title>
      <AlertDialog.Description>{@render children()}</AlertDialog.Description>
    </AlertDialog.Header>
    {#if error}<p role="alert" class="text-sm break-all text-destructive">
        {error}
      </p>{/if}
    <AlertDialog.Footer>
      <AlertDialog.Cancel disabled={busy}>取消</AlertDialog.Cancel>
      <Button
        variant={destructive ? "destructive" : "default"}
        disabled={busy}
        onclick={confirm}>{busy ? busyLabel : confirmLabel}</Button
      >
    </AlertDialog.Footer>
  </AlertDialog.Content>
</AlertDialog.Root>
