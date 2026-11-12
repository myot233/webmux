<script lang="ts">
  import type { ToolEntry } from "../../domains/chat/chat-model";
  import type { ChatCodeLink } from "../../domains/code/markdown-link";
  import ChatDisclosure from "./ChatDisclosure.svelte";
  import ToolDetails from "./ToolDetails.svelte";
  import ToolSummary from "./ToolSummary.svelte";

  // One tool call as a quiet row. It is keyed by toolCallId, so later
  // updates change this row in place; the details show its output and open
  // diffs in the code panel.
  let {
    tool,
    waiting = false,
    onreview,
    onopenlocation,
  }: {
    tool: ToolEntry;
    /** A permission request on this call waits for an answer. */
    waiting?: boolean;
    /** Opens a changed file in the code panel. */
    onreview: (path: string) => void;
    /** Opens a referenced code file or line in the code panel. */
    onopenlocation?: (target: ChatCodeLink) => void;
  } = $props();
</script>

<div class="min-w-0" data-tool-call={tool.id}>
  <ChatDisclosure>
    {#snippet summary()}<ToolSummary {tool} {waiting} />{/snippet}
    <ToolDetails {tool} {onreview} {onopenlocation} />
  </ChatDisclosure>
</div>
