<script lang="ts">
  import { errorMessage } from "../../platform/error";
  import { untrack } from "svelte";
  import { navigate, recentDirs, stored } from "../../platform/api";
  import type { LaunchConfigsState } from "../../domains/settings/launch-configs";
  import {
    buildNewSessionSummary,
    formatLocationSummaryText,
    formatSubmitButtonLabel,
    isCommandReady,
    isDirectoryNotFoundError,
    isLocationTargetReady,
    resolveEntryPreset,
    resolveFallbackAfterWorktreeRemoved,
    resolveInitialLaunch,
    resolveInitialTarget,
    resolveInstructionRepository,
    resolvePreferredStartBranch,
    storedView,
    submitNewSessionRequest,
    validateNewSessionSubmit,
    type LaunchEntry,
    type LaunchScope,
    type LaunchSource,
  } from "../../domains/sessions/new-session-defaults";
  import { sessionConflict } from "../../domains/sessions/recoverable";
  import ConnectorInstallHint from "./ConnectorInstallHint.svelte";
  import type {
    AgentConnector,
    Repository,
    SessionInfo,
    SessionView,
    Worktree,
  } from "../../contracts/schemas";
  import { Button } from "$lib/components/ui/button";
  import { Checkbox } from "$lib/components/ui/checkbox";
  import { Input } from "$lib/components/ui/input";
  import { Label } from "$lib/components/ui/label";
  import SplitDirectionChoice from "../terminal/SplitDirectionChoice.svelte";
  import * as AlertDialog from "$lib/components/ui/alert-dialog";
  import AdvancedSection from "../shared/AdvancedSection.svelte";
  import FirstMessageFields from "./FirstMessageFields.svelte";
  import LaunchPicker from "./LaunchPicker.svelte";
  import LocationPicker from "../workspace/LocationPicker.svelte";
  import LazyDialog from "../shared/LazyDialog.svelte";
  import ResponsiveDialog from "../shared/ResponsiveDialog.svelte";
  import WorktreeCreateFields from "../workspace/WorktreeCreateFields.svelte";
  import type { SplitDirection } from "../../domains/terminal/terminal-layout";

  interface Props {
    launchConfigs: LaunchConfigsState;
    defaultCwd: string;
    repositories?: Repository[];
    /** The contextual entry the dialog was opened from, if any. */
    entry?: LaunchEntry;
    /** Narrows the places offered; see LaunchScope. */
    scope?: LaunchScope;
    /** The home page's selected repository; read only. */
    selectedRepositoryId?: string;
    intent?: "session" | "worktree";
    split?: boolean;
    oncreated?: (session: SessionInfo, direction: SplitDirection) => void;
    onchange: () => void;
    onworktreecreated: (tree: Worktree) => void;
    /** A worktree was deleted from the location list. */
    onworktreeremoved?: (tree: Worktree) => void;
    onclose: () => void;
  }

  let {
    launchConfigs,
    defaultCwd,
    repositories = [],
    entry,
    scope,
    selectedRepositoryId = "",
    intent = "session",
    split = false,
    oncreated,
    onchange,
    onworktreecreated,
    onworktreeremoved,
    onclose,
  }: Props = $props();
  const recent = recentDirs();
  const initial = untrack(() =>
    resolveInitialTarget({
      entry,
      scope,
      selectedRepositoryId: scope?.repositoryId ?? selectedRepositoryId,
      repositories,
      recent,
      defaultCwd,
    }),
  );
  const preferredRepositoryId = initial.repositoryId;
  // A split keeps the pane's workspace while its directory is unchanged.
  const entryDirectory = untrack(() =>
    entry?.kind === "directory" ? entry : null,
  );
  let kind = $state(initial.kind);
  let cwd = $state(initial.cwd);
  let repositoryId = $state(initial.repositoryId);
  let worktreeId = $state(initial.worktreeId);
  let withSession = $state(untrack(() => intent === "session"));
  let direction = $state<SplitDirection>("horizontal");
  const treeOnly = $derived(intent === "worktree" && !withSession);

  const lastLaunch = stored("lastLaunch");
  let source = $state<LaunchSource>(
    lastLaunch === "custom" ? "custom" : "unset",
  );
  let configId = $state("");
  // Defaults apply once, and never after the user picked a source.
  let launchResolved = lastLaunch === "custom";
  let launchTouched = false;
  let command = $state(stored("customCommand"));
  let startSide = $state<SessionView>(storedView(stored("hybridStartSide")));
  let name = $state("");
  let prompt = $state("");
  let error = $state("");
  // Starting on the chat view failed: its connector is not installed.
  let missingConnector = $state.raw<AgentConnector>();
  let busy = $state(false);
  let mkdirOpen = $state(false);
  // Avoid initializing an unused portal on every open; keep it mounted after
  // first use so closing animations and focus restoration run.
  let mkdirMounted = $state(false);

  const configs = $derived(
    launchConfigs.status === "ready" ? launchConfigs.configs : [],
  );
  const chosenConfig = $derived(
    source === "config" ? configs.find((c) => c.id === configId) : undefined,
  );
  $effect(() => {
    if (!launchResolved && !launchTouched) {
      const resolved = resolveInitialLaunch(lastLaunch, launchConfigs);
      if (resolved) {
        launchResolved = true;
        source = resolved.source;
        configId = resolved.configId;
      }
    }
    // Never silently replace a config deleted while the dialog is open.
    if (
      launchConfigs.status !== "loading" &&
      source === "config" &&
      !chosenConfig
    )
      source = "unset";
    if (chosenConfig?.chat) {
      startSide = storedView(
        stored("hybridStartSide:" + chosenConfig.id) ||
          stored("hybridStartSide"),
      );
    }
  });
  const repository = $derived(repositories.find((r) => r.id === repositoryId));
  const instructionRepository = $derived(
    resolveInstructionRepository(
      kind,
      cwd,
      entryDirectory,
      repositories,
      repository,
    ),
  );
  const hasInstruction = $derived(!!instructionRepository?.initInstruction);
  const startsOnChat = $derived(!!chosenConfig?.chat && startSide === "chat");
  let instructionChoice = $state<boolean | undefined>();
  const useInstruction = $derived(instructionChoice ?? startsOnChat);
  const commandReady = $derived(isCommandReady(source, chosenConfig, command));

  let newPath = $state("");
  let branchMode = $state<"new" | "existing">("new");
  let branch = $state("");
  let base = $state("");
  let startBranch = $state("");
  let worktreeFields = $state<WorktreeCreateFields>();
  let fieldsRepositoryId = $state(
    initial.kind === "new-worktree" ? initial.repositoryId : "",
  );
  const fieldsRepository = $derived(
    repositories.find((r) => r.id === fieldsRepositoryId),
  );
  let createdTree = $state<Worktree | null>(null);
  const worktreeReady = $derived(worktreeFields?.isReady() ?? false);
  const worktree = $derived(
    createdTree?.id === worktreeId
      ? createdTree
      : repository?.worktrees.find((w) => w.id === worktreeId),
  );
  let removing = $state<Worktree | null>(null);
  function removed(tree: Worktree) {
    onworktreeremoved?.(tree);
    if (createdTree?.id === tree.id) createdTree = null;
    ({ kind, worktreeId } = resolveFallbackAfterWorktreeRemoved(
      repositories,
      kind,
      worktreeId,
      tree,
    ));
  }
  const treeTargetMissing = $derived(treeOnly && kind !== "new-worktree");
  const targetReady = $derived(
    isLocationTargetReady(
      treeTargetMissing,
      kind,
      cwd,
      worktree,
      repository,
      worktreeReady,
    ),
  );
  const newWorktreeSummary = $derived(
    branchMode === "new"
      ? `${branch || "新分支"} ← ${startBranch || "…"}`
      : `已有分支 ${branch || "未选择"}`,
  );
  const locationText = $derived(
    formatLocationSummaryText(kind, cwd, repository, worktree, branch),
  );
  const summary = $derived(
    buildNewSessionSummary({
      treeTargetMissing,
      targetReady,
      treeOnly,
      kind,
      locationText,
      commandReady,
      source,
      chosenConfig,
      command,
    }),
  );

  // A worktree made on the way becomes the chosen location.
  function adoptTree(tree: Worktree) {
    createdTree = tree;
    repositoryId = tree.repositoryId;
    worktreeId = tree.id;
    kind = "worktree";
    onworktreecreated(tree);
    onchange();
  }

  async function create(makeDirectory = false) {
    if (busy) return;
    const onlyTree = treeOnly;
    const validationError = validateNewSessionSubmit(
      onlyTree,
      kind,
      commandReady,
    );
    if (validationError) {
      error = validationError;
      return;
    }
    busy = true;
    error = "";
    missingConnector = undefined;
    try {
      const outcome = await submitNewSessionRequest({
        onlyTree,
        kind,
        repositoryId,
        worktreeId,
        newPath,
        branchMode,
        branch,
        base,
        cwd,
        entryDirectory,
        prompt,
        hasInstruction,
        useInstruction,
        chosenConfig,
        name,
        startSide,
        source,
        command,
        makeDirectory,
        recent,
        adoptTree,
      });
      if (outcome.kind === "tree") {
        onclose();
        return;
      }
      if (oncreated) oncreated(outcome.session, direction);
      else navigate("/s/" + outcome.session.id);
    } catch (err) {
      if (kind !== "directory") onchange();
      const conflict = sessionConflict(err);
      if (
        kind === "directory" &&
        !makeDirectory &&
        isDirectoryNotFoundError(err)
      ) {
        mkdirMounted = true;
        mkdirOpen = true;
      } else if (conflict?.reason === "connector_missing" && conflict.connector)
        missingConnector = conflict.connector;
      else error = errorMessage(err);
    } finally {
      busy = false;
    }
  }
