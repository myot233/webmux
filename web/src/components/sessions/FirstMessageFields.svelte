<script lang="ts">
  import { Checkbox } from "$lib/components/ui/checkbox";
  import { Label } from "$lib/components/ui/label";
  import { Textarea } from "$lib/components/ui/textarea";

  // The "first message" rows of the new-session dialog: the repository's
  // initialization instruction (when the location has one) and the user's
  // own instruction. Whatever is set is sent once the session is up, with
  // the instruction first: as the first chat prompt, or typed into the
  // terminal followed by Enter.
  let {
    repositoryName = "",
    hasInstruction,
    useInstruction = $bindable(),
    prompt = $bindable(),
    chat,
    disabled = false,
  }: {
    repositoryName?: string;
    hasInstruction: boolean;
    useInstruction: boolean;
    prompt: string;
    /** The session starts on the chat side. */
    chat: boolean;
    disabled?: boolean;
  } = $props();
</script>

<div class="space-y-3">
  {#if hasInstruction}
    <div class="flex items-start gap-2">
      <Checkbox
        id="use-init-instruction"
        class="mt-0.5"
        bind:checked={useInstruction}
        {disabled}
      />
      <div class="grid gap-0.5">
        <Label for="use-init-instruction">使用初始化指令</Label>
        <p class="text-xs text-muted-foreground">
          {#if chat}
            创建后先发送{repositoryName
              ? `“${repositoryName}”`
              : "仓库"}的初始化指令，可在仓库设置中修改。
          {:else}
            初始化指令通常是自然语言，从终端开始时默认不勾选；勾选后会随首条消息一起发送。可在仓库设置中修改。
          {/if}
        </p>
      </div>
    </div>
  {/if}
  <div class="space-y-2">
    <Label for="session-prompt"
      >用户指令 <span class="text-muted-foreground">可选</span></Label
    >
    <Textarea
      id="session-prompt"
      rows={3}
      bind:value={prompt}
      {disabled}
      aria-describedby="session-prompt-help"
      placeholder="创建后自动发送，例如：修复首页排序"
    />
    <p id="session-prompt-help" class="text-xs text-muted-foreground">
      {chat
        ? "作为聊天的第一条消息发送。"
        : "程序启动并安静下来后输入到终端，并自动回车。"}留空则不发送。
    </p>
  </div>
</div>
