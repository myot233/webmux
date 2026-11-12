// The transport may deliver bytes faster than xterm parses them. Drop live
// updates at a fixed budget, drain queued writes, then recover from one snapshot.
export function createOutputBudget(
  write: (data: Uint8Array, parsed: () => void) => void,
  requestReplay: () => void,
) {
  const limit = 4 * 1024 * 1024;
  let pending = 0;
  let recovering = false;
  let requested = false;
  let disposed = false;
  function recoverWhenDrained() {
    if (!disposed && recovering && !requested && pending === 0) {
      requested = true;
      requestReplay();
    }
  }
  return {
    get recovering() {
      return recovering;
    },
    write(data: Uint8Array): boolean {
      if (disposed || recovering) return false;
      const bytes = data.byteLength;
      if (pending + bytes > limit) {
        recovering = true;
        recoverWhenDrained();
        return false;
      }
      pending += bytes;
      write(data, () => {
        pending -= bytes;
        recoverWhenDrained();
      });
      return true;
    },
    replay() {
      recovering = false;
      requested = false;
    },
    dispose() {
      disposed = true;
    },
  };
}
