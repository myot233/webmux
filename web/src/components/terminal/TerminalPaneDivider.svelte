<script lang="ts">
  import {
    keyboardSplitRatio,
    pointerSplitRatio,
    type Divider,
  } from "../../domains/terminal/terminal-layout";

  let {
    divider,
    getWorkspaceBounds,
    onresize,
  }: {
    divider: Divider;
    getWorkspaceBounds: () => DOMRect;
    onresize: (id: string, ratio: number) => void;
  } = $props();

  function drag(event: PointerEvent) {
    if (event.button !== 0) return;
    const handle = event.currentTarget as HTMLElement;
    handle.setPointerCapture(event.pointerId);
    handle.focus();
    event.preventDefault();
  }

  function move(event: PointerEvent) {
    if (
      !(event.currentTarget as HTMLElement).hasPointerCapture(event.pointerId)
    )
      return;
    const ratio = pointerSplitRatio(event, getWorkspaceBounds(), divider);
    if (ratio !== undefined) onresize(divider.id, ratio);
  }

  function resizeKey(event: KeyboardEvent) {
    const ratio = keyboardSplitRatio(event.key, divider);
    if (ratio === undefined) return;
    event.preventDefault();
    onresize(divider.id, ratio);
  }
</script>

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
  onpointermove={move}
  onpointerup={(event) =>
    (event.currentTarget as HTMLElement).releasePointerCapture(event.pointerId)}
  onkeydown={resizeKey}
  ondblclick={() => {
    onresize(divider.id, 0.5);
  }}
></div>

<style>
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
    /* Where a row line meets this column line, the column keeps its grab
       area; the row's widened edge would otherwise cover its midpoint. */
    z-index: 3;
  }
  /* The line stays 1px; this widens the grab area around it. */
  .pane-divider::after {
    content: "";
    position: absolute;
    inset: -4px 0;
  }
  .pane-divider.horizontal::after {
    inset: 0 -4px;
  }
  @media (pointer: coarse) {
    .pane-divider::after {
      inset: -9px 0;
    }
    .pane-divider.horizontal::after {
      inset: 0 -9px;
    }
  }
  .pane-divider:hover,
  .pane-divider:focus-visible {
    background: var(--ring);
    outline: none;
  }
</style>
