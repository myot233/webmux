<script lang="ts">
  import { staticHtml } from "../code-html";
  let { text }: { text: string } = $props();
  const srcdoc = $derived(staticHtml(text));
</script>

<div class="flex min-h-0 flex-1 flex-col">
  <p class="shrink-0 border-b px-2 py-1 text-xs text-muted-foreground">
    静态预览，脚本与外部资源已禁用
  </p>
  <!-- An empty sandbox: opaque origin, no scripts, forms or navigation. -->
  <iframe
    title="HTML 静态预览"
    sandbox=""
    referrerpolicy="no-referrer"
    {srcdoc}
    class="code-html-frame min-h-0 w-full flex-1 border-0"
  ></iframe>
</div>

<style>
  /* Pages without their own background expect a white canvas. */
  .code-html-frame {
    background: white;
  }
</style>
