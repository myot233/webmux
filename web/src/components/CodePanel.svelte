<script lang="ts">
  import { onMount } from "svelte";
  import CodeChangesList from "./CodeChangesList.svelte";
  import { api } from "../api";
  import {
    CodeContextSchema,
    CodeListingSchema,
    CodeFileSchema,
    CodeChangesSchema,
    CodeDiffSchema,
    CodeCheckSchema,
    type CodeFile,
    type CodeDiff,
    type CodeChange,
    type CodeListing,
  } from "../schemas";
  import type { z } from "zod";
  import { nodeId } from "../code-reference";
  import { Button } from "$lib/components/ui/button";
  import * as Tabs from "$lib/components/ui/tabs";
  import CodeDocumentView from "./CodeDocumentView.svelte";
  let {
    id,
    visible,
    connection,
    paste,
  }: {
    id: string;
    visible: boolean;
    connection: string;
    paste: (text: string) => boolean;
  } = $props();
  type Tab = {
    key: string;
    file?: CodeFile;
    diff?: CodeDiff;
    status: string;
    sequence: number;
    comparison?: CodeDiff;
  };
  let context = $state<z.infer<typeof CodeContextSchema>>();
  let listing = $state<CodeListing>();
  let changeQuery = $state("");
  let changeSearch = $state("");
  let changeGroup = $state("");
  let changesError = $state("");
  let changes = $state<z.infer<typeof CodeChangesSchema>>();
  let tabs = $state<Tab[]>([]);
  let active = $state("");
  let nav = $state("files");
  let navOpen = $state(true);
  let includeIgnored = $state(false);
  let directory = $state("");
  let query = $state("");
  let error = $state("");
  let navStatus = $state("");
  let pending = $state(false);
  let controller: AbortController | undefined;
  const controllers = new Map<string, AbortController>();
  let navSequence = 0;
  let mounted = false;
  let checkController: AbortController | undefined;
  let checking = false;
  let navIdentity = "";
  let base = $derived(`/sessions/${id}/code`);
  function params(extra: Record<string, string> = {}) {
    return new URLSearchParams({
      contextId: context?.contextId ?? "",
      includeIgnored: String(includeIgnored),
      ...extra,
    });
  }
  function aborted(e: unknown) {
    return e instanceof DOMException && e.name === "AbortError";
  }
  async function load(append = false) {
    if (!context || (append && pending)) return;
    controller?.abort();
    controller = new AbortController();
    const request = controller;
    const seq = ++navSequence;
    pending = true;
    error = "";
    try {
      if (nav === "changes") {
        if (!context.diffAvailable) {
          changes = undefined;
          return;
        }
        changesError = "";
        const cursor = append ? (changes?.nextCursor ?? "") : "";
        const value = await api(
          `${base}/changes?${params({ cursor, q: changeQuery, group: changeGroup })}`,
          CodeChangesSchema,
          "GET",
          undefined,
          request.signal,
        );
        if (mounted && seq === navSequence) {
          changes =
            append && changes
              ? { ...value, entries: [...changes.entries, ...value.entries] }
              : value;
        }
      } else {
        const cursor = append ? (listing?.nextCursor ?? "") : "";
        const value = await api(
          `${base}/${query ? "files" : "tree"}?${params({ nodeId: nodeId(directory), q: query, cursor })}`,
          CodeListingSchema,
          "GET",
          undefined,
          request.signal,
        );
        if (mounted && seq === navSequence) {
          listing =
            append && listing
              ? { ...value, children: [...listing.children, ...value.children] }
              : value;
          navIdentity = query ? "" : nodeId(directory);
        }
      }
      if (seq === navSequence) navStatus = "";
    } catch (e) {
      if (!aborted(e) && mounted && seq === navSequence) {
        error = (e as Error).message;
        if (nav === "changes") changesError = error;
      }
    } finally {
      if (mounted && seq === navSequence) pending = false;
    }
  }
  let selectionSequence = 0;
  const beginOpen = () => ++selectionSequence;
  function activate(key: string) {
    beginOpen();
    active = key;
  }
  function install(file?: CodeFile, diff?: CodeDiff, shouldActivate = true) {
    const d = file?.document ?? diff?.new;
    if (!context || !d) return;
    const key = `${context.contextId}:${d.nodeId ?? nodeId(d.path)}:${diff ? "diff:" + diff.change.group : d.source}`;
    const existing = tabs.find((t) => t.key === key);
    if (existing) {
      if (shouldActivate) active = key;
      if (
        file
          ? file.document.contentVersion !==
            existing.file?.document.contentVersion
          : diff?.new.contentVersion !== existing.diff?.new.contentVersion ||
            diff?.old.contentVersion !== existing.diff?.old.contentVersion
      )
        existing.status = "updated";
      return;
    }
    if (tabs.length >= 20) {
      error = "最多打开 20 个标签，请先关闭一个";
      return;
    }
    tabs.push({ key, file, diff, status: "unchanged", sequence: 0 });
    if (shouldActivate) {
      active = key;
      if (matchMedia("(max-width:1023px)").matches) navOpen = false;
    }
  }
  async function openFile(path: string, opaqueId = nodeId(path)) {
    if (!context) return;
    const intent = beginOpen();
    const key = `${context.contextId}:${opaqueId}:worktree`;
    if (tabs.some((t) => t.key === key)) {
      active = key;
      navOpen = false;
      void check();
      return;
    }
    await read(
      key,
      () =>
        api(
          `${base}/file?${params({ nodeId: opaqueId })}`,
          CodeFileSchema,
          "GET",
          undefined,
          controllers.get(key)?.signal,
        ),
      (value) => install(value, undefined, intent === selectionSequence),
    );
  }
  async function openDiff(change: CodeChange) {
    if (!context) return;
    const intent = beginOpen();
    const key = `${context.contextId}:${change.nodeId ?? nodeId(change.newPath)}:diff:${change.group}`;
    if (tabs.some((t) => t.key === key)) {
      active = key;
      navOpen = false;
      void check();
      return;
    }
    await read(
      key,
      () =>
        api(
          `${base}/diff?${params({ changeId: change.changeId })}`,
          CodeDiffSchema,
          "GET",
          undefined,
          controllers.get(key)?.signal,
        ),
      (value) => install(undefined, value, intent === selectionSequence),
    );
  }
  async function read<T>(
    key: string,
    request: () => Promise<T>,
    done: (value: T) => void,
  ) {
    controllers.get(key)?.abort();
    const c = new AbortController();
    controllers.set(key, c);
    error = "";
    try {
      const value = await request();
      if (mounted && !c.signal.aborted && controllers.get(key) === c)
        done(value);
    } catch (e) {
      if (mounted && !aborted(e)) error = (e as Error).message;
    } finally {
      if (controllers.get(key) === c) controllers.delete(key);
    }
  }
  function close(key: string) {
    controllers.get(key)?.abort();
    controllers.delete(key);
    tabs = tabs.filter((t) => t.key !== key);
    if (active === key) activate(tabs.at(-1)?.key ?? "");
  }
  async function refresh(tab: Tab) {
    const seq = ++tab.sequence;
    await read(
      tab.key,
      async () => {
        const signal = controllers.get(tab.key)?.signal;
        if (tab.diff) {
          const next = await api(
            `${base}/changes?${params({ nodeId: tab.diff.change.nodeId ?? nodeId(tab.diff.change.newPath) })}`,
            CodeChangesSchema,
            "GET",
            undefined,
            signal,
          );
          const c = next.entries.find(
            (c) =>
              c.group === tab.diff!.change.group &&
              c.newPath === tab.diff!.change.newPath,
          );
          if (!c) throw new Error("此差异已不存在，请从文件导航打开完整文件");
          return {
            diff: await api(
              `${base}/diff?${params({ changeId: c.changeId })}`,
              CodeDiffSchema,
              "GET",
              undefined,
              signal,
            ),
          };
        }
        if (!tab.file) throw new Error("文件不可用");
        return {
          file: await api(
            `${base}/file?${params({ nodeId: tab.file.document.nodeId ?? nodeId(tab.file.document.path), source: tab.file.document.source })}`,
            CodeFileSchema,
            "GET",
            undefined,
            signal,
          ),
        };
      },
      (value) => {
        if (!tabs.includes(tab) || tab.sequence !== seq) return;
        tab.file = value.file;
        tab.diff = value.diff;
        tab.status = "unchanged";
        tab.comparison = undefined;
      },
    );
  }
  async function check() {
    if (!mounted || !visible || document.hidden || !context || checking) return;
    checking = true;
    checkController = new AbortController();
    const c = checkController;
    const documents = tabs.map((t) => ({
      tab: t,
      sid: t.file?.snapshotId ?? t.diff!.snapshotId,
      sequence: t.sequence,
    }));
    const listingVersion = listing?.contentVersion;
    const changesVersion = changes?.contentVersion;
    const identity = navIdentity;
    try {
      const r = await api(
        `${base}/check`,
        CodeCheckSchema,
        "POST",
        {
          contextId: context.contextId,
          snapshotIds: [
            ...new Set(
              documents.flatMap((d) =>
                d.tab.comparison
                  ? [d.sid, d.tab.comparison.snapshotId]
                  : [d.sid],
              ),
            ),
          ],
          directories:
            listing && nav === "files" && !query
              ? { [identity]: listingVersion }
              : {},
          changesVersion: changesVersion ?? "",
          includeIgnored,
        },
        c.signal,
      );
      if (!mounted || c.signal.aborted) return;
      for (const d of documents) {
        if (
          tabs.includes(d.tab) &&
          d.tab.sequence === d.sequence &&
          (d.tab.file?.snapshotId ?? d.tab.diff?.snapshotId) === d.sid
        )
          d.tab.status =
            d.tab.comparison &&
            r.snapshots[d.tab.comparison.snapshotId] !== "unchanged"
              ? (r.snapshots[d.tab.comparison.snapshotId] ?? "unknown")
              : (r.snapshots[d.sid] ?? "unknown");
      }
      if (
        identity === navIdentity &&
        listingVersion === listing?.contentVersion &&
        Object.values(r.directories).some((s) => s !== "unchanged")
      )
        navStatus = "目录已更新或检测失败";
      if (
        changesVersion === changes?.contentVersion &&
        r.changes !== "unchanged"
      )
        navStatus = "修改列表已更新或检测失败";
    } catch (e) {
      if (!aborted(e) && mounted) {
        for (const d of documents) {
          if (tabs.includes(d.tab) && d.tab.sequence === d.sequence)
            d.tab.status = "unknown";
        }
        navStatus = "检测失败，状态未知";
      }
    } finally {
      checking = false;
    }
  }
  onMount(() => {
    mounted = true;
    const initial = new AbortController();
    void api(
      `${base}/context`,
      CodeContextSchema,
      "GET",
      undefined,
      initial.signal,
    )
      .then((c) => {
        if (mounted) {
          context = c;
          void load();
        }
      })
      .catch((e) => {
        if (mounted && !aborted(e)) error = (e as Error).message;
      });
    const timer = window.setInterval(() => void check(), 5000);
    const visibility = () => {
      if (document.hidden) checkController?.abort();
      else void check();
    };
    document.addEventListener("visibilitychange", visibility);
    return () => {
      mounted = false;
      initial.abort();
      controller?.abort();
      checkController?.abort();
      for (const c of controllers.values()) c.abort();
      clearInterval(timer);
      document.removeEventListener("visibilitychange", visibility);
    };
  });
  $effect(() => {
    const show = visible;
    const state = connection;
    if (mounted) {
      if (show && state === "connected") void check();
      else checkController?.abort();
    }
  });
