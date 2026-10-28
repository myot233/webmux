import type { TerminalDraft, TerminalDrafts } from "../terminal-draft.svelte";

// Keeps a draft's stored selection in step with its textarea and restores the
// caret after external edits, such as an uploaded path.
export function useTextareaDraft(
  getTextarea: () => HTMLTextAreaElement | null,
  getDraft: () => TerminalDraft,
  getDrafts: () => TerminalDrafts,
) {
  let composing = $state(false);

  function remember() {
    const textarea = getTextarea();
    if (textarea)
      getDrafts().select(
        getDraft(),
        textarea.selectionStart,
        textarea.selectionEnd,
      );
  }

  function editFrom(event: CompositionEvent) {
    const textarea = event.currentTarget as HTMLTextAreaElement;
    composing = false;
    getDrafts().edit(
      getDraft(),
      textarea.value,
      textarea.selectionStart,
      textarea.selectionEnd,
    );
  }

  $effect(() => {
    const draft = getDraft();
    const start = draft.selectionStart;
    const end = draft.selectionEnd;
    const text = draft.text;
    const textarea = getTextarea();
    if (textarea && !composing) {
      // External insertions update the caret without opening the mobile keyboard.
      textarea.value = text;
      textarea.setSelectionRange(start ?? text.length, end ?? text.length);
    }
  });

  return {
    get composing() {
      return composing;
    },
    // Spread onto the textarea so caret moves and IME edits reach the draft.
    selectionProps: {
      onselect: remember,
      onkeyup: remember,
      onclick: remember,
      onblur: remember,
      oncompositionstart: () => (composing = true),
      oncompositionend: editFrom,
    },
  };
}
