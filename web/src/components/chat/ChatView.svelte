<script lang="ts">
  import { untrack, type Snippet } from "svelte";
  import { createChatScrollMode } from "../../hooks/chat/chat-scroll-mode.svelte";
  import IconArrowDown from "~icons/tabler/arrow-down";
  import {
    Button,
    type ButtonSize,
    type ButtonVariant,
  } from "$lib/components/ui/button";
  import { ScrollArea } from "$lib/components/ui/scroll-area";
  import FileDropZone from "../shared/FileDropZone.svelte";
  import type { ChatActions, RequestAnswer } from "../../platform/chat-api";
  import {
    conversationTurns,
    groupTimeline,
    type ChatModel,
  } from "../../domains/chat/chat-model";
  import { errorMessage } from "../../platform/error";
  import type { ChatUploads } from "../../hooks/shared/upload.svelte";
  import { observeTranscriptResize } from "../../hooks/chat/transcript-resize";
  import type { ChatCodeLink } from "../../domains/code/markdown-link";
  import type { ChatState, SessionInfo } from "../../contracts/schemas";
  import type {
    TerminalDraft,
    TerminalDrafts,
  } from "../../domains/terminal/terminal-draft.svelte";
  import type { Connection } from "../../platform/ws";
  import ChatComposer from "./ChatComposer.svelte";
  import ChatLocation from "./ChatLocation.svelte";
  import ChatPlan from "./ChatPlan.svelte";
  import ChatQueue from "./ChatQueue.svelte";
  import ChatTurnStatus from "./ChatTurnStatus.svelte";
  import ConversationTurns from "./ConversationTurns.svelte";
  import FormPrompt from "./FormPrompt.svelte";
  import PermissionPrompt from "./PermissionPrompt.svelte";
  import TimelineItem from "./TimelineItem.svelte";

  let {
    sessionId,
    model,
    chat,
    connection,
    draft,
    drafts,
    uploads,
    actions,
    unavailable = "",
    uploadUnavailable = "",
    location,
    onreview,
    onopenlocation,
    onterminal,
    quickMenu,
    documentScroll = false,
    oneditchange,
  }: {
    sessionId?: string;
    model: ChatModel;
    chat: ChatState | undefined;
    connection: Connection;
    draft: TerminalDraft;
    drafts: TerminalDrafts;
    uploads: ChatUploads;
    actions: ChatActions;
    unavailable?: string;
    uploadUnavailable?: string;
    location?: Pick<SessionInfo, "workspace" | "cwd"> &
      Partial<Pick<SessionInfo, "view">>;
    onreview: (path: string) => void;
    onopenlocation?: (target: ChatCodeLink) => void;
    onterminal: () => void;
    quickMenu?: Snippet<["top" | "bottom", ButtonSize?, ButtonVariant?]>;
    documentScroll?: boolean;
    oneditchange?: (editing: boolean) => void;
  } = $props();

  let root = $state<HTMLDivElement | null>(null);
  let scroller = $state<HTMLElement | null>(null);
  let transcript = $state<HTMLDivElement | null>(null);
  let dock = $state<HTMLDivElement | null>(null);
  let stick = $state(true);
  let activeTurnId = $state("");
  let error = $state("");
  let answering = $state(false);
  let composer = $state<ChatComposer>();
  let resizeFollower: ReturnType<typeof observeTranscriptResize> | undefined;
  const scrollMode = createChatScrollMode({
    root: () => root,
    viewport: () => scroller,
    scrollRoot,
    documentScroll: () => documentScroll,
    following: () => stick,
    scrollToBottom,
    follow,
    onEditingChange: (value) => oneditchange?.(value),
  });
  export function focus() {
    composer?.focus();
  }
  export function setVoiceError(message: string) {
    error = message;
  }
  export function releaseEditing() {
    scrollMode.releaseEditing();
  }
  const entries = $derived(groupTimeline(model.entries));
  const turns = $derived(conversationTurns(model.entries));
  const turnIds = $derived(new Set(turns.map((item) => item.id)));
  const request = $derived(chat?.requests[0]);
  const requestTool = $derived.by(() => {
    const at = request?.toolCallId
      ? model.index.get(`tool:${request.toolCallId}`)
      : undefined;
    const entry = at === undefined ? undefined : model.entries[at];
    return entry?.type === "tool" ? entry : undefined;
  });
  const waiting = $derived(
    (chat?.requests ?? []).flatMap((item) =>
      item.toolCallId ? [item.toolCallId] : [],
    ),
  );
  const blocked = $derived(
    unavailable ||
      (chat?.recovery
        ? "会话已中断，请先恢复"
        : connection !== "connected" || !chat
          ? "正在连接聊天…"
          : location?.view === "terminal"
            ? "会话不在聊天端"
            : ""),
  );
  const uploadBlocked = $derived(blocked || uploadUnavailable);

  function scrollRoot(): HTMLElement | null {
    return documentScroll
      ? ((document.scrollingElement as HTMLElement | null) ??
          document.documentElement)
      : scroller;
  }
  function scrollToBottom() {
    const target = scrollRoot();
    if (!target) return;
    stick = true;
    if (documentScroll) window.scrollTo(0, target.scrollHeight);
    else target.scrollTop = target.scrollHeight;
    scrollMode.recordOffset(documentScroll ? window.scrollY : target.scrollTop);
  }
  let activeTurnFrame = 0;
  let turnJumpFrame = 0;
  let jumpingToTurn = false;
  $effect(() => () => cancelAnimationFrame(turnJumpFrame));
  function updateActiveTurn() {
    const target = scrollRoot();
    if (!target || !transcript || !turns.length) {
      activeTurnId = "";
      return;
    }
    const viewportTop = documentScroll ? 0 : target.getBoundingClientRect().top;
    const readingLine = viewportTop + (documentScroll ? 64 : 16);
    const elements = transcript.querySelectorAll<HTMLElement>(
      "[data-conversation-message]",
    );
    let current = elements[0]?.dataset.conversationMessage ?? "";
    for (const element of elements) {
      if (element.getBoundingClientRect().top > readingLine) break;
      current = element.dataset.conversationMessage ?? current;
    }
    activeTurnId = current;
  }
  function scheduleActiveTurn() {
    if (activeTurnFrame) return;
    activeTurnFrame = requestAnimationFrame(() => {
      activeTurnFrame = 0;
      updateActiveTurn();
    });
  }
  function follow() {
    const target = scrollRoot();
    if (!target) return;
    const top = documentScroll ? window.scrollY : target.scrollTop;
    const height = documentScroll ? window.innerHeight : target.clientHeight;

    const sameOffset = top === scrollMode.offset;
    if (scrollMode.recordOffset(top)) resizeFollower?.interrupt();
    stick = jumpingToTurn
      ? false
      : (stick && (sameOffset || scrollMode.layoutOnlyScroll)) ||
        top + height >= target.scrollHeight - 32;
    scheduleActiveTurn();
  }
  $effect(() => {
    void turns;
    scheduleActiveTurn();
    return () => {
      cancelAnimationFrame(activeTurnFrame);
      activeTurnFrame = 0;
    };
  });
  function jumpToTurn(id: string) {
    const message = transcript?.querySelector<HTMLElement>(
      `[data-conversation-message="${CSS.escape(id)}"]`,
    );
    const target = scrollRoot();
    if (!message || !target) return;
    // Historical reading must not be undone by the next streaming update.
    // Keep the pause through the jump's scroll event, including when the
    // target is clamped to the maximum scroll offset.
    cancelAnimationFrame(turnJumpFrame);
    jumpingToTurn = true;
    stick = false;
    activeTurnId = id;
    const viewportTop = documentScroll
      ? 0
      : target.getBoundingClientRect().top + target.clientTop;
    const margin = Number.parseFloat(getComputedStyle(message).scrollMarginTop);
    const top =
      (documentScroll ? window.scrollY : target.scrollTop) +
      message.getBoundingClientRect().top -
      viewportTop -
      margin;
    // Scroll only the owner viewport, never its clipped pane or the page.
    if (documentScroll) window.scrollTo({ top, behavior: "instant" });
    else target.scrollTo({ top, behavior: "instant" });
    follow();
    turnJumpFrame = requestAnimationFrame(() => {
      turnJumpFrame = requestAnimationFrame(() => {
        turnJumpFrame = 0;
        jumpingToTurn = false;
      });
    });
  }
  // The viewport belongs to ScrollArea, which takes no scroll handler.
  $effect(() => {
    if (documentScroll) {
      window.addEventListener("scroll", follow, { passive: true });
      return () => window.removeEventListener("scroll", follow);
    }
    const element = scroller;
    if (!element) return;
    element.addEventListener("scroll", follow, { passive: true });
    return () => element.removeEventListener("scroll", follow);
  });
  function latest() {
    cancelAnimationFrame(turnJumpFrame);
    turnJumpFrame = 0;
    jumpingToTurn = false;
    const target = scrollRoot();
    if (documentScroll) window.scrollTo({ top: target?.scrollHeight ?? 0 });
    else target?.scrollTo({ top: target.scrollHeight });
    stick = true;
  }
  // New output keeps the newest entry in view unless the reader scrolled up.
  $effect(() => {
    void entries;
    void request;
    void chat?.turn.running;
    if (untrack(() => stick)) scrollToBottom();
  });
  // The composer growing (a tap on the compact bar) or the on-screen keyboard
  // shrinks the transcript from below: a reader at the end stays at the end.
  $effect(() => {
    if (!scroller) return;
    const follower = observeTranscriptResize({
      viewport: scroller,
      transcript,
      dock,
      scrollRoot,
      following: () => stick,
      jumping: () => jumpingToTurn,
      scrollToBottom,
      onResize: scheduleActiveTurn,
      onManualScroll: scrollMode.markManualScroll,
    });
    resizeFollower = follower;
    return () => {
      follower.dispose();
      resizeFollower = undefined;
    };
  });

  function answer(requestId: string, reply: RequestAnswer) {
    answering = true;
    actions
      .answer(requestId, reply)
      .then(
        () => (error = ""),
        (err: unknown) => (error = errorMessage(err)),
      )
      .finally(() => (answering = false));
  }
