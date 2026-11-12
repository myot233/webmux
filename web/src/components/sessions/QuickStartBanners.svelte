<script lang="ts">
  import IconAlertCircle from "~icons/tabler/alert-circle";
  import { Button } from "$lib/components/ui/button";
  import ConnectorInstallHint from "./ConnectorInstallHint.svelte";
  import {
    failedStep,
    type QuickStartTargetMode,
  } from "../../hooks/sessions/quick-start";
  import type {
    CreateSessionFailure,
    Repository,
  } from "../../contracts/schemas";
  import { openSession, type Shell } from "../../hooks/shared/shell.svelte";

  let {
    missing,
    targetMode,
    branchError,
    repository,
    failure,
    shell,
  }: {
    missing: "" | "agent" | "repository";
    targetMode: QuickStartTargetMode;
    branchError?: string;
    repository?: Repository;
    failure?: CreateSessionFailure;
    shell: Shell;
  } = $props();
</script>

{#if missing}
  <div
    class="flex flex-wrap items-center gap-2 rounded-lg border px-3 py-2 text-sm"
    role="status"
  >
    <IconAlertCircle class="text-warning" aria-hidden="true" />
    <span class="mr-auto">
      {missing === "agent"
        ? "还没有勾选混合模式的启动配置，无法以聊天方式启动 agent。"
        : "还没有可用的仓库。"}
    </span>
    {#if missing === "agent"}
      <Button variant="outline" size="sm-touch" href="/settings#launch-configs"
        >新建启动配置</Button
      >
    {:else}
      <Button variant="outline" size="sm-touch" onclick={shell.addRepository}
        >添加仓库</Button
      >
    {/if}
  </div>
{/if}
{#if targetMode === "new" && branchError}
  <p role="alert" class="px-1 text-sm text-destructive">
    读取 {repository?.name} 的分支失败：{branchError}
  </p>
{/if}
{#if failure}
  {@const failed = failure}
  <div
    class="space-y-2 rounded-lg border border-destructive/40 px-3 py-2 text-sm"
    role="alert"
  >
    <p class="flex items-start gap-2">
      <IconAlertCircle
        class="mt-0.5 shrink-0 text-destructive"
        aria-hidden="true"
      />
      <span class="min-w-0 break-words">
        <span class="font-medium">{failedStep[failed.step]}：</span
        >{failed.error}
        {failed.sessionId ? "" : "消息未发送，已保留在输入框中。"}
      </span>
    </p>
    {#if failed.connector}
      <ConnectorInstallHint connector={failed.connector} />
    {/if}
    {#if failed.worktree || failed.sessionId}
      <p class="flex flex-wrap items-center gap-2 pl-6">
        {#if failed.worktree}
          {@const tree = failed.worktree}
          <span
            class="min-w-0 truncate font-mono text-xs text-muted-foreground"
            title={tree.path}>已创建 worktree：{tree.path}</span
          >
          {#if !failed.sessionId}
            <Button
              variant="outline"
              size="sm-touch"
              onclick={() =>
                shell.start({
                  kind: "worktree",
                  repositoryId: tree.repositoryId,
                  worktreeId: tree.id,
                })}>在此 worktree 新建会话</Button
            >
          {/if}
        {/if}
        {#if failed.sessionId}
          {@const id = failed.sessionId}
          <Button
            variant="outline"
            size="sm-touch"
            onclick={() => openSession({ id })}>进入会话</Button
          >
        {/if}
      </p>
    {/if}
  </div>
{/if}
