<script lang="ts">
  import { findShell, shellUi } from "../hooks/shared/shell.svelte";
  import { onMount, onDestroy, tick, untrack } from "svelte";
  import { navigate, store, stored } from "../platform/api";
  import type { SessionInfo } from "../contracts/schemas";
  import { errorMessage } from "../platform/error";
  import { isIOS, hasTouchInput } from "../platform/touch-input";
  import { chatDocumentScroll, isStandalone } from "../platform/capabilities";
  import {
    setBrowserChrome,
    resetBrowserChrome,
  } from "../platform/browser-chrome";
  import TerminalSplitMenu from "../components/terminal/TerminalSplitMenu.svelte";
  import TerminalPaneDivider from "../components/terminal/TerminalPaneDivider.svelte";
  import TerminalPageDialogs from "../components/terminal/TerminalPageDialogs.svelte";
  import InlineNotice from "../components/shared/InlineNotice.svelte";
  import { useViewport } from "../hooks/shared/viewport.svelte";
  import type { TerminalDrafts } from "../domains/terminal/terminal-draft.svelte";
  import { Button } from "$lib/components/ui/button";
  import SessionTerminal from "../components/terminal/SessionTerminal.svelte";
  import {
    MAX_PANES,
    createSplitShellSession,
    directionalPane,
    layoutGeometry,
    paneIds,
    parsePrefixKeydown,
    removePane,
    resizeSplit,
    restoreLayout,
    splitPane,
    type SplitDirection,
  } from "../domains/terminal/terminal-layout";
  import {
    connectSessions,
    isPromptedPort,
  } from "../domains/workspace/forwards.svelte";
  import { applySessionEvent } from "../hooks/sessions/session-list";

  let {
    id,
    drafts,
    onUploadNotice,
  }: {
    id: string;
    drafts: TerminalDrafts;
    onUploadNotice: (message: string) => void;
  } = $props();
  const shell = findShell();
  const key = untrack(() => `layout.${id}`);
  let layout = $state(untrack(() => restoreLayout(stored(key), id)));
  let active = $state(
    untrack(() => {
      const saved = stored(`${key}.active`);
      return paneIds(layout).includes(saved) ? saved : paneIds(layout)[0]!;
    }),
  );
  let info = $state<Record<string, SessionInfo>>({});
  let connections = $state<Record<string, string>>({});
  let revisions = $state<Record<string, number>>({});
  let zoomed = $state(false);
  let prefix = $state(false);
  let busy = $state(false);
  let error = $state("");
  let choosing = $state(false);
  let creating = $state<SessionInfo | null>(null);
  const splitBusy = $derived(busy || creating !== null);
  function createSession(target: string) {
    if (splitBusy || ids.length >= MAX_PANES || !info[target]) return;
    active = target;
    creating = info[target]!;
  }
  let width = $state(0);
  let height = $state(0);
  let workspace: HTMLDivElement;
  let mounted = false;
  const terminals: Record<string, SessionTerminal | undefined> = {};
  let scrollStates = $state<
    Record<
      string,
      {
        chatView: boolean;
        desktop: boolean;
        codeShown: boolean;
        editing: boolean;
      }
    >
  >({});
  const ids = $derived(paneIds(layout));
  const documentScroll = $derived.by(() => {
    const firstId = ids[0];
    const current = firstId ? scrollStates[firstId] : undefined;
    if (!current) return false;
    return chatDocumentScroll({
      ...current,
      singlePane: ids.length === 1,
      standalone: isStandalone(),
    });
  });
  const viewport = useViewport(() => !documentScroll);
  const keyboardDismissed = () => hasTouchInput() && viewport.keyboardDismissed;
  onDestroy(resetBrowserChrome);
  $effect(() => {
    const current = info[active];
    if (current)
      setBrowserChrome(
        `${current.title} · WebMux`,
        current.status === "running" && connections[active] === "connected",
      );
  });
  const single = $derived(zoomed || width < 768);
  const geometry = $derived(layoutGeometry(layout, width, height));
  $effect(() => store(key, JSON.stringify(layout)));
  $effect(() => store(`${key}.active`, active));
  let forwarding = $state(false);
  // Every session, for the forwarding dialog's detected ports.
  let feedSessions = $state.raw<SessionInfo[]>([]);
  const listening = $derived(
    ids.some((sessionId) =>
      info[sessionId]?.listeningPorts.some((entry) =>
        isPromptedPort(entry.port),
      ),
    ),
  );
  // The session-list connection carries the forward list and live session
  // updates for panes on the chat view (whose terminal stream is detached).
  // Otherwise this page opens it only while a pane listens on a port or the
  // forwarding dialog is open.
  const onChatSide = $derived(
    ids.some(
      (sessionId) =>
        info[sessionId]?.views.includes("chat") &&
        info[sessionId]?.view !== "terminal",
    ),
  );
  const feedById = $derived(
    new Map(feedSessions.map((session) => [session.id, session])),
  );
  $effect(() => {
    if (!listening && !forwarding && !onChatSide) return;
    const socket = connectSessions(
      (event) => {
        feedSessions = applySessionEvent(feedSessions, event);
      },
      () => {},
      (message) => {
        error = message;
      },
    );
    return () => socket.close();
  });
  onMount(() => {
    mounted = true;
    return () => {
      mounted = false;
    };
  });

  async function focus(sessionId: string, keyboard = true) {
    active = sessionId;
    prefix = false;
    await tick();
    if (mounted && keyboard && !hasTouchInput()) terminals[sessionId]?.focus();
  }
  // Removes a pane; the last one returns home.
  export function activateSession(sessionId: string) {
    if (!ids.includes(sessionId)) return false;
    void focus(sessionId);
    return true;
  }

  function close(sessionId: string) {
    const next = removePane(layout, sessionId);
    delete terminals[sessionId];
    delete info[sessionId];
    delete scrollStates[sessionId];
    if (!next) {
      navigate(shellUi.home);
      return;
    }
    const previous = ids.indexOf(sessionId);
    layout = next;
    if (active === sessionId)
      void focus(paneIds(next)[Math.max(0, previous - 1)]!);
    if (paneIds(next).length === 1) zoomed = false;
  }
  // A session archived from the shell (sidebar or list) while open here:
  // leave it like the pane's own stop does, rather than offering to resume
  // what the user just ended.
  $effect(() =>
    shell?.archiveSignal.subscribe((archivedId) => {
      if (ids.includes(archivedId)) close(archivedId);
    }),
  );
  async function add(
    sessionId: string,
    direction: SplitDirection,
    target = active,
  ) {
    if (
      !mounted ||
      !ids.includes(target) ||
      ids.includes(sessionId) ||
      ids.length >= MAX_PANES
    )
      return;
    layout = splitPane(layout, target, sessionId, direction);
    zoomed = false;
    choosing = false;
    await focus(sessionId);
  }
  async function split(direction: SplitDirection, target: string) {
    if (splitBusy || ids.length >= MAX_PANES) return;
    const source = info[target];
    if (!source) return;
    active = target;
    busy = true;
    error = "";
    try {
      const session = await createSplitShellSession(source);
      if (mounted) {
        info[session.id] = session;
        await add(session.id, direction, target);
      }
    } catch (err) {
      if (mounted) error = `分屏失败：${errorMessage(err)}`;
    } finally {
      if (mounted) busy = false;
    }
  }
  function cycle() {
    void focus(ids[(ids.indexOf(active) + 1) % ids.length]!);
  }
  function zoom(sessionId: string) {
    active = sessionId;
    zoomed = !zoomed;
    void focus(sessionId);
  }
  function keydown(event: KeyboardEvent) {
    const cmd = parsePrefixKeydown(event, prefix);
    if (cmd.kind === "ignore") return;
    event.preventDefault();
    event.stopImmediatePropagation();
    if (cmd.kind === "enter-prefix") {
      prefix = true;
      return;
    }
    prefix = false;
    if (cmd.kind === "send-prefix") terminals[active]?.send("\x02");
    else if (cmd.kind === "split") void split(cmd.direction, active);
    else if (cmd.kind === "cycle") cycle();
    else if (cmd.kind === "zoom") zoom(active);
    else if (cmd.kind === "close" && !busy) close(active);
    else if (cmd.kind === "arrow") {
      const next = directionalPane(geometry.panes, active, cmd.key);
      if (next) void focus(next);
    }
  }
