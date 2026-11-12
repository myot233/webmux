<script lang="ts">
  import { onDestroy } from "svelte";
  import { api } from "../../platform/api";
  import {
    LaunchConfigsSchema,
    RepositoriesSchema,
    type Repository,
    type SessionInfo,
  } from "../../contracts/schemas";
  import { errorMessage } from "../../platform/error";
  import type { LaunchConfigsState } from "../../domains/settings/launch-configs";
  import { homeWorktrees } from "../../domains/workspace/home-worktrees.svelte";
  import { setPinned } from "../../domains/workspace/home-worktrees";
  import type { SplitDirection } from "../../domains/terminal/terminal-layout";
  import SplitSessionDialog from "./SplitSessionDialog.svelte";
  import LazyDialog from "../shared/LazyDialog.svelte";

  let {
    choosing = $bindable(false),
    creating = $bindable<SessionInfo | null>(null),
    forwarding = $bindable(false),
    excluded,
    feedSessions,
    onselect,
    oncreated,
  }: {
    choosing: boolean;
    creating: SessionInfo | null;
    forwarding: boolean;
    excluded: string[];
    feedSessions: SessionInfo[];
    onselect: (sessionId: string, direction: SplitDirection) => void;
    oncreated: (
      session: SessionInfo,
      direction: SplitDirection,
      targetId: string,
    ) => void;
  } = $props();

  let launchConfigs = $state.raw<LaunchConfigsState>({ status: "loading" });
  let repositories = $state.raw<Repository[]>([]);
  let creationController: AbortController | undefined;

  function closeCreation() {
    creationController?.abort();
    creating = null;
  }

  onDestroy(() => creationController?.abort());

  $effect(() => {
    if (!creating) return;
    launchConfigs = { status: "loading" };
    repositories = [];
    const controller = new AbortController();
    creationController = controller;
    void api(
      "/launch-configs",
      LaunchConfigsSchema,
      "GET",
      undefined,
      controller.signal,
    )
      .then((configs) => {
        if (!controller.signal.aborted)
          launchConfigs = { status: "ready", configs };
      })
      .catch((err) => {
        if (!controller.signal.aborted)
          launchConfigs = { status: "unavailable", error: errorMessage(err) };
      });
    void api(
      "/repositories",
      RepositoriesSchema,
      "GET",
      undefined,
      controller.signal,
    )
      .then((result) => {
        if (!controller.signal.aborted) repositories = result;
      })
      .catch(() => {
        // The current launch directory remains usable without Git management.
      });
    return () => controller.abort();
  });
</script>

{#if choosing}
  <SplitSessionDialog
    {excluded}
    onclose={() => {
      choosing = false;
    }}
    {onselect}
  />
{/if}
{#if creating}
  {@const target = creating}
  <LazyDialog
    load={() => import("../sessions/NewSessionDialog.svelte")}
    label="创建表单"
    onclose={closeCreation}
  >
    {#snippet children(page)}<page.default
        {launchConfigs}
        {repositories}
        defaultCwd={target.cwd}
        entry={{
          kind: "directory",
          cwd: target.cwd,
          workspace: target.workspace,
        }}
        split
        onchange={() => {}}
        onworktreecreated={(tree) => {
          // Keep the new target selectable even if command startup fails and
          // the form's temporary createdTree is cleared by a repository change.
          repositories = repositories.map((repo) =>
            repo.id === tree.repositoryId
              ? {
                  ...repo,
                  worktrees: [
                    ...repo.worktrees.filter((item) => item.id !== tree.id),
                    tree,
                  ],
                }
              : repo,
          );
          const repo = repositories.find(
            (item) => item.id === tree.repositoryId,
          );
          if (repo)
            homeWorktrees.save(
              setPinned(homeWorktrees.value, repo, [tree.id], true),
            );
        }}
        onworktreeremoved={(tree) => {
          repositories = repositories.map((repo) =>
            repo.id === tree.repositoryId
              ? {
                  ...repo,
                  worktrees: repo.worktrees.filter(
                    (item) => item.id !== tree.id,
                  ),
                }
              : repo,
          );
        }}
        oncreated={(session, direction) => {
          const targetId = target.id;
          closeCreation();
          oncreated(session, direction, targetId);
        }}
        onclose={closeCreation}
      />{/snippet}
  </LazyDialog>
{/if}
<!-- Opened from a port prompt, so this server detects listening ports. -->
{#if forwarding}
  <LazyDialog
    onclose={() => (forwarding = false)}
    label="端口转发"
    load={() => import("../workspace/ForwardDialog.svelte")}
  >
    {#snippet children(Dialog)}<Dialog.default
        detection
        sessions={feedSessions}
        onclose={() => (forwarding = false)}
      />{/snippet}
  </LazyDialog>
{/if}
