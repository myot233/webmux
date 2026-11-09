<script lang="ts">
  import { onMount, tick, untrack } from "svelte";
  import IconAlertCircle from "~icons/tabler/alert-circle";
  import IconBook2 from "~icons/tabler/book-2";
  import IconGitBranch from "~icons/tabler/git-branch";
  import IconLoader2 from "~icons/tabler/loader-2";
  import IconMessage from "~icons/tabler/message";
  import IconNotes from "~icons/tabler/notes";
  import IconTerminal2 from "~icons/tabler/terminal-2";
  import { Button } from "$lib/components/ui/button";
  import { Checkbox } from "$lib/components/ui/checkbox";
  import { Label } from "$lib/components/ui/label";
  import * as Popover from "$lib/components/ui/popover";
  import { api, navigate, store, stored, uploadQuickStartFile } from "../api";
  import { setBrowserChrome, resetBrowserChrome } from "../browser-chrome";
  import { remoteBranchBases } from "../branch-name";
  import type { ConfigValue } from "../chat-api";
  import ChatConfig from "../components/chat/ChatConfig.svelte";
  import ChatPermissionMode from "../components/chat/ChatPermissionMode.svelte";
  import ChatSelect from "../components/chat/ChatSelect.svelte";
  import QuietTrigger from "../components/chat/QuietTrigger.svelte";
  import ChatSessionOptions from "../components/chat/ChatSessionOptions.svelte";
  import ChatUploadButton from "../components/chat/ChatUploadButton.svelte";
  import ComposerCard from "../components/chat/ComposerCard.svelte";
  import BranchCommandList from "../components/BranchCommandList.svelte";
  import QuickCommandMenu from "../components/QuickCommandMenu.svelte";
  import VoiceButton from "../components/VoiceButton.svelte";
  import { voicePreference } from "../hooks/voice-preference.svelte";
  import ConnectorInstallHint from "../components/ConnectorInstallHint.svelte";
  import FileDropZone from "../components/FileDropZone.svelte";
  import LaunchConfigIcon from "../components/LaunchConfigIcon.svelte";
  import SessionSwitcherTrigger from "../components/SessionSwitcherTrigger.svelte";
  import SidebarButton from "../components/SidebarButton.svelte";
  import { errorMessage } from "../error";
  import { useChatUpload } from "../hooks/upload.svelte";
  import {
    configValues,
    failedStep,
    pickDefault,
    quickStartMemory,
    sendQuickStart,
  } from "../quick-start";
  import {
    expandInitInstruction,
    insertAtSelection,
    usesArtifactDir,
    usesWebmuxBrief,
  } from "../quick-commands";
  import { quickCommands } from "../quick-commands.svelte";
  import {
    BranchesSchema,
    LaunchOptionsRequestSchema,
    LaunchOptionsSchema,
    type ChatAttachment,
    type QuickCommand,
    type ChatConfig as ChatOption,
    type ChatPermissionMode as PermissionMode,
    type CreateSessionFailure,
    type Repository,
    type SessionView,
  } from "../schemas";
  import { getShell, openSession } from "../shell.svelte";
  import { enterSendIntent } from "../enter-send";
  import { hasTouchInput } from "../touch-input";
  import { z } from "zod";

  // Chat mode's home (openspec quick-start-chat): write the task, pick the
  // repository, base branch and agent under the input, and one send makes
  // the server create a worktree, start the agent's chat side, apply the
  // options and send the first message. Laid out like the Claude Code web
  // composer: an empty stage with the input card at the bottom.
  const shell = getShell();
  type Branches = z.infer<typeof BranchesSchema>;

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

  // Where the agent starts: its chat, or its terminal, which gets the message
  // typed in (with attachment paths appended). A terminal start has no agent
  // options or permission mode. Stored as "tui" before views were named.
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
  // The repository whose init instruction the user unticked for this send; a
  // repository change or sending ticks it again.
  let unticked = $state<string | undefined>(initialDraft.unticked);

  function persistDraft() {
    const edited = Object.keys(edits).length ? edits : undefined;
    quickStartMemory.saveDraftState({
      text,
      ...(attachments.length ? { attachments } : {}),
      ...(repositoryChoice && repositoryChoice !== remembered?.repositoryId
        ? { repositoryId: repositoryChoice }
        : {}),
      ...(agentChoice && agentChoice !== remembered?.configId
        ? { configId: agentChoice }
        : {}),
      ...(baseChoice ? { baseChoice } : {}),
      ...(permissionMode !== "auto" ? { permissionMode } : {}),
      ...(edited ? { edits: edited } : {}),
      ...(unticked ? { unticked } : {}),
    });
  }
  $effect(() => {
    persistDraft();
  });

  // Choosing an agent (and opening the page) reads the options it offers:
  // the server starts it in a throwaway session in the repository and
  // remembers what it offers. While the read runs the agent, its options
  // and sending are disabled; when it ends they are usable again. A failed
  // read keeps the remembered options, if any. A newer choice drops the
  // read in flight.
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

  // The base branch defaults to the repository's main worktree branch and
  // offers its local branches through the new-worktree form's searchable
  // list. Branches are told apart by name: several can share a commit.
  let branches = $state<{ id: string; data?: Branches; error?: string }>();
  let basePickerOpen = $state(false);
  let baseSearch = $state("");
  // Keyed on the ID alone: a refreshed repository list must not cancel the
  // read in flight.
  const repositoryId = $derived(repository?.id);
  $effect(() => {
    const id = repositoryId;
    if (!id || untrack(() => branches?.id) === id) return;
    branches = { id };
    const controller = new AbortController();
    api(
      `/repositories/${id}/branches`,
      BranchesSchema,
      "GET",
      undefined,
      controller.signal,
    )
      .then((data) => {
        if (branches?.id === id) branches = { id, data };
      })
      .catch((err: unknown) => {
        if (branches?.id === id && !controller.signal.aborted)
          branches = { id, error: errorMessage(err) };
      });
    return () => controller.abort();
  });
  const branchData = $derived(
    branches?.id === repository?.id ? branches?.data : undefined,
  );
  const localBranches = $derived.by(() => {
    if (!branchData) return [];
    const local = branchData.branches.map(({ name, head }) => ({ name, head }));
    const main = branchData.defaultBranch ?? "HEAD";
    if (branchData.defaultBase && !local.some((b) => b.name === main))
      local.unshift({ name: main, head: branchData.defaultBase });
    return local;
  });
  const remoteBranches = $derived(
    branchData
      ? remoteBranchBases(
          branchData.bases,
          localBranches.map((b) => b.name),
        ).map(({ name, head }) => ({ name, head }))
      : [],
  );
  const baseBranches = $derived([...localBranches, ...remoteBranches]);
  const baseName = $derived(
    baseChoice?.repositoryId === repository?.id &&
      baseBranches.some((b) => b.name === baseChoice?.name)
      ? (baseChoice?.name ?? "")
      : branchData?.defaultBase
        ? (branchData.defaultBranch ?? "HEAD")
        : "",
  );
  const base = $derived(
    baseBranches.find((b) => b.name === baseName)?.head ?? "",
  );
  function setBasePickerOpen(next: boolean) {
    basePickerOpen = next;
    if (!next) baseSearch = "";
  }

  const missing = $derived(
    shell.launchConfigs.state.status === "ready" && !agents.length
      ? "agent"
      : shell.repositories.settled && !repositories.some(usableRepository)
        ? "repository"
        : "",
  );
  const useInstruction = $derived(unticked !== repository?.id);
  const instructionPreview = $derived(
    expandInitInstruction(
      repository?.initInstruction ?? "",
      quickCommands.list,
    ),
  );
  let sending = $state(false);
  let failure = $state<CreateSessionFailure>();
  // Attachments wait in the server's staging folder until the quick start
  // hands them to its new session with the first message.
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
  // Reading the agent's options holds the agent row and sending.
  const locked = $derived(sending || (chat && probing));
  const canSend = $derived(
    !locked &&
      !uploads.progress &&
      !!repository &&
      !!agent &&
      !!base &&
      text.trim() !== "",
  );

  let input = $state<HTMLTextAreaElement | null>(null);
  onMount(() => {
    setBrowserChrome("新聊天 · WebMux", false);
    if (!hasTouchInput() && window.innerWidth >= 640) input?.focus();
    return resetBrowserChrome;
  });

  async function send() {
    if (!canSend || !repository || !agent || !branches?.data) return;
    sending = true;
    failure = undefined;
    const attachments = uploads.take();
    try {
      // The server names the new branch and its directory so they do not
      // collide with existing ones.
      const outcome = await sendQuickStart({
        newWorktree: { repositoryId: repository.id, mode: "new", base },
        configId: agent.id,
        view: startSide,
        prompt: text,
        useInitInstruction: !!repository.initInstruction && useInstruction,
        // The server fills {{dir}} and {{webmux}} in the message once the
        // session exists, as the quick command menu inserts them.
        expandPrompt: true,
        ...(attachments.length ? { attachments } : {}),
        // A terminal start takes no agent options or permission mode.
        ...(chat
          ? {
              agentConfig: configValues(options),
              permissionMode,
            }
          : {}),
      });
      if (outcome.ok) {
        store("quickStartSide", startSide);
        quickStartMemory.rememberChoice({
          repositoryId: repository.id,
          configId: agent.id,
        });
        text = "";
        unticked = undefined;
        quickStartMemory.clearDraft();
        openSession({ id: outcome.sessionId });
        return;
      }
      failure = outcome.failure;
      // Before the first prompt the files are still staged and can go
      // again; from then on they belong to the created session.
      if (outcome.failure.step !== "prompt") uploads.restore(attachments);
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

<main class="flex h-dvh flex-col">
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
      class="flex min-h-0 flex-1 flex-col items-center justify-center gap-2 px-4 text-center"
    >
      <h2 class="text-2xl font-medium">要让 agent 做什么？</h2>
      <p class="max-w-md text-sm text-muted-foreground">
        发送后会在所选仓库新建 worktree，{chat
          ? "以聊天方式启动 agent 并发出这条消息"
          : "在终端中启动 agent，待它就绪后输入这条消息并回车"}。
      </p>
    </div>

    <footer
      class="mx-auto w-full max-w-3xl space-y-1.5 px-3 pb-safe-4 sm:px-4 sm:pb-6"
      data-quick-start-composer
    >
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
            <Button
              variant="outline"
              size="sm-touch"
              href="/settings#launch-configs">新建启动配置</Button
            >
          {:else}
            <Button
              variant="outline"
              size="sm-touch"
              onclick={shell.addRepository}>添加仓库</Button
            >
          {/if}
        </div>
      {/if}
      {#if branches?.error && branches.id === repository?.id}
        <p role="alert" class="px-1 text-sm text-destructive">
          读取 {repository?.name} 的分支失败：{branches.error}
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

      <ComposerCard
        bind:textarea={input}
        value={text}
        oninput={(event) => {
          text = event.currentTarget.value;
          persistDraft();
        }}
        placeholder="描述任务，发送后在新 worktree 中开始"
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
          <div
            class={[
              "-my-1 flex min-w-0 flex-nowrap items-center gap-0.5 overflow-x-auto px-1 py-1 [scrollbar-width:none]",
              compact && "composer-mobile:hidden",
            ]}
            role="toolbar"
            aria-label="仓库与分支"
          >
            <ChatSelect
              labeled
              name="仓库"
              value={repository?.id ?? ""}
              choices={repositories.map((repo) => ({
                value: repo.id,
                name: repo.name,
                reason: usableRepository(repo)
                  ? undefined
                  : repo.error || "仓库当前不可用",
              }))}
              disabled={sending}
              onchange={(id) => {
                repositoryChoice = id;
                unticked = undefined;
                persistDraft();
              }}
            >
              {#snippet icon()}<IconBook2 aria-hidden="true" />{/snippet}
            </ChatSelect>
            <Popover.Root bind:open={() => basePickerOpen, setBasePickerOpen}>
              <Popover.Trigger disabled={sending || !baseBranches.length}>
                {#snippet child({ props })}
                  <QuietTrigger
                    {props}
                    label={`基准分支：${baseName}`}
                    title={baseName ? `基准分支：${baseName}` : "基准分支"}
                    text={baseName}
                  >
                    {#snippet icon()}<IconGitBranch
                        aria-hidden="true"
                      />{/snippet}
                  </QuietTrigger>
                {/snippet}
              </Popover.Trigger>
              <Popover.Content
                align="start"
                class="w-72 gap-0 overflow-hidden p-0"
              >
                <BranchCommandList
                  label="基准分支"
                  branches={localBranches}
                  {remoteBranches}
                  selected={baseName}
                  loading={!branchData && !branches?.error}
                  failed={!!branches?.error}
                  disabled={sending}
                  bind:search={baseSearch}
                  onselect={(name) => {
                    baseChoice = { repositoryId: repository?.id ?? "", name };
                    persistDraft();
                    setBasePickerOpen(false);
                  }}
                />
              </Popover.Content>
            </Popover.Root>
          </div>

          {#if uploadError}<p
              role="alert"
              class="px-1 text-sm text-destructive"
            >
              {uploadError}
            </p>{/if}
        {/snippet}
        {#snippet menu({ compact })}
          {#if repository?.initInstruction}
            <div
              class={[
                "flex items-center gap-2 border-b px-3 py-1",
                compact && "composer-mobile:hidden",
              ]}
            >
              <Checkbox
                id="use-init-instruction"
                disabled={sending}
                bind:checked={
                  () => useInstruction,
                  (next) => {
                    unticked = next ? undefined : repository?.id;
                    persistDraft();
                  }
                }
              />
              <Label for="use-init-instruction">使用初始化指令</Label>
              <Popover.Root>
                <Popover.Trigger>
                  {#snippet child({ props })}
                    <Button
                      {...props}
                      variant="quiet"
                      size="xs"
                      title="预览将附带的初始化指令"
                      ><IconNotes aria-hidden="true" />预览</Button
                    >
                  {/snippet}
                </Popover.Trigger>
                <Popover.Content align="start" class="w-80 space-y-2">
                  <p class="font-medium">{repository.name} 的初始化指令</p>
                  <p
                    class="max-h-64 overflow-y-auto whitespace-pre-wrap break-words text-muted-foreground"
                  >
                    {instructionPreview.hasUser
                      ? instructionPreview.text.replaceAll(
                          "{{user}}",
                          "〔你的消息〕",
                        )
                      : instructionPreview.text}
                  </p>
                  {#if instructionPreview.missing.length}
                    <p class="text-xs text-warning" role="status">
                      找不到快捷指令：{instructionPreview.missing
                        .map((name) => `{{${name}}}`)
                        .join("、")}，发送时会被服务端拒绝。
                    </p>
                  {/if}
                  {#if !instructionPreview.hasUser}
                    <p class="text-xs text-muted-foreground">
                      指令没有 {"{{user}}"}，会放在你的消息前面，空一行后作为第一条消息一起发送。
                    </p>
                  {/if}
                  {#if usesArtifactDir(instructionPreview.text)}
                    <p class="text-xs text-muted-foreground">
                      {"{{dir}}"} 会在会话创建后替换为它的临时文件夹路径。
                    </p>
                  {/if}
                  {#if instructionPreview.hasWebmux}
                    <p class="text-xs text-muted-foreground">
                      {"{{webmux}}"} 会在会话创建后替换为 WebMux 工具说明：手册地址、命令路径与临时文件夹。
                    </p>
                  {/if}
                  <p class="text-xs text-muted-foreground">
                    取消勾选只影响这次发送。在首页工具栏“⋯”菜单的“仓库设置…”中修改。
                  </p>
                </Popover.Content>
              </Popover.Root>
            </div>
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
              />正在创建 worktree 并启动 {agent?.name}…
            </span>
          {:else if agent}
            <!-- Narrow screens keep the agent and its options in the card, as
                 the chat composer does; the row above keeps the location. -->
            <span
              class="hidden min-w-0 items-center gap-0.5 composer-mobile:flex"
            >
              {@render agentPicker("hidden @md:inline")}
              {@render sidePicker("hidden @md:inline")}
              {#if chat}
                <ChatSessionOptions
                  mode={permissionMode}
                  config={options}
                  disabled={locked}
                  modeDisabled={locked}
                  loading={probing}
                  onmodechange={(mode) => {
                    permissionMode = mode;
                    persistDraft();
                  }}
                  onconfigchange={setOption}
                />
              {/if}
            </span>
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
          {#if agent}
            <div
              class="-my-1 flex min-w-0 items-center overflow-x-auto px-1 py-1 [scrollbar-width:none] composer-mobile:hidden"
              role="toolbar"
              aria-label="启动选项"
            >
              <div class="ms-auto flex shrink-0 items-center gap-0.5">
                {@render agentPicker()}
                {@render sidePicker()}
                {#if chat}
                  <ChatPermissionMode
                    mode={permissionMode}
                    disabled={locked}
                    onchange={(mode) => {
                      permissionMode = mode;
                      persistDraft();
                    }}
                  />
                  {#if options.length}
                    <ChatConfig
                      config={options}
                      disabled={locked}
                      onchange={setOption}
                    />
                  {/if}
                  {@render probeStatus()}
                {/if}
              </div>
            </div>
          {/if}
        {/snippet}
      </ComposerCard>
    </footer>
  </FileDropZone>
</main>

{#snippet agentPicker(labelClass = "")}
  <ChatSelect
    labeled
    {labelClass}
    name="Agent"
    value={agent?.id ?? ""}
    choices={agents.map((item) => ({ value: item.id, name: item.name }))}
    disabled={locked}
    onchange={(id) => {
      agentChoice = id;
      persistDraft();
    }}
  >
    {#snippet choiceIcon(id)}<LaunchConfigIcon
        icon={agents.find((item) => item.id === id)?.icon ?? ""}
        size="sm"
      />{/snippet}
  </ChatSelect>
{/snippet}

{#snippet sidePicker(labelClass = "")}
  <ChatSelect
    labeled
    {labelClass}
    name="启动方式"
    value={startSide}
    choices={[
      { value: "chat", name: "聊天" },
      { value: "terminal", name: "终端" },
    ]}
    disabled={sending}
    onchange={(value) => {
      if (value === "chat" || value === "terminal") startSide = value;
    }}
  >
    {#snippet choiceIcon(value)}{#if value === "terminal"}<IconTerminal2
          aria-hidden="true"
        />{:else}<IconMessage aria-hidden="true" />{/if}{/snippet}
  </ChatSelect>
{/snippet}

{#snippet probeStatus()}
  {#if probing}
    <span
      class="flex shrink-0 items-center gap-1.5 px-2 text-xs text-muted-foreground"
      role="status"
      ><IconLoader2
        class="size-3.5 animate-spin motion-reduce:animate-none"
        aria-hidden="true"
      />正在读取配置…</span
    >
  {/if}
{/snippet}
