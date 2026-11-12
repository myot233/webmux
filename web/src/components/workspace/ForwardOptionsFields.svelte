<script lang="ts">
  import { Label } from "$lib/components/ui/label";
  import * as SegmentedControl from "$lib/components/ui/segmented-control";
  import { Switch } from "$lib/components/ui/switch";
  import type { ForwardOptions } from "../../contracts/schemas";

  // The compatibility options of one forward. New forwards start with every
  // rewrite on, so a Vite-like dev server works through the gateway as is.
  let {
    id,
    port,
    options = $bindable(),
  }: {
    /** Prefix for the field IDs; unique per form. */
    id: string;
    /** The forwarded port, or null while the add form has none. */
    port: number | null;
    options: ForwardOptions;
  } = $props();
  const base = $derived(port ? `/forward/${port}/` : "/forward/{port}/");
  const switches = [
    {
      key: "rewriteHost",
      label: "改写 Host",
      help: "上游收到 localhost:{port}，通过 Vite 的 allowedHosts 检查。",
    },
    {
      key: "rewriteOrigin",
      label: "改写 Origin 和 Referer",
      help: "同源部分改写为 http://localhost:{port}，HMR WebSocket 可以连上。",
    },
    {
      key: "forwardedHeaders",
      label: "添加 X-Forwarded-*",
      help: "告诉上游原始主机、协议和路径前缀。",
    },
    {
      key: "rewriteResponse",
      label: "改写重定向和 Cookie",
      help: "Location、CORS 来源和 Set-Cookie 的 Path 指回转发地址。",
    },
  ] as const;
</script>

<div class="space-y-4">
  <div class="space-y-2">
    <p class="text-sm font-medium" id={`${id}-prefix-label`}>路径前缀</p>
    <SegmentedControl.Root
      size="form"
      bind:value={options.prefix}
      aria-labelledby={`${id}-prefix-label`}
      aria-describedby={`${id}-prefix-help`}
    >
      <SegmentedControl.Item value="keep">保留前缀</SegmentedControl.Item>
      <SegmentedControl.Item value="strip">去除前缀</SegmentedControl.Item>
    </SegmentedControl.Root>
    <p id={`${id}-prefix-help`} class="text-xs text-muted-foreground">
      {#if options.prefix === "keep"}
        原样转发 <code class="font-mono">{base}…</code>。Vite 需以
        <code class="font-mono">--base {base}</code> 启动，Next.js 设置
        <code class="font-mono">basePath</code>。
      {:else}
        去掉 <code class="font-mono">{base.slice(0, -1)}</code>
        后转发，适合只使用相对路径的服务。
      {/if}
    </p>
  </div>
  {#each switches as item (item.key)}
    <div class="flex items-start justify-between gap-3">
      <div class="min-w-0 space-y-0.5">
        <Label for={`${id}-${item.key}`}>{item.label}</Label>
        <p
          id={`${id}-${item.key}-help`}
          class="text-xs text-muted-foreground wrap-anywhere"
        >
          {item.help.replaceAll("{port}", String(port ?? "{port}"))}
        </p>
      </div>
      <Switch
        id={`${id}-${item.key}`}
        bind:checked={options[item.key]}
        aria-describedby={`${id}-${item.key}-help`}
      />
    </div>
  {/each}
</div>
