<script lang="ts">
  import IconPlugConnected from "~icons/tabler/plug-connected";
  import { Badge } from "$lib/components/ui/badge";
  import { Button } from "$lib/components/ui/button";
  import { forwards } from "../../domains/workspace/forwards.svelte";

  // The header entry to the port-forwarding dialog; the badge counts the
  // server's forwards.
  let { onclick }: { onclick: () => void } = $props();
  const count = $derived(forwards.list.length);
  const label = $derived(count ? `端口转发（${count} 个）` : "端口转发");
</script>

<Button
  variant="ghost"
  size={count ? "sm" : "icon-sm"}
  aria-label={label}
  title={label}
  aria-haspopup="dialog"
  onclick={() => onclick()}
  ><IconPlugConnected class="size-4" aria-hidden="true" />{#if count}<Badge
      variant="secondary"
      aria-hidden="true"><span class="tabular-nums">{count}</span></Badge
    >{/if}</Button
>
