<script lang="ts">
  import { onMount, onDestroy, tick, untrack } from "svelte";
  import { api, navigate, store, stored } from "../api";
  import {
    CreateSessionRequestSchema,
    SessionInfoSchema,
    type SessionInfo,
  } from "../schemas";
  import { errorMessage } from "../error";
  import { isIOS } from "../touchInput";
  import { setBrowserChrome, resetBrowserChrome } from "../browser-chrome";
  import TerminalSplitMenu from "../components/TerminalSplitMenu.svelte";
  import InlineNotice from "../components/InlineNotice.svelte";
  import { hasTouchInput } from "../touchInput";
  import { useViewport } from "../hooks/viewport.svelte";
  import type { TerminalDrafts } from "../terminal-draft.svelte";
  import { Button } from "$lib/components/ui/button";
  import SessionTerminal from "../components/SessionTerminal.svelte";
  import SplitSessionDialog from "../components/SplitSessionDialog.svelte";
  import {
    MAX_PANES,
    layoutGeometry,
    paneIds,
    removePane,
    resizeSplit,
    restoreLayout,
    splitPane,
    type Divider,
    type SplitDirection,
  } from "../terminal-layout";

  let {
    id,
    drafts,
    onUploadNotice,
  }: {
    id: string;
    drafts: TerminalDrafts;
    onUploadNotice: (message: string) => void;
  } = $props();
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
  let width = $state(0);
  let height = $state(0);
  let workspace: HTMLDivElement;
  let mounted = false;
  const terminals: Record<string, SessionTerminal | undefined> = {};
  const viewport = useViewport();
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
  const ids = $derived(paneIds(layout));
  const single = $derived(zoomed || width < 768);
  const geometry = $derived(layoutGeometry(layout, width, height));
  $effect(() => store(key, JSON.stringify(layout)));
  $effect(() => store(`${key}.active`, active));
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
  function close(sessionId: string) {
    const next = removePane(layout, sessionId);
    delete terminals[sessionId];
    delete info[sessionId];
    if (!next) {
      navigate("/");
      return;
    }
    const previous = ids.indexOf(sessionId);
    layout = next;
    if (active === sessionId)
      void focus(paneIds(next)[Math.max(0, previous - 1)]!);
    if (paneIds(next).length === 1) zoomed = false;
  }
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
    if (busy || ids.length >= MAX_PANES) return;
    const source = info[target];
    if (!source) return;
    active = target;
    busy = true;
    error = "";
    try {
      const session = await api(
        "/sessions",
        SessionInfoSchema,
        "POST",
        {
          cmd: ["/bin/sh", "-i"],
          ...(source.workspace
            ? {
                workspace: {
                  repositoryId: source.workspace.repositoryId,
                  worktreeId: source.workspace.worktreeId,
                },
              }
            : { cwd: source.cwd }),
        },
        undefined,
        CreateSessionRequestSchema,
      );
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
    if (
      event.isComposing ||
      event.keyCode === 229 ||
      !(event.target instanceof Element) ||
      !event.target.closest(".terminal-host")
    )
      return;
    // Shift is pressed before % and "; it must not consume the prefix.
    if (["Shift", "Control", "Alt", "Meta"].includes(event.key)) return;
    const isPrefix =
      event.ctrlKey &&
      !event.altKey &&
      !event.metaKey &&
      event.key.toLowerCase() === "b";
    if (!prefix && !isPrefix) return;
    event.preventDefault();
    event.stopImmediatePropagation();
    if (!prefix) {
      prefix = true;
      return;
    }
    prefix = false;
    if (isPrefix) terminals[active]?.send("\x02");
    else if (event.key === "%" || (event.shiftKey && event.code === "Digit5"))
      void split("horizontal", active);
    else if (event.key === '"' || (event.shiftKey && event.code === "Quote"))
      void split("vertical", active);
    else if (event.key === "o") cycle();
    else if (event.key === "z") zoom(active);
    else if (event.key === "x" && !busy) close(active);
    else if (event.key.startsWith("Arrow")) {
      const current = geometry.panes.find((pane) => pane.sessionId === active)!;
      const cx = current.x + current.width / 2;
      const cy = current.y + current.height / 2;
      const dx =
        event.key === "ArrowLeft" ? -1 : event.key === "ArrowRight" ? 1 : 0;
      const dy =
        event.key === "ArrowUp" ? -1 : event.key === "ArrowDown" ? 1 : 0;
      const candidates = geometry.panes
        .filter((pane) => pane.sessionId !== active)
        .map((pane) => {
          const x = pane.x + pane.width / 2 - cx;
          const y = pane.y + pane.height / 2 - cy;
          return { pane, forward: x * dx + y * dy, distance: Math.hypot(x, y) };
        })
        .filter((candidate) => candidate.forward > 0)
        .sort((a, b) => a.distance - b.distance);
      if (candidates[0]) void focus(candidates[0].pane.sessionId);
    }
  }
  function drag(event: PointerEvent) {
    if (event.button !== 0) return;
    const handle = event.currentTarget as HTMLElement;
    handle.setPointerCapture(event.pointerId);
    handle.focus();
    event.preventDefault();
  }
  function move(event: PointerEvent, divider: Divider) {
    if (
      !(event.currentTarget as HTMLElement).hasPointerCapture(event.pointerId)
    )
      return;
    const bounds = workspace.getBoundingClientRect();
    const horizontal = divider.direction === "horizontal";
    const position = horizontal
      ? event.clientX - bounds.x - divider.parent.x
      : event.clientY - bounds.y - divider.parent.y;
    const extent =
      (horizontal ? divider.parent.width : divider.parent.height) - 6;
    if (extent > 0)
      layout = resizeSplit(layout, divider.id, (position - 3) / extent);
  }
  function resizeKey(event: KeyboardEvent, divider: Divider) {
    const horizontal = divider.direction === "horizontal";
    const decrease = horizontal ? "ArrowLeft" : "ArrowUp";
    const increase = horizontal ? "ArrowRight" : "ArrowDown";
    if (![decrease, increase, "Home", "End", "Enter"].includes(event.key))
      return;
    event.preventDefault();
    layout = resizeSplit(
      layout,
      divider.id,
      event.key === "Home"
        ? 0.1
        : event.key === "End"
          ? 0.9
          : event.key === "Enter"
            ? 0.5
            : divider.ratio + (event.key === decrease ? -0.05 : 0.05),
    );
  }