</script>

<main
  class="terminal-page"
  class:ios={isIOS()}
  data-document-scroll={documentScroll ? "true" : undefined}
  style:--viewport-height={`${viewport.height}px`}
  onkeydowncapture={keydown}
>
  {#if ids.length > 1}
    <nav class="pane-tabs" aria-label="终端窗格">
      {#each ids as sessionId, index (sessionId)}
        <Button
          variant="ghost"
          size="sm"
          aria-label={`切换到窗格 ${index + 1}：${info[sessionId]?.title || sessionId}`}
          aria-current={active === sessionId ? "true" : undefined}
          onclick={() => {
            void focus(sessionId);
          }}
        >
          <span class="truncate"
            >{index + 1} · {info[sessionId]?.title || sessionId}</span
          >
        </Button>
      {/each}
      <Button
        variant="ghost"
        size="sm"
        aria-pressed={zoomed}
        onclick={() => zoom(active)}>{zoomed ? "恢复分屏" : "放大窗格"}</Button
      >
    </nav>
  {/if}
  {#if prefix}<p class="split-notice prefix-notice" role="status">
      Ctrl+B · % 左右分屏 · " 上下分屏 · 方向键 / o 切换 · z 放大 · x 关闭窗格 ·
      Esc 取消
    </p>{/if}
  {#if error}<InlineNotice tone="danger"
      ><div class="flex items-center justify-between gap-2">
        <span>{error}</span><Button
          variant="ghost"
          size="sm"
          onclick={() => (error = "")}>关闭</Button
        >
      </div></InlineNotice
    >{/if}
  <div
    class="split-workspace"
    bind:this={workspace}
    bind:clientWidth={width}
    bind:clientHeight={height}
  >
    {#each geometry.panes as pane (pane.sessionId)}
      {@const visible = !single || active === pane.sessionId}
      {@const fillsWorkspace = single || ids.length === 1}
      <section
        class="split-terminal"
        class:active={ids.length > 1 && active === pane.sessionId}
        aria-label={`终端窗格 ${ids.indexOf(pane.sessionId) + 1}`}
        data-session-id={pane.sessionId}
        data-active={active === pane.sessionId}
        data-visible={visible}
        style:--pane-left={`${fillsWorkspace ? 0 : pane.x}px`}
        style:--pane-top={`${fillsWorkspace ? 0 : pane.y}px`}
        style:--pane-width={fillsWorkspace ? "100%" : `${pane.width}px`}
        style:--pane-height={fillsWorkspace ? "100%" : `${pane.height}px`}
        onpointerdowncapture={() => {
          active = pane.sessionId;
          prefix = false;
        }}
        onfocusin={() => {
          if (active !== pane.sessionId) {
            active = pane.sessionId;
            prefix = false;
          }
        }}
      >
        {#key `${pane.sessionId}:${revisions[pane.sessionId] ?? 0}`}
          <SessionTerminal
            bind:this={terminals[pane.sessionId]}
            id={pane.sessionId}
            {drafts}
            {onUploadNotice}
            {visible}
            {keyboardDismissed}
            keyboardVisibility={() => viewport.keyboard}
            viewportHeight={() => viewport.height}
            active={active === pane.sessionId}
            reopen={() => {
              revisions[pane.sessionId] = (revisions[pane.sessionId] ?? 0) + 1;
            }}
            onremoved={() => close(pane.sessionId)}
            onarchived={() => close(pane.sessionId)}
            onleave={ids.length > 1 ? () => close(pane.sessionId) : undefined}
            showForwards={() => (forwarding = true)}
            feedSession={feedById.get(pane.sessionId)}
            documentScroll={documentScroll && ids.length === 1}
            onscrollstate={(state) => {
              scrollStates[pane.sessionId] = state;
            }}
            oninfo={(value, connection) => {
              info[value.id] = value;
              connections[value.id] = connection;
            }}
          >
            {#snippet paneActions(inline: boolean)}
              <TerminalSplitMenu
                {inline}
                busy={splitBusy}
                limit={ids.length >= MAX_PANES}
                ready={!!info[pane.sessionId]}
                onsplit={(direction) => {
                  void split(direction, pane.sessionId);
                }}
                oncreate={() => createSession(pane.sessionId)}
                onchoose={() => {
                  active = pane.sessionId;
                  choosing = true;
                }}
                onclose={() => close(pane.sessionId)}
                onzoom={ids.length > 1 ? () => zoom(pane.sessionId) : undefined}
                {zoomed}
              />
            {/snippet}
          </SessionTerminal>
        {/key}
      </section>
    {/each}
    {#if !single}
      {#each geometry.dividers as divider (divider.id)}
        <TerminalPaneDivider
          {divider}
          getWorkspaceBounds={() => workspace.getBoundingClientRect()}
          onresize={(dividerId, ratio) => {
            layout = resizeSplit(layout, dividerId, ratio);
          }}
        />
      {/each}
    {/if}
  </div>
</main>
<TerminalPageDialogs
  bind:choosing
  bind:creating
  bind:forwarding
  excluded={ids}
  {feedSessions}
  onselect={(sessionId, direction) => {
    void add(sessionId, direction);
  }}
  oncreated={(session, direction, targetId) => {
    info[session.id] = session;
    void add(session.id, direction, targetId);
  }}
/>

<style>
  .split-workspace {
    position: relative;
    flex: 1;
    min-inline-size: 0;
    min-block-size: 0;
  }
  .split-terminal {
    position: absolute;
    left: var(--pane-left);
    top: var(--pane-top);
    width: var(--pane-width);
    height: var(--pane-height);
    min-inline-size: 0;
    min-block-size: 0;
    container: terminal-pane / inline-size;
  }
  .split-terminal[data-visible="false"] {
    display: none;
  }
  .split-terminal.active {
    outline: 1px solid var(--ring);
    outline-offset: -1px;
  }
  .pane-tabs {
    display: flex;
    flex-shrink: 0;
    gap: 2px;
    padding: 3px 6px;
    overflow-x: auto;
    border-bottom: 1px solid var(--border);
    background: var(--card);
  }
  .pane-tabs :global(button) {
    flex-shrink: 0;
    max-inline-size: 12rem;
  }
  .split-notice {
    display: flex;
    flex-shrink: 0;
    align-items: center;
    justify-content: space-between;
    padding: 4px 12px;
    font-size: 12px;
    background: var(--muted);
  }
  .prefix-notice {
    position: absolute;
    inset: auto 8px 8px;
    z-index: 10;
    border: 1px solid var(--border);
    border-radius: var(--radius);
    pointer-events: none;
  }
</style>
