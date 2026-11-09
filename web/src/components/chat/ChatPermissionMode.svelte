<script lang="ts">
  import IconShield from "~icons/tabler/shield";
  import IconShieldCheck from "~icons/tabler/shield-check";
  import {
    changedPermissionMode,
    permissionChoices,
  } from "../../chat-permission";
  import type { ChatPermissionMode } from "../../schemas";
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
</script>

<ChatSelect
  name="权限"
  value={mode}
  choices={permissionChoices}
  {disabled}
  labeled
  title={mode === "auto"
    ? "权限：全部接受 Agent 的工具调用请求（表单仍需回答）"
    : "权限：每次工具调用都询问"}
  onchange={(value) => {
    const changed = changedPermissionMode(value, mode);
    if (changed) onchange(changed);
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
