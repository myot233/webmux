<script lang="ts">
  import type { Snippet } from "svelte";
  import { Button } from "$lib/components/ui/button";
  import * as Card from "$lib/components/ui/card";
  import TunnelStateBadge from "./TunnelStateBadge.svelte";
  import type { TunnelInfo } from "../../contracts/schemas";

  // The settings overview card of one way to reach WebMux from elsewhere
  // (Cloudflare Tunnel, Tailscale): what it is, its state, where it is
  // reachable and a link to its management page. The state line and the
  // address text differ per provider and come as snippets.
  let {
    id,
    title,
    description,
    statusLabel,
    state,
    error,
    loadingText,
    href,
    manageLabel,
    summary,
    detail,
  }: {
    id: string;
    title: string;
    description: string;
    /** Accessible name of the status region. */
    statusLabel: string;
    /** Undefined while the status is still being read. */
    state?: TunnelInfo["state"];
    error?: string;
    loadingText: string;
    href: string;
    manageLabel: string;
    /** Next to the state badge, e.g. the mode. */
    summary: Snippet;
    /** Under the state line, e.g. the public address. */
    detail: Snippet;
  } = $props();
</script>

<Card.Root flat {id} class="scroll-mt-4">
  <Card.Header>
    <Card.Title>{title}</Card.Title>
    <Card.Description>{description}</Card.Description>
  </Card.Header>
  <Card.Content class="space-y-4">
    <section aria-label={statusLabel} class="min-w-0 space-y-2">
      {#if error}
        <p role="alert" class="text-sm text-destructive">{error}</p>
      {:else if state}
        <div class="flex flex-wrap items-center gap-2" role="status">
          <TunnelStateBadge {state} />
          {@render summary()}
        </div>
        {@render detail()}
      {:else}
        <p class="text-sm text-muted-foreground">{loadingText}</p>
      {/if}
    </section>
    <Button {href} variant="outline">{manageLabel}</Button>
  </Card.Content>
</Card.Root>
