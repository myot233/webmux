<script lang="ts">
  import { Input } from "$lib/components/ui/input";

  // A hidden multi-file picker; the parent opens it from its own button.
  let {
    upload,
    label = "选择上传文件",
  }: {
    upload: (files: File[]) => Promise<void>;
    label?: string;
  } = $props();
  let input = $state<HTMLInputElement | null>(null);

  export function open() {
    input?.click();
  }
</script>

<Input
  bind:ref={input}
  type="file"
  multiple
  class="hidden"
  aria-label={label}
  onchange={(event) => {
    const files = Array.from(event.currentTarget.files ?? []);
    event.currentTarget.value = "";
    void upload(files);
  }}
/>
