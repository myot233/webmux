<script lang="ts">
  import { onDestroy, untrack } from "svelte";
  import { MediaQuery } from "svelte/reactivity";
  import ChatView from "../../src/components/chat/ChatView.svelte";
  import SwitchChoiceDialog from "../../src/components/chat/SwitchChoiceDialog.svelte";
  import SwitchProgress from "../../src/components/chat/SwitchProgress.svelte";
  import CodeChangesList from "../../src/components/CodeChangesList.svelte";
  import ConfirmDialog from "../../src/components/ConfirmDialog.svelte";
  import QuickCommandMenu from "../../src/components/QuickCommandMenu.svelte";
  import SoftKeys from "../../src/components/SoftKeys.svelte";
  import TerminalHeader from "../../src/components/TerminalHeader.svelte";
  import TerminalNotices from "../../src/components/TerminalNotices.svelte";
  import TerminalWorkspace from "../../src/components/TerminalWorkspace.svelte";
  import type { ChatActions } from "../../src/chat-api";
  import type { HybridNotice, HybridView } from "../../src/hooks/hybrid.svelte";
  import type {
    ChatUploads,
    UploadProgress,
  } from "../../src/hooks/upload.svelte";
  import {
    eligibilityText,
    shortId,
    switchSteps,
    switchTarget,
    type SwitchTarget,
  } from "../../src/hybrid";
  import type {
    ChatAttachment,
    ChatState,
    HybridPhase,
    SessionInfo,
  } from "../../src/schemas";
  import { createTerminalDrafts } from "../../src/terminal-draft.svelte";
  import { session } from "../fixtures";
  import {
    chatFixture,
    codeChanges,
    terminalLines,
    withEvents,
    type Agent,
    type Scenario,
  } from "./fixtures";

  // The session page of a hybrid session, assembled from the page's own
  // components (header, notices, workspace split, chat view, switch progress
  // and dialogs) around sample chat frames. Commands change the local sample
  // state the way the server's frames would; nothing leaves the page.
  let {
    scenario = "conversation",
    agent = "omp",
    uploading = false,
  }: { scenario?: Scenario; agent?: Agent; uploading?: boolean } = $props();
  const fixture = untrack(() => chatFixture(scenario, agent));
  let model = $state.raw(fixture.model);
  let chat = $state.raw<ChatState>(fixture.state);
  let confirming = $state<SwitchTarget | null>(null);
  let choiceOpen = $state(untrack(() => scenario === "awaiting-choice"));
  let codeShown = $state(false);
  let notice = $state("");
  let dismissed = $state<string[]>([]);
  let timer: ReturnType<typeof setTimeout> | undefined;
  onDestroy(() => clearTimeout(timer));
  const desktopMedia = new MediaQuery("(min-width: 1024px)");
  const drafts = createTerminalDrafts();
  const draft = drafts.get(session.id);
  if (untrack(() => scenario === "commands")) drafts.edit(draft, "/", 1);
  let uploadProgress = $state<UploadProgress | null>(
    untrack(() =>
      uploading
        ? {
            name: "screenshot.png",
            percent: 62,
            index: 1,
            total: 1,
            waiting: false,
          }
        : null,
    ),
  );
  let attachments = $state<ChatAttachment[]>(
    untrack(() =>
      uploading
        ? [
            {
              path: "/tmp/webmux-upload-story/notes.txt",
              name: "notes.txt",
              mimeType: "text/plain",
              size: 1_204,
              kind: "file",
            },
          ]
        : [],
    ),
  );
  const chatUploads: ChatUploads = {
    get progress() {
      return uploadProgress;
    },
    get attachments() {
      return attachments;
    },
    async uploadFiles() {},
    filePaste() {
      return false;
    },
    cancel() {
      uploadProgress = null;
    },
    remove(path) {
      attachments = attachments.filter((item) => item.path !== path);
    },
    take() {
      const taken = attachments;
      attachments = [];
      return taken;
    },
    restore(sent) {
      attachments = [...sent, ...attachments];
    },
  };

  const side = $derived<SwitchTarget | null>(
    chat.phase || chat.side === "switching" ? null : chat.side,
  );
  // The side before the current switch: chat unless it started on the TUI.
  let from = $state<SwitchTarget>(
    untrack(() => (scenario === "awaiting-choice" ? "tui" : "acp")),
  );
  const view = $derived<HybridView>(
    side === null ? "switching" : side === "tui" ? "terminal" : "chat",
  );
  const target = $derived(
    view === "switching" ? switchTarget(chat.phase, from) : null,
  );
  const chatUnavailable = $derived(
    side === "tui" ? eligibilityText(chat.eligibility) : "",
  );
  const info = $derived<SessionInfo>({
    ...session,
    title:
      model.title ?? (agent === "omp" ? "omp · webmux" : "OpenCode · webmux"),
    cwd: "/home/developer/webmux",
    cmd: [agent],
    status: chat.requests.length
      ? "waiting"
      : chat.turn.running
        ? "running"
        : "idle",
    attention: chat.requests.length
      ? { kind: "permission", source: "agent", since: session.lastActiveAt }
      : null,
    preview: "",
    clients: 1,
    reportedCwd: null,
    hybrid: {
      side: chat.side,
      phase: chat.phase,
      dormant: false,
      state: "active",
      agent,
      agentSessionId: chat.agentSessionId,
    },
  });
  const notices = $derived.by(() => {
    const list: HybridNotice[] = [];
    if (notice)
      list.push({ key: `notice:${notice}`, tone: "info", text: notice });
    const change = chat.idChange;
    if (change && side === "tui")
      list.push({
        key: `id:${change.from}>${change.to}`,
        tone: "info",
        text: `TUI 已切换到新会话 ${shortId(change.to)}（上一个：${shortId(change.from)}），切到聊天时接手新会话`,
      });
    return list.filter((item) => !dismissed.includes(item.key));
  });

  function update(patch: Partial<ChatState>) {
    chat = { ...chat, ...patch };
  }
  function interrupt() {
    model = withEvents(model, [
      {
        kind: "turn.end",
        turnEnd: { turnId: `turn-${model.next}`, stopReason: "cancelled" },
      },
    ]);
    update({
      turn: { running: false, stopReason: "cancelled" },
      requests: [],
      queue: { ...chat.queue, paused: chat.queue.items.length > 0 },
    });
  }
  const done = () => Promise.resolve();
  const actions: ChatActions = {
    send(text, _mode, sent) {
      if (chat.turn.running || chat.queue.paused) {
        const item = {
          id: `q${model.next}`,
          text,
          createdAt: session.lastActiveAt,
          ...(sent?.length ? { attachments: sent } : {}),
        };
        update({
          queue: { ...chat.queue, items: [...chat.queue.items, item] },
        });
      } else {
        model = withEvents(model, [
          {
            kind: "message.append",
            message: {
              messageId: `u${model.next}`,
              role: "user",
              text,
              ...(sent?.length ? { attachments: sent } : {}),
            },
          },
        ]);
        update({ turn: { running: true } });
      }
      return done();
    },
    cancel() {
      interrupt();
      return done();
    },
    editQueued: done,
    moveQueuedUp: done,
    removeQueued(itemId) {
      const items = chat.queue.items.filter((item) => item.id !== itemId);
      update({ queue: { paused: chat.queue.paused && !!items.length, items } });
      return done();
    },
    sendQueuedNow: done,
    resumeQueue() {
      update({
        queue: { ...chat.queue, paused: false },
        turn: { running: true },
      });
      return done();
    },
    clearQueue() {
      update({ queue: { paused: false, items: [] } });
      return done();
    },
    answer(requestId, answer) {
      model = withEvents(model, [
        {
          kind: "request.close",
          resolved: {
            requestId,
            outcome: answer.outcome,
            ...("optionId" in answer ? { optionId: answer.optionId } : {}),
          },
        },
      ]);
      update({
        requests: chat.requests.filter((r) => r.requestId !== requestId),
      });
      return done();
    },
    setConfig(configId, value) {
      update({
        config: chat.config.map((option) =>
          option.id === configId ? { ...option, ...value } : option,
        ),
      });
      return done();
    },
    setPermissionMode(mode) {
      update({ permissionMode: mode });
      return done();
    },
  };

  // The phases a switch passes through, as the server reports them.
  const phases: Record<SwitchTarget, HybridPhase[]> = {
    acp: ["interrupting", "stopping", "starting", "loading"],
    tui: ["cancelling", "stopping", "starting"],
  };
  function advance(to: SwitchTarget, rest: HybridPhase[]) {
    const [phase, ...next] = rest;
    if (!phase) {
      model = withEvents(model, [
        { kind: "marker", marker: { kind: "switched", side: to } },
      ]);
      update({ side: to, phase: undefined });
      return;
    }
    update({ side: "switching", phase });
    timer = setTimeout(() => advance(to, next), 700);
  }
  function run(to: SwitchTarget) {
    if (to === "tui" && chat.turn.running) interrupt();
    from = to === "tui" ? "acp" : "tui";
    advance(to, phases[to]);
  }
  const hybrid = {
    get view() {
      return view;
    },
    get side() {
      return side;
    },
    get target() {
      return target;
    },
    get chatUnavailable() {
      return chatUnavailable;
    },
    request(to: SwitchTarget) {
      if (!side || to === side) return;
      notice = "";
      if (to === "acp" && chatUnavailable) {
        notice = `现在不能切到聊天：${chatUnavailable}`;
        return;
      }
      if (to === "tui" && chat.turn.running) confirming = to;
      else run(to);
    },
  };
  const terminal = {
    get info() {
      return info;
    },
    connection: "connected" as const,
    exitCode: null,
    restoring: false,
    historyTruncated: false,
    canFind: true,
    scrollToBottom() {},
    clear() {},
    resync() {},
  };
  const noticeTerminal = {
    control: undefined,
    controlling: true,
    connection: "connected" as const,
    exitCode: null,
    error: "",
    claim() {},
  };
  const fullscreen = {
    offered: true,
    active: false,
    busy: false,
    toggle: async () => {},
  };
