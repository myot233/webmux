<script lang="ts">
  import type { ArchivedSession, SessionInfo } from "../../contracts/schemas";
  import type { ArchiveActions } from "../../domains/sessions/archived";
  import WorkspaceGroup from "../workspace/WorkspaceGroup.svelte";

  let {
    sessions,
    visibleIds,
    focused = false,
    compact = false,
    now,
    onclose,
    archived = [],
    visibleArchiveIds,
    archive,
  }: {
    // All sessions outside every registered repository.
    sessions: SessionInfo[];
    visibleIds?: ReadonlySet<string>;
    focused?: boolean;
    compact?: boolean;
    now: number;
    onclose: (session: SessionInfo) => void;
    // Archived records outside every registered repository.
    archived?: ArchivedSession[];
    // Archived records the filters keep; undefined while not filtering.
    visibleArchiveIds?: ReadonlySet<string>;
    archive?: ArchiveActions;
  } = $props();

  const visible = $derived(
    visibleIds
      ? sessions.filter((session) => visibleIds.has(session.id))
      : sessions,
  );
  const shownArchive = $derived(
    visibleArchiveIds
      ? archived.filter((record) => visibleArchiveIds.has(record.id))
      : archived,
  );

  // Shell-reported directories are navigation hints; grouping stays on the
  // immutable launch directory so a `cd` never moves a session between groups.
  const paths = $derived(
    [
      ...new Set([
        ...sessions.map((s) => s.cwd),
        ...archived.map((record) => record.cwd),
      ]),
    ].sort(),
  );
  const reported = $derived(visible.some((s) => s.reportedCwd));
</script>

<section
  class="min-w-0 space-y-8"
  aria-label="直接会话"
  hidden={!visible.length && !shownArchive.length}
>
  <h2 class={focused ? "sr-only" : "font-semibold"}>直接会话</h2>
  {#if reported}<p class="text-xs text-muted-foreground">
      按启动目录分组；Shell 报告目录仅作导航线索，不改变归属
    </p>{/if}
  {#each paths as path (path)}
    {@const pathArchive = shownArchive.filter((record) => record.cwd === path)}
    <WorkspaceGroup
      title={path.split("/").pop() || "/"}
      {path}
      sessions={sessions.filter((s) => s.cwd === path)}
      {visibleIds}
      {compact}
      hidden={Boolean(
        (visibleIds || visibleArchiveIds) &&
        !pathArchive.length &&
        !visible.some((session) => session.cwd === path),
      )}
      {now}
      {onclose}
      archived={pathArchive}
      {archive}
    />{/each}
</section>