</script>

<main
  class="terminal-page"
  class:ios={isIOS()}
  style:--viewport-height={`${viewport.height}px`}
  onkeydowncapture={keydown}
>
  {#if ids.length > 1}
    <nav class="pane-tabs" aria-label="终端窗格">
      {#each ids as sessionId, index (sessionId)}
        <Button
          variant={active === sessionId ? "secondary" : "ghost"}
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
            active={active === pane.sessionId}
            reopen={() => {
              revisions[pane.sessionId] = (revisions[pane.sessionId] ?? 0) + 1;
            }}
            onremoved={() => close(pane.sessionId)}
            oninfo={(value, connection) => {
              info[value.id] = value;
              connections[value.id] = connection;
            }}
          >
            {#snippet paneActions()}
              <TerminalSplitMenu
                {busy}
                limit={ids.length >= MAX_PANES}
                ready={!!info[pane.sessionId]}
                onsplit={(direction) => {
                  void split(direction, pane.sessionId);
                }}
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
        <!-- A focusable separator with a value is an interactive window splitter. -->
        <!-- svelte-ignore a11y_no_noninteractive_tabindex, a11y_no_noninteractive_element_interactions -->
        <div
          class="pane-divider"
          class:horizontal={divider.direction === "horizontal"}
          role="separator"
          tabindex="0"
          aria-label={divider.direction === "horizontal"
            ? "调整窗格宽度"
            : "调整窗格高度"}
          aria-orientation={divider.direction === "horizontal"
            ? "vertical"
            : "horizontal"}
          aria-valuemin={10}
          aria-valuemax={90}
          aria-valuenow={Math.round(divider.ratio * 100)}
          style:--pane-left={`${divider.x}px`}
          style:--pane-top={`${divider.y}px`}
          style:--pane-width={`${divider.width}px`}
          style:--pane-height={`${divider.height}px`}
          onpointerdown={drag}
          onpointermove={(event) => move(event, divider)}
          onpointerup={(event) =>
            (event.currentTarget as HTMLElement).releasePointerCapture(
              event.pointerId,
            )}
          onkeydown={(event) => resizeKey(event, divider)}
          ondblclick={() => {
            layout = resizeSplit(layout, divider.id, 0.5);
          }}
        ></div>
      {/each}
    {/if}
  </div>
</main>
{#if choosing}
  <SplitSessionDialog
    excluded={ids}
    onclose={() => {
      choosing = false;
    }}
    onselect={(sessionId, direction) => {
      void add(sessionId, direction);
    }}
  />
{/if}

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
  .pane-divider {
    position: absolute;
    left: var(--pane-left);
    top: var(--pane-top);
    width: var(--pane-width);
    height: var(--pane-height);
    cursor: row-resize;
    touch-action: none;
    background: var(--border);
    z-index: 2;
  }
  .pane-divider.horizontal {
    cursor: col-resize;
  }
  .pane-divider:hover,
  .pane-divider:focus-visible {
    background: var(--ring);
    outline: none;
  }
</style>
