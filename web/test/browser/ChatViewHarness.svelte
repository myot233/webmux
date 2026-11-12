<script lang="ts">
  import { onDestroy, untrack } from "svelte";
  import { uploadFile } from "../../src/platform/api";
  import QuickCommandMenu from "../../src/components/settings/QuickCommandMenu.svelte";
  import type { ButtonSize } from "$lib/components/ui/button";
  import ChatView from "../../src/components/chat/ChatView.svelte";
  import { chatActions } from "../../src/platform/chat-api";
  import { createChatSocket } from "../../src/hooks/chat/chat-socket.svelte";
  import { useChatUpload } from "../../src/hooks/shared/upload.svelte";
  import type { ChatCodeLink } from "../../src/domains/code/markdown-link";
  import type { SessionInfo } from "../../src/contracts/schemas";
  import {
    createTerminalDrafts,
    type TerminalDrafts,
  } from "../../src/domains/terminal/terminal-draft.svelte";

  // The chat view on its real channel and commands, as the session page
  // wires it, without the terminal around it.
  let {
    id,
    drafts: providedDrafts,
    location,
    allowDocumentScroll = false,
    clippedHost = false,
  }: {
    id: string;
    drafts?: TerminalDrafts;
    location?: Pick<SessionInfo, "workspace" | "cwd">;
    allowDocumentScroll?: boolean;
    clippedHost?: boolean;
  } = $props();
  const session = untrack(() => id);
  const chat = createChatSocket(session);
  chat.open();
  onDestroy(() => chat.dispose());
  const drafts = untrack(() => providedDrafts ?? createTerminalDrafts());
  const draft = drafts.get(session);
  const actions = chatActions(session);
  let reviewed = $state("");
  let openedLocation = $state<ChatCodeLink | null>(null);
  let error = $state("");
  let terminalRequests = $state(0);
  let editing = $state(false);
  const documentScroll = $derived(allowDocumentScroll && !editing);
  const uploads = useChatUpload(
    (file, progress, signal) => uploadFile(session, file, progress, signal),
    (message) => {
      error = message;
    },
    {
      get: () => draft.attachments,
      set: (next) => drafts.setAttachments(draft, next),
    },
  );
</script>

{#snippet quickMenu(side: "top" | "bottom", size?: ButtonSize)}
  <QuickCommandMenu
    commands={[]}
    target="chat"
    unavailable=""
    disabled={false}
    {side}
    {size}
    run={() => {}}
    manage={() => {}}
    onCloseAutoFocus={() => {}}
  />
{/snippet}

<div
  data-chat-test-host
  style={clippedHost ? "height: 700px; overflow: hidden" : undefined}
>
  {#if clippedHost}<div style="height: 96px"></div>{/if}
  <div style={documentScroll ? "min-height: 100dvh" : "height: 800px"}>
    <ChatView
      sessionId={session}
      model={chat.model}
      chat={chat.state}
      connection={chat.connection}
      {draft}
      {drafts}
      {uploads}
      {location}
      {actions}
      {quickMenu}
      {documentScroll}
      oneditchange={(value) => (editing = value)}
      onreview={(path) => (reviewed = path)}
      onopenlocation={(target) => (openedLocation = target)}
      onterminal={() => (terminalRequests += 1)}
    />
  </div>
</div>
<output aria-label="Reviewed path">{reviewed}</output>
<output aria-label="Opened location"
  >{openedLocation ? JSON.stringify(openedLocation) : ""}</output
>
<output aria-label="Draft">{draft.text}</output>
<output aria-label="Upload error">{error}</output>
<output aria-label="Terminal requests">{terminalRequests}</output>
<output aria-label="Editing">{editing}</output>
