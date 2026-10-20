<script module lang="ts">
  export type SearchSpec = {
    search: string;
    caseSensitive: boolean;
    regexp: boolean;
    wholeWord: boolean;
  };
</script>

<script lang="ts">
  import { Button } from "$lib/components/ui/button";
  import { Input } from "$lib/components/ui/input";
  import {
    IconArrowDown,
    IconArrowUp,
    IconSelectAll,
    IconX,
  } from "@tabler/icons-svelte";

  let {
    query,
    onquery,
    onnext,
    onprevious,
    onall,
    onclose,
  }: {
    query: SearchSpec;
    onquery: (query: SearchSpec) => void;
    onnext: () => void;
    onprevious: () => void;
    onall?: () => void;
    onclose: () => void;
  } = $props();
  // CodeMirror owns the query; the panel mirrors it and reports edits back.
  // svelte-ignore state_referenced_locally
  let spec = $state({ ...query });
  export function setQuery(next: SearchSpec) {
    spec = { ...next };
  }
  function commit() {
    onquery({ ...spec });
  }
  function toggle(key: "caseSensitive" | "regexp" | "wholeWord") {
    spec[key] = !spec[key];
    commit();
  }
</script>

<div class="code-search flex flex-wrap items-center gap-1 p-1">
  <Input
    bind:value={spec.search}
    {...{ "main-field": "true" }}
    aria-label="查找"
    placeholder="查找"
    class="h-7 min-w-32 flex-1"
    oninput={commit}
    onkeydown={(e) => {
      if (e.key !== "Enter") return;
      e.preventDefault();
      if (e.shiftKey) onprevious();
      else onnext();
    }}
  />
  <div class="flex items-center gap-1">
    {#each [{ key: "caseSensitive", label: "区分大小写", text: "Aa" }, { key: "wholeWord", label: "全词匹配", text: "W" }, { key: "regexp", label: "正则表达式", text: ".*" }] as const as option (option.key)}
      <Button
        variant={spec[option.key] ? "secondary" : "ghost"}
        size="icon-sm"
        aria-label={option.label}
        title={option.label}
        aria-pressed={spec[option.key]}
        class="font-mono text-xs"
        onclick={() => toggle(option.key)}>{option.text}</Button
      >
    {/each}
    <Button
      variant="ghost"
      size="icon-sm"
      aria-label="上一个匹配"
      title="上一个匹配（Shift+Enter）"
      onclick={onprevious}><IconArrowUp aria-hidden="true" /></Button
    >
    <Button
      variant="ghost"
      size="icon-sm"
      aria-label="下一个匹配"
      title="下一个匹配（Enter）"
      onclick={onnext}><IconArrowDown aria-hidden="true" /></Button
    >
    {#if onall}<Button
        variant="ghost"
        size="icon-sm"
        aria-label="选中全部匹配"
        title="选中全部匹配"
        onclick={onall}><IconSelectAll aria-hidden="true" /></Button
      >{/if}
    <Button
      variant="ghost"
      size="icon-sm"
      aria-label="关闭查找"
      title="关闭查找（Esc）"
      onclick={onclose}><IconX aria-hidden="true" /></Button
    >
  </div>
</div>
