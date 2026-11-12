<script lang="ts">
  import { onMount } from "svelte";
  import { Input } from "$lib/components/ui/input";
  import { Label } from "$lib/components/ui/label";
  import * as NativeSelect from "$lib/components/ui/native-select";
  import {
    branchOperations,
    type BranchFlowProps,
  } from "../../domains/workspace/branch-operations";
  import { errorMessage } from "../../platform/error";
  import { PreviewFlow } from "../../hooks/workspace/preview-flow.svelte";
  import type {
    PushPreview,
    PushResult,
    Remote,
  } from "../../contracts/schemas";
  import BranchCommitList from "./BranchCommitList.svelte";
  import FlowActions from "../shared/FlowActions.svelte";
  import FlowStatus from "../shared/FlowStatus.svelte";
  import InlineNotice from "../shared/InlineNotice.svelte";
  import PreviewIssues from "./PreviewIssues.svelte";

  // Pushes the previewed commit of a local branch to its upstream or, for a
  // branch without one (or whose upstream is gone), publishes it to a
  // remote and branch name the user may change; a published branch tracks
  // its target afterwards. The push URL is shown redacted. A refused push
  // answers with its reason and previews again.
  let {
    repositoryId,
    branch,
    busy = $bindable(false),
    source = branchOperations,
    onchange,
    onclose,
  }: BranchFlowProps & { branch: string } = $props();

  // Publishing: set from the first preview, then edited by the user and sent
  // with every preview after it.
  let publishing = $state(false);
  let remote = $state("");
  let target = $state("");
  const targetName = $derived(target.trim());
  const flow = new PreviewFlow<PushPreview>(async (signal) => {
    const preview = await source.pushPreview(
      repositoryId,
      publishing
        ? {
            branch,
            remote: remote || undefined,
            target: targetName || undefined,
          }
        : { branch },
      signal,
    );
    if (preview.publish && !publishing) {
      publishing = true;
      remote = preview.remote;
      target = preview.target;
    }
    return preview;
  });
  const preview = $derived(flow.preview);

  let remotes = $state<Remote[]>([]);
  let remotesError = $state("");
  let timer: ReturnType<typeof setTimeout> | undefined;
  onMount(() => {
    void flow.refresh();
    const controller = new AbortController();
    source
      .remotes(repositoryId, controller.signal)
      .then((value) => (remotes = value.remotes))
      .catch((err: unknown) => {
        if (!controller.signal.aborted) remotesError = errorMessage(err);
      });
    return () => {
      clearTimeout(timer);
      controller.abort();
      flow.dispose();
    };
  });
  // Typing a name previews it once the user pauses.
  function editTarget() {
    clearTimeout(timer);
    if (!targetName) {
      flow.invalidate();
      return;
    }
    timer = setTimeout(() => void flow.refresh(), 300);
  }

  let rejected = $state<PushResult | null>(null);
  let done = $state<PushPreview | null>(null);
  // The shown preview must be the one for the fields as they are now.
  const current = $derived(
    !!preview &&
      (!publishing ||
        (preview.remote === remote && preview.target === targetName)),
  );
  const ready = $derived(
    current && !!preview?.confirmationId && !flow.loading && !flow.running,
  );
  async function confirm() {
    const pushed = preview;
    if (!pushed || !ready) return;
    busy = true;
    rejected = null;
    const result = await flow.run(() =>
      source.push(repositoryId, pushed.confirmationId),
    );
    busy = false;
    onchange();
    if (result?.status === "ok") done = pushed;
    else if (result) {
      // The confirmation is used up; the next attempt needs a new preview.
      rejected = result;
      void flow.refresh();
    }
  }
  const destination = (shown: PushPreview) => `${shown.remote}/${shown.target}`;
</script>

<div class="min-w-0 space-y-3">
  {#if done}
    <InlineNotice tone="success"
      >已{done.publish ? "发布" : "推送"}到
      <span class="font-mono break-all">{destination(done)}</span>{done.publish
        ? "，并设为本地分支的上游"
        : ""}。</InlineNotice
    >
  {:else}
    {#if rejected}
      <InlineNotice tone="danger"
        >推送未完成：{rejected.message ||
          "远端拒绝了推送"}。本地分支与上游配置未改变。</InlineNotice
      >
    {/if}
    <FlowStatus {flow} />
    {#if publishing}
      <div class="grid gap-3 sm:grid-cols-2">
        <div class="min-w-0 space-y-1.5">
          <Label for="push-remote">远程</Label>
          <NativeSelect.Root
            id="push-remote"
            class="w-full"
            value={remote}
            disabled={busy}
            onchange={(event) => {
              remote = event.currentTarget.value;
              void flow.refresh();
            }}
          >
            {#if !remote}<NativeSelect.Option value="" disabled
                >选择远程</NativeSelect.Option
              >{/if}
            {#each remotes as item (item.name)}
              <NativeSelect.Option value={item.name}
                >{item.name}</NativeSelect.Option
              >
            {/each}
          </NativeSelect.Root>
        </div>
        <div class="min-w-0 space-y-1.5">
          <Label for="push-target">远端分支名</Label>
          <Input
            id="push-target"
            mono
            bind:value={target}
            disabled={busy}
            autocomplete="off"
            spellcheck={false}
            oninput={editTarget}
          />
        </div>
      </div>
      {#if remotesError}
        <p class="text-xs text-destructive">无法读取远程：{remotesError}</p>
      {:else if !targetName}
        <p class="text-xs text-destructive">请输入远端分支名。</p>
      {/if}
      <p class="text-xs text-muted-foreground">
        发布成功后，本地分支会把它设为上游。
      </p>
    {/if}
    {#if preview}
      <dl class="grid grid-cols-[auto_minmax(0,1fr)] gap-x-3 gap-y-2 text-xs">
        <dt class="text-muted-foreground">分支</dt>
        <dd class="font-mono break-all">
          {preview.branch}{preview.commit
            ? ` · ${preview.commit.slice(0, 8)}`
            : ""}
        </dd>
        {#if preview.remote}
          <dt class="text-muted-foreground">目标</dt>
          <dd class="font-mono break-all">{destination(preview)}</dd>
        {/if}
        {#if preview.pushUrl}
          <dt class="text-muted-foreground">推送地址</dt>
          <dd class="font-mono break-all">{preview.pushUrl}</dd>
        {/if}
      </dl>
      <PreviewIssues blockers={preview.blockers} warnings={preview.warnings} />
      {#if preview.commits.total > 0}
        <BranchCommitList title="待推送的提交" commits={preview.commits} />
      {/if}
    {/if}
  {/if}
  <FlowActions
    done={!!done}
    {busy}
    {ready}
    label={busy
      ? publishing
        ? "正在发布…"
        : "正在推送…"
      : publishing
        ? "发布"
        : "推送"}
    onconfirm={() => void confirm()}
    {onclose}
  />
</div>
