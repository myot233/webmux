<script lang="ts">
  import { onDestroy } from "svelte";
  import IconArrowDown from "~icons/tabler/arrow-down";
  import IconArrowLeft from "~icons/tabler/arrow-left";
  import IconArrowRight from "~icons/tabler/arrow-right";
  import IconArrowUp from "~icons/tabler/arrow-up";
  import IconCornerDownLeft from "~icons/tabler/corner-down-left";
  import IconSpace from "~icons/tabler/space";
  import { Button } from "$lib/components/ui/button";

  let { send }: { send: (input: string) => void } = $props();

  const repeatDelayMs = 350;
  const repeatIntervalMs = 75;
  const steerThresholdPx = 14;

  const arrowInputs = {
    up: "\x1b[A",
    down: "\x1b[B",
    left: "\x1b[D",
    right: "\x1b[C",
  } as const;
  type ArrowDirection = keyof typeof arrowInputs;

  let repeatDelay: ReturnType<typeof setTimeout> | undefined;
  let repeatTimer: ReturnType<typeof setInterval> | undefined;
  let activePointerId = $state<number | null>(null);
  let activeDirection = $state<ArrowDirection | null>(null);
  let originX = 0;
  let originY = 0;
  let pointerHandled = false;

  function stopRepeat() {
    clearTimeout(repeatDelay);
    clearInterval(repeatTimer);
    repeatDelay = undefined;
    repeatTimer = undefined;
    activePointerId = null;
    activeDirection = null;
  }

  function startRepeat(input: string) {
    clearTimeout(repeatDelay);
    clearInterval(repeatTimer);
    send(input);
    repeatDelay = setTimeout(() => {
      repeatDelay = undefined;
      repeatTimer = setInterval(() => {
        send(input);
      }, repeatIntervalMs);
    }, repeatDelayMs);
  }

  function keepFocus(event: MouseEvent) {
    event.preventDefault();
  }

  function capturePointer(target: EventTarget | null, pointerId: number) {
    if (!(target instanceof HTMLElement)) return;
    try {
      target.setPointerCapture(pointerId);
    } catch {
      // Synthetic pointer events in unit tests do not register a browser pointer.
    }
  }

  function releasePointer(target: EventTarget | null, pointerId: number) {
    if (!(target instanceof HTMLElement)) return;
    try {
      if (target.hasPointerCapture(pointerId))
        target.releasePointerCapture(pointerId);
    } catch {
      // Ignore synthetic pointer ids.
    }
  }

  function startDirection(direction: ArrowDirection, event: PointerEvent) {
    if (event.button !== 0 && event.pointerType === "mouse") return;
    event.preventDefault();
    event.stopPropagation();
    pointerHandled = true;
    activePointerId = event.pointerId;
    activeDirection = direction;
    originX = event.clientX;
    originY = event.clientY;
    capturePointer(event.currentTarget, event.pointerId);
    startRepeat(arrowInputs[direction]);
  }

  function moveDirection(event: PointerEvent) {
    if (activePointerId !== event.pointerId || !activeDirection) return;
    event.preventDefault();
    event.stopPropagation();
    const dx = event.clientX - originX;
    const dy = event.clientY - originY;
    if (Math.hypot(dx, dy) < steerThresholdPx) return;
    const nextDirection: ArrowDirection =
      Math.abs(dx) > Math.abs(dy)
        ? dx > 0
          ? "right"
          : "left"
        : dy > 0
          ? "down"
          : "up";
    if (nextDirection !== activeDirection) {
      activeDirection = nextDirection;
      originX = event.clientX;
      originY = event.clientY;
      startRepeat(arrowInputs[nextDirection]);
    }
  }

  function endPointer(event: PointerEvent) {
    if (activePointerId !== event.pointerId) return;
    event.preventDefault();
    event.stopPropagation();
    releasePointer(event.currentTarget, event.pointerId);
    stopRepeat();
  }

  function clickKey(input: string, event: MouseEvent) {
    event.stopPropagation();
    if (pointerHandled && event.detail > 0) {
      pointerHandled = false;
      return;
    }
    pointerHandled = false;
    send(input);
  }

  onDestroy(stopRepeat);
</script>

<div
  class="flex w-full items-end justify-between gap-2 pb-1.5"
  role="toolbar"
  aria-label="终端摇杆"
