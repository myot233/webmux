<script lang="ts" module>
  export type { FileTarget } from "../../domains/code/code-file-actions";
</script>

<script lang="ts">
  import { errorMessage } from "../../platform/error";
  import { untrack } from "svelte";
  import { MediaQuery } from "svelte/reactivity";
  import type { z } from "zod";
  import { formatBytes } from "../../domains/code/code-image";
  import {
    buildCodeFileActions,
    cloneClipboard,
    describeCodeRemoveDetail,
    downloadCodeNode,
    dropClipboardPaths,
    makeRootFileTarget,
    rawDownloadError,
    requestCodePaste,
    requestCodeRemove,
    runNamingMutation,
    selectFileStem,
    toMenuAnchor,
    uploadCodeFilesBatch,
    type CodeFileClipboard,
    type FileTarget,
  } from "../../domains/code/code-file-actions";
  import { CodeContextSchema } from "../../contracts/schemas";
  import type { UploadProgress } from "../../hooks/shared/upload.svelte";
  import { Button } from "$lib/components/ui/button";
  import { Input } from "$lib/components/ui/input";
  import { Label } from "$lib/components/ui/label";
  import * as DropdownMenu from "$lib/components/ui/dropdown-menu";
  import * as Drawer from "$lib/components/ui/drawer";
  import { ListButton } from "$lib/components/ui/list-button";
  import { Separator } from "$lib/components/ui/separator";
  import ConfirmDialog from "../shared/ConfirmDialog.svelte";
  import FileUploadProgress from "../shared/FileUploadProgress.svelte";
  import ResponsiveDialog from "../shared/ResponsiveDialog.svelte";
  import UploadFileInput from "../shared/UploadFileInput.svelte";
  import IconClipboard from "~icons/tabler/clipboard";
  import IconCopy from "~icons/tabler/copy";
  import IconDownload from "~icons/tabler/download";
  import IconEye from "~icons/tabler/eye";
  import IconFileZip from "~icons/tabler/file-zip";
  import IconFolderPlus from "~icons/tabler/folder-plus";
  import IconPencil from "~icons/tabler/pencil";
  import IconScissors from "~icons/tabler/scissors";
  import IconTrash from "~icons/tabler/trash";
  import IconUpload from "~icons/tabler/upload";

  type Clipboard = CodeFileClipboard;

  interface Props {
    id: string;
    context: z.infer<typeof CodeContextSchema> | undefined;
    includeIgnored: boolean;
    openFile: (path: string, nodeId: string) => void;
    /** After a change; removed lists paths that no longer exist. */
    changed: (removed: string[]) => void;
    /** The copy/cut clipboard to start with. */
    restore?: Clipboard;
  }

  let { id, context, includeIgnored, openFile, changed, restore }: Props =
    $props();

  const base = $derived(`/sessions/${id}/code`);
  const writable = $derived(!!context?.writableAvailable);
  let menuOpen = $state(false);
  let target = $state<FileTarget>();
  let anchor = $state<HTMLElement | { getBoundingClientRect(): DOMRect }>();
  let returnFocus: HTMLElement | undefined;
  let clipboard = $state<Clipboard | undefined>(untrack(() => restore));
  let notice = $state("");
  let error = $state("");
  let naming = $state<{ kind: "rename" | "mkdir"; node: FileTarget }>();
  let name = $state("");
  let busy = $state(false);
  let dialogError = $state("");
  let removing = $state<{ node: FileTarget; detail: string }>();
  let confirmOpen = $state(false);
  let picker = $state<UploadFileInput>();
  let pickerTarget: FileTarget | undefined;
  let progress = $state<UploadProgress | null>(null);
  let uploadController: AbortController | undefined;
  let download = $state<{ name: string; bytes: number }>();
  let downloadController: AbortController | undefined;
  const pasteReady = $derived(
    !!clipboard && clipboard.contextId === context?.contextId,
  );
  const desktop = new MediaQuery("(min-width: 640px)");
  let nameInput = $state<HTMLInputElement | null>(null);
  const rootTarget = $derived(makeRootFileTarget(context?.rootPath));
  const actions = $derived(
    buildCodeFileActions({
      node: target,
      writable,
      pasteReady,
      clipboard,
      contextId: context?.contextId ?? "",
      icons: {
        eye: IconEye,
        download: IconDownload,
        fileZip: IconFileZip,
        upload: IconUpload,
        folderPlus: IconFolderPlus,
        pencil: IconPencil,
        copy: IconCopy,
        scissors: IconScissors,
        clipboard: IconClipboard,
        trash: IconTrash,
      },
      openFile,
      save: (node, archive) => void save(node, archive),
      openUploadPicker: (node) => {
        pickerTarget = node;
        picker?.open();
      },
      startNaming,
      setClipboard: (next) => (clipboard = next),
      paste: (node) => void paste(node),
      askRemove: (node) => void askRemove(node),
    }),
  );
  $effect(() => {
    const input = nameInput;
    const current = naming;
    if (!input || !current) return;
    untrack(() => selectFileStem(input, current.kind, current.node.kind, name));
  });

  export function show(
    node: FileTarget,
    at: HTMLElement | { x: number; y: number },
    row: HTMLElement,
  ) {
    target = node;
    returnFocus = row;
    anchor = toMenuAnchor(at);
    menuOpen = true;
  }

  export function capture(): Clipboard | undefined {
    return cloneClipboard(clipboard);
  }

  export function uploadRefusal(): string {
    if (!writable) return "当前目录只读，无法上传";
    return uploadController ? "请等待当前上传完成" : "";
  }

  export function refuse(message: string) {
    notice = "";
    error = message;
  }

  async function run(action: () => Promise<string | void>) {
    error = "";
    notice = "";
    try {
      const outcome = (await action()) ?? "";
      error = "";
      notice = outcome;
    } catch (e) {
      notice = "";
      error = errorMessage(e);
    }
  }

  function forget(paths: string[]) {
    clipboard = dropClipboardPaths(clipboard, paths);
  }

  function startNaming(kind: "rename" | "mkdir", node: FileTarget) {
    naming = { kind, node };
    name = kind === "rename" ? node.name : "";
    dialogError = "";
  }

  async function submitName() {
    if (!naming || !context) return;
    busy = true;
    dialogError = "";
    try {
      const result = await runNamingMutation(
        base,
        context.contextId,
        naming.kind,
        naming.node,
        name,
        changed,
        forget,
      );
      if (result.error) {
        dialogError = result.error;
        return;
      }
      naming = undefined;
      error = "";
      notice = result.notice ?? "";
    } catch (e) {
      dialogError = errorMessage(e);
    } finally {
      busy = false;
    }
  }

  async function paste(into: FileTarget) {
    const current = clipboard;
    if (!current || !context) return;
    await run(() =>
      requestCodePaste(
        base,
        context.contextId,
        current,
        into,
        changed,
        () => (clipboard = undefined),
      ),
    );
  }

  async function askRemove(node: FileTarget) {
    await run(async () => {
      const detail = await describeCodeRemoveDetail(
        base,
        context?.contextId ?? "",
        node,
      );
      removing = { node, detail };
      confirmOpen = true;
    });
  }

  async function remove() {
    const node = removing?.node;
    confirmOpen = false;
    if (!node) return;
    await run(() =>
      requestCodeRemove(base, context?.contextId ?? "", node, forget, changed),
    );
  }

  export async function upload(files: File[], into: FileTarget) {
    if (!files.length) return "";
    uploadController?.abort();
    const controller = new AbortController();
    uploadController = controller;
    await run(() =>
      uploadCodeFilesBatch({
        base,
        contextId: context?.contextId ?? "",
        into,
        files,
        signal: controller.signal,
        onProgress: (next) => (progress = next),
        onCleanup: (storedCount) => {
          if (uploadController === controller) {
            uploadController = undefined;
            progress = null;
          }
          if (storedCount) changed([]);
        },
      }),
    );
    return error || notice;
  }

  async function save(node: FileTarget, archive: boolean) {
    downloadController?.abort();
    const controller = new AbortController();
    downloadController = controller;
    const filename = archive ? `${node.name}.zip` : node.name;
    download = { name: filename, bytes: 0 };
    error = "";
    notice = "";
    try {
      await downloadCodeNode({
        base,
        contextId: context?.contextId ?? "",
        node,
        archive,
        includeIgnored,
        signal: controller.signal,
        ownerDocument: returnFocus?.ownerDocument ?? document,
        filename,
        onProgress: (bytes) => {
          if (download) download.bytes = bytes;
        },
      });
    } catch (e) {
      if (!controller.signal.aborted) error = rawDownloadError(e);
    } finally {
      if (downloadController === controller) {
        downloadController = undefined;
        download = undefined;
      }
    }
  }

  $effect(() => () => {
    uploadController?.abort();
    downloadController?.abort();
  });
