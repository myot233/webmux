<script lang="ts">
  import { SvelteSet } from "svelte/reactivity";
  import IconAdjustments from "~icons/tabler/adjustments";
  import IconCopy from "~icons/tabler/copy";
  import IconExternalLink from "~icons/tabler/external-link";
  import IconPlugConnected from "~icons/tabler/plug-connected";
  import IconTrash from "~icons/tabler/trash";
  import { Button } from "$lib/components/ui/button";
  import * as Empty from "$lib/components/ui/empty";
  import { Input } from "$lib/components/ui/input";
  import { Label } from "$lib/components/ui/label";
  import AdvancedSection from "../shared/AdvancedSection.svelte";
  import ForwardOptionsFields from "./ForwardOptionsFields.svelte";
  import ResponsiveDialog from "../shared/ResponsiveDialog.svelte";
  import { copyText } from "../../platform/clipboard";
  import { errorMessage } from "../../platform/error";
  import {
    addForward,
    changeForward,
    defaultForwardOptions,
    forwardLink,
    forwards,
    isPromptedPort,
    removeForward,
  } from "../../domains/workspace/forwards.svelte";
  import type {
    Forward,
    ForwardOptions,
    SessionInfo,
  } from "../../contracts/schemas";

  // The "端口转发" dialog: add a forward, manage the server's forwards and
  // forward ports the sessions listen on. The list itself comes from the
  // live forwards store; every change goes to the server.
  let {
    sessions,
    detection,
    onclose,
  }: {
    /** Sessions whose listening ports are offered for forwarding. */
    sessions: readonly SessionInfo[];
    /** Whether the server detects listening ports on its platform. */
    detection: boolean;
    onclose: () => void;
  } = $props();

  let port = $state("");
  let attempted = $state(false);
  let adding = $state(false);
  // The server's answer to the last submit; cleared as the port is edited.
  let addError = $state("");
  let addOptions = $state<ForwardOptions>({ ...defaultForwardOptions });
  let editingPort = $state<number | null>(null);
  let draftLabel = $state("");
  let draftOptions = $state<ForwardOptions>({ ...defaultForwardOptions });
  let copied = $state<number | null>(null);
  // Failures of row actions, keyed like `busy`: `forward:5173`, `detected:…`.
  let rowErrors = $state<Record<string, string>>({});
  const busy = new SvelteSet<string>();

  const taken = $derived(new Set(forwards.list.map((entry) => entry.port)));
  const inputError = $derived.by(() => {
    const text = port.trim();
    if (!text) return attempted ? "请输入端口号" : "";
    const value = Number(text);
    if (!/^\d{1,5}$/.test(text) || value < 1 || value > 65535)
      return "请输入 1–65535 的端口号";
    return taken.has(value) ? "该端口已转发" : "";
  });
  const shownError = $derived(inputError || addError);
  const validPort = $derived(
    port.trim() && !inputError ? Number(port.trim()) : null,
  );
  const titles = $derived(
    new Map(sessions.map((session) => [session.id, session.title])),
  );
  const detected = $derived(
    sessions
      .flatMap((session) =>
        session.listeningPorts
          .filter(
            (entry) => isPromptedPort(entry.port) && !taken.has(entry.port),
          )
          .map((entry) => ({
            ...entry,
            session: session.id,
            title: session.title || session.id,
          })),
      )
      .sort((a, b) => a.port - b.port),
  );

  function source(forward: Forward) {
    if (forward.label) return forward.label;
    const parts = [titles.get(forward.session), forward.process].filter(
      Boolean,
    );
    return parts.length ? parts.join(" · ") : "手动添加";
  }
  // Runs one row action, keeping its failure beside the row.
  async function run(key: string, action: () => Promise<unknown>) {
    if (busy.has(key)) return false;
    busy.add(key);
    delete rowErrors[key];
    try {
      await action();
      return true;
    } catch (err) {
      rowErrors[key] = errorMessage(err);
      return false;
    } finally {
      busy.delete(key);
    }
  }
  async function submit(event: SubmitEvent) {
    event.preventDefault();
    attempted = true;
    if (inputError || adding) return;
    adding = true;
    addError = "";
    try {
      await addForward({
        port: Number(port.trim()),
        options: $state.snapshot(addOptions),
      });
      port = "";
      attempted = false;
    } catch (err) {
      addError = errorMessage(err);
    } finally {
      adding = false;
    }
  }
  function toggleEdit(forward: Forward) {
    if (editingPort === forward.port) {
      editingPort = null;
      return;
    }
    editingPort = forward.port;
    draftLabel = forward.label;
    draftOptions = { ...forward.options };
    delete rowErrors[`edit:${forward.port}`];
  }
  async function save(target: number) {
    const saved = await run(`edit:${target}`, () =>
      changeForward(target, {
        label: draftLabel.trim(),
        options: $state.snapshot(draftOptions),
      }),
    );
    if (saved) editingPort = null;
  }
  async function copy(forward: Forward) {
    const link = forwardLink(forward.path);
    if (await copyText(link)) {
      copied = forward.port;
      delete rowErrors[`forward:${forward.port}`];
    } else {
      copied = null;
      rowErrors[`forward:${forward.port}`] = `复制失败，请手动复制 ${link}`;
    }
  }
