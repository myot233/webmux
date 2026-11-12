<script lang="ts">
  import IconPaperclip from "~icons/tabler/paperclip";
  import { Button } from "$lib/components/ui/button";
  import type { ChatUploads } from "../../hooks/shared/upload.svelte";
  import UploadFileInput from "../shared/UploadFileInput.svelte";

  // The input card's "上传文件" button and its hidden file picker, shared by
  // the chat composer and quick start. The chosen files become attachments
  // of the next message.
  let {
    uploads,
    disabled,
    unavailable = "",
  }: {
    uploads: ChatUploads;
    disabled: boolean;
    /** Why uploads are refused; "" when they are possible. */
    unavailable?: string;
  } = $props();
  let fileInput = $state<{ open(): void }>();
</script>

<Button
  variant="ghost"
  size="icon"
  aria-label="上传文件"
  title={unavailable || "上传文件"}
  disabled={disabled || uploads.progress !== null || !!unavailable}
  onclick={() => fileInput?.open()}><IconPaperclip aria-hidden="true" /></Button
>
<UploadFileInput bind:this={fileInput} upload={uploads.uploadFiles} />
