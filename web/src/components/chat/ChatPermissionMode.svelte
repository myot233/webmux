<script lang="ts">
  import IconShield from "~icons/tabler/shield";
  import IconShieldCheck from "~icons/tabler/shield-check";
  import {
    ChatPermissionModeSchema,
    type ChatPermissionMode,
  } from "../../schemas";
  import ChatSelect from "./ChatSelect.svelte";

  // WebMux's permission mode beside the agent's options: ask shows every
  // permission request, auto has the server approve it. Forms always ask.
  let {
    mode,
    disabled,
    onchange,
  }: {
    mode: ChatPermissionMode;
    disabled: boolean;
    onchange: (mode: ChatPermissionMode) => void;
  } = $props();
  const choices = [
    { value: "ask", name: "每次询问" },
    { value: "auto", name: "自动批准" },
  ];
</script>

<ChatSelect
  name="权限"
  value={mode}
  {choices}
  {disabled}
  title={mode === "auto"
    ? "权限：自动批准 Agent 的工具调用请求（表单仍需回答）"
    : "权限：每次工具调用都询问"}
  onchange={(value) => {
    const parsed = ChatPermissionModeSchema.safeParse(value);
    if (parsed.success && parsed.data !== mode) onchange(parsed.data);
  }}
>
  {#snippet icon()}
    {#if mode === "auto"}
      <IconShieldCheck aria-hidden="true" />
    {:else}
      <IconShield aria-hidden="true" />
    {/if}
  {/snippet}
</ChatSelect>
