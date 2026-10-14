<script lang="ts">
  import { onMount, tick, untrack, type Component } from "svelte";
  import {
    IconArrowLeft,
    IconMaximize,
    IconMinimize,
    IconUpload,
  } from "@tabler/icons-svelte";
  import { apiBuffer, navigate, statusText, uploadFile } from "../api";
  import { useTerminal } from "../hooks/terminal.svelte";
  import { useViewport } from "../hooks/viewport.svelte";
  import { Button } from "$lib/components/ui/button";
  import { Textarea } from "$lib/components/ui/textarea";
  import { Label } from "$lib/components/ui/label";
  import { Slider } from "$lib/components/ui/slider";
  import * as Dialog from "$lib/components/ui/dialog";
  import * as DropdownMenu from "$lib/components/ui/dropdown-menu";
  import * as Resizable from "$lib/components/ui/resizable";
  import SoftKeys from "../components/SoftKeys.svelte";
  import CloseSessionDialog from "../components/CloseSessionDialog.svelte";

  let { id }: { id: string } = $props();
  let container: HTMLDivElement;
  let fileInput: HTMLInputElement;
  let dragging = $state(false);
  let upload = $state<{
    name: string;
    percent: number;
    index: number;
    total: number;
  } | null>(null);
  let uploadController: AbortController | undefined;
  let error = $state("");
  let closing = $state(false);
  let pasteOpen = $state(false);
  let pasteText = $state("");
  let copyOpen = $state(false);
  let copyText = $state("");
  let settingsOpen = $state(false);
  let fullscreen = $state(false);
  let fullscreenBusy = $state(false);
  let ownsFullscreen = false;
  let mounted = false;
  let codeShown = $state(false);
  let desktop = $state(matchMedia("(min-width:1024px)").matches);
  let CodePanel = $state<
    Component<{
      id: string;
      visible: boolean;
      connection: string;
      paste: (text: string) => boolean;
    }>
  >();
  let loadingCode = false;
  const terminalVisible = () => desktop || !codeShown;
  $effect(() => {
    if (codeShown && !CodePanel && !loadingCode) {
      loadingCode = true;
      void import("../components/CodePanel.svelte")
        .then((m) => {
          if (mounted) CodePanel = m.default;
        })
        .catch((e) => {
          error = (e as Error).message;
        })
        .finally(() => (loadingCode = false));
    }
    const showing = terminalVisible();
    if (showing) void tick().then(() => t.fit());
  });
  function insertReference(text: string) {
    const result = t.paste(text);
    if (result && !desktop) {
      codeShown = false;
      void tick().then(() => t.fit());
    }
    return result;
  }
  const viewport = useViewport();
  const t = useTerminal(
    () => container,
    untrack(() => id),
    {
      paste: () => {
        void paste();
      },
      copySelection: (text) => {
        void copyToClipboard(text);
      },
    },
    terminalVisible,
  );

  function filePaste(event: ClipboardEvent) {
    const files = Array.from(event.clipboardData?.files ?? []);
    if (!files.length) return;
    event.preventDefault();
    event.stopImmediatePropagation();
    pasteOpen = false;
    void uploadFiles(files);
  }

  onMount(() => {
    mounted = true;
    const media = matchMedia("(min-width:1024px)");
    const layout = () => {
      desktop = media.matches;
    };
    media.addEventListener("change", layout);
    const changed = () => {
      fullscreen = Boolean(document.fullscreenElement);
      if (!fullscreen) ownsFullscreen = false;
    };
    changed();
    document.addEventListener("fullscreenchange", changed);
    // Capture before xterm consumes paste/drop events; text still uses xterm.
    let dragDepth = 0;
    const dragEnter = (event: DragEvent) => {
      if (!event.dataTransfer?.types.includes("Files")) return;
      event.preventDefault();
      dragging = true;
      dragDepth++;
    };
    const dragOver = (event: DragEvent) => {
      if (!event.dataTransfer?.types.includes("Files")) return;
      event.preventDefault();
      event.dataTransfer.dropEffect = upload ? "none" : "copy";
    };
    const dragLeave = () => {
      if (--dragDepth <= 0) {
        dragDepth = 0;
        dragging = false;
      }
    };
    const drop = (event: DragEvent) => {
      if (!event.dataTransfer?.types.includes("Files")) return;
      event.preventDefault();
      event.stopImmediatePropagation();
      dragDepth = 0;
      dragging = false;
      void uploadFiles(Array.from(event.dataTransfer.files));
    };
    container.addEventListener("paste", filePaste, true);
    container.addEventListener("dragenter", dragEnter, true);
    container.addEventListener("dragover", dragOver, true);
    container.addEventListener("dragleave", dragLeave, true);
    container.addEventListener("drop", drop, true);
    return () => {
      media.removeEventListener("change", layout);
      mounted = false;
      uploadController?.abort();
      container.removeEventListener("paste", filePaste, true);
      container.removeEventListener("dragenter", dragEnter, true);
      container.removeEventListener("dragover", dragOver, true);
      container.removeEventListener("dragleave", dragLeave, true);
      container.removeEventListener("drop", drop, true);
      document.removeEventListener("fullscreenchange", changed);
      if (
        ownsFullscreen &&
        document.fullscreenElement === document.documentElement
      ) {
        ownsFullscreen = false;
        void document.exitFullscreen().catch((err) => {
          console.error("离开终端时退出全屏失败", err);
        });
      }
    };
  });

  async function toggleFullscreen() {
    if (fullscreenBusy) return;
    error = "";
    if (
      !document.fullscreenElement &&
      (!document.fullscreenEnabled ||
        typeof document.documentElement.requestFullscreen !== "function")
    ) {
      error =
        "当前浏览器环境未开放网页全屏；可尝试“添加到主屏幕”以独立模式打开，减少浏览器底栏。";
      return;
    }
    fullscreenBusy = true;
    try {
      if (document.fullscreenElement) {
        await document.exitFullscreen();
      } else {
        ownsFullscreen = true;
        // Fullscreen the document so body-level menus and dialogs remain visible.
        await document.documentElement.requestFullscreen({
          navigationUI: "hide",
        });
        if (
          !mounted &&
          ownsFullscreen &&
          document.fullscreenElement === document.documentElement
        ) {
          ownsFullscreen = false;
          await document.exitFullscreen();
        }
      }
    } catch (err) {
      if (!document.fullscreenElement) ownsFullscreen = false;
      if (mounted)
        error =
          "切换全屏失败：" + (err instanceof Error ? err.message : String(err));
      else console.error("切换全屏失败", err);
    } finally {
      if (mounted) fullscreenBusy = false;
    }
  }

  async function copyToClipboard(text: string) {
    try {
      await navigator.clipboard.writeText(text);
    } catch {
      copyText = text;
      copyOpen = true;
    }
  }
  async function copy() {
    try {
      await copyToClipboard(new TextDecoder().decode(await apiBuffer(id)));
    } catch (err) {
      error = (err as Error).message;
    }
  }
  async function uploadFiles(files: File[]) {
    if (!files.length) return;
    if (upload) {
      error = "正在上传，请等待当前上传完成。";
      return;
    }
    if (t.connection !== "connected" || t.exitCode !== null) {
      error = "终端未连接，无法上传文件。";
      return;
    }
    error = "";
    const controller = new AbortController();
    uploadController = controller;
    try {
      for (const [index, file] of files.entries()) {
        upload = {
          name: file.name,
          percent: 0,
          index: index + 1,
          total: files.length,
        };
        const { path } = await uploadFile(
          id,
          file,
          (percent) => {
            if (upload) upload.percent = percent;
          },
          controller.signal,
        );
        if (!mounted || controller.signal.aborted) return;
        if (t.connection !== "connected" || t.exitCode !== null)
          throw new Error(`文件已上传至 ${path}，但终端已断开，路径未发送。`);
        // Quote arbitrary filenames safely. No newline: uploading never runs a command.
        t.paste("'" + path.replaceAll("'", "'\\''") + "' ");
      }
      t.focus();
    } catch (err) {
      if (mounted) error = (err as Error).message;
    } finally {
      uploadController = undefined;
      if (mounted) upload = null;
    }
  }

  async function paste() {
    try {
      if (
        matchMedia("(pointer: fine)").matches &&
        typeof navigator.clipboard.read === "function"
      ) {
        const items = await navigator.clipboard.read();
        if (!mounted) return;
        const files: File[] = [];
        for (const [index, item] of items.entries()) {
          const type = item.types.find((type) => !type.startsWith("text/"));
          if (!type) continue;
          const blob = await item.getType(type);
          const extension =
            type === "image/jpeg"
              ? "jpg"
              : type.split("/")[1].replace(/[^a-z0-9]/gi, "") || "bin";
          files.push(
            new File([blob], `clipboard-${Date.now()}-${index}.${extension}`, {
              type,
            }),
          );
        }
        if (files.length) {
          await uploadFiles(files);
          return;
        }
        const textItem = items.find((item) =>
          item.types.includes("text/plain"),
        );
        if (textItem) {
          const text = await (await textItem.getType("text/plain")).text();
          if (mounted) t.paste(text);
          return;
        }
      }
      const text = await navigator.clipboard.readText();
      if (mounted) t.paste(text);
    } catch {
      if (mounted) {
        pasteText = "";
        pasteOpen = true;
      }
    }
  }
  async function focusTerminal(event: Event) {
    event.preventDefault();
    await tick();
    t.focus();
  }
