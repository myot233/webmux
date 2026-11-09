<script lang="ts">
  import type { Snippet } from "svelte";
  import type { HTMLAttributes } from "svelte/elements";
  import { attachFileDrop } from "../hooks/file-drop";
  import type { ChatUploads } from "../hooks/upload.svelte";

  // An area that takes files dropped on it as attachments of the next
  // message (the chat view and quick start), with a dashed overlay while
  // files hover. Other drags pass through untouched.
  let {
    uploads,
    blocked = false,
    label,
    children,
    ...rest
  }: {
    uploads: ChatUploads;
    /** Uploads are refused now; a drop does nothing. */
    blocked?: boolean;
    /** The overlay's text while files hover. */
    label: string;
    children: Snippet;
  } & Omit<HTMLAttributes<HTMLDivElement>, "children"> = $props();
  let dragging = $state(false);
</script>

<div
  {...rest}
  {@attach (host) =>
    attachFileDrop(host, {
      onEnter: () => {
        if (blocked) return false;
        dragging = true;
      },
      onOver: (transfer) => {
        transfer.dropEffect = blocked || uploads.progress ? "none" : "copy";
      },
      onLeave: () => {
        dragging = false;
      },
      onDrop: (transfer) => {
        dragging = false;
        if (!blocked) void uploads.uploadFiles(Array.from(transfer.files));
      },
    })}
>
  {#if dragging}
    <div
      class="pointer-events-none absolute inset-2 z-20 flex items-center justify-center rounded-lg border-2 border-dashed border-ring bg-background/85 text-sm"
      role="status"
    >
      {uploads.progress ? "请等待当前上传完成" : label}
    </div>
  {/if}
  {@render children()}
</div>
