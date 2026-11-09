<script lang="ts">
  import IconSearch from "~icons/tabler/search";
  import { Button, type ButtonSize } from "$lib/components/ui/button";
  import {
    findSessionSwitcher,
    sessionSwitcherKeyLabel,
    sessionSwitcherKeys,
  } from "../session-switcher";

  let { size }: { size: ButtonSize } = $props();
  const open = findSessionSwitcher();
</script>

{#if open}
  <Button
    variant="ghost"
    {size}
    aria-label="快速切换会话"
    aria-haspopup="dialog"
    aria-keyshortcuts={sessionSwitcherKeys}
    title={`快速切换会话（${sessionSwitcherKeyLabel}）`}
    onclick={(event) => open(event.currentTarget)}
  >
    <IconSearch aria-hidden="true" />
  </Button>
{/if}
