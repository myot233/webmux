<script lang="ts">
  import { errorMessage } from "../../platform/error";
  import { onMount } from "svelte";
  import { api } from "../../platform/api";
  import {
    EmptyRequestSchema,
    NoContentSchema,
    RemovalPreviewSchema,
    RemoveWorktreeRequestSchema,
    type ArchivedSession,
    type RemovalPreview,
    type Worktree,
  } from "../../contracts/schemas";
  import { Button } from "$lib/components/ui/button";
  import { Checkbox } from "$lib/components/ui/checkbox";
  import { Label } from "$lib/components/ui/label";
  import BlockerList from "../shared/BlockerList.svelte";
  import InlineNotice from "../shared/InlineNotice.svelte";
  import PreviewForm from "./PreviewForm.svelte";
  import ResponsiveDialog from "../shared/ResponsiveDialog.svelte";
  import PushCheckNotice from "../settings/PushCheckNotice.svelte";
  import WorktreeSessionList from "./WorktreeSessionList.svelte";
  let {
    worktree,
    forgetRecords = [],
    onclose,
    onchange,
    onremoved,
    onhide,
  }: {
    worktree: Worktree;
    /** Archived records the caller deletes after the worktree is gone. */
    forgetRecords?: readonly ArchivedSession[];
    onclose: () => void;
    onchange: () => void;
    /** The worktree was removed; runs before `onclose`. */
    onremoved?: () => void;
    /**
     * Hides the worktree from the home page without deleting anything; when
     * given, the dialog offers it as an alternative to removal.
     */
    onhide?: () => void;
  } = $props();
  let preview = $state<RemovalPreview | null>(null);
  let busy = $state(false);
  let error = $state("");
  let force = $state(false);
  let closeSessions = $state(false);
  const canHide = $derived(!!onhide && forgetRecords.length === 0);
  // The records a removal keeps are pinned to the commit, which needs a
  // resolvable HEAD. Records the caller deletes with the worktree are not
  // kept.
  const forgotten = $derived(new Set(forgetRecords.map((record) => record.id)));
  const retainedRecords = $derived(
    preview
      ? preview.archived.filter((record) => !forgotten.has(record.id))
      : [],
  );
  const retained = $derived(
    !!preview && preview.commit !== "" && retainedRecords.length > 0,
  );
  // A force removal also keeps the records of the chat sessions it closes.
  const keepsRecords = $derived(
    !!preview &&
      preview.commit !== "" &&
      (retainedRecords.length > 0 ||
        preview.sessions.some((session) => session.agent !== null)),
  );
  const route = $derived(
    `/repositories/${worktree.repositoryId}/worktrees/${worktree.id}`,
  );
  const dialogDescription = $derived(
    force
      ? `危险操作：停止已确认的 WebMux 会话，永久丢弃删除时目录中的全部内容${
          forgetRecords.length > 0
            ? "，并删除所选聊天记录；其他已归档记录保留，只保存提交"
            : "；已归档的混合会话记录保留，只保存提交"
        }。`
      : `先检查文件与会话。删除 worktree 不删除分支；${
          forgetRecords.length > 0
            ? retainedRecords.length > 0
              ? "所选聊天记录会一并删除，其他已归档记录会保留，之后可重建并恢复"
              : "所选聊天记录会一并删除"
            : "该目录下已归档的混合会话记录会保留，之后可重建并恢复"
        }。`,
  );
  async function load() {
    busy = true;
    error = "";
    preview = null;
    force = false;
    closeSessions = false;
    try {
      preview = await api(
        route + "/removal-preview",
        RemovalPreviewSchema,
        "POST",
        {},
        undefined,
        EmptyRequestSchema,
      );
    } catch (err) {
      error = errorMessage(err);
    } finally {
      busy = false;
    }
  }
  onMount(() => {
    void load();
  });
  function hide() {
    onhide?.();
    onclose();
  }
  async function remove() {
    if (!preview) return;
    busy = true;
    error = "";
    try {
      await api(
        route + "/remove",
        NoContentSchema,
        "POST",
        {
          confirmationId: preview.confirmationId,
          mode: force ? "force" : "safe",
          ...(force
            ? { confirmPath: preview.worktree.path, closeSessions }
            : {}),
        },
        undefined,
        RemoveWorktreeRequestSchema,
      );
      onremoved?.();
      onclose();
    } catch (err) {
      error = `${errorMessage(err)}。操作未完整完成；部分会话可能已关闭，请重新读取预览后确认。`;
      preview = null;
      force = false;
    } finally {
      busy = false;
      onchange();
    }
  }
</script>

