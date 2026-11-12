<script lang="ts">
  import IconX from "~icons/tabler/x";
  import { Badge } from "$lib/components/ui/badge";
  import { Button } from "$lib/components/ui/button";
  import { ListButton } from "$lib/components/ui/list-button";
  import {
    attentionToasts,
    dismissAttentionToast,
    pauseAttentionToast,
    resumeAttentionToast,
  } from "../../domains/notifications/attention-toast.svelte";
  import { openSession } from "../../hooks/shared/shell.svelte";
</script>

{#if attentionToasts.list.length > 0}
  <div
    class="pointer-events-none fixed top-12 right-3 z-50 flex w-[min(calc(100vw-1.5rem),22rem)] flex-col gap-2"
    role="region"
    aria-label="会话通知"
    aria-live="polite"
    data-slot="attention-toasts"
  >
    {#each attentionToasts.list as toast (toast.sessionId)}
      <div
        class="pointer-events-auto flex items-start gap-2 rounded-lg border bg-card p-3 text-sm text-card-foreground shadow-lg"
        role="status"
        data-slot="attention-toast"
        data-toast-session-id={toast.sessionId}
        data-kind={toast.kind}
        data-tone={toast.urgent ? "destructive" : "warning"}
        onpointerenter={() => pauseAttentionToast(toast.sessionId)}
        onpointerleave={() => resumeAttentionToast(toast.sessionId)}
        onfocusin={() => pauseAttentionToast(toast.sessionId)}
        onfocusout={() => resumeAttentionToast(toast.sessionId)}
      >
        <ListButton
          variant="content"
          class="flex min-w-0 flex-1 flex-col"
          aria-label={`打开会话 ${toast.title}：${toast.label} · ${toast.detail}`}
          onclick={() => {
            dismissAttentionToast(toast.sessionId);
            openSession({ id: toast.sessionId });
          }}
        >
          <span class="flex min-w-0 max-w-full flex-wrap items-center gap-1.5">
            <Badge
              variant={toast.urgent ? "destructive" : "warning"}
              data-slot="attention-toast-badge"
            >
              <span
                class="size-1.5 shrink-0 rounded-full bg-current"
                aria-hidden="true"
              ></span>
              <span>{toast.label}</span>
            </Badge>
            <span class="min-w-0 truncate font-medium">{toast.title}</span>
          </span>
          <span class="text-xs text-muted-foreground">{toast.detail}</span>
        </ListButton>
        <Button
          variant="ghost"
          size="icon-sm"
          class="shrink-0"
          aria-label={`关闭 ${toast.title} 的通知`}
          title="关闭通知"
          onclick={() => dismissAttentionToast(toast.sessionId)}
          ><IconX aria-hidden="true" /></Button
        >
      </div>
    {/each}
  </div>
{/if}
