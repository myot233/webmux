<script lang="ts">
  import { tick } from "svelte";
  import { Button } from "$lib/components/ui/button";
  import { Textarea } from "$lib/components/ui/textarea";
  import * as AlertDialog from "$lib/components/ui/alert-dialog";
  import type { TerminalDraft, TerminalDrafts } from "../terminal-draft.svelte";

  let {
    draft,
    drafts,
    unavailable,
    send,
    filePaste,
    controlNotice,
  }: {
    draft: TerminalDraft;
    drafts: TerminalDrafts;
    unavailable: string;
    send: () => void;
    filePaste: (event: ClipboardEvent) => boolean;
    controlNotice: () => void;
  } = $props();
  let textarea = $state<HTMLTextAreaElement | null>(null);
  let composing = $state(false);
  let clearOpen = $state(false);

  function remember() {
    if (textarea)
      drafts.select(draft, textarea.selectionStart, textarea.selectionEnd);
  }
  $effect(() => {
    const start = draft.selectionStart;
    const end = draft.selectionEnd;
    const text = draft.text;
    if (textarea && !composing) {
      // External insertions update the caret without opening the mobile keyboard.
      textarea.value = text;
      textarea.setSelectionRange(start ?? text.length, end ?? text.length);
    }
  });
  export function focus() {
    textarea?.focus({ preventScroll: true });
  }
  export function keyboard() {
    if (document.activeElement === textarea) textarea?.blur();
    else focus();
  }
  export function editKey(input: string) {
    if (composing) return;
    const text = draft.text;
    let start = draft.selectionStart ?? text.length;
    let end = draft.selectionEnd ?? start;
    const previous = () =>
      start - (text.codePointAt(start - 2)! > 0xffff ? 2 : 1);
    const next = () => end + (text.codePointAt(end)! > 0xffff ? 2 : 1);
    if (input === "\x7f" || input === "\b") {
      if (start === end && start > 0) start = previous();
      drafts.select(draft, start, end);
      drafts.insert(draft, "");
    } else if (input === "\x1b[3~") {
      if (start === end && end < text.length) end = next();
      drafts.select(draft, start, end);
      drafts.insert(draft, "");
    } else if (
      ["\x1b[D", "\x1b[C", "\x1b[A", "\x1b[B", "\x1b[H", "\x1b[F"].includes(
        input,
      )
    ) {
      let position = start;
      const lineStart = start === 0 ? 0 : text.lastIndexOf("\n", start - 1) + 1;
      const lineEnd = text.indexOf("\n", end);
      if (input === "\x1b[D")
        position = start !== end ? start : Math.max(0, previous());
      if (input === "\x1b[C")
        position = start !== end ? end : Math.min(text.length, next());
      if (input === "\x1b[H") position = lineStart;
      if (input === "\x1b[F") position = lineEnd < 0 ? text.length : lineEnd;
      if (input === "\x1b[A") {
        const previousStart =
          lineStart < 2 ? 0 : text.lastIndexOf("\n", lineStart - 2) + 1;
        position =
          lineStart === 0
            ? 0
            : Math.min(lineStart - 1, previousStart + start - lineStart);
      }
      if (input === "\x1b[B") {
        const nextStart = lineEnd < 0 ? text.length : lineEnd + 1;
        const nextEnd = text.indexOf("\n", nextStart);
        position = Math.min(
          nextEnd < 0 ? text.length : nextEnd,
          nextStart + start - lineStart,
        );
      }
      drafts.select(draft, position, position);
    } else if (
      input === "\r" ||
      input === "\n" ||
      input === "\t" ||
      !/[\x00-\x1f\x7f]/.test(input)
    ) {
      drafts.insert(draft, input === "\r" ? "\n" : input);
    } else controlNotice();
    void tick().then(focus);
  }
  export function terminalKey(event: KeyboardEvent) {
    focus();
    if (event.type !== "keydown" || event.isComposing) return;
    if (event.ctrlKey || event.metaKey || event.altKey) {
      controlNotice();
      return;
    }
    const keys: Record<string, string> = {
      Enter: "\n",
      Backspace: "\x7f",
      Delete: "\x1b[3~",
      Tab: "\t",
      ArrowLeft: "\x1b[D",
      ArrowRight: "\x1b[C",
      ArrowUp: "\x1b[A",
      ArrowDown: "\x1b[B",
      Home: "\x1b[H",
      End: "\x1b[F",
    };
    if (keys[event.key]) editKey(keys[event.key]);
    else if (Array.from(event.key).length === 1) editKey(event.key);
    else if (event.key === "Escape") controlNotice();
  }
</script>

<section class="terminal-composer" aria-label="终端输入草稿">
  <Textarea
    bind:ref={textarea}
    value={draft.text}
    aria-label="草稿内容"
    class="min-h-0 flex-1 resize-none overflow-y-auto font-mono [field-sizing:fixed]"
    autocapitalize="off"
    autocorrect="off"
    spellcheck={false}
    oninput={(event) =>
      drafts.edit(
        draft,
        event.currentTarget.value,
        event.currentTarget.selectionStart,
        event.currentTarget.selectionEnd,
      )}
    onselect={remember}
    onkeyup={remember}
    onclick={remember}
    onblur={remember}
    oncompositionstart={() => (composing = true)}
    oncompositionend={(event) => {
      composing = false;
      drafts.edit(
        draft,
        event.currentTarget.value,
        event.currentTarget.selectionStart,
        event.currentTarget.selectionEnd,
      );
    }}
    onpaste={(event) => {
      filePaste(event);
    }}
    onkeydown={(event) => {
      if (event.isComposing) return;
      if ((event.ctrlKey || event.metaKey) && event.key === "Enter")
        event.preventDefault();
    }}
  />
  <div class="flex shrink-0 flex-wrap items-center gap-2">
    <span
      class="min-w-0 flex-[1_1_12rem] [overflow-wrap:anywhere] text-xs text-muted-foreground"
      role="status"
      >{unavailable || "草稿模式 · 仅当前页面内存，刷新后丢失"}</span
    >
    <Button
      variant="outline"
      size="sm"
      disabled={!draft.text || composing}
      onclick={() => (clearOpen = true)}>清空草稿</Button
    >
    <Button
      size="sm"
      disabled={!draft.text || !!unavailable || composing}
      onclick={() => {
        if (!composing) send();
      }}>发送</Button
    >
  </div>
</section>

<AlertDialog.Root bind:open={clearOpen}>
  <AlertDialog.Content>
    <AlertDialog.Header>
      <AlertDialog.Title>清空草稿？</AlertDialog.Title>
      <AlertDialog.Description
        >仅清空当前草稿，不影响终端或已上传文件。此操作无法撤销。</AlertDialog.Description
      >
    </AlertDialog.Header>
    <AlertDialog.Footer>
      <AlertDialog.Cancel>取消</AlertDialog.Cancel>
      <AlertDialog.Action
        onclick={() => {
          drafts.clear(draft);
          clearOpen = false;
        }}>确认清空</AlertDialog.Action
      >
    </AlertDialog.Footer>
  </AlertDialog.Content>
</AlertDialog.Root>