</script>

<ResponsiveDialog
  title="端口转发"
  description="通过 WebMux 访问服务器本机端口。转发不会保存，服务重启后清空。"
  pinDrawerHeader
  {onclose}
>
  <div class="space-y-6">
    <form class="space-y-3" onsubmit={submit} novalidate>
      <div class="space-y-2">
        <Label for="forward-port">端口</Label>
        <div class="flex gap-2">
          <Input
            id="forward-port"
            class="max-w-40"
            inputmode="numeric"
            autocomplete="off"
            placeholder="例如 5173"
            bind:value={port}
            oninput={() => (addError = "")}
            aria-invalid={shownError ? true : undefined}
            aria-describedby="forward-port-error"
          />
          <Button type="submit" disabled={adding}>添加转发</Button>
        </div>
        <p
          id="forward-port-error"
          class="min-h-4 text-xs text-destructive"
          role={shownError ? "alert" : undefined}
        >
          {shownError}
        </p>
      </div>
      <AdvancedSection title="兼容选项" summary="默认适合 Vite 等开发服务器">
        <ForwardOptionsFields
          id="forward-new"
          port={validPort}
          bind:options={addOptions}
        />
      </AdvancedSection>
    </form>

    <section class="space-y-2" aria-labelledby="forward-list-title">
      <h3 id="forward-list-title" class="text-sm font-medium">
        已转发
        <span class="text-muted-foreground tabular-nums"
          >{forwards.list.length}</span
        >
      </h3>
      {#if forwards.list.length}
        <ul class="divide-y">
          {#each forwards.list as forward (forward.port)}
            {@const open = editingPort === forward.port}
            {@const key = `forward:${forward.port}`}
            <li
              class="grid grid-cols-[3.5rem_minmax(0,1fr)] items-center gap-x-3 gap-y-1 py-2 sm:grid-cols-[3.5rem_minmax(0,1fr)_auto]"
            >
              <span class="font-mono text-sm font-medium tabular-nums"
                >{forward.port}</span
              >
              <div class="min-w-0">
                <p class="truncate text-sm" title={source(forward)}>
                  {source(forward)}
                </p>
                <p
                  class="flex min-w-0 items-center gap-2 text-xs text-muted-foreground"
                >
                  <span
                    class={`status-dot ${forward.status === "listening" ? "status-running" : "status-idle"}`}
                    aria-hidden="true"
                  ></span>
                  <span class="shrink-0"
                    >{forward.status === "listening"
                      ? "可访问"
                      : "无法连接"}</span
                  >
                  <span class="min-w-0 truncate font-mono" title={forward.path}
                    >{forward.path}</span
                  >
                </p>
              </div>
              <div class="col-start-2 flex items-center gap-1 sm:col-start-3">
                <Button
                  href={forward.path}
                  target="_blank"
                  rel="noopener"
                  variant="ghost"
                  size="icon-touch"
                  aria-label={`在新标签页打开端口 ${forward.port}`}
                  title="在新标签页打开"
                  ><IconExternalLink aria-hidden="true" /></Button
                >
                <Button
                  variant="ghost"
                  size="icon-touch"
                  aria-label={`复制端口 ${forward.port} 的链接`}
                  title={copied === forward.port ? "已复制" : "复制链接"}
                  onclick={() => void copy(forward)}
                  ><IconCopy aria-hidden="true" /></Button
                >
                <Button
                  variant="ghost"
                  size="icon-touch"
                  aria-label={`端口 ${forward.port} 的设置`}
                  title="设置"
                  aria-expanded={open}
                  onclick={() => toggleEdit(forward)}
                  ><IconAdjustments aria-hidden="true" /></Button
                >
                <Button
                  variant="ghost"
                  size="icon-touch"
                  aria-label={`移除端口 ${forward.port} 的转发`}
                  title="移除转发"
                  disabled={busy.has(key)}
                  onclick={() =>
                    void run(key, () => removeForward(forward.port))}
                  ><IconTrash aria-hidden="true" /></Button
                >
              </div>
              {#if rowErrors[key]}<p
                  class="col-span-full text-xs text-destructive wrap-anywhere"
                  role="alert"
                >
                  {rowErrors[key]}
                </p>{/if}
              {#if open}
                {@const editKey = `edit:${forward.port}`}
                <form
                  class="col-span-full mt-2 space-y-4 rounded-lg border p-4"
                  aria-label={`端口 ${forward.port} 的设置`}
                  onsubmit={(event) => {
                    event.preventDefault();
                    void save(forward.port);
                  }}
                >
                  <div class="space-y-2">
                    <Label for={`forward-${forward.port}-label`}
                      >名称（可选）</Label
                    >
                    <Input
                      id={`forward-${forward.port}-label`}
                      maxlength={64}
                      placeholder={titles.get(forward.session) ||
                        "例如 前端预览"}
                      bind:value={draftLabel}
                    />
                  </div>
                  <ForwardOptionsFields
                    id={`forward-${forward.port}`}
                    port={forward.port}
                    bind:options={draftOptions}
                  />
                  {#if rowErrors[editKey]}<p
                      class="text-xs text-destructive wrap-anywhere"
                      role="alert"
                    >
                      {rowErrors[editKey]}
                    </p>{/if}
                  <div class="flex justify-end gap-2">
                    <Button
                      type="button"
                      variant="outline"
                      onclick={() => (editingPort = null)}>取消</Button
                    >
                    <Button type="submit" disabled={busy.has(editKey)}
                      >保存</Button
                    >
                  </div>
                </form>
              {/if}
            </li>
          {/each}
        </ul>
      {:else}
        <Empty.Root variant="outline">
          <Empty.Header>
            <Empty.Media variant="icon"
              ><IconPlugConnected aria-hidden="true" /></Empty.Media
            >
            <Empty.Title>还没有转发</Empty.Title>
            <Empty.Description
              >输入端口，或转发下方检测到的端口。</Empty.Description
            >
          </Empty.Header>
        </Empty.Root>
      {/if}
      <p class="sr-only" role="status">
        {copied === null ? "" : `已复制端口 ${copied} 的链接`}
      </p>
    </section>

    <section class="space-y-2" aria-labelledby="forward-detected-title">
      <h3 id="forward-detected-title" class="text-sm font-medium">
        检测到的端口
      </h3>
      {#if !detection}
        <p class="text-sm text-muted-foreground">
          当前平台不支持自动检测，可以手动输入端口。
        </p>
      {:else if detected.length}
        <ul class="divide-y">
          {#each detected as entry (`${entry.session}:${entry.port}`)}
            {@const key = `detected:${entry.session}:${entry.port}`}
            {@const text = [entry.process, entry.title]
              .filter(Boolean)
              .join(" · ")}
            <li
              class="grid grid-cols-[3.5rem_minmax(0,1fr)_auto] items-center gap-x-3 gap-y-1 py-2"
            >
              <span class="font-mono text-sm font-medium tabular-nums"
                >{entry.port}</span
              >
              <span
                class="min-w-0 truncate text-sm text-muted-foreground"
                title={text}>{text}</span
              >
              <Button
                variant="outline"
                size="sm-touch"
                aria-label={`转发端口 ${entry.port}`}
                disabled={busy.has(key)}
                onclick={() =>
                  void run(key, () =>
                    addForward({
                      port: entry.port,
                      session: entry.session,
                      process: entry.process,
                    }),
                  )}>转发</Button
              >
              {#if rowErrors[key]}<p
                  class="col-span-full text-xs text-destructive wrap-anywhere"
                  role="alert"
                >
                  {rowErrors[key]}
                </p>{/if}
            </li>
          {/each}
        </ul>
      {:else}
        <p class="text-sm text-muted-foreground">
          会话中没有其他正在监听的端口。
        </p>
      {/if}
    </section>

    <p class="text-xs text-muted-foreground">
      转发页面与 WebMux 同源，页面脚本可以访问 WebMux
      登录状态。只转发你信任的开发服务器。
    </p>
  </div>
</ResponsiveDialog>
