import { describe, expect, it } from "vitest";
import {
  VoiceClientMessageSchema,
  VoiceServerEventSchema,
} from "../src/contracts/schemas";
import {
  computeVoiceLevel,
  downsampleFloat32,
  floatTo16BitPCM,
} from "../src/platform/voice";

describe("voice audio helpers", () => {
  it("downsamples 48kHz Float32 audio to 16kHz", () => {
    const input = new Float32Array(960);
    for (let i = 0; i < input.length; i++) {
      input[i] = Math.sin((i / 48000) * 2 * Math.PI * 440);
    }
    const output = downsampleFloat32(input, 48000, 16000);
    expect(output.length).toBe(320);
  });

  it("upsamples lower-rate Float32 audio to 16kHz", () => {
    const input = new Float32Array([0, 0.5, 1, 0.5]);
    const output = downsampleFloat32(input, 8000, 16000);
    expect(output.length).toBe(8);
  });

  it("converts Float32 samples to clamped 16-bit PCM", () => {
    const pcm = floatTo16BitPCM(new Float32Array([-1.5, -1, 0, 0.5, 1, 1.5]));
    expect(Array.from(pcm)).toEqual([-32768, -32768, 0, 16384, 32767, 32767]);
  });

  it("computes a bounded volume level in [0, 1]", () => {
    expect(computeVoiceLevel(new Float32Array(0))).toBe(0);
    expect(computeVoiceLevel(new Float32Array([0, 0, 0]))).toBe(0);
    const moderate = computeVoiceLevel(new Float32Array([0.1, -0.1, 0.1]));
    expect(moderate).toBeGreaterThan(0.2);
    expect(moderate).toBeLessThan(1);
    expect(computeVoiceLevel(new Float32Array([1, -1, 1]))).toBe(1);
  });

  it("validates voice client and server schemas", () => {
    expect(VoiceClientMessageSchema.parse({ type: "stop" })).toEqual({
      type: "stop",
    });
    expect(VoiceServerEventSchema.parse({ type: "ready" })).toEqual({
      type: "ready",
    });
    expect(
      VoiceServerEventSchema.parse({ type: "partial", text: "你好" }),
    ).toEqual({
      type: "partial",
      text: "你好",
    });
    expect(
      VoiceServerEventSchema.parse({ type: "final", text: "你好世界" }),
    ).toEqual({
      type: "final",
      text: "你好世界",
    });
    expect(
      VoiceServerEventSchema.parse({ type: "error", error: "失败" }),
    ).toEqual({
      type: "error",
      error: "失败",
    });
  });
});
