<script lang="ts">
  import IconFolderOff from "~icons/tabler/folder-off";
  import { Badge } from "$lib/components/ui/badge";
  import type { HybridRelease } from "../../contracts/schemas";

  // Marks an archived record whose worktree was released to free disk space;
  // hovering or a screen reader gives the original path and saved commit.
  let { released }: { released: HybridRelease } = $props();
  const detail = $derived(
    `原路径 ${released.path} · 提交 ${released.commit.slice(0, 12)}`,
  );
</script>

<Badge variant="secondary" title={detail}
  ><IconFolderOff aria-hidden="true" />已释放空间<span class="sr-only"
    >（{detail}）</span
  ></Badge
>
