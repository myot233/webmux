<script lang="ts">
  import IconBell from "~icons/tabler/bell";
  import { Button } from "$lib/components/ui/button";
  import { Badge } from "$lib/components/ui/badge";
  import * as Popover from "$lib/components/ui/popover";
  import { buildAttentionToast } from "../../domains/notifications/attention-toast";
  import {
    attentionDeferred,
    deferAttention,
    wakeAttention,
  } from "../../domains/notifications/attention-toast.svelte";
  import { hasUrgentAttention } from "../../domains/sessions/session-status";
  import type { SessionInfo } from "../../contracts/schemas";
  import { openSession } from "../../hooks/shared/shell.svelte";

  let { sessions }: { sessions: readonly SessionInfo[] } = $props();
  let open = $state(false);
  const alerts = $derived(
    sessions
      .flatMap((session) => {
        const attention = session.attention;
        if (
          !attention ||
          session.status === "exited" ||
          !hasUrgentAttention(session)
        )
          return [];
        return [
          {
            session,
            alert: buildAttentionToast(session, attention),
            deferred: attentionDeferred(session),
          },
        ];
      })
      .sort(
        (a, b) =>
          Number(a.deferred) - Number(b.deferred) ||
          Date.parse(b.alert.since) - Date.parse(a.alert.since),
      ),
  );
  const pending = $derived(alerts.filter((item) => !item.deferred).length);
</script>

{#if alerts.length || open}
  <div class="px-2 pt-1">
    <Popover.Root bind:open>
      <Popover.Trigger>
        {#snippet child({ props })}
          <Button
            {...props}
            variant="ghost"
            size="sm-touch"
            class="w-full justify-start"
          >
            <IconBell aria-hidden="true" />待我处理
            {#if pending}<Badge variant="warning" class="ml-auto"
                >{pending}</Badge
              >{/if}
          </Button>
        {/snippet}
      </Popover.Trigger>
      <Popover.Content
        side="bottom"
        align="start"
        role="dialog"
        aria-label="待我处理"
        class="flex max-h-(--bits-popover-content-available-height) w-[min(21rem,calc(100vw-1.5rem))] flex-col overflow-hidden"
      >
        <Popover.Header>
          <Popover.Title>待我处理</Popover.Title>
          <Popover.Description
            >来自各仓库的授权、提问与错误。</Popover.Description
          >
        </Popover.Header>
        {#if alerts.length}
          <ul
            class="min-h-0 max-h-[min(22rem,45dvh)] space-y-2 overflow-y-auto px-0.5 py-0.5"
          >
            {#each alerts as { session, alert, deferred } (session.id)}
              <li class="flex min-w-0 flex-wrap items-center gap-1">
                <Button
                  variant="ghost"
                  size="sm"
                  class="min-w-0 flex-1 justify-start"
                  title={alert.title}
                  onclick={() => {
                    open = false;
                    openSession(session);
                  }}
                >
                  <span class="truncate">{alert.title}</span>
                </Button>
                <Badge variant="secondary"
                  >{deferred ? "已暂缓" : alert.label}</Badge
                >
                <Button
                  variant="outline"
                  size="sm"
                  onclick={() =>
                    deferred ? wakeAttention(session) : deferAttention(session)}
                  aria-label={`${deferred ? "立即提醒" : "稍后（15 分钟后提醒）"}：${alert.title}`}
                >
                  {deferred ? "立即提醒" : "稍后"}
                </Button>
              </li>
            {/each}
          </ul>
        {:else}
          <p class="text-sm text-muted-foreground">当前没有等待处理的会话。</p>
        {/if}
      </Popover.Content>
    </Popover.Root>
  </div>
{/if}
