<script lang="ts">
  import { onMount } from "svelte";
  import { z } from "zod";
  import { api } from "../api";
  import { SessionInfoSchema, type SessionInfo } from "../schemas";
  import type { SplitDirection } from "../terminal-layout";
  import { Button } from "$lib/components/ui/button";
  import { Input } from "$lib/components/ui/input";
  import { ListButton } from "$lib/components/ui/list-button";
  import ResponsiveDialog from "./ResponsiveDialog.svelte";
  import { errorMessage } from "../error";

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
    <div class="flex gap-2" role="group" aria-label="分屏方向">
      <Button
        variant={direction === "horizontal" ? "default" : "outline"}
        aria-pressed={direction === "horizontal"}
        onclick={() => (direction = "horizontal")}>左右分屏</Button
      >
      <Button
        variant={direction === "vertical" ? "default" : "outline"}
        aria-pressed={direction === "vertical"}
        onclick={() => (direction = "vertical")}>上下分屏</Button
      >
    </div>
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
