import { onDestroy } from "svelte";
import { copyText } from "../clipboard";

/**
 * Copy feedback expires after `duration` milliseconds. Failed attempts stay
 * visible until a successful copy unless `expireFailure` is set.
 */
export function useCopied({
  duration = 2000,
  expireFailure = false,
}: { duration?: number; expireFailure?: boolean } = {}) {
  let result = $state<boolean | null>(null);
  let timer: ReturnType<typeof setTimeout> | undefined;
  let destroyed = false;
  onDestroy(() => {
    destroyed = true;
    clearTimeout(timer);
  });

  return {
    get copied() {
      return result === true;
    },
    get failed() {
      return result === false;
    },
    async copy(text: string) {
      const success = await copyText(text);
      if (destroyed) return success;
      result = success;
      clearTimeout(timer);
      if (success || expireFailure)
        timer = setTimeout(() => (result = null), duration);
      return success;
    },
  };
}