</script>

<div class="terminal-page" style:height={`${viewport.height}px`}>
  <header
    class="terminal-header flex h-11 shrink-0 items-center gap-2 border-b bg-card px-2"
  >
    <Button
      variant="ghost"
      size="icon-sm"
      aria-label="返回会话列表"
      onclick={() => navigate("/")}
    >
      <IconArrowLeft class="size-4" aria-hidden="true" />
    </Button>
    <span class="min-w-0 flex-1 truncate text-sm font-medium"
      >{t.info?.title || "终端"}</span
    >
    {#if t.info}<span
        class={`status-dot status-${t.info.status}`}
        title={statusText[t.info.status]}
      ></span>{/if}
    <span class="text-xs text-muted-foreground" role="status"
      >{t.connection === "connected"
        ? "已连接"
        : t.connection === "exited"
          ? "已退出"
          : t.connection === "missing"
            ? "不存在"
            : "重连中"}</span
    >
    <Button
      variant="ghost"
      size="icon-sm"
      aria-label={fullscreen ? "退出全屏" : "全屏"}
      title={fullscreen ? "退出全屏" : "全屏"}
      aria-pressed={fullscreen}
      aria-busy={fullscreenBusy}
      disabled={fullscreenBusy}
      onclick={() => {
        void toggleFullscreen();
      }}
    >
      {#if fullscreen}<IconMinimize class="size-4" />
      {:else}<IconMaximize class="size-4" />{/if}
    </Button>
    <Button
      variant="ghost"
      size="icon-sm"
      aria-label="上传文件"
      title="上传文件"
      disabled={upload !== null ||
        t.connection !== "connected" ||
        t.exitCode !== null}
      onclick={() => fileInput.click()}
      ><IconUpload class="size-4" aria-hidden="true" /></Button
    >
    <input
      bind:this={fileInput}
      type="file"
      class="hidden"
      aria-label="选择上传文件"
      onchange={() => {
        const files = Array.from(fileInput.files ?? []);
        fileInput.value = "";
        void uploadFiles(files);
      }}
    />
    <Button
      variant="outline"
      size="sm"
      aria-pressed={codeShown}
      onclick={() => (codeShown = !codeShown)}
      >{codeShown ? (desktop ? "折叠代码" : "终端") : "代码"}</Button
    >
    <DropdownMenu.Root>
      <DropdownMenu.Trigger>
        {#snippet child({ props })}<Button
            {...props}
            variant="ghost"
            size="icon-sm"
            aria-label="终端菜单">⋯</Button
          >{/snippet}
      </DropdownMenu.Trigger>
      <DropdownMenu.Content align="end">
        <DropdownMenu.Item
          onSelect={() => {
            void copy();
          }}>复制全部缓冲</DropdownMenu.Item
        >
        <DropdownMenu.Item
          onSelect={() => {
            void paste();
          }}>粘贴</DropdownMenu.Item
        >
        <DropdownMenu.Item onSelect={() => t.clear()}>清屏</DropdownMenu.Item>
        <DropdownMenu.Item onSelect={() => t.resync()}
          >重新同步</DropdownMenu.Item
        >
        <DropdownMenu.Item
          onSelect={() => {
            settingsOpen = true;
          }}>字体大小</DropdownMenu.Item
        >
        <DropdownMenu.Separator />
        <DropdownMenu.Item
          variant="destructive"
          disabled={!t.info}
          onSelect={() => {
            closing = true;
          }}>{t.exitCode !== null ? "移除会话" : "关闭会话"}</DropdownMenu.Item
        >
      </DropdownMenu.Content>
    </DropdownMenu.Root>
  </header>
  {#if upload}
    <div
      class="shrink-0 space-y-1 border-b bg-muted px-3 py-2"
      aria-label="文件上传状态"
    >
      <div class="flex items-center gap-3 text-xs" role="status">
        <span class="min-w-0 flex-1 truncate"
          >正在上传 {upload.name}{upload.total > 1
            ? `（${upload.index}/${upload.total}）`
            : ""}</span
        >
        <span>{upload.percent}%</span>
        <Button
          variant="ghost"
          size="sm"
          onclick={() => uploadController?.abort()}>取消</Button
        >
      </div>
      <progress
        aria-label="文件上传进度"
        max={100}
        value={upload.percent}
        class="block h-1 w-full accent-primary"
      ></progress>
    </div>
  {/if}
  {#if error || t.error}<p
      role="alert"
      class="shrink-0 bg-destructive/10 px-4 py-2 text-sm text-destructive"
    >
      {error || t.error}
    </p>{/if}
  {#if t.connection === "missing"}<div class="notice-warning">
      会话不存在或已被移除。<Button
        variant="outline"
        size="sm"
        onclick={() => navigate("/")}>返回列表</Button
      >
    </div>{/if}
  {#if t.exitCode !== null}
    <div
      class="flex shrink-0 flex-wrap items-center gap-3 border-b bg-muted px-4 py-2 text-sm"
      role="status"
    >
      <span>进程已退出（code {t.exitCode}）</span>
      <Button variant="outline" size="sm" onclick={() => navigate("/")}
        >返回列表</Button
      >
      <Button
        variant="outline"
        size="sm"
        onclick={() => {
          closing = true;
        }}>移除会话</Button
      >
    </div>
  {/if}
  <div
    class="terminal-workspace"
    class:code-shown={codeShown}
    class:mobile-code={!desktop && codeShown}
  >
    <Resizable.PaneGroup direction="horizontal">
      <Resizable.Pane defaultSize={50} minSize={25} class="terminal-pane">
        <div class="terminal-column">
          <div class="terminal-area">
            <div class="terminal-host" bind:this={container}></div>
            {#if dragging}
              <div
                class="pointer-events-none absolute inset-0 z-20 flex items-center justify-center border-2 border-dashed border-primary bg-background/90"
                role="status"
              >
                <p class="text-sm">
                  {upload ? "请等待当前上传完成" : "松开以上传文件到临时目录"}
                </p>
              </div>
            {/if}
            {#if t.selection}
              <div
                class="absolute right-2 top-2 z-10 flex items-center gap-1 rounded-md border bg-popover p-1 shadow-md"
                role="toolbar"
                aria-label="终端文字选择"
              >
                <Button size="sm" onclick={() => t.copySelection()}
                  >复制选中</Button
                >
                <Button
                  variant="ghost"
                  size="sm"
                  onclick={() => t.clearSelection()}>取消选择</Button
                >
              </div>
            {/if}
            {#if t.restoring && t.connection !== "missing"}<div
                class="restore-overlay"
                role="status"
              >
                {t.connection === "connected" ? "正在恢复…" : "正在连接…"}
              </div>{/if}
          </div>
          <SoftKeys
            send={t.send}
            modifiers={t.modifiers}
            toggle={t.toggleModifier}
            paste={() => {
              void paste();
            }}
            keyboard={t.keyboard}
          />
        </div>
      </Resizable.Pane>
      <Resizable.Handle
        withHandle
        class="code-divider"
        aria-label="调整终端与代码宽度"
      />
      <Resizable.Pane defaultSize={50} minSize={25} class="code-pane">
        {#if CodePanel}<CodePanel
            {id}
            visible={codeShown}
            connection={t.connection}
            paste={insertReference}
          />{/if}
      </Resizable.Pane>
    </Resizable.PaneGroup>
  </div>
</div>

<Dialog.Root bind:open={pasteOpen}>
  <Dialog.Content class="sm:max-w-lg" onCloseAutoFocus={focusTerminal}>
    <Dialog.Header
      ><Dialog.Title>粘贴到终端</Dialog.Title><Dialog.Description
        >当前环境无法直接读取剪贴板，请在下方粘贴。多行文本遵循应用的 bracketed
        paste 模式。</Dialog.Description
      ></Dialog.Header
    >
    <Textarea
      aria-label="粘贴内容"
      onpaste={filePaste}
      bind:value={pasteText}
      class="min-h-40 font-mono"
      autocapitalize="off"
      autocorrect="off"
      spellcheck={false}
    />
    <Dialog.Footer
      ><Button
        variant="outline"
        onclick={() => {
          pasteOpen = false;
        }}>取消</Button
      ><Button
        onclick={() => {
          t.paste(pasteText);
          pasteOpen = false;
        }}>发送</Button
      ></Dialog.Footer
    >
  </Dialog.Content>
</Dialog.Root>
<Dialog.Root bind:open={copyOpen}>
  <Dialog.Content class="sm:max-w-lg" onCloseAutoFocus={focusTerminal}>
    <Dialog.Header
      ><Dialog.Title>复制终端文字</Dialog.Title><Dialog.Description
        >当前环境不支持剪贴板访问，请长按或全选复制。</Dialog.Description
      ></Dialog.Header
    >
    <Textarea
      aria-label="复制内容"
      readonly
      value={copyText}
      class="min-h-40 font-mono"
    />
    <Dialog.Footer
      ><Button
        onclick={() => {
          copyOpen = false;
        }}>关闭</Button
      ></Dialog.Footer
    >
  </Dialog.Content>
</Dialog.Root>
<Dialog.Root bind:open={settingsOpen}>
  <Dialog.Content onCloseAutoFocus={focusTerminal}>
    <Dialog.Header
      ><Dialog.Title>字体大小</Dialog.Title><Dialog.Description
        >设置保存在此设备上，也可以用双指捏合调整。</Dialog.Description
      ></Dialog.Header
    >
    <Label for="terminal-font">{t.fontSize}px</Label>
    <Slider
      id="terminal-font"
      type="single"
      min={12}
      max={20}
      step={1}
      value={t.fontSize}
      onValueChange={t.setFontSize}
      aria-label="字体大小"
    />
  </Dialog.Content>
</Dialog.Root>
{#if closing && t.info}<CloseSessionDialog
    session={t.info}
    onclose={() => {
      closing = false;
    }}
    onremoved={() => navigate("/")}
  />{/if}
