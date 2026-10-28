<script lang="ts">
  import IconShieldQuestion from "~icons/tabler/shield-question";
  import { ListButton } from "$lib/components/ui/list-button";
  import type { RequestAnswer } from "../../chat-api";
  import type { ToolEntry } from "../../chat-model";
  import type { ChatRequest } from "../../schemas";

  // A permission request docked where the input was: the agent's own options
  // as numbered choices (keys 1–9 pick one). The first answer from any
  // device wins; the request closes everywhere through the next state.
  let {
    request,
    tool,
    disabled,
    onanswer,
  }: {
    request: ChatRequest;
    tool: ToolEntry | undefined;
    disabled: boolean;
    onanswer: (answer: RequestAnswer) => void;
  } = $props();
  const options = $derived(request.options ?? []);
  const title = $derived(request.title || tool?.title || "Agent 请求授权");
  function shortcut(event: KeyboardEvent) {
    const option = options[Number(event.key) - 1];
    const target = event.target;
    if (
      !option ||
      disabled ||
      event.ctrlKey ||
      event.metaKey ||
      event.altKey ||
      (target instanceof HTMLElement &&
        (target.isContentEditable ||
          target.closest("input, textarea, select, .xterm")))
    )
      return;
    event.preventDefault();
    onanswer({ outcome: "selected", optionId: option.optionId });
  }
</script>

<svelte:window onkeydown={shortcut} />

<section
  class="space-y-3 rounded-xl border border-warning/40 bg-card p-3 shadow-sm"
  aria-label="等待授权"
>
  <div class="space-y-1.5">
    <p class="flex items-center gap-1.5 text-sm font-medium">
      <IconShieldQuestion
        class="size-4 shrink-0 text-warning"
        aria-hidden="true"
      />
      等待授权
    </p>
    <p
      class={tool?.kind === "execute"
        ? "rounded-md bg-muted px-3 py-2 font-mono text-xs break-all whitespace-pre-wrap"
        : "text-sm break-words"}
    >
      {title}
    </p>
  </div>
  <ol class="space-y-1">
    {#each options as option, i (option.optionId)}
      <li>
        <ListButton
          variant="option"
          class="flex w-full items-center any-pointer-coarse:min-h-11"
          {disabled}
          aria-keyshortcuts={i < 9 ? String(i + 1) : undefined}
          onclick={() =>
            onanswer({ outcome: "selected", optionId: option.optionId })}
        >
          <span class="flex min-w-0 items-center gap-3">
            <span
              class="flex size-5 shrink-0 items-center justify-center rounded border text-xs text-muted-foreground tabular-nums"
              aria-hidden="true">{i + 1}</span
            >
            <span
              class={option.kind.startsWith("reject")
                ? "text-sm text-destructive"
                : "text-sm"}>{option.name}</span
            >
          </span>
        </ListButton>
      </li>
    {/each}
  </ol>
</section>
