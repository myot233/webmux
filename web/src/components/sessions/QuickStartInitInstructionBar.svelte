<script lang="ts">
  import IconNotes from "~icons/tabler/notes";
  import { Button } from "$lib/components/ui/button";
  import { Checkbox } from "$lib/components/ui/checkbox";
  import { Label } from "$lib/components/ui/label";
  import * as Popover from "$lib/components/ui/popover";
  import {
    expandInitInstruction,
    usesArtifactDir,
  } from "../../domains/settings/quick-commands";
  import { quickCommands } from "../../domains/settings/quick-commands.svelte";
  import type { Repository } from "../../contracts/schemas";

  let {
    compact = false,
    repository,
    sending,
    useInstruction,
    onchange,
  }: {
    compact?: boolean;
    repository: Repository;
    sending: boolean;
    useInstruction: boolean;
    onchange: (use: boolean) => void;
  } = $props();

  const instructionPreview = $derived(
    expandInitInstruction(repository.initInstruction ?? "", quickCommands.list),
  );
</script>

<div
  class={[
    "flex items-center gap-2 border-b px-3 py-1",
    compact && "composer-mobile:hidden",
  ]}
>
  <Checkbox
    id="use-init-instruction"
    disabled={sending}
    bind:checked={() => useInstruction, onchange}
  />
  <Label for="use-init-instruction">使用初始化指令</Label>
  <Popover.Root>
    <Popover.Trigger>
      {#snippet child({ props })}
        <Button
          {...props}
          variant="quiet"
          size="xs"
          title="预览将附带的初始化指令"
          ><IconNotes aria-hidden="true" />预览</Button
        >
      {/snippet}
    </Popover.Trigger>
    <Popover.Content align="start" class="w-80 space-y-2">
      <p class="font-medium">{repository.name} 的初始化指令</p>
      <p
        class="max-h-64 overflow-y-auto whitespace-pre-wrap break-words text-muted-foreground"
      >
        {instructionPreview.hasUser
          ? instructionPreview.text.replaceAll("{{user}}", "〔你的消息〕")
          : instructionPreview.text}
      </p>
      {#if instructionPreview.missing.length}
        <p class="text-xs text-warning" role="status">
          找不到快捷指令：{instructionPreview.missing
            .map((name) => `{{${name}}}`)
            .join("、")}，发送时会被服务端拒绝。
        </p>
      {/if}
      {#if !instructionPreview.hasUser}
        <p class="text-xs text-muted-foreground">
          指令没有 {"{{user}}"}，会放在你的消息前面，空一行后作为第一条消息一起发送。
        </p>
      {/if}
      {#if usesArtifactDir(instructionPreview.text)}
        <p class="text-xs text-muted-foreground">
          {"{{dir}}"} 会在会话创建后替换为它的临时文件夹路径。
        </p>
      {/if}
      {#if instructionPreview.hasWebmux}
        <p class="text-xs text-muted-foreground">
          {"{{webmux}}"} 会在会话创建后替换为 WebMux 工具说明：手册地址、命令路径与临时文件夹。
        </p>
      {/if}
      <p class="text-xs text-muted-foreground">
        从终端开始时默认不勾选；这次的选择只影响本次发送。在首页工具栏“⋯”菜单的“仓库设置…”中修改。
      </p>
    </Popover.Content>
  </Popover.Root>
</div>
