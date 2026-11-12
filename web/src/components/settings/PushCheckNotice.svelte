<script lang="ts">
  import type { PushCheck } from "../../contracts/schemas";
  import InlineNotice from "../shared/InlineNotice.svelte";

  // Whether the commit a removal or release keeps is contained in a
  // remote-tracking ref. The server reads local refs only, so an unpushed
  // commit warns without blocking, and a missing remote or a failed check
  // stays silent.
  let { push }: { push: PushCheck } = $props();
  const branches = $derived(
    push.branches.join("、") +
      (push.more ? ` 等 ${push.more + push.branches.length} 个` : ""),
  );
</script>

{#if push.state === "unpushed"}
  <InlineNotice tone="warning">
    该提交尚未推送到任何远程；仓库本地丢失或被清理后将无法重建，请先推送。以本地远程跟踪引用为准，未联网核对。
  </InlineNotice>
{:else if push.state === "pushed"}
  <p class="text-xs text-muted-foreground">
    该提交已包含在远程分支：{branches}（以本地远程跟踪引用为准，未联网核对）。
  </p>
{/if}
