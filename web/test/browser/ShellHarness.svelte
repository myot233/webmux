<script lang="ts">
  import { onMount } from "svelte";
  import AppShell from "../../src/components/shell/AppShell.svelte";
  import QuickStart from "../../src/pages/QuickStart.svelte";
  import SessionList from "../../src/pages/SessionList.svelte";
  import { matchRoute } from "../../src/hooks/shared/route";
  import type { TerminalDrafts } from "../../src/domains/terminal/terminal-draft.svelte";

  // A shell page as App renders it, following the address: the home page,
  // quick start, or a stand-in for a session page inside the app shell.
  let { drafts }: { drafts: TerminalDrafts } = $props();
  let path = $state(location.pathname);
  onMount(() => {
    const change = () => (path = location.pathname);
    window.addEventListener("popstate", change);
    return () => window.removeEventListener("popstate", change);
  });
  const route = $derived.by(() => {
    const matched = matchRoute(path);
    return matched.name === "terminal" || matched.name === "quick-start"
      ? matched
      : ({ name: "sessions" } as const);
  });
</script>

<AppShell {route} {drafts}>
  {#if route.name === "quick-start"}<QuickStart />
  {:else if route.name === "terminal"}<div data-testid="session-page">
      {route.id}
    </div>
  {:else}<SessionList />{/if}
</AppShell>
