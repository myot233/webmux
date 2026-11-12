<script lang="ts">
  import AccessSummaryCard from "./AccessSummaryCard.svelte";
  import { useMountedRead } from "../../hooks/shared/remote-request.svelte";
  import { ConfigSchema } from "../../contracts/schemas";

  const read = useMountedRead("/config", ConfigSchema);
  const tunnel = $derived(read.value?.tunnel);
</script>

<AccessSummaryCard
  id="access"
  title="公网访问（Cloudflare Tunnel）"
  description="通过 HTTPS 入口从其他网络访问 WebMux。配置和启停立即生效，无需重启；公网入口仍要求 WebMux Token。"
  statusLabel="隧道状态"
  state={tunnel?.state}
  error={read.error}
  loadingText="正在读取隧道状态…"
  href="/settings/tunnel"
  manageLabel="管理公网访问"
>
  {#snippet summary()}
    <span class="text-sm"
      >{tunnel?.named ? "Named Tunnel" : "Quick Tunnel"}</span
    >
  {/snippet}
  {#snippet detail()}
    {#if tunnel?.publicUrl || tunnel?.hostname}
      <p class="text-sm wrap-anywhere">
        {tunnel.publicUrl || tunnel.hostname}
      </p>
    {/if}
  {/snippet}
</AccessSummaryCard>
