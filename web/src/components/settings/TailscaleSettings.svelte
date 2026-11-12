<script lang="ts">
  import AccessSummaryCard from "./AccessSummaryCard.svelte";
  import { useMountedRead } from "../../hooks/shared/remote-request.svelte";
  import { TailscaleStatusSchema } from "../../contracts/schemas";

  const read = useMountedRead("/tailscale", TailscaleStatusSchema);
  const status = $derived(read.value);
  const modeText = {
    off: "未启用",
    serve: "Serve（仅 tailnet）",
    funnel: "Funnel（公网）",
  };
</script>

<AccessSummaryCard
  id="tailscale"
  title="Tailscale（Serve / Funnel）"
  description="使用 Tailscale 自动签发的 HTTPS 证书。Serve 仅 tailnet 内设备可访问；Funnel 会公开到互联网。可在网页修改，保存后立即生效。"
  statusLabel="Tailscale 状态"
  state={status?.state}
  error={read.error}
  loadingText="正在读取 Tailscale 状态…"
  href="/settings/tailscale"
  manageLabel="管理 Tailscale"
>
  {#snippet summary()}
    {#if status}
      <span class="text-sm">{modeText[status.mode]}</span>
      {#if status.dnsName}
        <span class="text-sm text-muted-foreground">{status.dnsName}</span>
      {/if}
    {/if}
  {/snippet}
  {#snippet detail()}
    {#if status?.publicUrl}
      <p class="text-sm wrap-anywhere">{status.publicUrl}</p>
    {:else if status?.blockedReason || status?.error}
      <p class="text-sm text-muted-foreground wrap-anywhere">
        {status.blockedReason || status.error}
      </p>
    {:else}
      <p class="text-sm text-muted-foreground">
        尚未启用；进入 Tailscale 设置选择 Serve 或 Funnel。
      </p>
    {/if}
  {/snippet}
</AccessSummaryCard>
