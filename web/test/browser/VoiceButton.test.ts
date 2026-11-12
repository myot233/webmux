import { afterEach, beforeEach, expect, test, vi } from "vitest";
import { page, userEvent } from "vitest/browser";
import { cleanup, render } from "vitest-browser-svelte/pure";
import VoiceButton from "../../src/components/shared/VoiceButton.svelte";

class MockWebSocket {
  static CONNECTING = 0;
  static OPEN = 1;
  static CLOSING = 2;
  static CLOSED = 3;
  static instances: MockWebSocket[] = [];

  readyState = MockWebSocket.OPEN;
  binaryType = "";
  onopen: (() => void) | null = null;
  onmessage: ((event: { data: string | ArrayBuffer }) => void) | null = null;
  onclose: ((event: { code: number; reason?: string }) => void) | null = null;
  onerror: (() => void) | null = null;
  sent: Array<string | Uint8Array> = [];

  constructor(
    readonly url: string,
    readonly protocols: string[],
  ) {
    MockWebSocket.instances.push(this);
  }

  send(data: string | Uint8Array | ArrayBuffer) {
    if (typeof data === "string") {
      this.sent.push(data);
    } else if (data instanceof Uint8Array) {
      this.sent.push(data.slice());
    } else {
      this.sent.push(new Uint8Array(data).slice());
    }
  }

  close() {
    this.readyState = MockWebSocket.CLOSED;
  }
}

class MockAudioProcessor {
  onaudioprocess:
    | ((event: {
        inputBuffer: { getChannelData: (channel: number) => Float32Array };
      }) => void)
    | null = null;
  connect = vi.fn();
  disconnect = vi.fn();

  emit(samples: Float32Array) {
    this.onaudioprocess?.({
      inputBuffer: {
        getChannelData: () => samples,
      },
    });
  }
}

class MockAudioContext {
  static instances: MockAudioContext[] = [];
  sampleRate = 16000;
  state = "running";
  destination = {};
  processor = new MockAudioProcessor();

  constructor(options?: { sampleRate?: number }) {
    if (options?.sampleRate) this.sampleRate = options.sampleRate;
    MockAudioContext.instances.push(this);
  }

  resume = vi.fn(async () => {});
  close = vi.fn(async () => {});
  createMediaStreamSource = vi.fn(() => ({
    connect: vi.fn(),
    disconnect: vi.fn(),
  }));
  createScriptProcessor = vi.fn(() => this.processor);
  createGain = vi.fn(() => ({
    gain: { value: 1 },
    connect: vi.fn(),
    disconnect: vi.fn(),
  }));
}

let trackStop: ReturnType<typeof vi.fn>;

beforeEach(() => {
  MockWebSocket.instances = [];
  MockAudioContext.instances = [];
  trackStop = vi.fn();

  vi.stubGlobal("WebSocket", MockWebSocket);
  vi.spyOn(navigator.mediaDevices, "getUserMedia").mockResolvedValue({
    getTracks: () => [{ stop: trackStop }],
  } as unknown as MediaStream);
  vi.stubGlobal("AudioContext", MockAudioContext);
});

afterEach(async () => {
  await cleanup();
  await userEvent.cleanup();
  vi.restoreAllMocks();
  vi.unstubAllGlobals();
});

test("VoiceButton transitions through connecting -> recording -> finishing -> idle and delivers text without newline", async () => {
  const results: string[] = [];
  const errors: string[] = [];
  render(VoiceButton, {
    props: {
      onresult: (text: string) => results.push(text),
      onerror: (msg: string) => {
        if (msg) errors.push(msg);
      },
    },
  });

  const idleButton = page.getByRole("button", { name: "语音输入" });
  await expect.element(idleButton).toBeVisible();
  await expect.element(idleButton).toHaveAttribute("aria-pressed", "false");

  // 1. Click to start -> connecting
  await idleButton.click();
  const connectingButton = page.getByRole("button", {
    name: "正在连接语音识别…",
  });
  await expect.element(connectingButton).toHaveAttribute("aria-busy", "true");
  await expect.poll(() => MockWebSocket.instances.length).toBe(1);

  const ws = MockWebSocket.instances[0]!;
  expect(ws.url).toMatch(/\/ws\/voice$/);

  // 2. Server sends ready -> recording (shows volume meter)
  ws.onmessage?.({ data: JSON.stringify({ type: "ready" }) });
  const recordingButton = page.getByRole("button", {
    name: "停止录音并识别",
  });
  await expect.element(recordingButton).toHaveAttribute("aria-pressed", "true");
  expect(
    recordingButton.element().querySelector("[data-voice-meter]"),
  ).not.toBeNull();

  // Emit 320 samples (20ms at 16kHz) -> 640-byte PCM frame
  const ctx = MockAudioContext.instances[0]!;
  ctx.processor.emit(new Float32Array(320).fill(0.3));
  expect(ws.sent).toHaveLength(1);
  expect((ws.sent[0] as Uint8Array).byteLength).toBe(640);

  // 3. Click again -> finishing, sends {"type":"stop"}
  await recordingButton.click();
  const finishingButton = page.getByRole("button", {
    name: "正在完成语音识别…",
  });
  await expect.element(finishingButton).toHaveAttribute("aria-busy", "true");
  expect(trackStop).toHaveBeenCalledTimes(1);
  expect(ws.sent).toHaveLength(2);
  expect(JSON.parse(ws.sent[1] as string)).toEqual({ type: "stop" });

  // 4. Server sends final -> back to idle, delivers text without Enter
  ws.onmessage?.({
    data: JSON.stringify({
      type: "final",
      text: "重构 Kubelet 的 Pod 生命周期管理器",
    }),
  });
  await expect
    .element(page.getByRole("button", { name: "语音输入" }))
    .toBeVisible();
  expect(results).toEqual(["重构 Kubelet 的 Pod 生命周期管理器"]);
  expect(results[0]?.endsWith("\r")).toBe(false);
  expect(results[0]?.endsWith("\n")).toBe(false);
  expect(errors).toEqual([]);
});

test("VoiceButton reports server errors and returns to idle", async () => {
  const errors: string[] = [];
  render(VoiceButton, {
    props: {
      onresult: () => {},
      onerror: (msg: string) => {
        if (msg) errors.push(msg);
      },
    },
  });

  await page.getByRole("button", { name: "语音输入" }).click();
  await expect.poll(() => MockWebSocket.instances.length).toBe(1);
  const ws = MockWebSocket.instances[0]!;

  ws.onmessage?.({
    data: JSON.stringify({ type: "error", error: "API 密钥无效" }),
  });
  await expect
    .element(page.getByRole("button", { name: "语音输入" }))
    .toBeVisible();
  expect(errors).toEqual(["API 密钥无效"]);
});
