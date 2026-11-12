<script lang="ts" generics="T">
  import { untrack, type Snippet } from "svelte";

  let {
    load,
    label,
    loadingText,
    errorMessage,
    children,
  }: {
    load: () => Promise<T>;
    label: string;
    loadingText?: string;
    errorMessage?: (error: unknown) => string;
    children: Snippet<[T]>;
  } = $props();
  const pending = untrack(() => load());
</script>

{#await pending}
  <p role="status" class="p-6 text-muted-foreground">
    {loadingText ?? `正在加载${label}…`}
  </p>
{:then page}
  {@render children(page)}
{:catch error}
  <p role="alert" class="p-6 text-destructive">
    {errorMessage?.(error) ?? `${label}加载失败，请刷新页面后重试。`}
  </p>
{/await}
