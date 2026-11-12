<script lang="ts">
  import { untrack } from "svelte";
  import { getShell, type Shell } from "../../src/hooks/shared/shell.svelte";

  let { id, expose }: { id: string; expose?: (shell: Shell) => void } =
    $props();
  const shell = getShell();
  untrack(() => expose?.(shell));
</script>

<div data-testid="session-page">{id}</div>
