<script lang="ts">
  import { onDestroy } from "svelte";
  import IconLoader2 from "~icons/tabler/loader-2";
  import IconMicrophone from "~icons/tabler/microphone";
  import {
    Button,
    type ButtonSize,
    type ButtonVariant,
  } from "$lib/components/ui/button";
  import { createVoiceRecorder } from "../../hooks/shared/voice.svelte";

  let {
    size = "icon-sm",
    variant = "outline",
    disabled = false,
    class: className = "",
    onresult,
    onerror,
    onmousedown,
  }: {
    size?: ButtonSize;
    variant?: ButtonVariant;
    disabled?: boolean;
    class?: string;
    onresult: (text: string) => void;
    onerror?: (message: string) => void;
    onmousedown?: (event: MouseEvent) => void;
  } = $props();

  const recorder = createVoiceRecorder({
    onResult: (text) => onresult(text),
    onError: (message) => onerror?.(message),
  });

  onDestroy(() => {
    recorder.cancel();
  });

  const label = $derived(
    recorder.state === "connecting"
      ? "正在连接语音识别…"
      : recorder.state === "recording"
        ? "停止录音并识别"
        : recorder.state === "finishing"
          ? "正在完成语音识别…"
          : "语音输入",
  );
  const busy = $derived(
    recorder.state === "connecting" || recorder.state === "finishing",
  );
  const barLow = $derived(Math.min(1, recorder.level * 0.65).toFixed(3));
  const barHigh = $derived(Math.min(1, recorder.level).toFixed(3));
</script>

<Button
  {variant}
  {size}
  class={className}
  aria-label={label}
  title={label}
  aria-pressed={recorder.state === "recording"}
  aria-busy={busy}
  disabled={(disabled && recorder.state === "idle") ||
    recorder.state === "finishing"}
  {onmousedown}
  onclick={() => recorder.toggle()}
>
  {#if busy}
    <IconLoader2
      class="size-4 animate-spin motion-reduce:animate-none"
      aria-hidden="true"
    />
  {:else if recorder.state === "recording"}
    <span
      class="inline-flex h-4 items-center gap-0.5"
      aria-hidden="true"
      data-voice-meter
    >
      <span
        class="h-voice-bar w-0.5 rounded-sm bg-current transition-all duration-75"
        style:--voice-bar={barLow}
      ></span>
      <span
        class="h-voice-bar w-0.5 rounded-sm bg-current transition-all duration-75"
        style:--voice-bar={barHigh}
      ></span>
      <span
        class="h-voice-bar w-0.5 rounded-sm bg-current transition-all duration-75"
        style:--voice-bar={barLow}
      ></span>
    </span>
  {:else}
    <IconMicrophone class="size-4" aria-hidden="true" />
  {/if}
</Button>
