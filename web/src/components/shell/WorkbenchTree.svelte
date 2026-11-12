<script lang="ts" module>
  export {
    workbenchTree,
    type TreeNode,
  } from "../../domains/workspace/workbench-tree";
</script>

<script lang="ts">
  import { tick } from "svelte";
  import { recordTitle } from "../../domains/sessions/archived";
  import type {
    ArchivedSession,
    HomeWorktrees,
    Repository,
    SessionInfo,
  } from "../../contracts/schemas";
  import {
    rowStatus,
    rowStatusLabels,
  } from "../../domains/sessions/session-status";
  import {
    defaultNodeOpen,
    readFolderMemory,
    reconcileFolderMemory,
    workbenchTree,
    type FolderMemory,
    type TreeNode,
  } from "../../domains/workspace/workbench-tree";
  import IconAlertCircle from "~icons/tabler/alert-circle";
  import IconBook2 from "~icons/tabler/book-2";
  import IconChevronRight from "~icons/tabler/chevron-right";
  import IconFolder from "~icons/tabler/folder";
  import IconFolderOff from "~icons/tabler/folder-off";
  import IconGitBranch from "~icons/tabler/git-branch";
  import IconLayoutDashboard from "~icons/tabler/layout-dashboard";
  import IconLoader2 from "~icons/tabler/loader-2";
  import { Button } from "$lib/components/ui/button";
  import { ListButton } from "$lib/components/ui/list-button";
  import { store, stored } from "../../platform/api";
  import type { BranchOperation } from "../../domains/workspace/branch-actions";
  import LaunchConfigIcon from "../settings/LaunchConfigIcon.svelte";
  import RowStatus from "../shared/RowStatus.svelte";
  import { underRowAction } from "../../hooks/shared/sidebar-row";
  import {
    switchableWorktree,
    worktreeUpstream,
  } from "../../domains/workspace/worktree-sync";
  import UpstreamStatus from "../workspace/UpstreamStatus.svelte";
  import SidebarRepositoryMenu from "./SidebarRepositoryMenu.svelte";
  import SidebarRowMenu from "./SidebarRowMenu.svelte";
  import SidebarSessionMenu from "./SidebarSessionMenu.svelte";
  import WorktreeMenuItems from "../workspace/WorktreeMenuItems.svelte";

  // The workbench-mode sidebar: repositories → worktrees → live sessions.
  // Archived records that fit no worktree (a removed or released one) sit
  // under the repository's "已不可用工作区" like the home page's, resuming
  // when clicked. One roving tab stop for the rows (arrow keys move, left
  // and right fold), and the focused row's action button is the next stop.
  // Collapsed nodes keep their counts and waiting numbers, which cover live
  // sessions only.
  let {
    repositories,
    sessions,
    records,
    resuming = "",
    home,
    scope,
    worktree,
    current,
    canCreate,
    configIcons,
    onscope,
    onopen,
    onnewworktree,
    onhomeworktrees,
    onmanageworktrees,
    onrepositorydetails,
    onworktreedetails,
    onremoveworktree,
    onswitchbranch,
    onbranchoperation,
    onrename,
    onclose,
    onresume,
    onremove,
  }: {
    repositories: readonly Repository[];
    /** Live sessions, as the home page lists them. */
    sessions: readonly SessionInfo[];
    /** Archived records; those fitting no worktree sit under their repository's unavailable workspaces. */
    records: readonly ArchivedSession[];
    /** The record being resumed; its row shows the progress. */
    resuming?: string;
    home: HomeWorktrees;
    /** The home view shown ("all", "direct" or a repository ID); "" elsewhere. */
    scope: string;
    /** The worktree the home view narrows to; "" for none. */
    worktree: string;
    /** The session page shown; "" elsewhere. */
    current: string;
    /** Git and the registry can create worktrees. */
    canCreate: boolean;
    /** Launch config icons by ID, for ordinary sessions. */
    configIcons: ReadonlyMap<string, string>;
    onscope: (scope: string, worktreeId?: string) => void;
    onopen: (session: SessionInfo) => void;
    onnewworktree: (repositoryId: string) => void;
    onhomeworktrees: (repositoryId: string) => void;
    onmanageworktrees: (repositoryId: string) => void;
    onrepositorydetails: (repositoryId: string) => void;
    /** Opens a worktree's details. */
    onworktreedetails: (repositoryId: string, worktreeId: string) => void;
    /** Opens the removal dialog of a linked worktree. */
    onremoveworktree: (repositoryId: string, worktreeId: string) => void;
    /** Opens the switch of a worktree's branch. */
    onswitchbranch: (repositoryId: string, worktreeId: string) => void;
    /** Starts a sync action of a worktree's branch. */
    onbranchoperation?: (
      repositoryId: string,
      operation: BranchOperation,
    ) => void;
    onrename: (session: SessionInfo) => void;
    onclose: (session: SessionInfo) => void;
    /** Resumes an archived record, rebuilding a released worktree first. */
    onresume: (record: ArchivedSession) => void;
    /** Asks to delete an archived record. */
    onremove: (record: ArchivedSession) => void;
  } = $props();

  const nodes = $derived(workbenchTree(repositories, sessions, home, records));
  let saved = $state<FolderMemory>(
    readFolderMemory(stored("workbenchTree", "{}")),
  );
  const effective = $derived(reconcileFolderMemory(saved, nodes));
  $effect(() => {
    const next = effective;
    if (next !== saved) {
      saved = next;
      store("workbenchTree", JSON.stringify(next));
    }
  });
  // The current session's repository and worktree open without being saved.
  const forced = $derived.by(() => {
    // The keys of the current session's ancestors.
    const find = (list: TreeNode[], path: string[]): string[] => {
      for (const node of list) {
        if (
          current &&
          (node.session?.id === current || node.record?.id === current)
        )
          return path;
        const found = find(node.children, [...path, node.key]);
        if (found.length) return found;
      }
      return [];
    };
    return find(nodes, []);
  });
  function isOpen(node: TreeNode) {
    if (!node.children.length || node.reason) return false;
    if (forced.includes(node.key)) return true;
    // Folders with reachable live sessions start open down to the session rows;
    // folders without live sessions and unavailable workspaces start closed.
    return effective.open[node.key] ?? defaultNodeOpen(node);
  }
  function setOpen(node: TreeNode, open: boolean) {
    saved = {
      open: { ...effective.open, [node.key]: open },
      sessions: effective.sessions,
    };
    store("workbenchTree", JSON.stringify(saved));
  }
  const visible = $derived.by(() => {
    const rows: TreeNode[] = [];
    const walk = (list: TreeNode[]) => {
      for (const node of list) {
        rows.push(node);
        if (isOpen(node)) walk(node.children);
      }
    };
    walk(nodes);
    return rows;
  });
  let focusKey = $state("all");
  const tabStop = $derived(
    visible.some((node) => node.key === focusKey)
      ? focusKey
      : (visible.find((node) => isCurrent(node))?.key ?? "all"),
  );
  let root = $state<HTMLElement>();
  async function focusRow(key: string) {
    focusKey = key;
    await tick();
    root
      ?.querySelector<HTMLElement>(`[data-tree-key="${CSS.escape(key)}"]`)
      ?.focus();
  }
  function keydown(event: KeyboardEvent, node: TreeNode) {
    const index = visible.findIndex((row) => row.key === node.key);
    let next: TreeNode | undefined;
    switch (event.key) {
      case "ArrowDown":
        next = visible[index + 1];
        break;
      case "ArrowUp":
        next = visible[index - 1];
        break;
      case "Home":
        next = visible[0];
        break;
      case "End":
        next = visible.at(-1);
        break;
      case "ArrowRight":
        if (node.children.length && !node.reason) {
          if (!isOpen(node)) setOpen(node, true);
          else next = node.children[0];
        }
        break;
      case "ArrowLeft":
        if (isOpen(node)) setOpen(node, false);
        else next = visible.find((row) => row.key === node.parent);
        break;
      default:
        return;
    }
    event.preventDefault();
    if (next) void focusRow(next.key);
  }
  function isCurrent(node: TreeNode) {
    if (node.kind === "session") return node.session?.id === current;
    if (node.kind === "archived") return node.record?.id === current;
    if (current || node.kind === "missing") return false;
    if (node.kind === "worktree")
      return scope === node.repositoryId && worktree === node.worktreeId;
    return node.kind === "repository"
      ? scope === node.repositoryId && !worktree
      : scope === node.key;
  }
  function activate(node: TreeNode) {
    focusKey = node.key;
    if (node.session) onopen(node.session);
    else if (node.record) onresume(node.record);
    else if (node.kind === "worktree" || node.kind === "missing")
      onscope(node.repositoryId ?? "all", node.worktreeId);
    else
      onscope(
        node.kind === "repository" ? (node.repositoryId ?? "all") : node.key,
      );
  }
  function sessionIcon(session: SessionInfo) {
    return (
      session.agent?.icon ??
      (session.configId ? configIcons.get(session.configId) : undefined) ??
      ""
    );
  }
  function recordTitleText(record: ArchivedSession) {
    return record.released
      ? `${recordTitle(record)}（${record.agent.name} · 已释放空间：${record.released.path} · ${record.released.commit.slice(0, 12)}，点击重建并恢复）`
      : `${recordTitle(record)}（${record.agent.name} · 已结束，点击恢复）`;
  }
