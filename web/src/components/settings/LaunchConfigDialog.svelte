<script lang="ts">
  import { errorMessage } from "../../platform/error";
  import { untrack } from "svelte";
  import EnvRowsEditor from "./EnvRowsEditor.svelte";
  import LaunchConfigBehaviorFields from "./LaunchConfigBehaviorFields.svelte";
  import { Button } from "$lib/components/ui/button";
  import { Checkbox } from "$lib/components/ui/checkbox";
  import { Input } from "$lib/components/ui/input";
  import { Label } from "$lib/components/ui/label";
  import { Textarea } from "$lib/components/ui/textarea";
  import * as Dialog from "$lib/components/ui/dialog";
  import * as Select from "$lib/components/ui/select";
  import ConnectorInstallHint from "../sessions/ConnectorInstallHint.svelte";
  import InlineNotice from "../shared/InlineNotice.svelte";
  import LaunchConfigIcon from "./LaunchConfigIcon.svelte";
  import { api, APIError } from "../../platform/api";
  import {
    ChatSupportSchema,
    LaunchConfigSchema,
    type AttentionSettings,
    type ChatSupport,
    type LaunchConfig,
    type PreviewSettings,
  } from "../../contracts/schemas";
  import {
    connectorNoticeText,
    chatSupportReason,
  } from "../../domains/sessions/hybrid";
  import {
    LAUNCH_ENV_MAX,
    LAUNCH_ICON_OPTIONS,
    launchConfigIcon,
    launchCommandDraft,
    validateLaunchDraft,
    type EnvRow,
    type StartupShell,
  } from "../../domains/settings/launch-configs";

  let {
    open = $bindable(false),
    configId,
    defaultShell = "",
    onsaved,
  }: {
    open: boolean;
    /** Config being edited; undefined creates a new one. */
    configId: string | undefined;
    /** Startup file shell to preselect; "" falls back to Bash. */
    defaultShell?: "" | StartupShell;
    onsaved: (config: LaunchConfig) => void;
  } = $props();

  let loadedConfig = $state<LaunchConfig | null>(null);
  let name = $state("");
  let icon = $state("auto");
  let command = $state("");
  let loadStartup = $state(false);
  let startupShell = $state<StartupShell>("bash");
  let rows = $state<EnvRow[]>([]);
  let preview = $state<PreviewSettings | null>(null);
  let attention = $state<AttentionSettings | null>(null);
  let nextKey = 0;
  let loading = $state(false);
  let loadError = $state("");
  let error = $state("");
  let busy = $state(false);
  let hybrid = $state(false);
  // The server decides which commands can run as hybrid sessions; the
  // checkbox only reflects its answer for the command being edited.
  let hybridSupport = $state<ChatSupport | null>(null);
  const hybridReason = $derived(chatSupportReason(hybridSupport));
  // The argv (as JSON) that hybridSupport answers; not rendered.
  let supportFor = "";
  const draft = $derived(
    validateLaunchDraft(
      name,
      command,
      rows,
      loadStartup ? startupShell : null,
      preview,
      attention,
    ),
  );
  const iconOptions = [
    { value: "auto", label: "自动匹配" },
    ...LAUNCH_ICON_OPTIONS,
  ];
  const startupShellOptions: { value: StartupShell; label: string }[] = [
    { value: "bash", label: "Bash（~/.bashrc）" },
    { value: "zsh", label: "Zsh（~/.zshrc）" },
  ];
  const autoIcon = $derived(
    draft.ok ? launchConfigIcon(draft.value) : "terminal",
  );

  function row(name = "", value = ""): EnvRow {
    return { key: nextKey++, name, value };
  }

  // Each opening starts from the server copy, so cancel/Esc discards edits.
  // Editing never saves from a summary: the full config (with env values) must
  // load first, otherwise a whole-replacement PUT would drop the variables.
  $effect.pre(() => {
    if (!open) return;
    const controller = new AbortController();
    untrack(() => {
      loadedConfig = null;
      name = "";
      icon = "auto";
      command = "";
      loadStartup = false;
      startupShell = defaultShell === "zsh" ? "zsh" : "bash";
      rows = [];
      preview = null;
      attention = null;
      error = "";
      loadError = "";
      busy = false;
      hybrid = false;
      hybridSupport = null;
      supportFor = "";
      loading = !!configId;
      if (configId)
        api(
          `/launch-configs/${configId}`,
          LaunchConfigSchema,
          "GET",
          undefined,
          controller.signal,
        )
          .then((config) => {
            loadedConfig = config;
            name = config.name;
            icon = config.icon || "auto";
            const restored = launchCommandDraft(config.cmd);
            command = restored.command;
            loadStartup = restored.startupShell !== null;
            startupShell =
              restored.startupShell ??
              (defaultShell === "zsh" ? "zsh" : "bash");
            rows = Object.entries(config.env).map(([key, value]) =>
              row(key, value),
            );
            hybrid = config.chat;
            hybridSupport = config.chatSupport;
            supportFor = JSON.stringify(config.cmd);
          })
          .catch((err: unknown) => {
            if (!controller.signal.aborted)
              loadError = `无法读取启动配置：${errorMessage(err)}`;
          })
          .finally(() => {
            if (!controller.signal.aborted) loading = false;
          });
    });
    return () => controller.abort();
  });

  // Ask the server again, debounced, whenever the effective argv differs
  // from the one the current answer is for.
  const supportCmd = $derived(draft.ok ? JSON.stringify(draft.value.cmd) : "");
  $effect(() => {
    if (!open || loading || !supportCmd || supportCmd === supportFor) return;
    const asked = supportCmd;
    // supportCmd is the serialized form of this argv; read it untracked so
    // only an argv change (not other draft edits) restarts the debounce.
    const cmd = untrack(() => (draft.ok ? draft.value.cmd : null));
    if (!cmd) return;
    const controller = new AbortController();
    const timer = window.setTimeout(() => {
      api(
        "/launch-configs/chat-support",
        ChatSupportSchema,
        "POST",
        { cmd },
        controller.signal,
      )
        .then((support) => {
          hybridSupport = support;
          supportFor = asked;
        })
        .catch(() => {
          // Keep the last known answer; saving still validates on the server.
        });
    }, 300);
    return () => {
      clearTimeout(timer);
      controller.abort();
    };
  });

  async function submit(event: SubmitEvent) {
    event.preventDefault();
    if (!draft.ok || busy || loading || loadError) return;
    busy = true;
    error = "";
    try {
      const saved = await api(
        configId ? `/launch-configs/${configId}` : "/launch-configs",
        LaunchConfigSchema,
        configId ? "PUT" : "POST",
        { ...draft.value, chat: hybrid, ...(icon === "auto" ? {} : { icon }) },
      );
      onsaved(saved);
      open = false;
    } catch (err) {
      error =
        err instanceof APIError && err.status === 409
          ? `已有同名启动配置“${draft.value.name}”，请换一个名称。`
          : errorMessage(err);
    } finally {
      busy = false;
    }
  }
