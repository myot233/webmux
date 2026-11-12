<script lang="ts">
  import { untrack } from "svelte";
  import { Button } from "$lib/components/ui/button";
  import { createHandoffEditor } from "../hooks/sessions/handoff.svelte";
  import { navigate } from "../platform/api";
  import HandoffForm from "../components/chat/HandoffForm.svelte";
  import SidebarButton from "../components/shell/SidebarButton.svelte";
  let { sourceId }: { sourceId: string } = $props();
  const editor = createHandoffEditor(
    untrack(() => sourceId),
    true,
  );
</script>

<div class="flex h-full min-h-0 min-w-0 flex-col">
  <header
    class="app-header flex shrink-0 items-center gap-2 border-b bg-card px-2"
  >
    <SidebarButton class="sm:hidden" />
    <h1 class="min-w-0 flex-1 truncate text-base font-medium">交接到新聊天</h1>
    <Button variant="ghost" size="sm" onclick={() => navigate("/new")}
      >返回新聊天</Button
    >
  </header>
  <main class="min-h-0 flex-1 overflow-y-auto p-4 sm:p-6">
    <div class="mx-auto max-w-2xl space-y-4">
      <p class="text-sm text-muted-foreground">
        从服务端已保存的对话准备交接。新聊天可沿用来源位置，或选择其他目录、已有工作树、新工作树。
      </p>
      {#if editor.loading}<p role="status" class="text-sm">
          正在恢复交接草稿…
        </p>{:else}<HandoffForm {editor} fork />{/if}
    </div>
  </main>
</div>
