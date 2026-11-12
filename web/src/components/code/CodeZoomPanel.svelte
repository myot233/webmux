<script lang="ts">
  import {
    codeFont,
    defaultCodeFontSize,
    maxCodeFontSize,
    minCodeFontSize,
  } from "../../hooks/code/code-font-preference.svelte";
  import { Button } from "$lib/components/ui/button";
  import { Slider } from "$lib/components/ui/slider";
  import IconX from "~icons/tabler/x";

  let { onclose }: { onclose: () => void } = $props();
</script>

<div data-code-zoom class="flex items-center gap-2 px-2 py-1 text-xs">
  <span class="shrink-0 text-muted-foreground">字号</span>
  <Slider
    type="single"
    min={minCodeFontSize}
    max={maxCodeFontSize}
    step={1}
    value={codeFont.size}
    onValueChange={(value) => codeFont.set(value)}
    aria-label="代码字号"
    class="min-w-24 flex-1"
  />
  <output class="min-w-9 text-right tabular-nums" aria-label="当前代码字号"
    >{codeFont.size}px</output
  >
  {#if codeFont.size !== defaultCodeFontSize}
    <Button variant="ghost" size="sm" onclick={() => codeFont.reset()}
      >重置</Button
    >
  {/if}
  <Button
    variant="ghost"
    size="icon-sm"
    aria-label="关闭缩放"
    title="关闭缩放（Esc）"
    onclick={onclose}><IconX aria-hidden="true" /></Button
  >
</div>
