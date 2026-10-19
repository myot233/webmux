<script lang="ts">
  import type { Snippet } from "svelte";
  import { MediaQuery } from "svelte/reactivity";
  import * as Dialog from "$lib/components/ui/dialog";
  import * as Drawer from "$lib/components/ui/drawer";

  let {
    title,
    description,
    onclose,
    children,
    busy = false,
  }: {
    title: string;
    description: string;
    onclose: () => void;
    children: Snippet;
    busy?: boolean;
  } = $props();
  const desktop = new MediaQuery("(min-width: 640px)");
  let open = $state(true);
  function changed(value: boolean) {
    if (!value && !busy) onclose();
    else if (!value) open = true;
  }
</script>

{#if desktop.current}
  <Dialog.Root bind:open onOpenChange={changed}>
    <Dialog.Content
      class="max-h-[90dvh] overflow-y-auto sm:max-w-xl"
      onEscapeKeydown={(event) => {
        if (busy) event.preventDefault();
      }}
      onInteractOutside={(event) => {
        if (busy) event.preventDefault();
      }}
    >
      <Dialog.Header>
        <Dialog.Title>{title}</Dialog.Title>
        <Dialog.Description>{description}</Dialog.Description>
      </Dialog.Header>
      {@render children()}
    </Dialog.Content>
  </Dialog.Root>
{:else}
  <Drawer.Root bind:open onOpenChange={changed} dismissible={!busy}>
    <Drawer.Content class="data-[vaul-drawer-direction=bottom]:max-h-[95dvh]">
      <div class="min-h-0 overflow-y-auto">
        <Drawer.Header>
          <Drawer.Title>{title}</Drawer.Title>
          <Drawer.Description>{description}</Drawer.Description>
        </Drawer.Header>
        <div class="px-4 pt-1 pb-[max(1rem,env(safe-area-inset-bottom))]">
          {@render children()}
        </div>
      </div>
    </Drawer.Content>
  </Drawer.Root>
{/if}
