<script lang="ts">
  import type { CodeHorizontalTrack } from "../../domains/code/code-reader-controls";

  let {
    track,
    trackElement = $bindable(),
    onscroll,
  }: {
    track: CodeHorizontalTrack;
    trackElement?: HTMLDivElement;
    onscroll: () => void;
  } = $props();
</script>

<div
  bind:this={trackElement}
  data-code-scrollbar
  tabindex="-1"
  aria-hidden="true"
  {onscroll}
  class={[
    "h-3 shrink-0 overflow-x-auto overflow-y-hidden border-t bg-card",
    !track.shown && "hidden",
  ]}
>
  <div
    style:--code-scroll-width={`${track.width}px`}
    class="h-px w-(--code-scroll-width)"
  ></div>
</div>