</script>

<ResponsiveDialog
  title={treeOnly ? "新建工作树" : "新建会话"}
  description={treeOnly
    ? "创建独立的代码副本，可选择同时启动会话。"
    : "浏览器断开后，会话仍会继续。"}
  class="p-5 sm:max-w-3xl"
  pinDrawerHeader
  {busy}
  {onclose}
>
  <form
    class="min-w-0"
    onsubmit={(event) => (event.preventDefault(), void create())}
  >
    <fieldset disabled={busy} class="min-w-0 space-y-4">
      <LocationPicker
        {repositories}
        {scope}
        {recent}
        {preferredRepositoryId}
        {createdTree}
        {newWorktreeSummary}
        onremove={(tree) => (removing = tree)}
        bind:kind={
          () => kind,
          (next) => {
            kind = next;
            if (kind === "new-worktree") fieldsRepositoryId = repositoryId;
          }
        }
        bind:cwd
        bind:repositoryId={
          () => repositoryId,
          (next) => {
            repositoryId = next;
            if (kind === "new-worktree") fieldsRepositoryId = repositoryId;
          }
        }
        bind:worktreeId
        disabled={busy}
      />
      {#if fieldsRepository}
        <fieldset
          disabled={busy || kind !== "new-worktree"}
          hidden={kind !== "new-worktree"}
          class="min-w-0"
        >
          {#key fieldsRepository.id}<WorktreeCreateFields
              bind:this={worktreeFields}
              repository={fieldsRepository}
              preferredStartBranch={resolvePreferredStartBranch(
                fieldsRepository,
                scope?.worktreeId,
              )}
              preset={resolveEntryPreset(entry, fieldsRepository.id)}
              disabled={busy || kind !== "new-worktree"}
              bind:path={newPath}
              bind:mode={branchMode}
              bind:branch
              bind:base
              bind:startBranch
            />{/key}
        </fieldset>
      {/if}
      {#if createdTree && kind === "worktree" && worktreeId === createdTree.id}
        <p role="status" class="text-sm text-primary">
          工作树已创建。再次提交只启动会话，不会重复创建分支或目录。
        </p>
      {/if}
      {#if intent === "worktree"}
        <div class="flex items-center gap-2">
          <Checkbox id="worktree-with-session" bind:checked={withSession} />
          <Label for="worktree-with-session">创建后启动会话</Label>
        </div>
      {/if}
      {#if !treeOnly}
        <LaunchPicker
          {launchConfigs}
          bind:source
          bind:configId
          bind:command
          bind:startSide
          disabled={busy}
          onchoose={() => (launchTouched = true)}
        />
        <FirstMessageFields
          repositoryName={instructionRepository?.name}
          {hasInstruction}
          bind:useInstruction={
            () => useInstruction, (next) => (instructionChoice = next)
          }
          bind:prompt
          chat={startsOnChat}
          disabled={busy}
        />
        {#if split}<SplitDirectionChoice bind:direction />{/if}
        <AdvancedSection title="高级：会话名称" summary={name}>
          <div class="space-y-2">
            <Label for="session-name"
              >会话名称 <span class="text-muted-foreground">可选</span></Label
            ><Input
              id="session-name"
              bind:value={name}
              placeholder="留空则用首条提示，没有则跟随终端标题"
            />
          </div>
        </AdvancedSection>
      {/if}
      {#if error}<p role="alert" class="text-sm text-destructive">
          {error}
        </p>{/if}
      {#if missingConnector}
        <ConnectorInstallHint connector={missingConnector}>
          {#snippet actions()}<Button
              type="button"
              size="sm"
              variant="outline"
              disabled={busy}
              onclick={() => {
                startSide = "terminal";
                void create();
              }}>改为从终端开始</Button
            >{/snippet}
        </ConnectorInstallHint>
      {/if}
      <div
        class="flex flex-col gap-3 border-t pt-4 sm:flex-row sm:items-center"
      >
        <p
          id="session-summary"
          class="min-w-0 flex-1 text-sm wrap-anywhere text-muted-foreground"
        >
          {summary}
        </p>
        <div class="flex justify-end gap-2">
          <Button
            type="button"
            variant="outline"
            disabled={busy}
            onclick={onclose}>取消</Button
          >
          <Button
            type="submit"
            aria-describedby="session-summary"
            disabled={busy || !targetReady || (!treeOnly && !commandReady)}
            >{formatSubmitButtonLabel(busy, treeOnly, split)}</Button
          >
        </div>
      </div>
    </fieldset>
  </form>
</ResponsiveDialog>

{#if removing}
  {@const target = removing}
  <LazyDialog
    load={() => import("../workspace/RemoveWorktreeDialog.svelte")}
    label="删除确认"
    onclose={() => (removing = null)}
  >
    {#snippet children(page)}<page.default
        worktree={target}
        {onchange}
        onremoved={() => removed(target)}
        onclose={() => (removing = null)}
      />{/snippet}
  </LazyDialog>
{/if}

{#if mkdirMounted}<AlertDialog.Root bind:open={mkdirOpen}>
    <AlertDialog.Content>
      <AlertDialog.Header
        ><AlertDialog.Title>目录不存在，是否创建？</AlertDialog.Title
        ><AlertDialog.Description class="break-all"
          >将在 {cwd} 创建目录，然后启动会话。</AlertDialog.Description
        ></AlertDialog.Header
      >
      <AlertDialog.Footer>
        <AlertDialog.Cancel>取消</AlertDialog.Cancel>
        <AlertDialog.Action onclick={() => void create(true)}
          >创建目录并启动</AlertDialog.Action
        >
      </AlertDialog.Footer>
    </AlertDialog.Content>
  </AlertDialog.Root>{/if}
