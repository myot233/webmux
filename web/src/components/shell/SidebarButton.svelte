<script lang="ts">
  import IconMenu2 from "~icons/tabler/menu-2";
  import { Button } from "$lib/components/ui/button";
  import { shellUi } from "../../hooks/shared/shell.svelte";

  // Opens the shell's sidebar drawer on phones, from the left of a page's
  // top bar; focus returns here when the drawer closes.
  let { class: className = "" }: { class?: string } = $props();
</script>

<Button
  class={className}
  variant="ghost"
  size="icon-touch"
  aria-label="打开侧栏"
  title="打开侧栏"
  aria-haspopup="dialog"
  aria-expanded={shellUi.drawerOpen}
  onclick={() => (shellUi.drawerOpen = true)}
  ><IconMenu2 aria-hidden="true" /></Button
>
