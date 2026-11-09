<script lang="ts">
  import IconArrowRight from "~icons/tabler/arrow-right";
  import { Button } from "$lib/components/ui/button";
  import * as Dialog from "$lib/components/ui/dialog";
  import { Textarea } from "$lib/components/ui/textarea";
  import type { TimelineEntry } from "../../chat-model";
  import { quickStartMemory } from "../../quick-start";
  import type { SessionInfo } from "../../schemas";
  import { openPage } from "../../shell.svelte";

  let {
    entries,
    location,
  }: {
    entries: readonly TimelineEntry[];
    location: Pick<SessionInfo, "cwd" | "workspace">;
  } = $props();
  let open = $state(false);
  let text = $state("");
  let error = $state("");
  const messages = $derived(
    entries.flatMap((entry) =>
      entry.type === "message" && entry.role !== "thought" && entry.text
        ? [entry]
        : [],
    ),
  );

  function prepare() {
    error = "";
    const first = messages.find((entry) => entry.role === "user");
    const recent = messages.filter((entry) => entry !== first).slice(-8);
    const excerpts = (first ? [first, ...recent] : recent).map((entry) => {
      const text = entry.text.slice(0, 1600);
      return `${entry.role === "user" ? "用户" : "Agent"}：${text}${text.length < entry.text.length || entry.truncated ? "…" : ""}`;
    });
    const files = new Set(
      entries.flatMap((entry) =>
        entry.type === "tool"
          ? entry.content.flatMap((item) =>
              item.type === "diff" && item.path ? [item.path] : [],
            )
          : [],
      ),
    );
    text = [
      "请基于下面的会话交接继续工作。",
      `原工作目录：${location.cwd}`,
      ...(files.size
        ? [`报告过改动的文件：${[...files].slice(0, 12).join("、")}`]
        : []),
      ...excerpts,
    ].join("\n\n");
  }

  function continueHandoff() {
    const saved = quickStartMemory.loadDraft();
    const combined = [saved.text, text.trim()].filter(Boolean).join("\n\n");
    if (combined.length > 100_000) {
      error = "合并后的草稿过长，请缩短交接内容。";
      return;
    }
    quickStartMemory.saveDraftState({
      ...saved,
      repositoryId: saved.repositoryId ?? location.workspace?.repositoryId,
      text: combined,
    });
    open = false;
    openPage("/new");
  }
</script>

{#if messages.length}
  <Dialog.Root
    bind:open
    onOpenChange={(next) => {
      if (next) prepare();
    }}
  >
    <Dialog.Trigger>
      {#snippet child({ props })}
        <Button {...props} variant="ghost" size="xs">
          <IconArrowRight aria-hidden="true" />交接到新聊天
        </Button>
      {/snippet}
    </Dialog.Trigger>
    <Dialog.Content class="sm:max-w-lg">
      <Dialog.Header>
        <Dialog.Title>交接到新聊天</Dialog.Title>
        <Dialog.Description
          >检查并编辑交接内容，再到新聊天选择
          Agent。交接会追加到已有的新聊天草稿。</Dialog.Description
        >
      </Dialog.Header>
      <Textarea aria-label="交接内容" bind:value={text} rows={8} />
      {#if error}<p role="alert" class="text-sm text-destructive">
          {error}
        </p>{/if}
      <Dialog.Footer>
        <Button disabled={!text.trim()} onclick={continueHandoff}
          >打开交接草稿</Button
        >
      </Dialog.Footer>
    </Dialog.Content>
  </Dialog.Root>
{/if}
