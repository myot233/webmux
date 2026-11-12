<script lang="ts">
  import { tick, type Snippet } from "svelte";
  import { navigate } from "../../platform/api";
  import AgentHandoff from "./AgentHandoff.svelte";
  import * as DropdownMenu from "$lib/components/ui/dropdown-menu";
  import IconArrowsExchange from "~icons/tabler/arrows-exchange";
  import IconGitFork from "~icons/tabler/git-fork";
  let {
    sourceId,
    root,
    children,
  }: {
    sourceId: string | undefined;
    root: HTMLElement | undefined;
    children: Snippet<[Snippet | undefined]>;
  } = $props();
  let open = $state(false);
</script>

{@render children(sourceId ? menu : undefined)}

{#snippet menu()}
  <DropdownMenu.Item onSelect={() => (open = true)}
    ><IconArrowsExchange aria-hidden="true" />更换 Agent…</DropdownMenu.Item
  >
  <DropdownMenu.Item
    onSelect={() => {
      if (sourceId) navigate(`/handoff/${sourceId}`);
    }}><IconGitFork aria-hidden="true" />交接到新聊天…</DropdownMenu.Item
  >
{/snippet}

{#if open && sourceId}
  {#key sourceId}<AgentHandoff
      {sourceId}
      onclose={() => {
        open = false;
        void tick().then(() =>
          root
            ?.querySelector<HTMLButtonElement>(
              '[aria-label="会话菜单"], [aria-label="终端菜单"]',
            )
            ?.focus(),
        );
      }}
    />{/key}
{/if}
