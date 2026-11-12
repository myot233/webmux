<script lang="ts">
  import { Button } from "$lib/components/ui/button";
  import { untrack } from "svelte";
  import { createHandoffEditor } from "../../hooks/sessions/handoff.svelte";
  import HandoffForm from "./HandoffForm.svelte";
  import ResponsiveDialog from "../shared/ResponsiveDialog.svelte";
  let { sourceId, onclose }: { sourceId: string; onclose: () => void } =
    $props();
  const editor = createHandoffEditor(
    untrack(() => sourceId),
    false,
  );
  let closing = false;
  function close() {
    if (closing || editor.busy) return;
    closing = true;
    void editor.closeSave().finally(onclose);
  }
</script>

<ResponsiveDialog
  title="更换 Agent"
  description="保持当前对话和工作位置，先检查交接历史，再单独发送用户任务。"
  onclose={close}
  busy={editor.busy || editor.saving}
  class="sm:max-w-2xl"
>
  {#if editor.loading}<p role="status" class="text-sm">
      正在恢复交接草稿…
    </p>{:else}<HandoffForm {editor} fork={false} />{/if}
  <div class="flex justify-end pt-4">
    <Button
      variant="outline"
      disabled={editor.busy || editor.saving}
      onclick={close}>保存并关闭</Button
    >
  </div>
</ResponsiveDialog>
