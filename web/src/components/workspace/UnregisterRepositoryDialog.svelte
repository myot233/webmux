<script lang="ts">
  import ConfirmDialog from "../shared/ConfirmDialog.svelte";
  import { api } from "../../platform/api";
  import { NoContentSchema, type Repository } from "../../contracts/schemas";

  let {
    repository,
    open = $bindable(),
    onchange,
  }: {
    // Kept after closing so the closing animation still shows its path.
    repository: Repository | null;
    open: boolean;
    onchange: () => Promise<void>;
  } = $props();

  async function unregister() {
    if (!repository) return;
    await api(`/repositories/${repository.id}`, NoContentSchema, "DELETE");
    open = false;
    await onchange();
  }
</script>

<ConfirmDialog
  bind:open
  title="从列表移除仓库？"
  confirmLabel="确认移除登记"
  busyLabel="正在移除…"
  onconfirm={unregister}
>
  <span class="break-all whitespace-pre-wrap"
    >{repository?.anchorPath}<br
    />只移除持久登记，不删除任何目录或分支，不关闭会话。原会话仍可在「直接会话」中访问。</span
  >
</ConfirmDialog>