</script>

{#snippet counts(node: TreeNode, actioned: boolean)}
  <!-- Right-aligned, so counts end where session rows' status dots do; a
       row's "⋯" menu takes their place on hover. -->
  {#if node.waiting}<span
      class={`w-12 shrink-0 truncate text-right text-xs text-warning ${actioned ? underRowAction : ""}`}
      title={`${node.waiting} 个会话在等待`}>{node.waiting} 等待</span
    >{:else if node.count}<span
      class={`w-12 shrink-0 text-right text-xs text-muted-foreground tabular-nums ${actioned ? underRowAction : ""}`}
      >{node.count}</span
    >{/if}
{/snippet}

{#snippet chevron(node: TreeNode)}
  {#if node.children.length && !node.reason}
    {@const label = `${isOpen(node) ? "收起" : "展开"}${node.label}`}
    <!-- Keyboard users fold with the arrow keys, so this is not a tab stop. -->
    <Button
      variant="quiet"
      size="icon-xs"
      class="size-6 shrink-0"
      data-tree-toggle
      tabindex={-1}
      aria-label={label}
      title={label}
      onclick={() => setOpen(node, !isOpen(node))}
      ><IconChevronRight
        class={`transition-transform motion-reduce:transition-none ${isOpen(node) ? "rotate-90" : ""}`}
        aria-hidden="true"
      /></Button
    >
  {:else}<span class="size-6 shrink-0"></span>{/if}
{/snippet}

{#snippet row(node: TreeNode)}
  {@const status = node.session
    ? rowStatus(node.session)
    : node.record
      ? "ended"
      : undefined}
  {@const stateText = node.record
    ? resuming === node.record.id
      ? "正在恢复"
      : node.record.released
        ? "已释放空间，已结束"
        : "已结束"
    : status
      ? rowStatusLabels[status]
      : undefined}
  {@const open = isOpen(node)}
  <li
    role="none"
    class={`flex items-center ${node.level === 3 ? "pl-8" : node.level === 2 ? "pl-4" : ""}`}
  >
    {#if node.kind !== "session"}{@render chevron(node)}{:else}<span
        class="size-6 shrink-0"
      ></span>{/if}
    <div class="group/row relative min-w-0 flex-1">
      <ListButton
        variant="file-tree-overlay"
        role="treeitem"
        aria-level={node.level}
        aria-expanded={node.children.length && !node.reason ? open : undefined}
        aria-selected={isCurrent(node)}
        aria-current={isCurrent(node) ? "page" : undefined}
        aria-label={stateText
          ? `${node.label}（${stateText}）`
          : node.reason
            ? `${node.label}（不可用）`
            : node.waiting
              ? `${node.label}，${node.count} 个会话，${node.waiting} 个等待`
              : node.count
                ? `${node.label}，${node.count} 个会话`
                : node.label}
        title={node.reason
          ? `${node.label}：${node.reason}`
          : node.record
            ? recordTitleText(node.record)
            : status
              ? `${node.label}（${rowStatusLabels[status]}）`
              : (node.path ?? node.label)}
        tabindex={tabStop === node.key ? 0 : -1}
        data-tree-key={node.key}
        onfocus={() => (focusKey = node.key)}
        onkeydown={(event: KeyboardEvent) => keydown(event, node)}
        aria-busy={node.record ? resuming === node.record.id : undefined}
        disabled={!!resuming && !!node.record}
        onclick={() => activate(node)}
      >
        {#if node.kind === "all"}<IconLayoutDashboard aria-hidden="true" />
        {:else if node.kind === "repository"}<IconBook2 aria-hidden="true" />
        {:else if node.kind === "direct"}<IconFolder aria-hidden="true" />
        {:else if node.kind === "worktree" || node.kind === "missing"}<IconGitBranch
            aria-hidden="true"
          />
        {:else if node.session}<LaunchConfigIcon
            icon={sessionIcon(node.session)}
            size="xs"
          />
        {:else if node.record}<LaunchConfigIcon
            icon={node.record.configId
              ? (configIcons.get(node.record.configId) ?? "")
              : ""}
            size="xs"
          />{/if}
        <span
          class={`min-w-0 flex-1 truncate ${node.kind === "repository" ? "font-medium" : ""} ${node.reason || status === "ended" ? "text-muted-foreground" : ""}`}
          >{node.label}</span
        >
        {#if node.worktree}
          {@const upstream = worktreeUpstream(node.worktree)}
          {#if upstream?.kind === "gone" || (upstream?.kind === "tracked" && (upstream.ahead || upstream.behind))}<span
              class="flex shrink-0 items-center gap-1"
              ><UpstreamStatus state={upstream} indicator /></span
            >{/if}
        {/if}
        {#if node.record?.released}<IconFolderOff
            class="text-muted-foreground"
            aria-hidden="true"
          />{/if}
        {#if node.reason}<IconAlertCircle
            class="text-warning"
            aria-hidden="true"
          />
        {:else if node.record && resuming === node.record.id}<span
            class="flex shrink-0 items-center gap-1 text-xs font-normal text-muted-foreground"
            ><IconLoader2
              class="size-3.5 animate-spin motion-reduce:animate-none"
              aria-hidden="true"
            />恢复中…</span
          >
        {:else if status}<RowStatus {status} />
        {:else}{@render counts(
            node,
            (node.kind === "repository" && !node.reason && canCreate) ||
              node.kind === "worktree",
          )}{/if}
      </ListButton>
      {#if node.kind === "repository" && canCreate}
        <SidebarRepositoryMenu
          unavailable={!!node.reason}
          label={node.label}
          tabindex={tabStop === node.key ? 0 : -1}
          onnewworktree={() => onnewworktree(node.repositoryId ?? "")}
          onhome={() => onhomeworktrees(node.repositoryId ?? "")}
          onmanage={() => onmanageworktrees(node.repositoryId ?? "")}
          ondetails={() => onrepositorydetails(node.repositoryId ?? "")}
        />
      {:else if node.kind === "worktree" && node.repositoryId && node.worktreeId}
        {@const repositoryId = node.repositoryId}
        {@const worktreeId = node.worktreeId}
        <SidebarRowMenu
          ariaLabel={`工作树操作：${node.label}`}
          tabindex={tabStop === node.key ? 0 : -1}
        >
          <WorktreeMenuItems
            ondetails={() => onworktreedetails(repositoryId, worktreeId)}
            onswitch={node.worktree && switchableWorktree(node.worktree)
              ? () => onswitchbranch(repositoryId, worktreeId)
              : undefined}
            onremove={node.removable
              ? () => onremoveworktree(repositoryId, worktreeId)
              : undefined}
            worktree={node.worktree}
            onoperation={onbranchoperation &&
              ((operation) => onbranchoperation(repositoryId, operation))}
          />
        </SidebarRowMenu>
      {:else if node.session}
        <SidebarSessionMenu
          label={node.label}
          tabindex={tabStop === node.key ? 0 : -1}
          target={{ kind: "live", session: node.session, onrename, onclose }}
        />
      {:else if node.record}
        {@const record = node.record}
        <SidebarSessionMenu
          label={node.label}
          tabindex={tabStop === node.key ? 0 : -1}
          disabled={!!resuming}
          target={{
            kind: "archived",
            onresume: () => onresume(record),
            onremove: () => onremove(record),
          }}
        />
      {/if}
    </div>
  </li>
  {#if open}
    <li role="none">
      <ul role="group" class="space-y-0.5">
        {#each node.children as child (child.key)}{@render row(child)}{/each}
      </ul>
    </li>
  {/if}
{/snippet}

<ul
  bind:this={root}
  role="tree"
  aria-label="仓库与会话"
  class="space-y-0.5 text-sm"
>
  {#each nodes as node (node.key)}{@render row(node)}{/each}
</ul>
