<script lang="ts">
  import IconNotes from "~icons/tabler/notes";
  import { Button } from "$lib/components/ui/button";
  import { cn } from "$lib/utils.js";
  import type { TerminalDraft } from "../../domains/terminal/terminal-draft.svelte";

  // The draft-mode switch. Wide screens keep it in the header; phones keep it
  // beside the soft keys, where input actions live. An icon like its
  // neighbours; green while draft mode is on, a dot when scratch notes wait.
  let {
    draft,
    draftSaved,
    scratchSaved,
    disabled,
    toggle,
    class: className,
  }: {
    draft: TerminalDraft;
    draftSaved: string;
    scratchSaved: string;
    disabled: boolean;
    toggle: () => void;
    class?: string;
  } = $props();
</script>

<Button
  variant={draft.enabled ? "success" : "ghost"}
  size="icon-sm"
  class={cn("relative", className)}
  aria-label={draft.scratch.length
    ? `草稿模式，${draft.scratch.length} 条暂存`
    : "草稿模式"}
  aria-pressed={draft.enabled}
  {disabled}
  title={draft.enabled
    ? "关闭只隐藏草稿，不发送或清空"
    : draft.scratchUpdatedWhileHidden
      ? `暂存已更新，开启查看；${scratchSaved}`
      : draft.scratch.length
        ? `开启查看草稿与 ${draft.scratch.length} 条暂存；${scratchSaved}`
        : draft.updatedWhileHidden
          ? `隐藏草稿已更新，开启查看；${draftSaved}`
          : draft.text
            ? `开启查看保留的草稿；${draftSaved}`
            : "开启草稿模式"}
  onclick={toggle}
  ><IconNotes
    class="size-4"
    aria-hidden="true"
  />{#if !draft.enabled && draft.scratch.length}<span
      class="absolute top-0.5 right-0.5 size-1.5 rounded-full bg-warning"
      aria-hidden="true"
    ></span>{/if}</Button
>
