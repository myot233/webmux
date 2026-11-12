<script lang="ts">
  import { onMount } from "svelte";
  import { z } from "zod";
  import { api } from "../../platform/api";
  import { SessionInfoSchema, type SessionInfo } from "../../contracts/schemas";
  import type { SplitDirection } from "../../domains/terminal/terminal-layout";
  import { Button } from "$lib/components/ui/button";
  import { Input } from "$lib/components/ui/input";
  import { ListButton } from "$lib/components/ui/list-button";
  import SplitDirectionChoice from "./SplitDirectionChoice.svelte";
  import ResponsiveDialog from "../shared/ResponsiveDialog.svelte";
  import { errorMessage } from "../../platform/error";

  let {
    excluded,
    onclose,
    onselect,
  }: {
    excluded: string[];
    onclose: () => void;
    onselect: (id: string, direction: SplitDirection) => void;
  } = $props();
  let sessions = $state<SessionInfo[]>([]);
  let direction = $state<SplitDirection>("horizontal");
  let query = $state("");
  let loading = $state(true);
  let error = $state("");
  const choices = $derived(
    sessions.filter(
      (session) =>
        !excluded.includes(session.id) &&
        `${session.title} ${session.cwd}`
          .toLowerCase()
          .includes(query.toLowerCase()),
    ),
  );
  onMount(() => {
    const controller = new AbortController();
    void api(
      "/sessions",
      z.array(SessionInfoSchema),
      "GET",
      undefined,
      controller.signal,
    )
      .then((result) => {
        sessions = result;
      })
      .catch((err) => {
        if (!controller.signal.aborted) error = errorMessage(err);
      })
      .finally(() => {
        loading = false;
      });
    return () => controller.abort();
  });
</script>

<ResponsiveDialog
  title="分屏加入已有会话"
  description="选择会话放入新窗格；当前已显示的会话不会重复加入。"
  {onclose}
>
  <div class="space-y-4">
    <SplitDirectionChoice bind:direction />
    <Input
      aria-label="搜索已有会话"
      placeholder="搜索名称或目录"
      bind:value={query}
    />
    <div class="max-h-[50dvh] space-y-1 overflow-auto">
      {#if loading}<p role="status">正在加载会话…</p>
      {:else if error}<p role="alert" class="text-destructive">{error}</p>
      {:else if choices.length === 0}<p class="text-sm text-muted-foreground">
          没有可加入的会话。可从分屏菜单新建 Shell。
        </p>
      {:else}{#each choices as session (session.id)}
          <ListButton
            variant="option"
            class="w-full"
            onclick={() => onselect(session.id, direction)}
          >
            <span class="min-w-0"
              ><span class="block truncate"
                >{session.title}{session.status === "exited"
                  ? "（已退出）"
                  : ""}</span
              ><span class="block truncate text-xs text-muted-foreground"
                >{session.cwd}</span
              ></span
            >
          </ListButton>
        {/each}{/if}
    </div>
    <div class="flex justify-end">
      <Button variant="outline" onclick={onclose}>取消</Button>
    </div>
  </div>
</ResponsiveDialog>