</script>

<section
  class="code-panel flex h-full min-h-0 min-w-0 flex-col overflow-hidden bg-background"
  aria-label="代码工作区"
>
  <header class="flex shrink-0 flex-wrap items-center gap-2 border-b p-2">
    <Button variant="outline" size="sm" onclick={() => (navOpen = !navOpen)}
      >{navOpen ? "收起导航" : "文件导航"}</Button
    >
    <span class="min-w-0 flex-1 truncate text-xs" title={context?.rootPath}
      >{context?.rootPath ?? "读取代码目录…"}</span
    >
    <Button
      variant="outline"
      size="sm"
      disabled={pending}
      onclick={() => {
        void load();
        for (const tab of tabs) void refresh(tab);
      }}>刷新全部</Button
    >
  </header>
  {#if error}<p
      role="alert"
      class="shrink-0 break-all p-2 text-sm text-destructive"
    >
      {error}
    </p>{/if}
  {#if context?.reason}<p class="shrink-0 p-2 text-xs text-muted-foreground">
      {context.reason}
    </p>{/if}
  {#if navStatus}<p role="status" class="shrink-0 p-2 text-xs">
      {navStatus}
      <Button variant="outline" size="sm" onclick={() => void load()}
        >刷新导航</Button
      >
    </p>{/if}
  <div class="code-inner flex min-h-0 flex-1 overflow-hidden">
    {#if navOpen}<nav
        class="code-navigation flex min-h-0 shrink-0 flex-col overflow-hidden border-r"
        aria-label="文件与修改导航"
      >
        <Tabs.Root
          value={nav}
          onValueChange={(v) => {
            nav = v;
            void load();
          }}
        >
          <Tabs.List class="w-full"
            ><Tabs.Trigger value="files">文件</Tabs.Trigger><Tabs.Trigger
              value="changes">修改</Tabs.Trigger
            ></Tabs.List
          >
        </Tabs.Root>
        {#if nav === "files"}
          <form
            class="flex flex-col gap-1 p-2"
            onsubmit={(e) => {
              e.preventDefault();
              void load();
            }}
          >
            <input
              aria-label="搜索文件路径"
              placeholder="文件名或路径"
              bind:value={query}
              class="min-w-0 rounded border bg-background p-1 text-sm"
            />
            <Button size="sm" variant="outline" type="submit">搜索路径</Button>
            <label class="text-xs"
              ><input
                type="checkbox"
                bind:checked={includeIgnored}
                disabled={!context?.ignoreAvailable}
                onchange={() => void load()}
              /> 显示忽略项</label
            >
          </form>
          <div class="flex flex-wrap gap-1 p-2 text-xs" aria-label="路径导航">
            <button
              onclick={() => {
                directory = "";
                query = "";
                void load();
              }}>根目录</button
            >
            {#each directory.split("/").filter(Boolean) as part, i}<span>/</span
              ><button
                class="break-all"
                onclick={() => {
                  directory = directory
                    .split("/")
                    .slice(0, i + 1)
                    .join("/");
                  query = "";
                  void load();
                }}>{part}</button
              >{/each}
          </div>
          <div class="min-h-0 flex-1 overflow-auto p-1">
            {#each listing?.children ?? [] as node (node.nodeId)}
              <button
                class="block w-full rounded px-2 py-1 text-left text-xs hover:bg-muted"
                title={node.path}
                disabled={node.kind === "special" ||
                  node.kind === "boundary" ||
                  node.kind === "symlink"}
                onclick={() => {
                  if (node.kind === "directory") {
                    directory = node.path;
                    query = "";
                    void load();
                  } else void openFile(node.path, node.nodeId);
                }}
              >
                <span class="break-all"
                  >{node.kind === "directory"
                    ? "▸ "
                    : node.kind === "boundary"
                      ? "边界 "
                      : ""}{query ? node.path : node.name}</span
                >
                {node.gitStatus}{node.ignored ? " · 忽略" : ""}{node.linkTarget
                  ? ` → ${node.linkTarget}`
                  : ""}
              </button>
            {/each}
            {#if listing?.nextCursor}<Button
                size="sm"
                variant="outline"
                onclick={() => void load(true)}>下一页</Button
              >{/if}
            {#if listing?.limited || listing?.complete === false}<p
                role="status"
                class="p-2 text-xs"
              >
                结果受限，请收窄路径；未列出全部文件。
              </p>{/if}
            {#if listing?.children.length === 0 && !pending}<p
                class="p-2 text-xs"
              >
                没有匹配项
              </p>{/if}
          </div>
        {:else}
          {#if !context?.diffAvailable}<p class="p-2 text-xs">Git 差异不可用</p>
          {:else}
            <label class="flex items-center gap-2 px-2 pt-2 text-xs"
              >来源
              <select
                aria-label="修改来源"
                class="min-w-0 flex-1 rounded border bg-background p-1"
                bind:value={changeGroup}
                onchange={() => void load()}
              >
                <option value="">全部来源</option><option value="unstaged"
                  >未暂存</option
                ><option value="staged">已暂存</option><option value="untracked"
                  >未跟踪</option
                >
              </select>
            </label>
            <form
              class="flex gap-1 p-2"
              onsubmit={(e) => {
                e.preventDefault();
                changeQuery = changeSearch;
                void load();
              }}
            >
              <input
                aria-label="筛选修改路径"
                class="min-w-0 flex-1 rounded border bg-background p-1 text-xs"
                bind:value={changeSearch}
                placeholder="筛选修改路径"
              />
              <Button type="submit" variant="outline" size="sm">筛选修改</Button
              >
            </form>
            {#if changesError}<Button
                variant="outline"
                size="sm"
                onclick={() => void load()}>刷新修改列表</Button
              >{/if}
            <CodeChangesList
              entries={changes?.entries ?? []}
              total={changes?.total ?? 0}
              nextCursor={changes?.nextCursor ?? ""}
              version={`${changes?.contentVersion ?? ""}:${changeQuery}:${changeGroup}`}
              group={changeGroup}
              {pending}
              enabled={visible && !changesError && !navStatus}
              onmore={() => void load(true)}
              onopen={(c) => void openDiff(c)}
            />
          {/if}
        {/if}
      </nav>{/if}
    <div class="flex min-h-0 min-w-0 flex-1 flex-col overflow-hidden">
      <div
        class="flex shrink-0 overflow-x-auto border-b"
        role="tablist"
        aria-label="文件标签"
      >
        {#each tabs as tab (tab.key)}<div
            class="flex shrink-0 items-center border-r"
          >
            <button
              role="tab"
              aria-selected={active === tab.key}
              class="max-w-52 truncate p-2 text-xs"
              onclick={() => activate(tab.key)}
              >{tab.file?.document.path ?? tab.diff?.new.path} · {tab.diff
                ? tab.diff.change.group
                : tab.file?.document.source}{tab.status !== "unchanged"
                ? " ●"
                : ""}</button
            >
            <button
              aria-label={`关闭 ${tab.file?.document.path ?? tab.diff?.new.path}`}
              class="px-2"
              onclick={() => close(tab.key)}>×</button
            >
          </div>{/each}
      </div>
      {#each tabs as tab (tab.key)}<div
          class="min-h-0 flex-1 overflow-hidden"
          class:hidden={active !== tab.key}
          role="tabpanel"
        >
          <CodeDocumentView
            {id}
            active={visible && active === tab.key}
            file={tab.file}
            diff={tab.diff}
            status={tab.status}
            {paste}
            refresh={() => void refresh(tab)}
            contextId={context?.contextId ?? ""}
            comparison={tab.comparison}
            onComparison={(value) => (tab.comparison = value)}
            onState={(state) => (tab.status = state)}
            {beginOpen}
            openSide={(file, intent) =>
              install(file, undefined, intent === selectionSequence)}
          />
        </div>{/each}
      {#if !tabs.length}<p class="p-4 text-sm text-muted-foreground">
          选择文件阅读完整内容，或从修改列表打开 Diff。
        </p>{/if}
    </div>
  </div>
</section>
