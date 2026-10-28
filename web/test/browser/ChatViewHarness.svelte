<script lang="ts">
  import { onDestroy, untrack } from "svelte";
  import ChatView from "../../src/components/chat/ChatView.svelte";
  import { chatActions } from "../../src/chat-api";
  import { createChatSocket } from "../../src/hooks/chat-socket.svelte";
  import { useChatUpload } from "../../src/hooks/upload.svelte";
  import { createTerminalDrafts } from "../../src/terminal-draft.svelte";

  // The chat view on its real channel and commands, as the session page
  // wires it, without the terminal around it.
  let { id }: { id: string } = $props();
  const session = untrack(() => id);
  const chat = createChatSocket(session);
  chat.open();
  onDestroy(() => chat.dispose());
  const drafts = createTerminalDrafts();
  const draft = drafts.get(session);
  const actions = chatActions(session);
  let reviewed = $state("");
  let error = $state("");
  const uploads = useChatUpload(session, drafts, draft, (message) => {
    error = message;
  });
</script>

<div style="height: 800px">
  <ChatView
    model={chat.model}
    chat={chat.state}
    connection={chat.connection}
    {draft}
    {drafts}
    {uploads}
    {actions}
    onreview={(path) => (reviewed = path)}
    onlogin={() => {}}
  />
</div>
<output aria-label="Reviewed path">{reviewed}</output>
<output aria-label="Draft">{draft.text}</output>
<output aria-label="Upload error">{error}</output>
