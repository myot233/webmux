<script lang="ts">
  import type { Snippet } from "svelte";
  import { Button } from "$lib/components/ui/button";
  import IconX from "~icons/tabler/x";
  import type { HybridNotice } from "../../hooks/sessions/hybrid.svelte";
  import type { UploadProgress } from "../../hooks/shared/upload.svelte";
  import FileUploadProgress from "../shared/FileUploadProgress.svelte";
  import StripNotice from "../shared/StripNotice.svelte";
  import TerminalPortPrompts from "./TerminalPortPrompts.svelte";
  import type { ControlState, ListeningPort } from "../../contracts/schemas";
  import type { TerminalDraft } from "../../domains/terminal/terminal-draft.svelte";
  import type { Connection } from "../../platform/ws";

  interface NoticeTerminal {
    readonly control: ControlState | undefined;
    readonly controlling: boolean;
    readonly connection: Connection;
    readonly exitCode: number | null;
    readonly error: string;
    claim(): void;
  }
  interface NoticeUploads {
    readonly progress: UploadProgress | null;
    cancel(): void;
  }

  // Status strips between the header and the terminal, in display order.
  let {
    terminal: t,
    uploads,
    draft,
    draftSaved,
    error,
    pendingCopy = false,
    copyPending = () => {},
    dismissCopy = () => {},
    notice = $bindable(),
    showScratch,
    dismissScratch,
    remove,
    notices = [],
    dismiss = () => {},
    recovery,
    session = "",
    listening,
    showForwards,
    leave,
  }: {
    terminal: NoticeTerminal;
    uploads: NoticeUploads;
    draft: TerminalDraft;
    draftSaved: string;
    error: string;
    pendingCopy?: boolean;
    copyPending?: () => void;
    dismissCopy?: () => void;
    notice: string;
    showScratch: () => void;
    dismissScratch: () => void;
    remove: () => void;
    /** A hybrid session's switch and identity notices. */
    notices?: HybridNotice[];
    dismiss?: (key: string) => void;
    /** Resume choices that replace the missing and exited strips. */
    recovery?: Snippet;
    /** The session ID whose listening ports get forwarding prompts. */
    session?: string;
    /** Listening ports, or undefined while unknown (see TerminalPortPrompts). */
    listening?: readonly ListeningPort[];
    /** Opens the port-forwarding dialog. */
    showForwards?: () => void;
    /** Leaves this session: the page returns home, a split closes its pane. */
    leave: () => void;
  } = $props();
  const alertText = $derived([error, t.error].filter(Boolean).join(" "));
  let dismissedError = $state("");
  $effect(() => {
    if (dismissedError && dismissedError !== alertText) dismissedError = "";
  });
</script>

{#if !draft.enabled && !draft.scratchUpdatedWhileHidden && (draft.text || draft.updatedWhileHidden || draft.scratch.length)}
  <p class="sr-only" role="status">
    {#if draft.updatedWhileHidden}隐藏草稿已更新{:else if draft.text}有保留的隐藏草稿{/if}
    {#if draft.scratch.length}
      · 共 {draft.scratch.length} 条暂存{/if}
    · 开启草稿模式查看；{draftSaved}
  </p>
{/if}
{#if !draft.enabled && draft.scratchUpdatedWhileHidden}
  <StripNotice ondismiss={dismissScratch}>
    {#if draft.updatedWhileHidden}隐藏草稿已更新 ·
    {/if}暂存已更新 · 共 {draft.scratch.length} 条，不会自动发送
    {#snippet actions()}
      <Button variant="outline" size="sm" onclick={showScratch}>查看暂存</Button
      >
    {/snippet}
  </StripNotice>
{/if}
{#if uploads.progress}<FileUploadProgress
    progress={uploads.progress}
    cancel={() => uploads.cancel()}
  />{/if}
{#if pendingCopy}
  <StripNotice label="待处理的复制请求">
    此窗格有待复制的文字，仅保留最新请求。
    {#snippet actions()}
      <Button variant="outline" size="sm" onclick={copyPending}
        >复制待处理文字</Button
      >
      <Button variant="ghost" size="sm" onclick={dismissCopy}>忽略</Button>
    {/snippet}
  </StripNotice>
{/if}
{#if notice}<StripNotice ondismiss={() => (notice = "")}>{notice}</StripNotice
  >{/if}
{#each notices as item (item.key)}
  <StripNotice tone={item.tone} ondismiss={() => dismiss(item.key)}
    >{item.text}</StripNotice
  >
{/each}
{#if session}<TerminalPortPrompts
    {session}
    {listening}
    onshowall={showForwards}
  />{/if}
{#if t.control && !t.controlling && t.connection === "connected" && t.exitCode === null}
  <StripNotice label="终端控制">
    {t.control.controller
      ? `观察中 · ${t.control.controller.label || "另一设备"}控制输入与尺寸`
      : "观察中 · 当前没有控制端"}（{t.control.cols}×{t.control.rows}）
    {#snippet actions()}
      <Button variant="outline" size="sm" onclick={() => t.claim()}
        >接管控制</Button
      >
    {/snippet}
  </StripNotice>
{/if}
{#if alertText && alertText !== dismissedError}<StripNotice tone="danger">
    {alertText}
    {#snippet actions()}
      <Button
        variant="ghost"
        size="icon-sm"
        aria-label="关闭提示"
        title="关闭提示"
        onclick={() => (dismissedError = alertText)}
        ><IconX aria-hidden="true" /></Button
      >
    {/snippet}
  </StripNotice>{/if}
{#if recovery}{@render recovery()}{:else if t.connection === "missing"}<StripNotice
    tone="warning"
  >
    会话不存在或已被移除。
    {#snippet actions()}
      <Button variant="outline" size="sm" onclick={leave}>返回列表</Button>
    {/snippet}
  </StripNotice>{/if}
{#if t.exitCode !== null && !recovery}
  <StripNotice>
    进程已退出（code {t.exitCode}）
    {#snippet actions()}
      <Button variant="outline" size="sm" class="shrink-0" onclick={leave}
        >返回列表</Button
      >
      <Button variant="outline" size="sm" class="shrink-0" onclick={remove}
        >移除会话</Button
      >
    {/snippet}
  </StripNotice>
{/if}
