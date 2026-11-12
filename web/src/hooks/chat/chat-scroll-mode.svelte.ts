import { tick, untrack } from "svelte";
import {
  hasOpenPortal,
  isEditableElement,
} from "../../domains/chat/chat-focus";

type ChatScrollMode = {
  root: () => HTMLElement | null;
  viewport: () => HTMLElement | null;
  scrollRoot: () => HTMLElement | null;
  documentScroll: () => boolean;
  following: () => boolean;
  scrollToBottom: () => void;
  follow: () => void;
  onEditingChange: (editing: boolean) => void;
};

// Focus can natively scroll the document before focusin is dispatched. Preserve
// the reader's offset at the input gesture, before moving between scroll roots.
export function createChatScrollMode(options: ChatScrollMode) {
  let editing = $state(false);
  let lastScrollOffset = 0;
  let previousOwner: HTMLElement | null = null;
  let previousScrollHeight = 0;
  let previousClientHeight = 0;
  let manualScroll = false;
  let layoutOnlyScroll = false;
  let focusOffset: number | null = null;
  let pendingScrollOffset: number | null = null;
  let previousDocumentScroll = untrack(options.documentScroll);

  function setEditing(next: boolean) {
    if (next === editing) return;
    if (next && options.documentScroll())
      pendingScrollOffset = focusOffset ?? lastScrollOffset;
    else if (!next && !options.documentScroll())
      pendingScrollOffset = options.viewport()?.scrollTop ?? lastScrollOffset;
    focusOffset = null;
    editing = next;
    options.onEditingChange(next);
  }
  function syncEditing() {
    const active = document.activeElement;
    const root = options.root();
    const focusedInside =
      !!root && active instanceof Node && root.contains(active);
    if (focusedInside && isEditableElement(active)) setEditing(true);
    else if (!editing || !hasOpenPortal()) setEditing(false);
    focusOffset = null;
  }
  $effect(() => {
    const root = options.root();
    if (!root) return;
    const capture = (event: PointerEvent | KeyboardEvent) => {
      const editable =
        event.target instanceof Element && isEditableElement(event.target);
      focusOffset =
        options.documentScroll() &&
        (event instanceof KeyboardEvent ? event.key === "Tab" : editable)
          ? lastScrollOffset
          : null;
    };
    const clear = () => {
      focusOffset = null;
    };
    root.addEventListener("pointerdown", capture, true);
    root.addEventListener("keydown", capture, true);
    root.addEventListener("pointerup", clear, true);
    root.addEventListener("pointercancel", clear, true);
    return () => {
      root.removeEventListener("pointerdown", capture, true);
      root.removeEventListener("keydown", capture, true);
      root.removeEventListener("pointerup", clear, true);
      root.removeEventListener("pointercancel", clear, true);
    };
  });
  $effect(() => {
    if (!editing) return;
    const check = () => {
      requestAnimationFrame(syncEditing);
    };
    window.addEventListener("pointerdown", check, true);
    window.addEventListener("keydown", check, true);
    return () => {
      window.removeEventListener("pointerdown", check, true);
      window.removeEventListener("keydown", check, true);
    };
  });
  $effect.pre(() => {
    const documentScroll = options.documentScroll();
    if (documentScroll === previousDocumentScroll) return;
    pendingScrollOffset ??= lastScrollOffset;
    previousDocumentScroll = documentScroll;
  });
  $effect(() => {
    void options.documentScroll();
    if (pendingScrollOffset === null) return;
    const saved = pendingScrollOffset;
    const wasStick = untrack(options.following);
    pendingScrollOffset = null;
    void tick().then(() => {
      if (options.documentScroll()) {
        if (wasStick) options.scrollToBottom();
        else window.scrollTo(0, saved);
      } else {
        window.scrollTo(0, 0);
        const viewport = options.viewport();
        if (viewport)
          viewport.scrollTop = wasStick ? viewport.scrollHeight : saved;
      }
      options.follow();
    });
  });
  return {
    get offset() {
      return lastScrollOffset;
    },
    get layoutOnlyScroll() {
      return layoutOnlyScroll;
    },
    markManualScroll() {
      manualScroll = true;
    },
    recordOffset(offset: number) {
      const owner = options.scrollRoot();
      const scrollHeight = owner?.scrollHeight ?? 0;
      const clientHeight = options.documentScroll()
        ? window.innerHeight
        : (owner?.clientHeight ?? 0);
      const sameOwner = owner === previousOwner;
      const sameGeometry =
        sameOwner &&
        scrollHeight === previousScrollHeight &&
        clientHeight === previousClientHeight;
      // Layout can move the offset before ResizeObserver gets its first turn.
      // Keep that movement distinct from input-driven historical reading.
      layoutOnlyScroll = !sameGeometry && !manualScroll;
      const movedUp =
        sameOwner &&
        offset < lastScrollOffset &&
        (sameGeometry || manualScroll);
      if (!sameOwner || offset !== lastScrollOffset) manualScroll = false;
      previousOwner = owner;
      previousScrollHeight = scrollHeight;
      previousClientHeight = clientHeight;
      lastScrollOffset = offset;
      return movedUp;
    },
    focusEditing(event: FocusEvent) {
      if (event.target instanceof Node && isEditableElement(event.target))
        setEditing(true);
      focusOffset = null;
    },
    syncEditing,
    releaseEditing() {
      const active = document.activeElement;
      const root = options.root();
      if (root && active instanceof HTMLElement && root.contains(active))
        active.blur();
      setEditing(false);
    },
  };
}
