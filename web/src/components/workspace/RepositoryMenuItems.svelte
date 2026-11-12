<script lang="ts">
  import * as DropdownMenu from "$lib/components/ui/dropdown-menu";

  // The repository-level entries shared by the sidebar row's "⋯" menu and the
  // home page's: creating a worktree, choosing which ones the home page
  // lists, batch management, settings (home page only) and details. An
  // unavailable repository offers only settings and details, where it can be
  // removed from the list.
  let {
    unavailable = false,
    onnewworktree,
    onhome,
    onmanage,
    onsettings,
    ondetails,
  }: {
    unavailable?: boolean;
    onnewworktree: () => void;
    onhome: () => void;
    onmanage: () => void;
    onsettings?: () => void;
    ondetails: () => void;
  } = $props();
</script>

{#if !unavailable}
  <DropdownMenu.Item onSelect={onnewworktree}>新建工作树…</DropdownMenu.Item>
  <DropdownMenu.Item onSelect={onhome}>主页显示…</DropdownMenu.Item>
  <DropdownMenu.Item onSelect={onmanage}>管理工作树…</DropdownMenu.Item>
{/if}
{#if onsettings}
  <DropdownMenu.Item onSelect={onsettings}>仓库设置…</DropdownMenu.Item>
{/if}
<DropdownMenu.Item onSelect={ondetails}>仓库详情…</DropdownMenu.Item>