{#snippet recordList(
  records: readonly { id: string; title: string; state: string; cwd: string }[],
)}
  <ul class="space-y-1">
    {#each records as record (record.id)}<li class="break-all">
        {record.title} · {record.state === "closed" ? "已关闭" : "已中断"} · {record.cwd}
      </li>{/each}
  </ul>
{/snippet}

<ResponsiveDialog
  title={force ? "强制删除 worktree" : "删除 worktree"}
  description={dialogDescription}
  {onclose}
  {busy}
>
  <PreviewForm {busy} {error} onsubmit={() => void remove()} {onclose}>
    {#snippet summary()}
      <div class="rounded-lg border bg-muted/40 p-3 text-sm">
        <p class="font-medium">
          {worktree.branch ||
            (worktree.detached
              ? `detached · ${worktree.head?.slice(0, 12)}`
              : "无分支")}
        </p>
        <p class="mt-1 font-mono break-all whitespace-pre-wrap">
          {worktree.path}
        </p>
        {#if worktree.detached && !retained}<p class="mt-2 text-destructive">
            detached 提交可能没有分支保护，请先在终端保留需要的提交。
          </p>{:else}<p class="mt-2 text-muted-foreground">
            {worktree.detached
              ? "提交由保留的会话记录保护"
              : "分支保留"}；目录内未提交、未跟踪及忽略文件不会保留，只保留提交。
          </p>{/if}
      </div>
    {/snippet}
    {#if preview}
      {#if preview.blockers.length}
        <BlockerList title="安全删除被阻止" blockers={preview.blockers} />
      {:else}<p class="text-sm">
          {preview.worktree.prunable && !preview.worktree.available
            ? "工作树目录已缺失，当前可安全清理该失效的 Git 登记。"
            : "当前工作树可安全删除：没有受保护的文件或活动会话。"}
        </p>{/if}
      {#if preview.warnings.length}
        <InlineNotice tone="warning">
          <ul class="space-y-1">
            {#each preview.warnings as warning (warning)}<li>
                {warning}
              </li>{/each}
          </ul>
        </InlineNotice>
      {/if}
      {#if preview.sessions.length}
        <WorktreeSessionList
          title="关联会话（按启动目录识别）"
          sessions={preview.sessions}
        />
      {/if}
      {#if forgetRecords.length}
        <div class="space-y-2 text-sm">
          <p class="font-medium">将删除的已归档聊天</p>
          {@render recordList(forgetRecords)}
        </div>
      {/if}
      {#if retainedRecords.length}
        <div class="space-y-2 text-sm">
          <p class="font-medium">
            {retained
              ? "保留的已归档会话（之后可重建并恢复）"
              : "无法保存提交，已归档会话将原样保留（只能查看或手动删除，不能重建）"}
          </p>
          {@render recordList(retainedRecords)}
        </div>
      {/if}
      {#if keepsRecords}<PushCheckNotice push={preview.push} />{/if}
      <p class="text-xs text-muted-foreground">
        不追踪终端内 cd
        后的位置，也无法停止外部进程。外部写入和确认后产生的文件也可能丢失。确认
        5 分钟内有效；目标或会话变化后必须重新预览。
      </p>
      {#if force}
        <Label class="items-start">
          <Checkbox
            bind:checked={closeSessions}
            disabled={busy}
            class="mt-0.5"
          />
          <span
            >我同意关闭上面列出的活动会话，并永久删除该目录的全部内容（包括忽略文件）。</span
          >
        </Label>
      {/if}
    {/if}
    {#snippet actions()}
      {#if !preview}<Button type="button" disabled={busy} onclick={load}
          >重新读取预览</Button
        >
      {:else if force}
        <Button
          type="button"
          variant="outline"
          disabled={busy}
          onclick={() => {
            force = false;
            closeSessions = false;
          }}>返回安全预览</Button
        >
        <Button
          type="submit"
          variant="destructive"
          disabled={busy || !closeSessions}>关闭会话并强制删除</Button
        >
      {:else if !preview.blockers.length}
        <Button type="submit" disabled={busy}>确认安全删除</Button>
      {:else if preview.forceAllowed}
        <Button
          type="button"
          variant="destructive"
          disabled={busy}
          onclick={() => {
            force = true;
          }}>进入强制删除确认</Button
        >
      {/if}
      {#if canHide}
        <Button
          type="button"
          variant="success"
          disabled={busy}
          title="只移出主页，不删除目录、分支与会话记录；有会话的工作树仍会显示"
          onclick={hide}>仅从主页隐藏</Button
        >
      {/if}
    {/snippet}
  </PreviewForm>
</ResponsiveDialog>
