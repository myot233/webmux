<script lang="ts">
  import InlineNotice from "../shared/InlineNotice.svelte";
  import { untrack } from "svelte";
  import IconBolt from "~icons/tabler/bolt";
  import { Button } from "$lib/components/ui/button";
  import { Input } from "$lib/components/ui/input";
  import { Label } from "$lib/components/ui/label";
  import { Switch } from "$lib/components/ui/switch";
  import { Textarea } from "$lib/components/ui/textarea";
  import * as Dialog from "$lib/components/ui/dialog";
  import {
    QuickCommandSchema,
    type QuickCommand,
  } from "../../contracts/schemas";
  import {
    QUICK_COMMAND_NAME_MAX,
    newQuickCommandId,
    normalizeCommand,
    quickCommandNameError,
  } from "../../domains/settings/quick-commands";

  let {
    open = $bindable(false),
    initial,
    save,
    existing,
    disabled = false,
  }: {
    open: boolean;
    disabled?: boolean;
    /** Command being edited; undefined creates a new one. */
    initial: QuickCommand | undefined;
    save: (cmd: QuickCommand) => string | Promise<string>;
    /** The saved list, for the unique-name check. */
    existing: QuickCommand[];
  } = $props();
  let name = $state("");
  let command = $state("");
  let enter = $state(false);
  let error = $state("");
  let saving = $state(false);
  let commandInput = $state<HTMLTextAreaElement | null>(null);
  const draft = $derived(
    QuickCommandSchema.safeParse({
      id: initial?.id ?? "pending",
      name: name.trim(),
      command: normalizeCommand(command),
      enter,
    }),
  );
  const nameError = $derived(
    quickCommandNameError(
      name,
      existing.filter((cmd) => cmd.id !== initial?.id).map((cmd) => cmd.name),
    ),
  );
  // An untouched new form is not an error yet; a saved empty name is.
  const showNameError = $derived(!!nameError && (name !== "" || !!initial));

  // Each opening starts from the saved value, so cancel/Esc discards edits.
  $effect.pre(() => {
    if (!open) return;
    untrack(() => {
      name = initial?.name ?? "";
      command = initial?.command ?? "";
      enter = initial?.enter ?? false;
      error = "";
    });
  });

  async function submit(event: SubmitEvent) {
    event.preventDefault();
    if (!draft.success || nameError) return;
    saving = true;
    try {
      error = await save({
        ...draft.data,
        id: initial?.id ?? newQuickCommandId(),
      });
      if (!error) open = false;
    } catch (err) {
      error = (err as Error).message;
    } finally {
      saving = false;
    }
  }
</script>

<Dialog.Root bind:open>
  <Dialog.Content
    class="sm:max-w-lg"
    onOpenAutoFocus={(event) => {
      if (initial) return;
      event.preventDefault();
      commandInput?.focus();
    }}
  >
    <form class="grid gap-4" onsubmit={submit}>
      <Dialog.Header>
        <Dialog.Title>{initial ? "编辑快捷指令" : "添加快捷指令"}</Dialog.Title>
        <Dialog.Description
          >在终端点击 <IconBolt
            class="inline size-3.5 align-[-2px]"
            aria-label="闪电"
          /> 按钮即可发送这条指令。</Dialog.Description
        >
      </Dialog.Header>
      <div class="grid gap-2">
        <Label for="quick-command-name">名称</Label>
        <Input
          id="quick-command-name"
          bind:value={name}
          maxlength={QUICK_COMMAND_NAME_MAX}
          placeholder="例如：查看状态"
          autocomplete="off"
          aria-invalid={showNameError}
          aria-describedby="quick-command-name-help"
        />
        <p
          id="quick-command-name-help"
          class={[
            "text-xs",
            showNameError ? "text-destructive" : "text-muted-foreground",
          ]}
        >
          {showNameError
            ? nameError
            : "名称唯一；仓库初始化指令用 {{名称}} 引用这条指令。"}
        </p>
      </div>
      <div class="grid gap-2">
        <Label for="quick-command-body">指令</Label>
        <Textarea
          id="quick-command-body"
          bind:ref={commandInput}
          bind:value={command}
          rows={4}
          maxlength={4096}
          placeholder="例如：git status"
          mono
          class="max-h-[40dvh]"
          autocapitalize="off"
          autocorrect="off"
          spellcheck={false}
        />
      </div>
      <div class="flex items-start gap-3">
        <Switch
          id="quick-command-enter"
          bind:checked={enter}
          aria-describedby="quick-command-enter-help"
        />
        <div class="grid gap-1">
          <Label for="quick-command-enter">发送后自动回车</Label>
          <p
            id="quick-command-enter-help"
            class="text-xs text-muted-foreground"
          >
            开启后点击即执行；关闭时只填入，由你确认后再回车。
          </p>
        </div>
      </div>
      {#if error}
        <InlineNotice tone="danger">{error}</InlineNotice>
      {/if}
      <Dialog.Footer>
        <Button
          type="button"
          variant="outline"
          onclick={() => {
            open = false;
          }}>取消</Button
        >
        <Button
          type="submit"
          disabled={!draft.success || !!nameError || saving || disabled}
          >保存</Button
        >
      </Dialog.Footer>
    </form>
  </Dialog.Content>
</Dialog.Root>
