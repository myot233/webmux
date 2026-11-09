<script lang="ts">
  import type { Snippet } from "svelte";
  import type { HTMLTextareaAttributes } from "svelte/elements";
  import IconArrowUp from "~icons/tabler/arrow-up";
  import IconPlayerStopFilled from "~icons/tabler/player-stop-filled";
  import {
    Button,
    type ButtonSize,
    type ButtonVariant,
  } from "$lib/components/ui/button";
  import { Textarea } from "$lib/components/ui/textarea";
  import { useComposerCompact } from "../../hooks/composer-compact.svelte";
  import { useComposerMorph } from "../../hooks/composer-morph.svelte";
  import type { ChatUploads } from "../../hooks/upload.svelte";
  import FileUploadProgress from "../FileUploadProgress.svelte";
  import ChatAttachmentList from "./ChatAttachmentList.svelte";

  // The message box of an open chat and of the quick start page: one card
  // with the text, its attachments, the controls that act on the message and
  // a send (or stop) button. Layout, sizes and the phone's one-line resting
  // bar live only here; the two screens fill the slots with what differs.
  export type ComposerContext = {
    /** Resting one-line bar on a phone: secondary controls are hidden. */
    compact: boolean;
    /** The size every control in the toolbar row uses. */
    size: ButtonSize;
    /** Secondary controls beside send are borderless: send is the one action. */
    variant: ButtonVariant;
  };
  export type ComposerAction = {
    kind: "send" | "stop";
    label: string;
    /** Tooltip; the label when absent. */
    title?: string;
    disabled: boolean;
    onclick: () => void;
  };

  let {
    value,
    oninput,
    placeholder,
    label,
    disabled = false,
    uploads,
    uploadsDisabled = false,
    busy,
    action,
    textarea = $bindable(null),
    textareaProps = {},
    location,
    menu,
    notices,
    hints,
    leading,
    accessories,
    below,
  }: {
    value: string;
    oninput: (event: Event & { currentTarget: HTMLTextAreaElement }) => void;
    placeholder: string;
    /** Accessible name of the text box. */
    label: string;
    disabled?: boolean;
    uploads: ChatUploads;
    /** Disables removing attachments. */
    uploadsDisabled?: boolean;
    /** Content or work that keeps the card expanded on a phone. */
    busy: boolean;
    action: ComposerAction;
    textarea?: HTMLTextAreaElement | null;
    /** Extra text box attributes: key handling, selection, ARIA. */
    textareaProps?: HTMLTextareaAttributes;
    /** Where the work happens, above the card. */
    location?: Snippet<[ComposerContext]>;
    /** At the top of the card, e.g. a command list. */
    menu?: Snippet<[ComposerContext]>;
    /** Warnings between the attachments and the text. */
    notices?: Snippet;
    /** Notes right under the text. */
    hints?: Snippet;
    /** Left of the toolbar row, e.g. attach and the options. */
    leading?: Snippet<[ComposerContext]>;
    /** Right of the toolbar row, before send, e.g. voice and commands. */
    accessories?: Snippet<[ComposerContext]>;
    /** Under the card. */
    below?: Snippet;
  } = $props();

  let container = $state<HTMLDivElement | null>(null);
  let card = $state<HTMLDivElement | null>(null);
  const compactState = useComposerCompact(
    () => container,
    () => busy,
  );
  const context = $derived<ComposerContext>({
    compact: compactState.compact,
    size: "icon",
    variant: "ghost",
  });
  const compact = $derived(compactState.compact);
  useComposerMorph(
    () => card,
    () => compact,
  );

  export function focus() {
    textarea?.focus();
  }
</script>

<div
  bind:this={container}
  class="space-y-2 sm:space-y-1.5"
  data-chat-composer
  data-compact={compact ? "" : undefined}
  {...compactState.containerProps}
>
  {@render location?.(context)}
  <!-- The card shows the message box's focus once; the box itself has none. -->
  <div
    bind:this={card}
    data-composer-card
    class={[
      "@container rounded-lg border bg-card shadow-sm transition has-[textarea:focus]:border-ring has-[textarea:focus]:ring-3 has-[textarea:focus]:ring-ring/20",
      compact &&
        "composer-mobile:flex composer-mobile:items-center composer-mobile:gap-1 composer-mobile:p-0.5",
    ]}
  >
    {@render menu?.(context)}
    <ChatAttachmentList {uploads} disabled={uploadsDisabled} />
    {@render notices?.()}
    <div
      class={[
        "px-2 pt-1",
        compact &&
          "composer-mobile:min-w-0 composer-mobile:flex-1 composer-mobile:px-1 composer-mobile:py-0",
      ]}
    >
      <Textarea
        bind:ref={textarea}
        variant="bare"
        rows={compact ? 1 : 2}
        class={[
          "max-h-48 min-h-14 resize-none",
          compact &&
            "composer-mobile:max-h-9 composer-mobile:min-h-9 composer-mobile:overflow-hidden",
        ]}
        {placeholder}
        aria-label={label}
        {disabled}
        {value}
        {oninput}
        onpaste={(event) => uploads.filePaste(event)}
        {...textareaProps}
      />
      {@render hints?.()}
    </div>
    {#if uploads.progress}<FileUploadProgress
        progress={uploads.progress}
        cancel={() => uploads.cancel()}
        variant="card"
      />{/if}
    <div
      class={[
        "flex flex-wrap items-center justify-end gap-1 px-2 pb-2",
        compact && "composer-mobile:shrink-0 composer-mobile:p-0",
      ]}
    >
      <div
        class={[
          "mr-auto flex min-w-0 items-center gap-1",
          compact && "composer-mobile:hidden",
        ]}
      >
        {@render leading?.(context)}
      </div>
      <span
        class={["flex items-center gap-1", compact && "composer-mobile:hidden"]}
      >
        {@render accessories?.(context)}
      </span>
      <Button
        data-composer-action
        size={context.size}
        aria-label={action.label}
        title={action.title ?? action.label}
        disabled={action.disabled}
        onclick={action.onclick}
      >
        {#if action.kind === "stop"}<IconPlayerStopFilled
            aria-hidden="true"
          />{:else}<IconArrowUp aria-hidden="true" />{/if}
      </Button>
    </div>
  </div>
  {@render below?.()}
</div>