>
  <div class="grid grid-cols-2 gap-1.5">
    <Button
      variant="hud"
      size="sm"
      data-gamepad-key="esc"
      aria-label="Esc"
      title="Esc"
      onmousedown={keepFocus}
      onclick={(event) => {
        event.stopPropagation();
        send("\x1b");
      }}>Esc</Button
    >
    <Button
      variant="hud"
      size="sm"
      data-gamepad-key="tab"
      aria-label="Tab"
      title="Tab"
      onmousedown={keepFocus}
      onclick={(event) => {
        event.stopPropagation();
        send("\t");
      }}>Tab</Button
    >
    <Button
      variant="hud"
      size="sm"
      data-gamepad-key="ctrl-c"
      aria-label="Ctrl+C"
      title="Ctrl+C"
      onmousedown={keepFocus}
      onclick={(event) => {
        event.stopPropagation();
        send("\x03");
      }}>Ctrl+C</Button
    >
    <Button
      variant="hud"
      size="sm"
      data-gamepad-key="enter"
      aria-label="回车"
      title="回车"
      onmousedown={keepFocus}
      onclick={(event) => {
        event.stopPropagation();
        send("\r");
      }}
    >
      <IconCornerDownLeft aria-hidden="true" />
      <span>Enter</span>
    </Button>
  </div>

  <div class="grid grid-cols-3 gap-1.5">
    <div class="col-start-1 row-start-1">
      <Button
        variant="hud"
        size="icon-sm"
        class="w-full"
        data-gamepad-key="space"
        aria-label="空格"
        title="空格（切换选择）"
        onmousedown={keepFocus}
        onclick={(event) => {
          event.stopPropagation();
          send(" ");
        }}
      >
        <IconSpace aria-hidden="true" />
      </Button>
    </div>
    <div class="col-start-2 row-start-1">
      <Button
        variant="hud"
        size="icon-sm"
        class="w-full"
        data-gamepad-key="up"
        aria-label="向上方向键"
        title="向上方向键（可按住滑动切换方向）"
        aria-pressed={activeDirection === "up"}
        onmousedown={keepFocus}
        onpointerdown={(event) => startDirection("up", event)}
        onpointermove={moveDirection}
        onpointerup={endPointer}
        onpointercancel={endPointer}
        onlostpointercapture={stopRepeat}
        onclick={(event) => clickKey(arrowInputs.up, event)}
      >
        <IconArrowUp aria-hidden="true" />
      </Button>
    </div>
    <div class="col-start-1 row-start-2">
      <Button
        variant="hud"
        size="icon-sm"
        class="w-full"
        data-gamepad-key="left"
        aria-label="向左方向键"
        title="向左方向键（可按住滑动切换方向）"
        aria-pressed={activeDirection === "left"}
        onmousedown={keepFocus}
        onpointerdown={(event) => startDirection("left", event)}
        onpointermove={moveDirection}
        onpointerup={endPointer}
        onpointercancel={endPointer}
        onlostpointercapture={stopRepeat}
        onclick={(event) => clickKey(arrowInputs.left, event)}
      >
        <IconArrowLeft aria-hidden="true" />
      </Button>
    </div>
    <div class="col-start-2 row-start-2">
      <Button
        variant="hud"
        size="icon-sm"
        class="w-full"
        data-gamepad-key="down"
        aria-label="向下方向键"
        title="向下方向键（可按住滑动切换方向）"
        aria-pressed={activeDirection === "down"}
        onmousedown={keepFocus}
        onpointerdown={(event) => startDirection("down", event)}
        onpointermove={moveDirection}
        onpointerup={endPointer}
        onpointercancel={endPointer}
        onlostpointercapture={stopRepeat}
        onclick={(event) => clickKey(arrowInputs.down, event)}
      >
        <IconArrowDown aria-hidden="true" />
      </Button>
    </div>
    <div class="col-start-3 row-start-2">
      <Button
        variant="hud"
        size="icon-sm"
        class="w-full"
        data-gamepad-key="right"
        aria-label="向右方向键"
        title="向右方向键（可按住滑动切换方向）"
        aria-pressed={activeDirection === "right"}
        onmousedown={keepFocus}
        onpointerdown={(event) => startDirection("right", event)}
        onpointermove={moveDirection}
        onpointerup={endPointer}
        onpointercancel={endPointer}
        onlostpointercapture={stopRepeat}
        onclick={(event) => clickKey(arrowInputs.right, event)}
      >
        <IconArrowRight aria-hidden="true" />
      </Button>
    </div>
  </div>
</div>
