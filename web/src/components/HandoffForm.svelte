<script lang="ts">
  import { untrack } from "svelte";
  import { Button } from "$lib/components/ui/button";
  import { Input } from "$lib/components/ui/input";
  import { Label } from "$lib/components/ui/label";
  import * as Select from "$lib/components/ui/select";
  import { uploadQuickStartFile } from "../api";
  import { emptyLocation } from "../handoff-draft";
  import type { HandoffEditor } from "../hooks/handoff.svelte";
  import { useChatUpload } from "../hooks/upload.svelte";
  import type { TargetKind } from "../new-session-defaults";
  import { getShell } from "../shell.svelte";
  import ComposerCard from "./chat/ComposerCard.svelte";
  import ChatUploadButton from "./chat/ChatUploadButton.svelte";
  import HandoffPreviewCard from "./HandoffPreviewCard.svelte";
  import InlineNotice from "./InlineNotice.svelte";
  import LocationPicker from "./LocationPicker.svelte";
  import WorktreeCreateFields from "./WorktreeCreateFields.svelte";

  let { editor: e, fork }: { editor: HandoffEditor; fork: boolean } = $props();
  const shell = getShell();
  const agents = $derived(
    shell.launchConfigs.state.status === "ready"
      ? shell.launchConfigs.state.configs.filter((item) => item.chat)
      : [],
  );
  const repositories = $derived(shell.repositories.list);
  const agent = $derived(agents.find((item) => item.id === e.configId));
  const repository = $derived(
    repositories.find((item) => item.id === e.location.repositoryId),
  );
  let worktreeFields = $state<WorktreeCreateFields>();
  let managedPath = $state("");
  const uploads = useChatUpload(
    uploadQuickStartFile,
    (message) => e.reportError(message),
    { get: () => e.attachments, set: (next) => (e.attachments = next) },
  );
  const locationReady = $derived(
    !fork || e.locationKind === "directory"
      ? !fork || !!e.location.cwd.trim()
      : e.locationKind === "new-worktree"
        ? (worktreeFields?.isReady() ?? false)
        : !!e.location.repositoryId,
  );
  const regenerate = $derived(
    !e.preview ||
      e.sealed ||
      e.choiceChanged ||
      ((e.preview.stale || e.preview.budget.overBudget) &&
        e.delivery?.status !== "failed"),
  );
  const disabled = $derived(
    e.delivery?.status === "pending"
      ? e.busy || e.saving || e.loading || !e.delivery.clientRequestId
      : e.busy ||
          e.saving ||
          !!uploads.progress ||
          e.loading ||
          !e.source ||
          e.conflict ||
          e.locked ||
          !agent ||
          (!locationReady &&
            !(e.delivery?.status === "failed" && !e.choiceChanged)) ||
          (!regenerate && !e.task.trim()),
  );
  const actionLabel = $derived(
    e.delivery?.status === "unknown"
      ? "等待对账"
      : e.delivery?.status === "pending"
        ? "继续准备交接"
        : regenerate
          ? e.preview
            ? "重新预览"
            : "生成预览"
          : e.delivery?.status === "failed"
            ? "明确重试交接"
            : "发送交接",
  );
  $effect(() => {
    if (e.sealed && e.preview) uploads.commit(e.preview.attachments);
  });
  function kind(next: TargetKind) {
    e.locationKind = next;
    const prior = untrack(() => e.location);
    e.location =
      next === "directory"
        ? { ...emptyLocation(), cwd: prior.cwd }
        : {
            ...emptyLocation(),
            repositoryId: prior.repositoryId,
            worktreeId: next === "worktree" ? prior.worktreeId : "",
          };
  }
  function repositoryId(value: string) {
    if (untrack(() => e.locationKind) === "directory") return;
    if (value !== untrack(() => e.location.repositoryId))
      e.location = { ...emptyLocation(), repositoryId: value };
  }
  function worktreeId(value: string) {
    if (untrack(() => e.locationKind) !== "worktree") return;
    e.location = {
      ...untrack(() => e.location),
      worktreeId: value,
      cwd: "",
      newWorktreeBranch: "",
      newWorktreeBase: "",
    };
  }
</script>

