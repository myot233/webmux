<script lang="ts">
  import { tick } from "svelte";
  import { Button } from "$lib/components/ui/button";
  import { Label } from "$lib/components/ui/label";
  import { Textarea } from "$lib/components/ui/textarea";
  import {
    DIR_REFERENCE_NAME,
    initInstructionReferences,
    USER_REFERENCE_NAME,
    WEBMUX_REFERENCE_NAME,
  } from "../../domains/settings/quick-commands";
  import {
    INIT_INSTRUCTION_MAX,
    type QuickCommand,
  } from "../../contracts/schemas";
  import InlineNotice from "../shared/InlineNotice.svelte";

  // The repository init instruction: a template quick start sends before the
  // first message. `{{name}}` references a quick command; the counter turns
  // red past the server limit.
  let {
    value = $bindable(),
    id,
    commands,
    label = "初始化指令",
  }: {
    value: string;
    id: string;
    commands: QuickCommand[];
    label?: string;
  } = $props();
  let field = $state<HTMLTextAreaElement | null>(null);
  const over = $derived(value.length > INIT_INSTRUCTION_MAX);
  const names = $derived([
    ...new Set(commands.map((cmd) => cmd.name.trim()).filter(Boolean)),
  ]);
  const unknown = $derived(
    initInstructionReferences(value).filter(
      (name) =>
        name !== USER_REFERENCE_NAME &&
        name !== DIR_REFERENCE_NAME &&
        name !== WEBMUX_REFERENCE_NAME &&
        !names.includes(name),
    ),
  );

  // Inserts at the caret (replacing a selection), or at the end when the
  // field has never been focused.
  async function insert(name: string) {
    const token = `{{${name}}}`;
    const start = field?.selectionStart ?? value.length;
    const end = field?.selectionEnd ?? value.length;
    value = value.slice(0, start) + token + value.slice(end);
    await tick();
    field?.focus();
    field?.setSelectionRange(start + token.length, start + token.length);
  }
</script>

<div class="space-y-2">
  <Label for={id}>{label}</Label>
  <p id={`${id}-help`} class="text-xs text-muted-foreground">
    这是一段模板：写 {"{{快捷指令名称}}"} 会插入该快捷指令的文本（只展开一层，不递归）；写
    <code>{"{{dir}}"}</code>
    会换成新会话的临时文件夹路径，写 <code>{"{{webmux}}"}</code>
    会换成 WebMux 工具说明（手册地址、命令路径与临时文件夹），写
    <code>{"{{user}}"}</code>
    可指定你的第一条消息放在哪里，不写则接在指令后面空一行。启动会话时勾选“使用初始化指令”才会随首条消息发送；从聊天开始时默认勾选，从终端开始时默认不勾选。留空保存即清除。
  </p>
  <Textarea
    {id}
    rows={4}
    bind:ref={field}
    bind:value
    aria-invalid={over}
    aria-describedby={`${id}-help ${id}-count`}
    placeholder="例如：先阅读 AGENTS.md，改完运行 make check"
  />
  <p id={`${id}-count`} class="flex justify-end text-xs text-muted-foreground">
    <span class={`tabular-nums ${over ? "text-destructive" : ""}`}
      >{value.length} / {INIT_INSTRUCTION_MAX}</span
    >
  </p>
  <div class="space-y-1">
    <p class="text-xs text-muted-foreground">可插入</p>
    <div class="flex flex-wrap gap-1" role="group" aria-label="快捷指令">
      <Button
        type="button"
        variant="outline"
        size="xs"
        title={"插入 {{user}}：你的第一条消息"}
        aria-label="插入用户输入"
        onclick={() => void insert(USER_REFERENCE_NAME)}>用户输入</Button
      >
      <Button
        type="button"
        variant="outline"
        size="xs"
        title={"插入 {{webmux}}：WebMux 工具说明"}
        aria-label="插入 WebMux 工具说明"
        onclick={() => void insert(WEBMUX_REFERENCE_NAME)}>WebMux 说明</Button
      >
      {#each names as name (name)}
        <Button
          type="button"
          variant="outline"
          size="xs"
          title={`插入 {{${name}}}`}
          aria-label={`插入 ${name}`}
          onclick={() => void insert(name)}>{name}</Button
        >
      {/each}
    </div>
    {#if !names.length}
      <p class="text-xs text-muted-foreground">
        还没有带名称的快捷指令，请先在设置中添加。
      </p>
    {/if}
  </div>
  {#if unknown.length}
    <InlineNotice tone="warning">
      <p>
        找不到这些快捷指令：{unknown
          .map((name) => `{{${name}}}`)
          .join("、")}。仍可保存，但快速开始勾选使用时会被拒绝。
      </p>
    </InlineNotice>
  {/if}
</div>
