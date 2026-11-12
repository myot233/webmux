<script lang="ts">
  import { onMount, tick, untrack } from "svelte";
  import IconLoader2 from "~icons/tabler/loader-2";
  import { api, navigate, stored, uploadQuickStartFile } from "../platform/api";
  import {
    setBrowserChrome,
    resetBrowserChrome,
  } from "../platform/browser-chrome";
  import type { ConfigValue } from "../platform/chat-api";
  import ChatUploadButton from "../components/chat/ChatUploadButton.svelte";
  import ComposerCard from "../components/chat/ComposerCard.svelte";
  import QuickCommandMenu from "../components/settings/QuickCommandMenu.svelte";
  import QuickStartAgentToolbar from "../components/sessions/QuickStartAgentToolbar.svelte";
  import QuickStartBanners from "../components/sessions/QuickStartBanners.svelte";
  import QuickStartInitInstructionBar from "../components/sessions/QuickStartInitInstructionBar.svelte";
  import QuickStartLocationBar from "../components/sessions/QuickStartLocationBar.svelte";
  import VoiceButton from "../components/shared/VoiceButton.svelte";
  import { voicePreference } from "../hooks/shared/voice-preference.svelte";
  import FileDropZone from "../components/shared/FileDropZone.svelte";
  import SessionSwitcherTrigger from "../components/shell/SessionSwitcherTrigger.svelte";
  import SidebarButton from "../components/shell/SidebarButton.svelte";
  import { useChatUpload } from "../hooks/shared/upload.svelte";
  import { useViewport } from "../hooks/shared/viewport.svelte";
  import {
    executeQuickStart,
    pickDefault,
    quickStartMemory,
    saveQuickStartDraftFromPage,
    type QuickStartBranches,
    type QuickStartTargetMode,
  } from "../hooks/sessions/quick-start";
  import {
    insertAtSelection,
    usesArtifactDir,
    usesWebmuxBrief,
  } from "../domains/settings/quick-commands";
  import { quickCommands } from "../domains/settings/quick-commands.svelte";
  import {
    LaunchOptionsRequestSchema,
    LaunchOptionsSchema,
    type ChatAttachment,
    type QuickCommand,
    type ChatConfig as ChatOption,
    type ChatPermissionMode as PermissionMode,
    type CreateSessionFailure,
    type Repository,
    type SessionView,
    type Worktree,
  } from "../contracts/schemas";
  import { getShell, openSession } from "../hooks/shared/shell.svelte";
  import { enterSendIntent } from "../hooks/shared/enter-send";
  import { hasTouchInput } from "../platform/touch-input";

  const shell = getShell();

  const repositories = $derived(shell.repositories.list);
  const agents = $derived(
    shell.launchConfigs.state.status === "ready"
      ? shell.launchConfigs.state.configs.filter((config) => config.chat)
      : [],
  );
  const initialDraft = quickStartMemory.loadDraft();
  const remembered = quickStartMemory.choice();
  let repositoryChoice = $state(
    initialDraft.repositoryId ?? remembered?.repositoryId,
  );
  let agentChoice = $state(initialDraft.configId ?? remembered?.configId);
  const usableRepository = (repo: Repository) =>
    repo.availability === "available";
  const repository = $derived(
    pickDefault(repositories, repositoryChoice, usableRepository),
  );
  const agent = $derived(pickDefault(agents, agentChoice, () => true));

  let startSide = $state<SessionView>(
    stored("quickStartSide") === "tui" ||
      stored("quickStartSide") === "terminal"
      ? "terminal"
      : "chat",
  );
  const chat = $derived(startSide === "chat");

  let text = $state(initialDraft.text);
  let attachments = $state<ChatAttachment[]>(initialDraft.attachments ?? []);
  let permissionMode = $state<PermissionMode>(
    initialDraft.permissionMode ?? "auto",
  );
  let edits = $state<Record<string, Record<string, ConfigValue>>>(
    initialDraft.edits ?? {},
  );
  let baseChoice = $state<{ repositoryId: string; name: string } | undefined>(
    initialDraft.baseChoice,
  );
  let targetModeChoice = $state<QuickStartTargetMode | undefined>(
    initialDraft.targetMode ?? remembered?.targetMode,
  );
  let worktreeChoice = $state<
    { repositoryId: string; worktreeId: string } | undefined
  >(initialDraft.worktreeChoice ?? remembered?.worktreeChoice);
  let instructionChoice = $state(initialDraft.instructionChoice);

  function persistDraft() {
    saveQuickStartDraftFromPage({
      text,
      attachments,
      repositoryChoice,
      agentChoice,
      targetModeChoice,
      baseChoice,
      worktreeChoice,
      permissionMode,
      edits,
      instructionChoice,
      remembered,
    });
  }
  $effect(() => {
    persistDraft();
  });

  type Probe = { agentId: string; options?: ChatOption[]; done: boolean };
  let probe = $state<Probe>();
  const agentId = $derived(agent?.id);
  const repositoryKnown = $derived(!!repository);
  $effect(() => {
    const id = agentId;
    if (!id || !repositoryKnown) return;
    const repo = untrack(() => repository?.id) ?? "";
    probe = { agentId: id, done: false };
    const controller = new AbortController();
    api(
      `/launch-configs/${id}/options`,
      LaunchOptionsSchema,
      "POST",
      { repositoryId: repo },
      controller.signal,
      LaunchOptionsRequestSchema,
    )
      .then((options) => (probe = { agentId: id, options, done: true }))
      .catch(() => {
        if (!controller.signal.aborted) probe = { agentId: id, done: true };
      });
    return () => controller.abort();
  });
  const probing = $derived(
    !!agent && probe?.agentId === agent.id && !probe.done,
  );

  // The page's option values: the agent's catalog plus edits.
  const catalog = $derived(
    probe?.agentId === agent?.id && probe?.options
      ? probe.options
      : (agent?.chatOptions ?? []),
  );
  const options = $derived<ChatOption[]>(
    catalog.map((option) => {
      const edit = agent ? edits[agent.id]?.[option.id] : undefined;
      return edit ? { ...option, ...edit } : option;
    }),
  );
  function setOption(id: string, value: ConfigValue) {
    if (!agent) return;
    edits = { ...edits, [agent.id]: { ...edits[agent.id], [id]: value } };
    persistDraft();
  }

  let targetMode = $state<QuickStartTargetMode>("new");
  let selectedWorktree = $state<Worktree | undefined>();
  let branchData = $state<QuickStartBranches | undefined>();
  let branchError = $state<string | undefined>();
  let base = $state("");

  const missing = $derived<"" | "agent" | "repository">(
    shell.launchConfigs.state.status === "ready" && !agents.length
      ? "agent"
      : shell.repositories.settled && !repositories.some(usableRepository)
        ? "repository"
        : "",
  );
  const useInstruction = $derived(
    instructionChoice && instructionChoice.repositoryId === repository?.id
      ? instructionChoice.use
      : chat,
  );
  let sending = $state(false);
  let failure = $state<CreateSessionFailure>();
  let uploadError = $state("");
  const uploads = useChatUpload(
    uploadQuickStartFile,
    (message) => (uploadError = message),
    {
      get: () => attachments,
      set: (next) => {
        attachments = next;
        persistDraft();
      },
    },
  );
  const locked = $derived(sending || (chat && probing));
  const hasTarget = $derived(
    targetMode === "existing" ? !!selectedWorktree : !!base,
  );
  const canSend = $derived(
    !locked &&
      !uploads.progress &&
      !!repository &&
      !!agent &&
      hasTarget &&
      text.trim() !== "",
  );

  let input = $state<HTMLTextAreaElement | null>(null);
  const viewport = useViewport();
  onMount(() => {
    setBrowserChrome("新聊天 · WebMux", false);
    if (!hasTouchInput() && window.innerWidth >= 640) input?.focus();
    return resetBrowserChrome;
  });

  async function send() {
    if (!canSend || !repository || !agent) return;
    if (targetMode === "new" && !branchData) return;
    if (targetMode === "existing" && !selectedWorktree) return;
    sending = true;
    failure = undefined;
    const staged = uploads.take();
    try {
      failure = await executeQuickStart({
        repository,
        agentId: agent.id,
        startSide,
        text,
        useInstruction,
        attachments: staged,
        options,
        permissionMode,
        targetMode,
        selectedWorktree,
        branchData,
        base,
        commitAttachments: (items) => uploads.commit(items),
        restoreAttachments: (items) => uploads.restore(items),
        onSuccess: (sessionId) => {
          text = "";
          instructionChoice = undefined;
          openSession({ id: sessionId });
        },
      });
    } finally {
      sending = false;
      void shell.repositories.refresh();
    }
  }
  async function insertQuickCommand(cmd: QuickCommand) {
    await insertTextAtCursor(cmd.command);
  }
  async function insertTextAtCursor(snippet: string) {
    const result = insertAtSelection(
      text,
      input?.selectionStart ?? text.length,
      input?.selectionEnd ?? text.length,
      snippet,
    );
    text = result.text;
    persistDraft();
    await tick();
    input?.focus();
    input?.setSelectionRange(result.caret, result.caret);
  }
  function keydown(event: KeyboardEvent) {
    if (!enterSendIntent(event)) return;
    event.preventDefault();
    void send();
  }
