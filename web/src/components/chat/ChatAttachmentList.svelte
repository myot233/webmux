<script lang="ts">
  import { onMount, tick } from "svelte";
  import { animate } from "motion/mini";
  import IconFile from "~icons/tabler/file";
  import IconX from "~icons/tabler/x";
  import { Button } from "$lib/components/ui/button";
  import type { ChatUploads } from "../../hooks/upload.svelte";
  import ChatAttachmentImage from "./ChatAttachmentImage.svelte";

  // The attachments waiting to go with the next message, at the top of an
  // input card (the chat composer and quick start), each removable.
  let { uploads, disabled }: { uploads: ChatUploads; disabled: boolean } =
    $props();

  let list = $state<HTMLDivElement | null>(null);
  let last: string | undefined;
  let revision = 0;
  let restore: (() => void) | undefined;
  function reset() {
    revision++;
    restore?.();
    restore = undefined;
  }
  $effect.pre(() => {
    const key = JSON.stringify(uploads.attachments.map((item) => item.path));
    const node = list;
    if (!node) return;
    const changed = last !== undefined && last !== key;
    last = key;
    if (!changed) return;
    const before = node.getBoundingClientRect();
    const composer = node.closest("[data-chat-composer]");
    const wasCompact = composer?.hasAttribute("data-compact");
    const boxes = new WeakMap(
      Array.from(node.querySelectorAll("li"), (child) => [
        child,
        child.getBoundingClientRect(),
      ]),
    );
    reset();
    const version = revision;
    void tick().then(() => {
      if (
        version !== revision ||
        !node.isConnected ||
        matchMedia("(prefers-reduced-motion: reduce)").matches
      )
        return;
      const after = node.getBoundingClientRect();
      const controls: ReturnType<typeof animate>[] = [];
      const children = Array.from(node.querySelectorAll("li"));
      // The composer already owns height during its compact/full morph.
      if (
        wasCompact === composer?.hasAttribute("data-compact") &&
        before.height !== after.height
      ) {
        node.hidden = false;
        node.style.overflow = "clip";
        controls.push(
          animate(
            node,
            { height: [`${before.height}px`, `${after.height}px`] },
            { duration: 0.22, ease: [0.22, 1, 0.36, 1] },
          ),
        );
      }
      const current = node.getBoundingClientRect();
      for (const child of children) {
        const old = boxes.get(child);
        const box = child.getBoundingClientRect();
        controls.push(
          old
            ? animate(
                child,
                {
                  transform: [
                    `translate(${old.left - before.left - (box.left - current.left)}px, ${old.top - before.top - (box.top - current.top)}px)`,
                    "translate(0px, 0px)",
                  ],
                },
                { duration: 0.22 },
              )
            : animate(child, { opacity: [0, 1] }, { duration: 0.18 }),
        );
      }
      restore = () => {
        controls.forEach((control) => control.cancel());
        node.style.removeProperty("height");
        node.style.removeProperty("overflow");
        node.hidden = !uploads.attachments.length;
        children.forEach((child) => {
          child.style.removeProperty("transform");
          child.style.removeProperty("opacity");
        });
      };
      void Promise.all(controls).then(() => {
        if (revision === version) reset();
      });
    });
  });
  onMount(() => {
    const reduced = matchMedia("(prefers-reduced-motion: reduce)");
    window.addEventListener("resize", reset);
    reduced.addEventListener("change", reset);
    return () => {
      reset();
      window.removeEventListener("resize", reset);
      reduced.removeEventListener("change", reset);
    };
  });
</script>

<div bind:this={list} hidden={!uploads.attachments.length}>
  <ul class="flex flex-wrap gap-1 border-b px-2 py-1.5" aria-label="待发送附件">
    {#each uploads.attachments as item (item.path)}
      <li
        class="flex max-w-full min-w-0 items-center gap-1.5 rounded-sm border bg-muted/50 px-2 py-1 text-xs"
      >
        {#if item.kind === "image"}
          <ChatAttachmentImage
            attachment={item}
            preview={uploads.preview?.(item.path)}
            size="chip"
          />
        {:else}
          <IconFile class="size-3.5 shrink-0" aria-hidden="true" />
        {/if}
        <span class="min-w-0 truncate">{item.name ?? item.path}</span>
        <Button
          variant="quiet"
          size="icon-xs"
          aria-label={`移除附件 ${item.name ?? item.path}`}
          title="移除附件"
          {disabled}
          onclick={() => uploads.remove(item.path)}
          ><IconX aria-hidden="true" /></Button
        >
      </li>
    {/each}
  </ul>
</div>
