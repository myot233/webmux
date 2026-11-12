<script lang="ts">
  import { errorMessage } from "../../platform/error";
  import { dragWidth, keyWidth } from "../../hooks/shared/splitter";
  import { onMount, untrack } from "svelte";
  import CodeNavigator from "./CodeNavigator.svelte";
  import CodeWorkspaceHeader from "./CodeWorkspaceHeader.svelte";
  import { api } from "../../platform/api";
  import {
    CodeFileSchema,
    CodeChangesSchema,
    CodeDiffSchema,
    CodeLocationSchema,
    type CodeChange,
    type CodeReveal,
    type CodeWorkspaceSnapshot,
  } from "../../contracts/schemas";
  import type { z } from "zod";
  import type { CodeHost } from "../../domains/code/code-host";
  import { nodeId } from "../../domains/code/code-reference";
  import {
    createMarkdownWorkspace,
    locateRevealTarget,
  } from "../../domains/code/code-reveal";
  import {
    shouldCloseOverlayNav,
    type CodeTabOrigin,
    type CodeWorkspaceTab,
  } from "../../domains/code/code-workspace";
  import { useCodeWorkspaceState } from "../../hooks/code/code-workspace-state.svelte";
  import * as Tabs from "$lib/components/ui/tabs";
  import CodeDocumentView from "./CodeDocumentView.svelte";
  import "../../fonts-terminal.css";

  let {
    id,
    visible,
    host,
    initial,
    popout,
  }: {
    id: string;
    visible: boolean;
    host: CodeHost;
    /** Reading state captured by another instance, such as another window. */
    initial?: CodeWorkspaceSnapshot;
    /** The pop-out toggle; absent where the panel cannot change windows. */
    popout?: { popped: boolean; toggle: () => void };
  } = $props();
  const seed = untrack(() => initial);
  const navMin = 120,
    navMax = 640;
  let section = $state<HTMLElement>();
  let inner = $state<HTMLDivElement>();
  let navigator = $state<CodeNavigator>();
  let dropStatus = $state("");
  const views = new WeakMap<CodeWorkspaceTab, CodeDocumentView>();
  const shown = $derived(visible || !!popout?.popped);
  let base = $derived(`/sessions/${id}/code`);
  const ws = useCodeWorkspaceState({
    id: () => id,
    base: () => base,
    shown: () => shown,
    seed,
    closeOverlayNav: () => {
      if (shouldCloseOverlayNav(section)) ws.s.navOpen = false;
    },
  });
  const activePath = $derived(
    ws.s.tabs.find((tab) => tab.key === ws.s.active)?.file?.document.path ?? "",
  );
  const activeMenu = $derived(
    ws.s.tabs.find((tab) => tab.key === ws.s.active)?.menu,
  );

  /** The reading state of this workspace, for another instance to restore. */
  export function capture(): CodeWorkspaceSnapshot {
    return ws.snapshotState(
      (tab) => views.get(tab)?.capture() ?? $state.snapshot(tab.restore),
      () => navigator?.capture() ?? seed?.navigator,
    );
  }
  function setNavWidth(width: number) {
    const limit = Math.min(navMax, (inner?.clientWidth ?? navMax) - 160);
    ws.s.navWidth = Math.round(Math.max(navMin, Math.min(limit, width)));
  }
  function resizeStart(event: PointerEvent) {
    dragWidth(event, ws.s.navWidth, setNavWidth);
  }
  function resizeKey(event: KeyboardEvent) {
    keyWidth(event, ws.s.navWidth, { min: navMin, max: navMax }, setNavWidth);
  }
  async function locate(input: string) {
    if (!ws.s.context) return;
    await show(
      await api(
        `${base}/locate?${ws.params({ path: input })}`,
        CodeLocationSchema,
      ),
    );
  }
  async function show(target: z.infer<typeof CodeLocationSchema>) {
    if (target.kind === "file") return openFile(target.path, target.nodeId);
    ws.s.nav = "files";
    ws.s.navOpen = true;
    ws.s.directory = "";
    ws.s.query = "";
    await ws.load();
    ws.s.directory = target.path;
    return "";
  }
  async function openFileInOrigin(
    origin: CodeTabOrigin,
    path: string,
    opaqueId = nodeId(path),
  ) {
    return ws.openTab(
      `${origin.contextId}:${opaqueId}:worktree`,
      `${base}/file?${ws.params({ contextId: origin.contextId, nodeId: opaqueId })}`,
      CodeFileSchema,
      (value, activate) => ws.install(origin, value, undefined, activate),
    );
  }
  async function openFile(path: string, opaqueId = nodeId(path)) {
    const origin = ws.s.context;
    if (!origin) return "";
    return openFileInOrigin(origin, path, opaqueId);
  }
  let headingSequence = 0;
  let jumpSequence = 0;
  const markdownWorkspace = createMarkdownWorkspace({
    id: () => id,
    base: () => base,
    getContext: () => ws.s.context,
    params: (extra) => ws.params(extra),
    show,
    getTabs: () => ws.s.tabs,
    nextHeadingKey: () => ++headingSequence,
  });
  async function openDiff(change: CodeChange) {
    const origin = ws.s.context;
    if (!origin) return;
    await ws.openTab(
      `${origin.contextId}:${change.nodeId}:diff:${change.group}`,
      `${base}/diff?${ws.params({ changeId: change.changeId })}`,
      CodeDiffSchema,
      (value, activate) => ws.install(origin, undefined, value, activate),
    );
  }
  onMount(() => ws.mountWorkspace());
  ws.syncConnectionCheck(() => host.connection);

  let revealed = untrack(() => (seed ? (host.reveal?.key ?? 0) : 0));
  $effect(() => {
    const target = host.reveal;
    const current = ws.s.context;
    if (!target || target.key === revealed) return;
    if (untrack(() => ws.s.root) !== "workspace") {
      untrack(() => ws.switchRoot("workspace"));
      return;
    }
    if (!current) return;
    revealed = target.key;
    untrack(() => void showReveal(target));
  });
  async function showReveal(target: CodeReveal) {
    const current = ws.s.context;
    if (!current) return;
    try {
      const { origin, location, usedRawPath } = await locateRevealTarget(
        base,
        (extra) => ws.params(extra),
        current,
        target,
      );
      if (location.kind !== "file") {
        if (origin.contextId === ws.s.context?.contextId) await show(location);
        return;
      }
      const mode =
        target.mode ??
        (target.line !== undefined || target.heading ? "file" : "diff");
      if (mode === "diff" && origin.diffAvailable) {
        const found = await api(
          `${base}/changes?${ws.params({ contextId: origin.contextId, nodeId: location.nodeId })}`,
          CodeChangesSchema,
        );
        const change =
          found.entries.find((entry) => entry.group !== "staged") ??
          found.entries[0];
        if (change) {
          await openDiff(change);
          return;
        }
      }
      const failure = await openFileInOrigin(
        origin,
        location.path,
        location.nodeId,
      );
      if (failure || mode !== "file") return;
      const tab = ws.s.tabs.find(
        (t) => t.key === `${origin.contextId}:${location.nodeId}:worktree`,
      );
      if (!tab) return;
      if (!usedRawPath && target.line !== undefined) {
        tab.jump = {
          line: target.line,
          endLine: target.endLine,
          column: target.column,
          key: ++jumpSequence,
        };
      } else if (!usedRawPath && target.heading) {
        tab.heading = { slug: target.heading, key: ++headingSequence };
      } else {
        tab.jump = { key: ++jumpSequence };
      }
    } catch (e) {
      if (ws.mounted) ws.s.error = errorMessage(e);
    }
  }
