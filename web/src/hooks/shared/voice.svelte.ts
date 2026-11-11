import {
  VoiceClientMessageSchema,
  VoiceServerEventSchema,
  type VoiceServerEvent,
} from "../../contracts/schemas";
import {
  SOCKET_CONNECTING,
  SOCKET_OPEN,
  type TransportSocket,
} from "../../platform/transport";
import {
  computeVoiceLevel,
  downsampleFloat32,
  floatTo16BitPCM,
} from "../../platform/voice";
import { openSocket } from "../../platform/ws";

type VoiceState = "idle" | "connecting" | "recording" | "finishing";

export interface VoiceRecorderOptions {
  /** Called with the recognized text once the session completes. */
  onResult: (text: string) => void;
  /** Called with an error message when capture or recognition fails. */
  onError?: (message: string) => void;
}

export interface VoiceRecorder {
  readonly state: VoiceState;
  /** Normalized microphone volume level in [0, 1] while recording. */
  readonly level: number;
  /** Starts recording from idle, or finishes recording when active. */
  toggle(): void;
  /** Aborts any active recording or connection immediately. */
  cancel(): void;
}

const TARGET_SAMPLE_RATE = 16000;
// 20ms at 16kHz = 320 samples (640 bytes of s16le PCM).
const FRAME_SAMPLES = 320;
const FINISH_TIMEOUT_MS = 15000;

function microphoneErrorMessage(err: unknown): string {
  if (typeof window !== "undefined" && !window.isSecureContext) {
    return "浏览器需要 HTTPS 或 localhost 安全上下文才能访问麦克风";
  }
  if (err instanceof DOMException) {
    if (
      err.name === "NotAllowedError" ||
      err.name === "PermissionDeniedError"
    ) {
      return "麦克风权限被拒绝，请在浏览器地址栏允许访问麦克风";
    }
    if (err.name === "NotFoundError" || err.name === "DevicesNotFoundError") {
      return "未检测到可用的麦克风设备";
    }
    if (err.name === "NotReadableError" || err.name === "TrackStartError") {
      return "麦克风正被其他应用占用或无法读取";
    }
  }
  return err instanceof Error && err.message
    ? `无法启动麦克风：${err.message}`
    : "无法启动麦克风";
}

