const TARGET_SAMPLE_RATE = 16000;

/**
 * Downsamples mono Float32 audio from `sourceRate` to `targetRate` (16kHz) by
 * averaging the source samples in each output window.
 */
export function downsampleFloat32(
  input: Float32Array,
  sourceRate: number,
  targetRate = TARGET_SAMPLE_RATE,
): Float32Array {
  if (input.length === 0) return new Float32Array(0);
  if (sourceRate === targetRate || sourceRate <= 0 || targetRate <= 0)
    return input;
  if (sourceRate < targetRate) {
    const ratio = sourceRate / targetRate;
    const outLength = Math.max(1, Math.round(input.length / ratio));
    const output = new Float32Array(outLength);
    for (let i = 0; i < outLength; i++) {
      const pos = i * ratio;
      const idx = Math.floor(pos);
      const frac = pos - idx;
      const a = input[Math.min(idx, input.length - 1)] ?? 0;
      const b = input[Math.min(idx + 1, input.length - 1)] ?? a;
      output[i] = a + (b - a) * frac;
    }
    return output;
  }
  const ratio = sourceRate / targetRate;
  const outLength = Math.floor(input.length / ratio);
  const output = new Float32Array(outLength);
  let offsetResult = 0;
  let offsetBuffer = 0;
  while (offsetResult < outLength) {
    const nextOffsetBuffer = Math.min(
      input.length,
      Math.round((offsetResult + 1) * ratio),
    );
    let accum = 0;
    let count = 0;
    for (let i = offsetBuffer; i < nextOffsetBuffer; i++) {
      accum += input[i] ?? 0;
      count++;
    }
    output[offsetResult] = count > 0 ? accum / count : 0;
    offsetResult++;
    offsetBuffer = nextOffsetBuffer;
  }
  return output;
}

/** Converts mono Float32 samples in [-1, 1] to 16-bit little-endian PCM. */
export function floatTo16BitPCM(input: Float32Array): Int16Array {
  const output = new Int16Array(input.length);
  for (let i = 0; i < input.length; i++) {
    const s = Math.max(-1, Math.min(1, input[i] ?? 0));
    output[i] = s < 0 ? Math.round(s * 0x8000) : Math.round(s * 0x7fff);
  }
  return output;
}

/** Computes a normalized visual volume level in [0, 1] from Float32 samples. */
export function computeVoiceLevel(samples: Float32Array): number {
  if (samples.length === 0) return 0;
  let sumSquares = 0;
  for (let i = 0; i < samples.length; i++) {
    const v = samples[i] ?? 0;
    sumSquares += v * v;
  }
  const rms = Math.sqrt(sumSquares / samples.length);
  // Typical speech RMS sits around 0.02..0.25; curve it into [0, 1].
  return Math.min(1, Math.pow(rms * 4.5, 0.65));
}
