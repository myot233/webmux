<script lang="ts">
  import IconTriangle from "~icons/tabler/triangle";
  import {
    rowStatusLabels,
    type RowStatus,
  } from "../../domains/sessions/session-status";
  import { underRowAction } from "../../hooks/shared/sidebar-row";

  // The state at the end of a sidebar row: a coloured dot, right-aligned
  // with the row's other trailing marks, or a hollow triangle once the
  // record's worktree space was released (the dot's hollow sibling for
  // "ended"). The row's accessible name and tooltip carry the state's
  // text, so it never depends on colour or shape alone; the mark keeps the
  // text as its own tooltip too. With a mouse, the row's "⋯" button takes
  // its place while the row is hovered, focused or its menu is open.
  let { status }: { status: RowStatus } = $props();
  const tones: Record<Exclude<RowStatus, "released">, string> = {
    working: "bg-success",
    waiting: "bg-warning",
    idle: "bg-muted-foreground/70",
    ended: "border border-muted-foreground/70",
  };
</script>

<span
  class={`flex size-4 shrink-0 items-center justify-end ${underRowAction}`}
  title={rowStatusLabels[status]}
  data-status={status}
  >{#if status === "released"}<IconTriangle
      class="size-2.5 text-muted-foreground/70"
      aria-hidden="true"
    />{:else}<span
      class={`size-2 rounded-full ${tones[status]}`}
      aria-hidden="true"
    ></span>{/if}</span
>