export function createVoiceRecorder(
  options: VoiceRecorderOptions,
): VoiceRecorder {
  let state = $state<VoiceState>("idle");
  let level = $state(0);
  let sessionToken = 0;

  let socket: TransportSocket | undefined;
  let stream: MediaStream | undefined;
  let audioContext: AudioContext | undefined;
  let sourceNode: MediaStreamAudioSourceNode | undefined;
  let processorNode: ScriptProcessorNode | undefined;
  let gainNode: GainNode | undefined;
  let finishTimer: ReturnType<typeof setTimeout> | undefined;
  let pendingSamples: number[] = [];
  let latestPartial = "";
  let stopRequested = false;

  function stopAudioCapture() {
    if (processorNode) {
      processorNode.onaudioprocess = null;
      try {
        processorNode.disconnect();
      } catch {
        /* ignore */
      }
      processorNode = undefined;
    }
    if (sourceNode) {
      try {
        sourceNode.disconnect();
      } catch {
        /* ignore */
      }
      sourceNode = undefined;
    }
    if (gainNode) {
      try {
        gainNode.disconnect();
      } catch {
        /* ignore */
      }
      gainNode = undefined;
    }
    if (audioContext) {
      void audioContext.close().catch(() => undefined);
      audioContext = undefined;
    }
    if (stream) {
      for (const track of stream.getTracks()) track.stop();
      stream = undefined;
    }
    level = 0;
  }

  function cleanup() {
    sessionToken++;
    clearTimeout(finishTimer);
    finishTimer = undefined;
    stopAudioCapture();
    const prev = socket;
    socket = undefined;
    if (
      prev &&
      (prev.readyState === SOCKET_OPEN || prev.readyState === SOCKET_CONNECTING)
    ) {
      prev.close();
    }
    pendingSamples = [];
    latestPartial = "";
    stopRequested = false;
    state = "idle";
  }

  function flushFrame(pcm: Int16Array) {
    if (socket?.readyState !== SOCKET_OPEN) return;
    socket.send(
      new Uint8Array(pcm.buffer, pcm.byteOffset, pcm.byteLength).slice(),
    );
  }

  function sendStopFrame() {
    if (socket?.readyState !== SOCKET_OPEN) return;
    if (pendingSamples.length > 0) {
      const remaining = new Int16Array(pendingSamples);
      pendingSamples = [];
      flushFrame(remaining);
    }
    const msg = VoiceClientMessageSchema.parse({ type: "stop" });
    socket.send(JSON.stringify(msg));
  }

  function handleAudioSamples(samples: Float32Array, sampleRate: number) {
    const nextLevel = computeVoiceLevel(samples);
    level = level * 0.35 + nextLevel * 0.65;
    const resampled = downsampleFloat32(
      samples,
      sampleRate,
      TARGET_SAMPLE_RATE,
    );
    const pcm = floatTo16BitPCM(resampled);
    for (let i = 0; i < pcm.length; i++) {
      pendingSamples.push(pcm[i]!);
    }
    if (state !== "recording" || socket?.readyState !== SOCKET_OPEN) {
      // Bound pre-ready buffered audio to ~5 seconds so memory stays bounded.
      const maxBuffered = TARGET_SAMPLE_RATE * 5;
      if (pendingSamples.length > maxBuffered) {
        pendingSamples.splice(0, pendingSamples.length - maxBuffered);
      }
      return;
    }
    while (pendingSamples.length >= FRAME_SAMPLES) {
      const chunk = new Int16Array(pendingSamples.splice(0, FRAME_SAMPLES));
      flushFrame(chunk);
    }
  }

  function fail(message: string) {
    cleanup();
    options.onError?.(message);
  }

  function handleServerEvent(event: VoiceServerEvent) {
    if (event.type === "ready") {
      if (state === "connecting") {
        if (stopRequested) {
          state = "finishing";
          stopAudioCapture();
          sendStopFrame();
          return;
        }
        state = "recording";
        while (pendingSamples.length >= FRAME_SAMPLES) {
          const chunk = new Int16Array(pendingSamples.splice(0, FRAME_SAMPLES));
          flushFrame(chunk);
        }
      }
      return;
    }
    if (event.type === "partial") {
      latestPartial = event.text;
      return;
    }
    if (event.type === "final") {
      const finalText = event.text || latestPartial;
      cleanup();
      if (finalText) options.onResult(finalText);
      return;
    }
    if (event.type === "error") {
      fail(event.error || "语音识别失败");
    }
  }

  async function start() {
    if (state !== "idle") return;
    if (
      typeof navigator === "undefined" ||
      !navigator.mediaDevices?.getUserMedia
    ) {
      fail(microphoneErrorMessage(new Error("当前环境不支持麦克风采集")));
      return;
    }
    const token = ++sessionToken;
    state = "connecting";
    level = 0;
    pendingSamples = [];
    latestPartial = "";
    stopRequested = false;
    options.onError?.("");

    let mediaStream: MediaStream;
    try {
      mediaStream = await navigator.mediaDevices.getUserMedia({
        audio: {
          channelCount: 1,
          echoCancellation: true,
          noiseSuppression: true,
          autoGainControl: true,
        },
      });
    } catch (err) {
      if (token !== sessionToken) return;
      fail(microphoneErrorMessage(err));
      return;
    }
    if (token !== sessionToken) {
      for (const track of mediaStream.getTracks()) track.stop();
      return;
    }
    stream = mediaStream;

    try {
      const AudioCtx =
        window.AudioContext ||
        (window as unknown as { webkitAudioContext: typeof AudioContext })
          .webkitAudioContext;
      let ctx: AudioContext;
      try {
        ctx = new AudioCtx({ sampleRate: TARGET_SAMPLE_RATE });
      } catch {
        ctx = new AudioCtx();
      }
      audioContext = ctx;
      if (ctx.state === "suspended") {
        await ctx.resume().catch(() => undefined);
      }
      const source = ctx.createMediaStreamSource(mediaStream);
      const processor = ctx.createScriptProcessor(1024, 1, 1);
      const silentGain = ctx.createGain();
      silentGain.gain.value = 0;
      processor.onaudioprocess = (audioEvent) => {
        if (token !== sessionToken) return;
        const inputData = audioEvent.inputBuffer.getChannelData(0);
        handleAudioSamples(inputData, ctx.sampleRate);
      };
      source.connect(processor);
      processor.connect(silentGain);
      silentGain.connect(ctx.destination);
      sourceNode = source;
      processorNode = processor;
      gainNode = silentGain;
    } catch (err) {
      if (token !== sessionToken) return;
      fail(microphoneErrorMessage(err));
      return;
    }

    const ws = openSocket("/ws/voice", false);
    socket = ws;

    ws.onmessage = (msgEvent) => {
      if (token !== sessionToken || socket !== ws) return;
      if (typeof msgEvent.data !== "string") return;
      let parsed: VoiceServerEvent;
      try {
        parsed = VoiceServerEventSchema.parse(JSON.parse(msgEvent.data));
      } catch {
        fail("无效的语音识别响应");
        return;
      }
      handleServerEvent(parsed);
    };

    ws.onerror = () => {
      if (token !== sessionToken || socket !== ws) return;
      fail("语音识别连接异常");
    };

    ws.onclose = (closeEvent) => {
      if (token !== sessionToken || socket !== ws) return;
      if (state === "finishing" && latestPartial) {
        const text = latestPartial;
        cleanup();
        options.onResult(text);
        return;
      }
      if (state !== "idle") {
        const reason =
          closeEvent.reason ||
          (closeEvent.code === 1006
            ? "语音识别服务未配置或连接已断开"
            : "语音识别连接已关闭");
        fail(reason);
      }
    };
  }

  function finish() {
    if (state === "connecting") {
      stopRequested = true;
      state = "finishing";
      stopAudioCapture();
      finishTimer = setTimeout(() => {
        if (state === "finishing") fail("语音识别超时");
      }, FINISH_TIMEOUT_MS);
      return;
    }
    if (state !== "recording") return;
    state = "finishing";
    stopAudioCapture();
    sendStopFrame();
    finishTimer = setTimeout(() => {
      if (state === "finishing") fail("语音识别超时");
    }, FINISH_TIMEOUT_MS);
  }

  return {
    get state() {
      return state;
    },
    get level() {
      return level;
    },
    toggle() {
      if (state === "idle") {
        void start();
      } else if (state === "recording" || state === "connecting") {
        finish();
      }
    },
    cancel() {
      cleanup();
    },
  };
}