</script>

{#snippet agentToolbar(compact = false)}
  {#if agent}
    <QuickStartAgentToolbar
      {compact}
      {agents}
      {agent}
      {startSide}
      {chat}
      {permissionMode}
      {options}
      {locked}
      {probing}
      {sending}
      onagentchange={(id) => {
        agentChoice = id;
        persistDraft();
      }}
      onsidechange={(side) => {
        startSide = side;
      }}
      onmodechange={(mode) => {
        permissionMode = mode;
        persistDraft();
      }}
      onoptionchange={setOption}
    />
  {/if}
{/snippet}

<main
  class="flex h-(--viewport-height) flex-col overflow-hidden"
  style:--viewport-height={`${viewport.height}px`}
>
  <header
    class="app-header flex min-h-(--app-header-height) shrink-0 items-center gap-1 border-b bg-card px-1 sm:px-4"
  >
    <SidebarButton class="sm:hidden" />
    <h1 class="flex-1 text-sm font-medium">新聊天</h1>
    <SessionSwitcherTrigger size="icon-sm" />
  </header>

  <!-- Files dropped anywhere below the header become attachments. -->
  <FileDropZone
    class="relative flex min-h-0 flex-1 flex-col"
    {uploads}
    blocked={sending}
    label="松开以附加到这条消息"
  >
    <div
      class="flex min-h-0 flex-1 flex-col items-center justify-center gap-2 overflow-hidden px-4 text-center"
    >
      <h2 class="text-2xl font-medium">要让 agent 做什么？</h2>
      <p class="max-w-md text-sm text-muted-foreground">
        发送后会在{targetMode === "existing"
          ? "所选工作树中"
          : "所选仓库新建 worktree，"}{chat
          ? "以聊天方式启动 agent 并发出这条消息"
          : "在终端中启动 agent，待它就绪后输入这条消息并回车"}。
      </p>
    </div>

    <footer
      class="mx-auto w-full max-w-3xl space-y-1.5 px-3 pb-safe-2 sm:px-4 sm:pb-6"
      data-quick-start-composer
    >
      <QuickStartBanners
        {missing}
        {targetMode}
        {branchError}
        {repository}
        {failure}
        {shell}
      />

      <ComposerCard
        bind:textarea={input}
        value={text}
        oninput={(event) => {
          text = event.currentTarget.value;
          persistDraft();
        }}
        placeholder={targetMode === "existing"
          ? "描述任务，发送后在所选工作树中开始"
          : "描述任务，发送后在新 worktree 中开始"}
        label="任务描述"
        disabled={sending}
        {uploads}
        uploadsDisabled={sending}
        busy={Boolean(
          text || attachments.length > 0 || uploads.progress || sending,
        )}
        action={{
          kind: "send",
          label: "发送",
          disabled: !canSend,
          onclick: () => void send(),
        }}
        textareaProps={{ onkeydown: keydown }}
      >
        {#snippet location({ compact })}
          <QuickStartLocationBar
            {compact}
            {repositories}
            {repository}
            {sending}
            {targetModeChoice}
            {worktreeChoice}
            {baseChoice}
            {uploadError}
            bind:targetMode
            bind:selectedWorktree
            bind:branchData
            bind:branchError
            bind:base
            onrepositorychange={(id) => {
              repositoryChoice = id;
              instructionChoice = undefined;
              persistDraft();
            }}
            ontargetmodechange={(next) => {
              if (targetModeChoice !== next) {
                targetModeChoice = next;
                persistDraft();
              }
            }}
            onselectworktree={(worktreeId) => {
              targetModeChoice = "existing";
              worktreeChoice = {
                repositoryId: repository?.id ?? "",
                worktreeId,
              };
              persistDraft();
            }}
            onselectbranch={(name) => {
              targetModeChoice = "new";
              baseChoice = { repositoryId: repository?.id ?? "", name };
              persistDraft();
            }}
          />
        {/snippet}
        {#snippet menu({ compact })}
          {#if repository?.initInstruction}
            <QuickStartInitInstructionBar
              {compact}
              {repository}
              {sending}
              {useInstruction}
              onchange={(next) => {
                instructionChoice = repository
                  ? { repositoryId: repository.id, use: next }
                  : undefined;
                persistDraft();
              }}
            />
          {/if}
        {/snippet}
        {#snippet hints()}
          {#if usesArtifactDir(text)}
            <p class="px-1 pt-1 text-xs text-muted-foreground">
              {"{{dir}}"} 会在会话创建后替换为它的临时文件夹路径。
            </p>
          {/if}
          {#if usesWebmuxBrief(text)}
            <p class="px-1 pt-1 text-xs text-muted-foreground">
              {"{{webmux}}"} 会在会话创建后替换为 WebMux 工具说明。
            </p>
          {/if}
        {/snippet}
        {#snippet leading()}
          <ChatUploadButton {uploads} disabled={sending} />
          {#if sending}
            <span
              class="flex items-center gap-1.5 text-xs text-muted-foreground"
              role="status"
            >
              <IconLoader2
                class="size-3.5 animate-spin motion-reduce:animate-none"
                aria-hidden="true"
              />{targetMode === "existing"
                ? `正在启动 ${agent?.name}…`
                : `正在创建 worktree 并启动 ${agent?.name}…`}
            </span>
          {:else}
            {@render agentToolbar(true)}
          {/if}
        {/snippet}
        {#snippet accessories({ size, variant })}
          {#if shell.setup.value.voice && voicePreference.enabled}
            <VoiceButton
              {size}
              {variant}
              disabled={sending}
              onresult={(recognized) => void insertTextAtCursor(recognized)}
              onerror={(message) => (uploadError = message)}
            />
          {/if}
          <QuickCommandMenu
            commands={quickCommands.list}
            target="chat"
            unavailable=""
            disabled={sending}
            side="top"
            {size}
            {variant}
            run={insertQuickCommand}
            manage={() => navigate("/settings#quick-commands")}
            onCloseAutoFocus={(event) => {
              event.preventDefault();
              input?.focus();
            }}
          />
        {/snippet}
        {#snippet below()}
          {@render agentToolbar(false)}
        {/snippet}
      </ComposerCard>
    </footer>
  </FileDropZone>
</main>