</script>

<Dialog.Root bind:open>
  <Dialog.Content
    class="flex max-h-[90dvh] flex-col gap-0 overflow-hidden p-0 sm:max-w-xl"
  >
    <form class="flex min-h-0 flex-1 flex-col" onsubmit={submit}>
      <div class="shrink-0 border-b px-5 py-4 pr-12">
        <Dialog.Header>
          <Dialog.Title
            >{configId ? "编辑启动配置" : "新建启动配置"}</Dialog.Title
          >
          <Dialog.Description
            >保存在服务端，所有设备的新建会话页都能选用。</Dialog.Description
          >
        </Dialog.Header>
      </div>
      <div
        class="grid min-h-0 min-w-0 flex-1 gap-4 overflow-y-auto overscroll-contain p-5"
        data-testid="launch-config-scroll"
      >
        {#if loading}
          <p role="status" class="text-sm text-muted-foreground">
            正在读取启动配置…
          </p>
        {:else if loadError}
          <InlineNotice tone="danger">{loadError}</InlineNotice>
        {:else}
          <fieldset disabled={busy} class="grid min-w-0 gap-4">
            <div class="grid gap-2">
              <Label for="launch-config-name">名称</Label>
              <Input
                id="launch-config-name"
                bind:value={name}
                placeholder="例如：Claude 恢复"
                autocomplete="off"
                aria-invalid={!!name && !!draft.errors.name}
                aria-describedby={name && draft.errors.name
                  ? "launch-config-name-error"
                  : undefined}
              />
              {#if name && draft.errors.name}<p
                  id="launch-config-name-error"
                  class="text-xs text-destructive"
                >
                  {draft.errors.name}
                </p>{/if}
            </div>
            <div class="grid gap-2">
              <Label for="launch-config-icon">图标</Label>
              <Select.Root type="single" bind:value={icon} items={iconOptions}>
                <Select.Trigger id="launch-config-icon" aria-label="图标">
                  <span class="flex items-center gap-2">
                    <LaunchConfigIcon
                      icon={icon === "auto" ? autoIcon : icon}
                    />
                    {iconOptions.find((option) => option.value === icon)
                      ?.label ?? "终端"}
                  </span>
                </Select.Trigger>
                <Select.Content>
                  {#each iconOptions as option (option.value)}
                    <Select.Item value={option.value} label={option.label}>
                      <LaunchConfigIcon
                        icon={option.value === "auto" ? autoIcon : option.value}
                      />
                      {option.label}
                    </Select.Item>
                  {/each}
                </Select.Content>
              </Select.Root>
              <p class="text-xs text-muted-foreground">
                自动匹配启动程序，也可指定图标。品牌图标来自 LobeHub。
              </p>
            </div>
            <div class="grid gap-2">
              <Label for="launch-config-command">启动命令</Label>
              <Textarea
                id="launch-config-command"
                bind:value={command}
                rows={3}
                placeholder="claude --resume 或 bash -lc 'npm run dev'"
                mono
                class="max-h-[30dvh]"
                autocapitalize="off"
                autocorrect="off"
                spellcheck={false}
                aria-invalid={!!command.trim() && !!draft.errors.command}
                aria-describedby="launch-config-command-help"
              />
              {#if command.trim() && draft.errors.command}<p
                  class="text-xs text-destructive"
                >
                  {draft.errors.command}
                </p>{/if}
              <p
                id="launch-config-command-help"
                class="text-xs text-muted-foreground"
              >
                支持引号和转义；直接执行程序，不展开 shell 变量或管道。需要
                shell 语法时，请显式使用 bash -lc '命令'。
              </p>
              <div class="flex items-center gap-2">
                <Checkbox
                  id="launch-config-startup"
                  bind:checked={loadStartup}
                  aria-describedby="launch-config-startup-help"
                />
                <Label for="launch-config-startup">加载 shell 启动文件</Label>
              </div>
              {#if loadStartup}
                <div class="grid gap-2">
                  <Label for="launch-config-startup-shell">启动文件</Label>
                  <Select.Root
                    type="single"
                    value={startupShell}
                    onValueChange={(value) => {
                      if (value === "bash" || value === "zsh")
                        startupShell = value;
                    }}
                    items={startupShellOptions}
                  >
                    <Select.Trigger
                      id="launch-config-startup-shell"
                      aria-label="启动文件 shell"
                    >
                      {startupShellOptions.find(
                        (option) => option.value === startupShell,
                      )?.label}
                    </Select.Trigger>
                    <Select.Content>
                      {#each startupShellOptions as option (option.value)}
                        <Select.Item value={option.value} label={option.label}>
                          {option.label}
                        </Select.Item>
                      {/each}
                    </Select.Content>
                  </Select.Root>
                </div>
              {/if}
              <p
                id="launch-config-startup-help"
                class="text-xs text-muted-foreground"
              >
                使用交互式 Bash（~/.bashrc）或 Zsh（~/.zshrc）读取 PATH
                和环境变量；只需填写普通命令，参数仍原样传递，不展开变量或管道。
              </p>
              <div class="flex items-center gap-2">
                <Checkbox
                  id="launch-config-hybrid"
                  bind:checked={hybrid}
                  disabled={!hybrid && !hybridSupport?.supported}
                  aria-describedby="launch-config-hybrid-help"
                />
                <Label for="launch-config-hybrid">混合模式（聊天 ↔ 终端）</Label
                >
              </div>
              <p
                id="launch-config-hybrid-help"
                class="text-xs text-muted-foreground"
              >
                {#if hybridReason}{hybridReason}{:else}从这个配置启动的会话可以在网页聊天视图与终端之间切换{hybridSupport
                    ? `，适用于 ${hybridSupport.supportedAgents.join("、")}`
                    : ""}。{/if}
              </p>
              {#if hybridSupport?.supported && hybridSupport.connector}
                <ConnectorInstallHint
                  connector={hybridSupport.connector}
                  message={connectorNoticeText(hybridSupport.connector)}
                  tone="info"
                />
              {/if}
            </div>
            <EnvRowsEditor
              bind:rows
              errors={draft.errors.rows}
              error={draft.errors.env}
              max={LAUNCH_ENV_MAX}
              makeRow={row}
            >
              {#snippet help()}变量值保存在服务端状态文件中，默认遮挡；列表与新建会话页只显示变量个数，不显示值。{/snippet}
            </EnvRowsEditor>
            <LaunchConfigBehaviorFields
              config={loadedConfig}
              bind:preview
              bind:attention
              previewError={draft.errors.preview}
              attentionError={draft.errors.attention}
            />
            {#if error}<InlineNotice tone="danger">{error}</InlineNotice>{/if}
          </fieldset>
        {/if}
      </div>
      <div class="shrink-0 border-t bg-muted/30 px-5 py-3">
        <Dialog.Footer>
          <Button
            type="button"
            variant="outline"
            onclick={() => {
              open = false;
            }}>取消</Button
          >
          {#if !loadError}<Button
              type="submit"
              disabled={loading || busy || !draft.ok}
              >{busy ? "正在保存…" : "保存"}</Button
            >{/if}
        </Dialog.Footer>
      </div>
    </form>
  </Dialog.Content>
</Dialog.Root>