</script>

<UploadFileInput
  bind:this={picker}
  upload={async (files) => {
    if (pickerTarget) await upload(files, pickerTarget);
  }}
  label="选择上传到此目录的文件"
/>

{#if progress}<FileUploadProgress
    variant="card"
    {progress}
    cancel={() => uploadController?.abort()}
  />{/if}
{#if download}<div
    class="flex shrink-0 items-center gap-2 px-2 py-1 text-xs"
    role="status"
  >
    <span class="min-w-0 flex-1 truncate"
      >正在{download.name.endsWith(".zip") ? "压缩并" : ""}下载 {download.name}
      · {formatBytes(download.bytes)}</span
    >
    <Button
      variant="ghost"
      size="xs"
      onclick={() => downloadController?.abort()}>取消</Button
    >
  </div>{/if}
{#if error}<p
    role="alert"
    class="shrink-0 px-2 py-1 text-xs break-all text-destructive"
  >
    {error}
  </p>{:else if notice}<p
    role="status"
    class="shrink-0 px-2 py-1 text-xs break-all text-muted-foreground"
  >
    {notice}
  </p>{/if}

{#if pasteReady && clipboard}<div
    class="flex shrink-0 items-center gap-1 border-y bg-muted/50 px-2 py-1 text-xs"
    role="status"
    aria-label="剪贴板"
  >
    <span class="min-w-0 flex-1 truncate" title="在目标文件夹的菜单中选择“粘贴”"
      >{clipboard.mode === "move" ? "已剪切" : "已复制"}
      {clipboard.nodes.map((n) => n.name).join("、")}</span
    >
    <Button
      variant="ghost"
      size="xs"
      disabled={!writable}
      onclick={() => void paste(rootTarget)}>粘贴到根目录</Button
    >
    <Button variant="ghost" size="xs" onclick={() => (clipboard = undefined)}
      >清除</Button
    >
  </div>{/if}

{#if desktop.current}
  <DropdownMenu.Root bind:open={menuOpen}>
    <DropdownMenu.Content
      customAnchor={anchor}
      align="start"
      class="w-56"
      aria-label={target ? `文件操作 ${target.name}` : "文件操作"}
      onCloseAutoFocus={(event) => {
        event.preventDefault();
        returnFocus?.focus();
      }}
    >
      {#each actions as action, index (action.label)}
        {#if index > 0 && action.group !== actions[index - 1]?.group}<DropdownMenu.Separator
          />{/if}
        <DropdownMenu.Item
          variant={action.destructive ? "destructive" : "default"}
          disabled={action.disabled}
          onSelect={action.run}
          >{#if action.icon}<action.icon aria-hidden="true" />{/if}<span
            class="truncate">{action.label}</span
          ></DropdownMenu.Item
        >
      {/each}
    </DropdownMenu.Content>
  </DropdownMenu.Root>
{:else}
  <Drawer.Root bind:open={menuOpen}>
    <Drawer.Content
      aria-label={target ? `文件操作 ${target.name}` : "文件操作"}
    >
      <Drawer.Header>
        <Drawer.Title>{target?.name ?? ""}</Drawer.Title>
        <Drawer.Description
          >{target?.path
            ? `${context?.rootPath ?? ""}/${target.path}`
            : (context?.rootPath ?? "")}</Drawer.Description
        >
      </Drawer.Header>
      <div class="flex min-h-0 flex-col overflow-y-auto px-2 pb-safe-4">
        {#each actions as action, index (action.label)}
          {#if index > 0 && action.group !== actions[index - 1]?.group}<div
              class="px-3 py-1"
            >
              <Separator />
            </div>{/if}
          <ListButton
            variant="action-sheet"
            data-destructive={action.destructive}
            disabled={action.disabled}
            onclick={() => {
              menuOpen = false;
              action.run();
            }}
            >{#if action.icon}<action.icon aria-hidden="true" />{/if}<span
              class="min-w-0 truncate">{action.label}</span
            ></ListButton
          >
        {/each}
        <Button
          variant="outline"
          class="mt-2 w-full"
          onclick={() => (menuOpen = false)}>取消</Button
        >
      </div>
    </Drawer.Content>
  </Drawer.Root>
{/if}

{#if naming}
  {@const { kind, node } = naming}
  <ResponsiveDialog
    title={kind === "rename" ? "重命名" : "新建文件夹"}
    description={kind === "rename"
      ? `在原目录中重命名 ${node.path}；名称已被占用时自动追加 (n)。`
      : `在 ${node.path || "根目录"} 中新建文件夹；名称已被占用时自动追加 (n)。`}
    onclose={() => (naming = undefined)}
    {busy}
  >
    <form
      class="space-y-4"
      onsubmit={(event) => {
        event.preventDefault();
        void submitName();
      }}
    >
      <div class="space-y-2">
        <Label for="code-file-name">名称</Label>
        <Input
          id="code-file-name"
          bind:ref={nameInput}
          bind:value={name}
          required
          autocomplete="off"
          autocapitalize="off"
          autocorrect="off"
          spellcheck={false}
        />
      </div>
      {#if dialogError}<p
          role="alert"
          class="text-sm break-all text-destructive"
        >
          {dialogError}
        </p>{/if}
      <div class="flex justify-end gap-2">
        <Button
          type="button"
          variant="outline"
          disabled={busy}
          onclick={() => (naming = undefined)}>取消</Button
        >
        <Button type="submit" disabled={busy || !name}
          >{kind === "rename" ? "重命名" : "新建"}</Button
        >
      </div>
    </form>
  </ResponsiveDialog>
{/if}

<ConfirmDialog
  bind:open={confirmOpen}
  title={`删除 ${removing?.node.name ?? ""}`}
  onconfirm={() => void remove()}
>
  {removing?.detail ?? ""}
</ConfirmDialog>
