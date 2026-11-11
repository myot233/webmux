<script lang="ts">
  import IconLoader2 from "~icons/tabler/loader-2";
  import IconMessage from "~icons/tabler/message";
  import IconTerminal2 from "~icons/tabler/terminal-2";
  import type { ConfigValue } from "../../platform/chat-api";
  import ChatConfig from "../chat/ChatConfig.svelte";
  import ChatPermissionMode from "../chat/ChatPermissionMode.svelte";
  import ChatSelect from "../chat/ChatSelect.svelte";
  import ChatSessionOptions from "../chat/ChatSessionOptions.svelte";
  import LaunchConfigIcon from "../settings/LaunchConfigIcon.svelte";
  import type {
    ChatConfig as ChatOption,
    ChatPermissionMode as PermissionMode,
    LaunchConfigSummary,
    SessionView,
  } from "../../contracts/schemas";

  let {
    compact = false,
    agents,
    agent,
    startSide,
    chat,
    permissionMode,
    options,
    locked,
    probing,
    sending,
    onagentchange,
    onsidechange,
    onmodechange,
    onoptionchange,
  }: {
    compact?: boolean;
    agents: LaunchConfigSummary[];
    agent: LaunchConfigSummary;
    startSide: SessionView;
    chat: boolean;
    permissionMode: PermissionMode;
    options: ChatOption[];
    locked: boolean;
    probing: boolean;
    sending: boolean;
    onagentchange: (id: string) => void;
    onsidechange: (side: SessionView) => void;
    onmodechange: (mode: PermissionMode) => void;
    onoptionchange: (id: string, value: ConfigValue) => void;
  } = $props();
</script>

{#snippet agentPicker(labelClass = "")}
  <ChatSelect
    labeled
    {labelClass}
    name="Agent"
    value={agent.id}
    choices={agents.map((item) => ({ value: item.id, name: item.name }))}
    disabled={locked}
    onchange={onagentchange}
  >
    {#snippet choiceIcon(id)}<LaunchConfigIcon
        icon={agents.find((item) => item.id === id)?.icon ?? ""}
        size="sm"
      />{/snippet}
  </ChatSelect>
{/snippet}

{#snippet sidePicker(labelClass = "")}
  <ChatSelect
    labeled
    {labelClass}
    name="启动方式"
    value={startSide}
    choices={[
      { value: "chat", name: "聊天" },
      { value: "terminal", name: "终端" },
    ]}
    disabled={sending}
    onchange={(value) => {
      if (value === "chat" || value === "terminal") onsidechange(value);
    }}
  >
    {#snippet choiceIcon(value)}{#if value === "terminal"}<IconTerminal2
          aria-hidden="true"
        />{:else}<IconMessage aria-hidden="true" />{/if}{/snippet}
  </ChatSelect>
{/snippet}

{#if compact}
  <span class="hidden min-w-0 items-center gap-0.5 composer-mobile:flex">
    {@render agentPicker("hidden @md:inline")}
    {@render sidePicker("hidden @md:inline")}
    {#if chat}
      <ChatSessionOptions
        mode={permissionMode}
        config={options}
        disabled={locked}
        modeDisabled={locked}
        loading={probing}
        {onmodechange}
        onconfigchange={onoptionchange}
      />
    {/if}
  </span>
{:else}
  <div
    class="-my-1 flex min-w-0 items-center overflow-x-auto px-1 py-1 [scrollbar-width:none] composer-mobile:hidden"
    role="toolbar"
    aria-label="启动选项"
  >
    <div class="ms-auto flex shrink-0 items-center gap-0.5">
      {@render agentPicker()}
      {@render sidePicker()}
      {#if chat}
        <ChatPermissionMode
          mode={permissionMode}
          disabled={locked}
          onchange={onmodechange}
        />
        {#if options.length}
          <ChatConfig
            config={options}
            disabled={locked}
            onchange={onoptionchange}
          />
        {/if}
        {#if probing}
          <span
            class="flex shrink-0 items-center gap-1.5 px-2 text-xs text-muted-foreground"
            role="status"
            ><IconLoader2
              class="size-3.5 animate-spin motion-reduce:animate-none"
              aria-hidden="true"
            />正在读取配置…</span
          >
        {/if}
      {/if}
    </div>
  </div>
{/if}
