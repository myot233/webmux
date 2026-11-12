<script lang="ts">
  import IconTerminal2 from "~icons/tabler/terminal-2";
  import { joinArgs } from "../../platform/api";
  import { connectorNoticeText } from "../../domains/sessions/hybrid";
  import {
    launchConfigIcon,
    type LaunchConfigsState,
  } from "../../domains/settings/launch-configs";
  import type { LaunchSource } from "../../domains/sessions/new-session-defaults";
  import type { SessionView } from "../../contracts/schemas";
  import * as Command from "$lib/components/ui/command";
  import { Input } from "$lib/components/ui/input";
  import { Label } from "$lib/components/ui/label";
  import * as RadioGroup from "$lib/components/ui/radio-group";
  import ChoiceDetails from "../shared/ChoiceDetails.svelte";
  import LaunchConfigIcon from "../settings/LaunchConfigIcon.svelte";
  import PickerField from "../shared/PickerField.svelte";

  // The "what" row of the new-session dialog: saved launch configs and a
  // custom command in one list. Switching keeps both the chosen config and
  // the typed command; only the current source is submitted.
  let {
    launchConfigs,
    source = $bindable(),
    configId = $bindable(),
    command = $bindable(),
    startSide = $bindable(),
    disabled = false,
    onchoose,
  }: {
    launchConfigs: LaunchConfigsState;
    source: LaunchSource;
    configId: string;
    command: string;
    startSide: SessionView;
    disabled?: boolean;
    /** The user picked a source; defaults must no longer replace it. */
    onchoose: () => void;
  } = $props();
  let open = $state(false);
  let search = $state("");
  // The desktop list stays mounted while closed; each opening starts unfiltered.
  function setOpen(next: boolean) {
    open = next;
    if (!next) search = "";
  }
  const configs = $derived(
    launchConfigs.status === "ready" ? launchConfigs.configs : [],
  );
  const chosen = $derived(
    source === "config"
      ? configs.find((config) => config.id === configId)
      : undefined,
  );

  function choose(next: LaunchSource, id = configId) {
    source = next;
    configId = id;
    setOpen(false);
    onchoose();
  }
</script>

<div class="space-y-2">
  <Label for="session-launch">运行</Label>
  <PickerField id="session-launch" bind:open={() => open, setOpen} {disabled}>
    {#snippet trigger()}
      {#if chosen}
        <LaunchConfigIcon icon={launchConfigIcon(chosen)} />
        <ChoiceDetails
          primary={chosen.name}
          secondary={joinArgs(chosen.cmd)}
          envCount={chosen.envNames.length}
        />
      {:else if source === "custom"}
        <IconTerminal2
          class="size-4 shrink-0 text-muted-foreground"
          aria-hidden="true"
        />
        <ChoiceDetails primary="自定义命令" />
      {:else if launchConfigs.status === "loading"}
        <span class="text-sm text-muted-foreground">正在读取启动配置…</span>
      {:else}
        <span class="text-sm text-muted-foreground">选择启动方式</span>
      {/if}
    {/snippet}
    <Command.Root label="启动方式" class="h-auto min-w-0">
      <Command.Input
        bind:value={search}
        placeholder="搜索启动配置…"
        aria-label="搜索启动方式"
        autocomplete="off"
        autocapitalize="off"
        autocorrect="off"
        spellcheck={false}
      />
      <Command.List
        aria-label="启动方式列表"
        class="max-h-[min(20rem,50dvh)] p-1"
      >
        <Command.Empty>没有匹配的启动配置</Command.Empty>
        <Command.Group heading="启动配置" value="configs">
          {#if launchConfigs.status === "loading"}
            <p role="status" class="px-2 py-1.5 text-xs text-muted-foreground">
              正在读取启动配置…
            </p>
          {:else if launchConfigs.status === "unavailable"}
            <p class="px-2 py-1.5 text-xs text-destructive wrap-anywhere">
              启动配置暂不可用：{launchConfigs.error}
            </p>
          {:else if !configs.length}
            <p class="px-2 py-1.5 text-xs text-muted-foreground">
              还没有启动配置。<a
                class="text-foreground underline underline-offset-2"
                href="/settings#launch-configs">前往设置创建启动配置</a
              >
            </p>
          {/if}
          {#each configs as config (config.id)}
            <Command.Item
              value="config:{config.id}"
              keywords={[config.name, joinArgs(config.cmd)]}
              data-checked={chosen?.id === config.id}
              onSelect={() => choose("config", config.id)}
            >
              <LaunchConfigIcon icon={launchConfigIcon(config)} />
              <ChoiceDetails
                primary={config.name}
                secondary={joinArgs(config.cmd)}
                envCount={config.envNames.length}
              />
            </Command.Item>
          {/each}
        </Command.Group>
        <Command.Group heading="其他" value="other">
          <Command.Item
            value="custom"
            keywords={["自定义命令", "custom"]}
            data-checked={source === "custom"}
            onSelect={() => choose("custom")}
          >
            <IconTerminal2 class="text-muted-foreground" aria-hidden="true" />
            <span class="text-sm">自定义命令…</span>
          </Command.Item>
        </Command.Group>
      </Command.List>
    </Command.Root>
  </PickerField>
  {#if source === "custom"}
    <Input
      id="custom-command"
      aria-label="自定义启动命令"
      aria-describedby="custom-command-help"
      bind:value={command}
      mono
      placeholder="agent --resume 或 bash -lc 'npm run dev'"
      autocomplete="off"
      autocapitalize="off"
      autocorrect="off"
      spellcheck={false}
    />
    <p id="custom-command-help" class="text-xs text-muted-foreground">
      直接执行，不展开变量或管道；需要 shell 语法请用 bash -lc '命令'。
    </p>
  {:else if chosen?.chat}
    <RadioGroup.Root
      bind:value={
        () => startSide,
        (next) => {
          if (next === "terminal" || next === "chat") startSide = next;
        }
      }
      aria-label="起始端"
      {disabled}
    >
      <RadioGroup.Item
        value="terminal"
        aria-label="从终端开始"
        title="从终端开始"
        description="先在终端里运行，随时可切到聊天"
      />
      <RadioGroup.Item
        value="chat"
        aria-label="从聊天开始"
        title="从聊天开始"
        description="直接打开聊天界面，随时可切到终端"
      />
    </RadioGroup.Root>
    {#if chosen.chatSupport.connector}<p class="text-xs text-muted-foreground">
        {connectorNoticeText(chosen.chatSupport.connector)}。
      </p>{/if}
  {/if}
</div>
