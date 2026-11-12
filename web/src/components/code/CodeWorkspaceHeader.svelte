<script lang="ts">
  import type { Snippet } from "svelte";
  import { Button } from "$lib/components/ui/button";
  import * as DropdownMenu from "$lib/components/ui/dropdown-menu";
  import { ListButton } from "$lib/components/ui/list-button";
  import * as Tabs from "$lib/components/ui/tabs";
  import {
    safeAreaPadding,
    type CodeContext,
    type CodeWorkspaceTab,
  } from "../../domains/code/code-workspace";
  import IconDots from "~icons/tabler/dots";
  import IconLayoutSidebarLeftCollapse from "~icons/tabler/layout-sidebar-left-collapse";
  import IconLayoutSidebarLeftExpand from "~icons/tabler/layout-sidebar-left-expand";
  import IconPhoto from "~icons/tabler/photo";
  import IconPictureInPictureOff from "~icons/tabler/picture-in-picture-off";
  import IconPictureInPictureOn from "~icons/tabler/picture-in-picture-on";
  import IconRefresh from "~icons/tabler/refresh";
  import IconX from "~icons/tabler/x";

  let {
    navOpen = $bindable(),
    navStatus,
    tabs,
    context,
    pending,
    popout,
    activeMenu,
    section,
    error,
    dropStatus,
    beginOpen,
    supersedeOpenOnTabKey,
    close,
    refreshAll,
  }: {
    navOpen: boolean;
    navStatus: string;
    tabs: CodeWorkspaceTab[];
    context?: CodeContext;
    pending: boolean;
    popout?: { popped: boolean; toggle: () => void };
    activeMenu?: Snippet;
    section?: HTMLElement;
    error: string;
    dropStatus: string;
    beginOpen: () => void;
    supersedeOpenOnTabKey: (event: KeyboardEvent) => void;
    close: (key: string) => void;
    refreshAll: () => void;
  } = $props();
</script>

<div class="flex shrink-0 items-stretch border-b">
  <div class="flex shrink-0 items-center px-2 py-1">
    <Button
      variant="ghost"
      size="icon-sm"
      class="relative"
      aria-label={navOpen ? "收起导航" : "文件导航"}
      aria-expanded={navOpen}
      title={navOpen
        ? "收起导航"
        : navStatus
          ? `文件导航（${navStatus}）`
          : "文件导航"}
      onclick={() => (navOpen = !navOpen)}
      >{#if navOpen}<IconLayoutSidebarLeftCollapse
          aria-hidden="true"
        />{:else}<IconLayoutSidebarLeftExpand
          aria-hidden="true"
        />{/if}{#if navStatus && !navOpen}<span
          class="absolute top-0.5 right-0.5 size-1.5 rounded-full bg-warning"
          aria-hidden="true"
        ></span>{/if}</Button
    >
  </div>
  <Tabs.List variant="line" aria-label="文件标签">
    {#snippet child({ props })}
      <div
        {...props}
        class="flex min-w-0 flex-1 overflow-x-auto scrollbar-thin"
      >
        {#each tabs as tab (tab.key)}
          {@const path = tab.file?.document.path ?? tab.diff?.new.path ?? ""}
          {@const meta = `${tab.rootKind === "artifacts" ? "临时文件夹" : tab.diff ? tab.diff.change.group : tab.file?.document.source}${tab.status !== "unchanged" ? " ●" : ""}`}
          {@const image = [
            tab.file?.document.mediaType,
            tab.diff?.new.mediaType,
            tab.diff?.old.mediaType,
          ].some((type) => type?.startsWith("image/"))}
          <div class="flex shrink-0 items-center border-r first:border-l">
            <Tabs.Trigger
              value={tab.key}
              aria-label={`${path} · ${meta}`}
              title={tab.rootKind === "artifacts"
                ? `临时文件夹 · ${path}`
                : path}
              onclick={beginOpen}
              onkeydown={supersedeOpenOnTabKey}
            >
              {#snippet child({ props })}
                <ListButton {...props} class={undefined} variant="tab"
                  >{#if image}<IconPhoto
                      class="mr-1 inline size-3.5 align-[-0.125em]"
                      aria-hidden="true"
                    />{/if}{path.slice(path.lastIndexOf("/") + 1)} · {meta}</ListButton
                >
              {/snippet}
            </Tabs.Trigger>
            <ListButton
              variant="tab-close"
              aria-label={`关闭 ${path}`}
              title={`关闭 ${path}`}
              onclick={() => close(tab.key)}
              ><IconX aria-hidden="true" /></ListButton
            >
          </div>{/each}
      </div>
    {/snippet}
  </Tabs.List>
  <div class="flex shrink-0 items-center gap-1 px-2 py-1">
    <Button
      variant="ghost"
      size="icon-sm"
      aria-label="刷新全部"
      title={context?.rootPath ? `刷新全部（${context.rootPath}）` : "刷新全部"}
      disabled={pending}
      onclick={refreshAll}><IconRefresh aria-hidden="true" /></Button
    >
    {#if popout}<Button
        variant="ghost"
        size="icon-sm"
        class="max-sm:hidden"
        aria-label={popout.popped ? "收回代码窗口" : "在独立窗口中显示代码"}
        aria-pressed={popout.popped}
        title={popout.popped ? "收回代码窗口" : "在独立窗口中显示代码"}
        onclick={() => popout?.toggle()}
        >{#if popout.popped}<IconPictureInPictureOff
            aria-hidden="true"
          />{:else}<IconPictureInPictureOn aria-hidden="true" />{/if}</Button
      >{/if}
    {#if activeMenu}<DropdownMenu.Root>
        <DropdownMenu.Trigger>
          {#snippet child({ props })}<Button
              {...props}
              variant="ghost"
              size="icon-sm"
              class="sm:hidden"
              aria-label="代码菜单"
              title="代码菜单"><IconDots aria-hidden="true" /></Button
            >{/snippet}
        </DropdownMenu.Trigger>
        <DropdownMenu.Content
          align="end"
          collisionPadding={safeAreaPadding(section)}
          class="w-72 max-w-[calc(100vw-1rem)]"
        >
          {@render activeMenu()}
        </DropdownMenu.Content>
      </DropdownMenu.Root>{/if}
  </div>
</div>
{#if error}<p
    role="alert"
    class="shrink-0 px-2 py-1 text-sm break-all text-destructive"
  >
    {error}
  </p>{/if}
{#if context?.reason}<p
    class="shrink-0 px-2 py-1 text-xs text-muted-foreground"
  >
    {context.reason}
  </p>{/if}
{#if dropStatus && !navOpen}<p
    role="status"
    class="shrink-0 px-2 py-1 text-xs text-muted-foreground"
  >
    {dropStatus}
  </p>{/if}