</script>

<FileDropZone
  bind:ref={root}
  class="@container relative flex h-full min-h-0 flex-col bg-background"
  data-chat-view
  data-document-scroll={documentScroll ? "true" : undefined}
  role="region"
  aria-label="聊天"
  {uploads}
  blocked={!!uploadBlocked}
  label="松开以上传文件到聊天"
  onfocusin={scrollMode.focusEditing}
  onfocusout={() => {
    requestAnimationFrame(scrollMode.syncEditing);
  }}
>
  <div class="relative flex min-h-0 flex-1 flex-col">
    <ScrollArea
      bind:viewportRef={scroller}
      type="hover"
      class="min-h-0 flex-1 [&_[data-slot=scroll-area-viewport]>div]:!block"
      scrollbarYClasses="[&_[data-slot=scroll-area-thumb]]:bg-foreground/25 hover:[&_[data-slot=scroll-area-thumb]]:bg-foreground/40"
    >
      <div
        bind:this={transcript}
        class="mx-auto flex max-w-3xl flex-col gap-2.5 px-4 py-6"
      >
        <ChatTurnStatus mode="notices" truncated={model.truncated} {chat} />
        <ol class="flex flex-col gap-2.5" aria-label="对话">
          {#each entries as entry (`${entry.type}:${entry.id}`)}
            <li
              class={documentScroll
                ? "min-w-0 scroll-mt-16"
                : "min-w-0 scroll-mt-4"}
              data-conversation-message={entry.type === "message" &&
              entry.role !== "thought" &&
              turnIds.has(entry.id)
                ? entry.id
                : undefined}
            >
              <TimelineItem
                {entry}
                {sessionId}
                preview={(path) => uploads.preview?.(path)}
                {waiting}
                {onreview}
                {onopenlocation}
              />
            </li>
          {/each}
        </ol>
        <ChatTurnStatus {chat} entries={model.entries} {request} />
      </div>
    </ScrollArea>
    {#if turns.length > 1}
      <ConversationTurns
        items={turns}
        activeId={activeTurnId}
        {documentScroll}
        onselect={jumpToTurn}
      />
    {/if}
    {#if chat?.plan?.entries.length}
      <!-- Wide panes leave empty space right of the centred transcript: pin
           the plan there instead of stacking it above the input. -->
      <div
        class="pointer-events-none absolute top-3 right-6 bottom-6 z-10 hidden w-60 @[82rem]:flex @[82rem]:flex-col"
      >
        <ChatPlan plan={chat.plan} variant="panel" />
      </div>
    {/if}
    {#if !stick && !documentScroll}
      <!-- Anchored to the message area, so a taller composer never covers it. -->
      <Button
        variant="outline"
        size="sm"
        raised
        class="absolute bottom-3 left-1/2 z-10 -translate-x-1/2"
        onclick={latest}><IconArrowDown aria-hidden="true" />回到最新</Button
      >
    {/if}
  </div>
  <div
    bind:this={dock}
    data-chat-dock
    class="relative mx-auto w-full max-w-3xl space-y-2 px-3 pb-safe-2 sm:px-4 [@media(height<480px)]:max-h-full [@media(height<480px)]:overflow-y-auto"
  >
    {#if !stick && documentScroll}
      <div
        class="pointer-events-none absolute inset-x-0 bottom-full pb-3 text-center"
      >
        <Button
          variant="outline"
          size="sm"
          raised
          class="pointer-events-auto"
          onclick={latest}><IconArrowDown aria-hidden="true" />回到最新</Button
        >
      </div>
    {/if}
    <ChatTurnStatus mode="dock" {error} {unavailable} {onterminal} {chat} />
    {#if chat?.plan?.entries.length}
      <div class="@[82rem]:hidden"><ChatPlan plan={chat.plan} /></div>
    {/if}
    {#if chat?.queue.items.length}
      <ChatQueue
        queue={chat.queue}
        running={chat.turn.running}
        disabled={!!blocked}
        {actions}
        onerror={(message) => (error = message)}
      />
    {/if}
    {#if location && (request?.kind === "permission" || request?.kind === "form")}<ChatLocation
        workspace={location.workspace}
        cwd={location.cwd}
      />{/if}
    {#if request?.kind === "permission"}
      <PermissionPrompt
        {request}
        tool={requestTool}
        disabled={!!blocked || answering}
        onanswer={(reply) => answer(request.requestId, reply)}
      />
    {:else if request?.kind === "form"}
      {#key request.requestId}
        <FormPrompt
          {request}
          disabled={!!blocked || answering}
          onanswer={(reply) => answer(request.requestId, reply)}
        />
      {/key}
    {:else if chat}
      <ChatComposer
        bind:this={composer}
        {draft}
        {drafts}
        {chat}
        entries={model.entries}
        {uploads}
        {location}
        unavailable={blocked}
        {uploadUnavailable}
        {actions}
        onerror={(message) => (error = message)}
        {quickMenu}
      />
    {/if}
  </div>
</FileDropZone>