<div class="min-w-0 space-y-4" data-handoff-form>
  {#if e.error}<InlineNotice tone="danger">{e.error}</InlineNotice>{/if}
  {#if e.conflict}
    <div class="flex flex-wrap gap-2">
      <Button
        variant="outline"
        disabled={e.busy}
        onclick={() => void e.serverVersion()}>读取服务端版本</Button
      >
      <Button
        variant="outline"
        disabled={e.busy || e.locked || !e.configId}
        onclick={() => void e.generate()}>保留编辑重新预览</Button
      >
    </div>
  {/if}
  <fieldset class="min-w-0 space-y-3" disabled={e.busy || e.locked}>
    <div class="grid min-w-0 gap-3 sm:grid-cols-2">
      <div class="min-w-0 space-y-2">
        <Label for="handoff-agent">目标 Agent</Label>
        <Select.Root
          type="single"
          value={e.configId}
          onValueChange={(value) => {
            e.configId = value;
            e.model = "";
          }}
        >
          <Select.Trigger id="handoff-agent" aria-label="目标 Agent"
            >{agent?.name ?? "选择目标 Agent"}</Select.Trigger
          >
          <Select.Content
            >{#each agents as item (item.id)}<Select.Item
                value={item.id}
                label={item.name}>{item.name}</Select.Item
              >{/each}</Select.Content
          >
        </Select.Root>
      </div>
      <div class="min-w-0 space-y-2">
        <Label for="handoff-model">模型（可选）</Label>
        <Input
          id="handoff-model"
          value={e.model}
          oninput={(event) => (e.model = event.currentTarget.value)}
          placeholder="使用目标已保存的模型"
          autocomplete="off"
        />
      </div>
    </div>
    {#if !agents.length}<p class="text-sm text-muted-foreground">
        请先添加支持聊天的启动配置。
      </p>{/if}
    {#if fork}
      <p class="break-all text-xs text-muted-foreground">
        历史来源位置：{e.preview
          ? e.preview.sourceLocation.cwd || "未记录，请重新预览"
          : (e.source?.cwd ?? "")}（仅供参考）
      </p>
      <LocationPicker
        {repositories}
        recent={[]}
        preferredRepositoryId={e.source?.repositoryId ?? ""}
        createdTree={null}
        newWorktreeSummary={e.location.newWorktreeBranch ||
          "发送时创建新工作树"}
        bind:kind={() => e.locationKind, kind}
        bind:cwd={
          () => e.location.cwd,
          (value) => (e.location = { ...untrack(() => e.location), cwd: value })
        }
        bind:repositoryId={() => e.location.repositoryId, repositoryId}
        bind:worktreeId={() => e.location.worktreeId, worktreeId}
        disabled={e.busy || e.locked}
      />
      {#if e.locationKind === "new-worktree" && repository}
        {#key repository.id}<WorktreeCreateFields
            bind:this={worktreeFields}
            {repository}
            deferred
            mode="new"
            bind:path={managedPath}
            bind:branch={
              () => e.location.newWorktreeBranch,
              (value) =>
                (e.location = {
                  ...untrack(() => e.location),
                  newWorktreeBranch: value,
                })
            }
            bind:base={
              () => e.location.newWorktreeBase,
              (value) =>
                (e.location = {
                  ...untrack(() => e.location),
                  newWorktreeBase: value,
                })
            }
            disabled={e.busy || e.locked}
          />{/key}
        <p class="text-xs text-muted-foreground">
          预览不创建工作树；首次发送时由服务端分配目录并创建新分支。
        </p>
      {/if}
    {:else}
      <div class="min-w-0 space-y-1 text-sm">
        <Label>工作位置（保持不变）</Label>
        <p class="break-all text-muted-foreground">{e.source?.cwd ?? ""}</p>
      </div>
    {/if}
  </fieldset>
  {#if e.choiceChanged && !e.sealed}<InlineNotice tone="warning"
      >目标或位置已修改，请重新预览。当前历史与用户任务仍保留。</InlineNotice
    >{/if}
  <HandoffPreviewCard editor={e} />
  <ComposerCard
    value={e.task}
    oninput={(event) => (e.task = event.currentTarget.value)}
    label="交接后的用户任务"
    placeholder="写给目标 Agent 的新任务，与历史分别发送…"
    {uploads}
    disabled={e.busy || e.locked}
    uploadsDisabled={e.busy || e.locked}
    busy={e.busy || !!uploads.progress || !!e.preview}
    action={{
      kind: "send",
      label: actionLabel,
      disabled,
      onclick: () =>
        void (e.delivery?.status === "pending"
          ? e.send()
          : regenerate
            ? e.generate()
            : e.send()),
    }}
  >
    {#snippet leading()}<ChatUploadButton
        {uploads}
        disabled={e.busy || e.locked}
      />{/snippet}
    {#snippet hints()}<p class="px-1 text-xs text-muted-foreground">
        {e.locked
          ? "投递状态尚未确定，暂不能修改或重复发送。"
          : e.saving
            ? "正在保存任务…"
            : e.preview && !e.dirty && !e.sealed
              ? "用户任务已独立保存。"
              : "生成预览后，历史与任务分别保存到服务端。"}
      </p>{/snippet}
  </ComposerCard>
  {#if e.preview && !e.locked}
    <div class="flex flex-wrap gap-2">
      <Button
        variant="outline"
        disabled={e.busy || !e.source || !agent || !locationReady}
        onclick={() => void e.generate()}
        >重新预览{e.preview.manual || e.context !== e.preview.renderedText
          ? "（保留编辑）"
          : ""}</Button
      >
    </div>
  {/if}
</div>