</script>

{#snippet quickMenu(side: "top" | "bottom")}
  <QuickCommandMenu
    commands={[]}
    target="terminal"
    unavailable=""
    disabled={false}
    {side}
    run={() => {}}
    manage={() => {}}
    onCloseAutoFocus={() => {}}
  />
{/snippet}

<main class="terminal-page">
  <TerminalHeader
    {terminal}
    {fullscreen}
    {draft}
    draftSaved="已保存在本机"
    scratchSaved="已保存在本机"
    uploading={false}
    desktop={desktopMedia.current}
    sizeLocked={false}
    setSizeLocked={() => {}}
    bind:codeShown
    forceSoftKeys={false}
    {quickMenu}
    upload={async () => {}}
    toggleDraft={() => {}}
    copy={async () => {}}
    paste={async () => {}}
    showScratch={() => {}}
    showFontSize={() => {}}
    showFind={() => {}}
    close={() => {}}
    {hybrid}
  />
  <TerminalNotices
    terminal={noticeTerminal}
    uploads={{ progress: null, cancel() {} }}
    {draft}
    draftSaved="已保存在本机"
    error=""
    notice=""
    showScratch={() => {}}
    dismissScratch={() => {}}
    remove={() => {}}
    {notices}
    dismiss={(key) => {
      if (key.startsWith("notice:")) notice = "";
      else dismissed = [...dismissed, key];
    }}
  />
  <TerminalWorkspace
    {codeShown}
    desktop={desktopMedia.current}
    draftOpen={false}
    onfind={() => {}}
  >
    {#snippet terminalPane()}
      {#if view === "chat"}
        <ChatView
          {model}
          {chat}
          connection="connected"
          {draft}
          {drafts}
          uploads={chatUploads}
          {actions}
          onreview={() => (codeShown = true)}
          onlogin={() => hybrid.request("tui")}
        />
      {:else if view === "switching"}
        <SwitchProgress
          {target}
          steps={chat.phase ? switchSteps(target, chat.phase) : []}
          awaitingChoice={chat.phase === "awaiting_choice"}
          onchoose={() => (choiceOpen = true)}
        />
      {:else}
        <!-- Same structure as TerminalViewport: xterm area plus the soft-key
             bar that touch devices show. -->
        <div class="terminal-column">
          <div class="terminal-area">
            <pre
              class="absolute inset-0 overflow-auto p-2 font-mono text-xs leading-relaxed break-all whitespace-pre-wrap text-(--terminal-foreground)"
              aria-label="终端（示意）">{terminalLines}</pre>
          </div>
          <SoftKeys
            send={() => {}}
            modifiers={{ ctrl: false, alt: false }}
            toggle={() => {}}
            paste={() => {}}
            keyboard={() => {}}
          >
            {#snippet trailing()}{@render quickMenu("top")}{/snippet}
          </SoftKeys>
        </div>
      {/if}
    {/snippet}
    {#snippet draftPane()}{/snippet}
    {#snippet codePane()}
      <div class="h-full overflow-y-auto bg-card p-2">
        <CodeChangesList
          entries={codeChanges}
          total={codeChanges.length}
          nextCursor=""
          version="story"
          pending={false}
          enabled
          onmore={() => {}}
          onopen={() => {}}
        />
      </div>
    {/snippet}
  </TerminalWorkspace>
</main>

<ConfirmDialog
  bind:open={
    () => !!confirming,
    (open) => {
      if (!open) confirming = null;
    }
  }
  title="切换到终端？"
  onconfirm={() => {
    confirming = null;
    run("tui");
  }}>当前回合会被中断，已输出的内容会保留。</ConfirmDialog
>
<SwitchChoiceDialog
  bind:open={choiceOpen}
  {agent}
  busy={false}
  onchoose={(choice) => {
    choiceOpen = false;
    if (choice === "cancel") update({ side: "tui", phase: undefined });
    else advance("acp", phases.acp.slice(1));
  }}
/>
