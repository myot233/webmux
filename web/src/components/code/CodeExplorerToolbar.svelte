<script lang="ts">
  import { errorMessage } from "../../platform/error";
  import type { z } from "zod";
  import { CodeContextSchema, type ReportedCwd } from "../../contracts/schemas";
  import { Button } from "$lib/components/ui/button";
  import { Input } from "$lib/components/ui/input";
  import * as Popover from "$lib/components/ui/popover";
  import * as DropdownMenu from "$lib/components/ui/dropdown-menu";
  import IconSearch from "~icons/tabler/search";
  import IconRefresh from "~icons/tabler/refresh";
  import IconFold from "~icons/tabler/fold";
  import IconDots from "~icons/tabler/dots";
  import IconFileSearch from "~icons/tabler/file-search";

  let {
    context,
    pending,
    root,
    sessionCwd,
    reportedCwd,
    query = $bindable(""),
    directory = $bindable(""),
    includeIgnored = $bindable(false),
    collapse = $bindable(0),
    load,
    reload,
    locate,
  }: {
    context: z.infer<typeof CodeContextSchema> | undefined;
    pending: boolean;
    root: "workspace" | "artifacts";
    sessionCwd: string;
    reportedCwd: ReportedCwd | null;
    query: string;
    directory: string;
    includeIgnored: boolean;
    collapse: number;
    load: (append?: boolean) => void | Promise<void>;
    reload: () => void;
    locate: (input: string) => Promise<void>;
  } = $props();

  let searchOpen = $state(false);
  let searchInput = $state("");
  let locateOpen = $state(false);
  let locateInput = $state("");
  let locateError = $state("");
  const locateBase = $derived.by(() => {
    // The artifacts root resolves relative paths against the folder itself.
    if (root === "artifacts")
      return context ? { path: context.rootPath, label: "临时文件夹" } : null;
    if (reportedCwd?.remote) return null;
    if (reportedCwd) return { path: reportedCwd.path, label: "Shell 报告目录" };
    return sessionCwd
      ? { path: sessionCwd, label: "启动目录（Shell 未报告）" }
      : null;
  });
  const locateHint = $derived(
    reportedCwd?.remote && root !== "artifacts"
      ? `Shell 报告的是远端目录（${reportedCwd.host}），不能用于定位本机文件。`
      : locateBase
        ? `基点：${locateBase.label} ${locateBase.path}`
        : "暂无可用基点。",
  );
  let locating = $state(false);
  async function submitLocate(event: SubmitEvent) {
    event.preventDefault();
    locateError = "";
    if (!context || !locateBase || locating) return;
    locating = true;
    try {
      await locate(locateInput);
      locateInput = "";
      locateOpen = false;
    } catch (err) {
      locateError = errorMessage(err);
    } finally {
      locating = false;
    }
  }
</script>

<div class="flex shrink-0 items-center gap-0.5 px-2 py-1">
  <span
    class="min-w-0 flex-1 truncate text-xs font-medium text-muted-foreground"
    >资源管理器</span
  >
  <Button
    variant="ghost"
    size="icon-xs"
    aria-label="切换路径搜索"
    title="搜索文件路径"
    aria-expanded={searchOpen}
    onclick={() => {
      searchOpen = !searchOpen;
      if (!searchOpen && query) {
        query = "";
        void load();
      }
    }}><IconSearch aria-hidden="true" /></Button
  >
  <Popover.Root bind:open={locateOpen}>
    <Popover.Trigger>
      {#snippet child({ props })}<Button
          {...props}
          variant="ghost"
          size="icon-xs"
          aria-label="按路径打开"
          title="按路径打开"><IconFileSearch aria-hidden="true" /></Button
        >{/snippet}
    </Popover.Trigger>
    <Popover.Content align="start" class="w-72">
      <Popover.Header
        ><Popover.Title>按路径打开</Popover.Title><Popover.Description
          >{locateHint}</Popover.Description
        ></Popover.Header
      >
      <form class="mt-2 flex flex-col gap-2" onsubmit={submitLocate}>
        <Input
          size="sm"
          aria-label="定位文件路径"
          placeholder="相对路径，如 src/main.go"
          bind:value={locateInput}
          disabled={!locateBase}
        />
        <Button
          size="sm"
          variant="outline"
          type="submit"
          disabled={!locateBase || locating || !locateInput.trim()}>打开</Button
        >
      </form>
      {#if locateError}<p
          role="alert"
          class="mt-1 break-all text-xs text-destructive"
        >
          {locateError}
        </p>{/if}
    </Popover.Content>
  </Popover.Root>
  <Button
    variant="ghost"
    size="icon-xs"
    aria-label="刷新文件树"
    title="刷新文件树"
    disabled={pending}
    onclick={reload}><IconRefresh aria-hidden="true" /></Button
  >
  <Button
    variant="ghost"
    size="icon-xs"
    aria-label="全部折叠"
    title="全部折叠"
    onclick={() => {
      collapse++;
      query = "";
      searchInput = "";
      directory = "";
      void load();
    }}><IconFold aria-hidden="true" /></Button
  >
  <DropdownMenu.Root>
    <DropdownMenu.Trigger>
      {#snippet child({ props })}<Button
          {...props}
          variant="ghost"
          size="icon-xs"
          aria-label="文件导航选项"
          title="文件导航选项"><IconDots aria-hidden="true" /></Button
        >{/snippet}
    </DropdownMenu.Trigger>
    <DropdownMenu.Content align="end">
      <DropdownMenu.CheckboxItem
        closeOnSelect
        checked={includeIgnored}
        disabled={!context?.ignoreAvailable}
        onCheckedChange={(checked) => {
          includeIgnored = checked;
          void load();
        }}>显示忽略项</DropdownMenu.CheckboxItem
      >
    </DropdownMenu.Content>
  </DropdownMenu.Root>
</div>
{#if searchOpen}
  <form
    class="flex shrink-0 gap-1 px-2 pb-2"
    onsubmit={(event) => {
      event.preventDefault();
      query = searchInput;
      directory = "";
      void load();
    }}
  >
    <Input
      size="sm"
      aria-label="搜索文件路径"
      placeholder="文件名或路径"
      bind:value={searchInput}
    />
    <Button size="sm" variant="outline" type="submit">搜索路径</Button>
  </form>
{/if}
{#if includeIgnored}<p class="px-2 pb-1 text-xs text-muted-foreground">
    包含忽略项
  </p>{/if}