</script>

<Tabs.Root value={ws.s.active} onValueChange={ws.activate}>
  {#snippet child({ props })}
    <section
      {...props}
      bind:this={section}
      class="code-panel relative flex h-full min-h-0 min-w-0 flex-col overflow-hidden bg-background"
      aria-label="代码工作区"
    >
      <CodeWorkspaceHeader
        bind:navOpen={ws.s.navOpen}
        navStatus={ws.s.navStatus}
        tabs={ws.s.tabs}
        context={ws.s.context}
        pending={ws.s.pending}
        {popout}
        {activeMenu}
        {section}
        error={ws.s.error}
        {dropStatus}
        beginOpen={ws.beginOpen}
        supersedeOpenOnTabKey={ws.supersedeOpenOnTabKey}
        close={ws.close}
        refreshAll={() => {
          void ws.load();
          for (const tab of ws.s.tabs) void ws.refresh(tab);
        }}
      />
      <div
        class="code-inner flex min-h-0 flex-1 overflow-hidden"
        style:--code-nav-width={`${ws.s.navWidth}px`}
        bind:this={inner}
      >
        <div class="contents" class:hidden={!ws.s.navOpen}>
          <CodeNavigator
            bind:this={navigator}
            restore={seed?.navigator}
            dropHost={section}
            bind:dropStatus
            {id}
            context={ws.s.context}
            listing={ws.s.listing}
            rootListing={ws.s.rootListing}
            fetchDirectory={ws.fetchDirectory}
            treeRefresh={ws.s.treeRefresh}
            {activePath}
            changes={ws.s.changes}
            changesError={ws.s.changesError}
            pending={ws.s.pending}
            sessionCwd={host.sessionCwd}
            reportedCwd={host.reportedCwd}
            navStatus={ws.s.navStatus}
            root={ws.s.root}
            rootError={ws.s.rootError}
            switchRoot={ws.switchRoot}
            retryRoot={() => void ws.loadContext()}
            reload={() => void ws.load()}
            changed={ws.filesChanged}
            changesEnabled={shown && !ws.s.changesError && !ws.s.navStatus}
            bind:nav={ws.s.nav}
            bind:query={ws.s.query}
            bind:directory={ws.s.directory}
            bind:includeIgnored={ws.s.includeIgnored}
            bind:changeGroup={ws.s.changeGroup}
            bind:changeQuery={ws.s.changeQuery}
            bind:changeSearch={ws.s.changeSearch}
            load={(append) => ws.load(append)}
            openFile={(path, opaqueId) => void openFile(path, opaqueId)}
            {locate}
            openDiff={(change) => void openDiff(change)}
          />
          <!-- A focusable separator with aria-valuenow is an interactive widget (WAI-ARIA window splitter). -->
          <!-- svelte-ignore a11y_no_noninteractive_tabindex, a11y_no_noninteractive_element_interactions -->
          <div
            class="code-nav-divider"
            role="separator"
            aria-orientation="vertical"
            aria-label="调整导航宽度"
            aria-valuemin={navMin}
            aria-valuemax={navMax}
            aria-valuenow={ws.s.navWidth}
            tabindex="0"
            onpointerdown={resizeStart}
            onkeydown={resizeKey}
          ></div>
        </div>
        <div class="flex min-h-0 min-w-0 flex-1 flex-col overflow-hidden">
          {#each ws.s.tabs as tab (tab.key)}
            <Tabs.Content value={tab.key} tabindex={-1}>
              {#snippet child({ props })}
                <div {...props} class="min-h-0 flex-1 overflow-hidden">
                  <CodeDocumentView
                    bind:this={
                      () => views.get(tab),
                      (view) =>
                        view ? views.set(tab, view) : views.delete(tab)
                    }
                    restore={tab.restore}
                    {id}
                    active={shown && ws.s.active === tab.key}
                    file={tab.file}
                    diff={tab.diff}
                    status={tab.status}
                    paste={(text) => host.paste(text)}
                    draftEnabled={host.draftEnabled}
                    refresh={() => void ws.refresh(tab)}
                    contextId={tab.contextId}
                    comparison={tab.comparison}
                    onComparison={(value) => (tab.comparison = value)}
                    onState={(state) => (tab.status = state)}
                    beginOpen={ws.beginOpen}
                    openSide={(file, intent) =>
                      ws.install(
                        tab,
                        file,
                        undefined,
                        intent === ws.selectionSequence,
                      )}
                    bind:menu={tab.menu}
                    workspace={tab.contextId === ws.s.context?.contextId
                      ? markdownWorkspace
                      : undefined}
                    heading={tab.heading}
                    onHeading={() => (tab.heading = undefined)}
                    jump={tab.jump}
                    onJump={() => (tab.jump = undefined)}
                    diffAvailable={tab.rootKind !== "artifacts"}
                  />
                </div>
              {/snippet}
            </Tabs.Content>
          {/each}
          {#if !ws.s.tabs.length}<p class="p-4 text-sm text-muted-foreground">
              选择文件阅读完整内容，或从修改列表打开 Diff。
            </p>{/if}
        </div>
      </div>
    </section>
  {/snippet}
</Tabs.Root>
