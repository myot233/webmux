<script lang="ts">
  import IconRefresh from "~icons/tabler/refresh";
  import IconX from "~icons/tabler/x";
  import { Button } from "$lib/components/ui/button";
  import * as Item from "$lib/components/ui/item";
  import { restartMethod } from "../../platform/pending-update";
  import type { PendingUpdate } from "../../contracts/schemas";

  // Without ondismiss the notice cannot be closed (the settings page).
  let {
    update,
    ondismiss,
    class: className,
  }: {
    update: PendingUpdate;
    ondismiss?: () => void;
    class?: string;
  } = $props();
  const method = $derived(restartMethod(update.launcher));
</script>

<Item.Root
  variant="outline"
  role="status"
  aria-label="WebMux 待生效更新"
  class={className}
>
  <Item.Media variant="icon"><IconRefresh aria-hidden="true" /></Item.Media>
  <Item.Content>
    <Item.Title
      >已安装 WebMux {update.installedVersion}，重启 WebMux 后生效</Item.Title
    >
    <Item.Description>重启会结束所有会话。</Item.Description>
    <Item.Description>
      重启方式：{#if "command" in method}<code>{method.command}</code
        >{:else}{method.text}{/if}
    </Item.Description>
  </Item.Content>
  {#if ondismiss}
    <Item.Actions>
      <Button
        variant="ghost"
        size="icon-sm"
        aria-label="关闭更新提示"
        title="关闭"
        onclick={ondismiss}><IconX aria-hidden="true" /></Button
      >
    </Item.Actions>
  {/if}
</Item.Root>
