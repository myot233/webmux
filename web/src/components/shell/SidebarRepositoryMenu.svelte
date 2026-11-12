<script lang="ts">
  import RepositoryMenuItems from "../workspace/RepositoryMenuItems.svelte";
  import SidebarRowMenu from "./SidebarRowMenu.svelte";

  // The "⋯" action menu on a repository row in the workbench tree.
  let {
    label,
    tabindex,
    unavailable = false,
    onnewworktree,
    onhome,
    onmanage,
    ondetails,
  }: {
    label: string;
    tabindex?: number;
    unavailable?: boolean;
    onnewworktree: () => void;
    onhome: () => void;
    onmanage: () => void;
    ondetails: () => void;
  } = $props();
</script>

<SidebarRowMenu ariaLabel={`仓库操作：${label}`} {tabindex}>
  <RepositoryMenuItems
    {unavailable}
    {onnewworktree}
    {onhome}
    {onmanage}
    {ondetails}
  />
</SidebarRowMenu>
