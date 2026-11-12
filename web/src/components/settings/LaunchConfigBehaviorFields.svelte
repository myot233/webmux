<script lang="ts">
  import { tick } from "svelte";
  import { Checkbox } from "$lib/components/ui/checkbox";
  import { Input } from "$lib/components/ui/input";
  import { Label } from "$lib/components/ui/label";
  import * as SegmentedControl from "$lib/components/ui/segmented-control";
  import {
    defaultPreviewSettings,
    previewSettings,
  } from "../../domains/settings/preview-settings.svelte";
  import type {
    AttentionMode,
    AttentionSettings,
    LaunchConfig,
    PreviewSettings,
  } from "../../contracts/schemas";
  import {
    defaultLaunchAttentionSettings,
    LAUNCH_ATTENTION_DONE_DELAY_MAX,
    LAUNCH_ATTENTION_DONE_DELAY_MIN,
    LAUNCH_ATTENTION_MIN_TURN_MAX,
    LAUNCH_ATTENTION_MIN_TURN_MIN,
    LAUNCH_ATTENTION_MODE_OPTIONS,
    LAUNCH_PREVIEW_LINES_MAX,
    LAUNCH_PREVIEW_LINES_MIN,
    LAUNCH_PREVIEW_SKIP_MAX,
    LAUNCH_PREVIEW_SKIP_MIN,
  } from "../../domains/settings/launch-configs";

  let {
    config,
    preview = $bindable<PreviewSettings | null>(null),
    attention = $bindable<AttentionSettings | null>(null),
    previewError,
    attentionError,
  }: {
    config: LaunchConfig | null;
    preview: PreviewSettings | null;
    attention: AttentionSettings | null;
    previewError?: string;
    attentionError?: string;
  } = $props();

  let defaultPreview = $state(true);
  let previewLines = $state<number | undefined>(defaultPreviewSettings.lines);
  let previewSkip = $state<number | undefined>(defaultPreviewSettings.skip);
  let defaultAttention = $state(true);
  let attentionMode = $state<AttentionMode>(
    defaultLaunchAttentionSettings.mode,
  );
  let attentionProgramStatus = $state(
    defaultLaunchAttentionSettings.programStatus,
  );
  let attentionAgentProtocol = $state(
    defaultLaunchAttentionSettings.agentProtocol,
  );
  let attentionNotify = $state(defaultLaunchAttentionSettings.notify);
  let attentionBell = $state(defaultLaunchAttentionSettings.bell);
  let attentionMinTurnSeconds = $state<number | undefined>(
    defaultLaunchAttentionSettings.minTurnSeconds,
  );
  let attentionDoneDelaySeconds = $state<number | undefined>(
    defaultLaunchAttentionSettings.doneDelaySeconds,
  );

  $effect.pre(() => {
    defaultPreview = !config?.preview;
    previewLines = config?.preview?.lines ?? previewSettings.lines;
    previewSkip = config?.preview?.skip ?? previewSettings.skip;
    defaultAttention = !config?.attention;
    attentionMode =
      config?.attention?.mode ?? defaultLaunchAttentionSettings.mode;
    attentionProgramStatus =
      config?.attention?.programStatus ??
      defaultLaunchAttentionSettings.programStatus;
    attentionAgentProtocol =
      config?.attention?.agentProtocol ??
      defaultLaunchAttentionSettings.agentProtocol;
    attentionNotify =
      config?.attention?.notify ?? defaultLaunchAttentionSettings.notify;
    attentionBell =
      config?.attention?.bell ?? defaultLaunchAttentionSettings.bell;
    attentionMinTurnSeconds =
      config?.attention?.minTurnSeconds ??
      defaultLaunchAttentionSettings.minTurnSeconds;
    attentionDoneDelaySeconds =
      config?.attention?.doneDelaySeconds ??
      defaultLaunchAttentionSettings.doneDelaySeconds;
  });

  $effect.pre(() => {
    preview = defaultPreview
      ? null
      : {
          lines: typeof previewLines === "number" ? previewLines : Number.NaN,
          skip: typeof previewSkip === "number" ? previewSkip : Number.NaN,
        };
    attention = defaultAttention
      ? null
      : {
          mode: attentionMode,
          programStatus: attentionProgramStatus,
          agentProtocol: attentionAgentProtocol,
          notify: attentionNotify,
          bell: attentionBell,
          minTurnSeconds:
            attentionMode === "all"
              ? typeof attentionMinTurnSeconds === "number"
                ? attentionMinTurnSeconds
                : Number.NaN
              : 0,
          doneDelaySeconds:
            attentionMode === "all"
              ? typeof attentionDoneDelaySeconds === "number"
                ? attentionDoneDelaySeconds
                : Number.NaN
              : 0,
        };
  });

  const previewLinesInvalid = $derived(
    !defaultPreview &&
      (typeof previewLines !== "number" ||
        !Number.isInteger(previewLines) ||
        previewLines < LAUNCH_PREVIEW_LINES_MIN ||
        previewLines > LAUNCH_PREVIEW_LINES_MAX),
  );
  const previewSkipInvalid = $derived(
    !defaultPreview &&
      (typeof previewSkip !== "number" ||
        !Number.isInteger(previewSkip) ||
        previewSkip < LAUNCH_PREVIEW_SKIP_MIN ||
        previewSkip > LAUNCH_PREVIEW_SKIP_MAX),
  );
  const attentionMinTurnInvalid = $derived(
    !defaultAttention &&
      attentionMode === "all" &&
      (typeof attentionMinTurnSeconds !== "number" ||
        !Number.isInteger(attentionMinTurnSeconds) ||
        attentionMinTurnSeconds < LAUNCH_ATTENTION_MIN_TURN_MIN ||
        attentionMinTurnSeconds > LAUNCH_ATTENTION_MIN_TURN_MAX),
  );
  const attentionDoneDelayInvalid = $derived(
    !defaultAttention &&
      attentionMode === "all" &&
      (typeof attentionDoneDelaySeconds !== "number" ||
        !Number.isInteger(attentionDoneDelaySeconds) ||
        attentionDoneDelaySeconds < LAUNCH_ATTENTION_DONE_DELAY_MIN ||
        attentionDoneDelaySeconds > LAUNCH_ATTENTION_DONE_DELAY_MAX),
  );
  const attentionModeDescription = $derived(
    LAUNCH_ATTENTION_MODE_OPTIONS.find(
      (option) => option.value === attentionMode,
    )?.description ?? "",
  );
</script>

<fieldset class="grid min-w-0 gap-2">
  <legend class="mb-2 text-sm font-medium">
    输出预览 <span class="font-normal text-muted-foreground">可选</span>
  </legend>
  <div class="flex items-center gap-2">
    <Checkbox
      id="launch-config-preview-default"
      bind:checked={defaultPreview}
      aria-describedby="launch-config-preview-help"
    />
    <Label for="launch-config-preview-default"
      >跟随全局默认<span class="sr-only">（输出预览）</span></Label
    >
  </div>
  {#if defaultPreview}
    <p id="launch-config-preview-help" class="text-xs text-muted-foreground">
      使用设置页「会话输出预览」中的行数与跳过末尾行数。
    </p>
  {:else}
    <div class="grid min-w-0 gap-4 sm:grid-cols-2">
      <div class="grid gap-2">
        <Label for="launch-config-preview-lines">显示行数</Label>
        <Input
          id="launch-config-preview-lines"
          type="number"
          min={LAUNCH_PREVIEW_LINES_MIN}
          max={LAUNCH_PREVIEW_LINES_MAX}
          step={1}
          bind:value={previewLines}
          aria-invalid={previewLinesInvalid}
          aria-describedby={previewError
            ? "launch-config-preview-help launch-config-preview-error"
            : "launch-config-preview-help"}
        />
      </div>
      <div class="grid gap-2">
        <Label for="launch-config-preview-skip">跳过末尾行数</Label>
        <Input
          id="launch-config-preview-skip"
          type="number"
          min={LAUNCH_PREVIEW_SKIP_MIN}
          max={LAUNCH_PREVIEW_SKIP_MAX}
          step={1}
          bind:value={previewSkip}
          aria-invalid={previewSkipInvalid}
          aria-describedby={previewError
            ? "launch-config-preview-help launch-config-preview-error"
            : "launch-config-preview-help"}
        />
      </div>
    </div>
    {#if previewError}<p
        id="launch-config-preview-error"
        role="alert"
        class="text-xs text-destructive"
      >
        {previewError}
      </p>{/if}
    <p id="launch-config-preview-help" class="text-xs text-muted-foreground">
      不同 agent
      的底部状态栏高度不同；按此配置启动的会话在首页使用这些值。修改配置后回到首页生效。
    </p>
  {/if}
</fieldset>
<fieldset id="launch-config-attention-fieldset" class="grid min-w-0 gap-2">
  <legend class="mb-2 text-sm font-medium">
    完成与介入提醒 <span class="font-normal text-muted-foreground">可选</span>
  </legend>
  <div class="flex items-center gap-2">
    <Checkbox
      id="launch-config-attention-default"
      bind:checked={defaultAttention}
      onCheckedChange={(checked) => {
        if (!checked) {
          void tick().then(() => {
            document
              .getElementById("launch-config-attention-fieldset")
              ?.scrollIntoView({
                block: "nearest",
                behavior: "smooth",
              });
          });
        }
      }}
      aria-describedby="launch-config-attention-help"
    />
    <Label for="launch-config-attention-default"
      >跟随全局默认<span class="sr-only">（完成与介入提醒）</span></Label
    >
  </div>
  {#if defaultAttention}
    <p id="launch-config-attention-help" class="text-xs text-muted-foreground">
      使用服务端全局提醒配置；新建会话按「状态协议 (OSC 7501) &gt; Agent 事件
      &gt; 桌面通知 &gt; 终端响铃」优先级触发完成与介入提醒。
    </p>
  {:else}
    <div class="grid min-w-0 gap-3 pt-1">
      <div class="grid gap-1.5">
        <Label for="launch-config-attention-mode">提醒范围</Label>
        <SegmentedControl.Root
          id="launch-config-attention-mode"
          aria-label="提醒范围"
          size="form"
          wrap
          bind:value={attentionMode}
        >
          {#each LAUNCH_ATTENTION_MODE_OPTIONS as option (option.value)}
            <SegmentedControl.Item value={option.value}>
              {option.label}
            </SegmentedControl.Item>
          {/each}
        </SegmentedControl.Root>
        <p class="text-xs text-muted-foreground">
          {attentionModeDescription}
        </p>
      </div>
      {#if attentionMode !== "off"}
        <div class="grid gap-2">
          <span class="text-xs font-medium text-foreground">信号来源</span>
          <div class="grid gap-2 sm:grid-cols-2">
            <div class="flex items-center gap-2">
              <Checkbox
                id="launch-config-attention-program-status"
                bind:checked={attentionProgramStatus}
              />
              <Label for="launch-config-attention-program-status"
                >状态协议 (OSC 7501)</Label
              >
            </div>
            <div class="flex items-center gap-2">
              <Checkbox
                id="launch-config-attention-agent-protocol"
                bind:checked={attentionAgentProtocol}
              />
              <Label for="launch-config-attention-agent-protocol"
                >Agent 事件 (Warp 协议)</Label
              >
            </div>
            <div class="flex items-center gap-2">
              <Checkbox
                id="launch-config-attention-notify"
                bind:checked={attentionNotify}
              />
              <Label for="launch-config-attention-notify"
                >桌面通知 (OSC 9 / 777)</Label
              >
            </div>
            <div class="flex items-center gap-2">
              <Checkbox
                id="launch-config-attention-bell"
                bind:checked={attentionBell}
              />
              <Label for="launch-config-attention-bell">终端响铃 (BEL)</Label>
            </div>
          </div>
          <p
            id="launch-config-attention-help"
            class="text-xs text-muted-foreground"
          >
            优先级：状态协议 (OSC 7501) &gt; Agent 事件 &gt; 桌面通知 &gt;
            终端响铃。高优先级信号生效时自动忽略低优先级信号；若某工具的 OSC
            7501 存在子任务误报，可关闭该项以回退到 Agent 事件。
          </p>
        </div>
      {/if}
      {#if attentionMode === "all"}
        <div class="grid min-w-0 gap-4 sm:grid-cols-2">
          <div class="grid gap-2">
            <Label for="launch-config-attention-min-turn"
              >最短回合耗时（秒）</Label
            >
            <Input
              id="launch-config-attention-min-turn"
              type="number"
              min={LAUNCH_ATTENTION_MIN_TURN_MIN}
              max={LAUNCH_ATTENTION_MIN_TURN_MAX}
              step={1}
              bind:value={attentionMinTurnSeconds}
              aria-invalid={attentionMinTurnInvalid}
              aria-describedby={attentionError
                ? "launch-config-attention-threshold-help launch-config-attention-error"
                : "launch-config-attention-threshold-help"}
            />
          </div>
          <div class="grid gap-2">
            <Label for="launch-config-attention-done-delay"
              >完成确认延迟（秒）</Label
            >
            <Input
              id="launch-config-attention-done-delay"
              type="number"
              min={LAUNCH_ATTENTION_DONE_DELAY_MIN}
              max={LAUNCH_ATTENTION_DONE_DELAY_MAX}
              step={1}
              bind:value={attentionDoneDelaySeconds}
              aria-invalid={attentionDoneDelayInvalid}
              aria-describedby={attentionError
                ? "launch-config-attention-threshold-help launch-config-attention-error"
                : "launch-config-attention-threshold-help"}
            />
          </div>
        </div>
        {#if attentionError}<p
            id="launch-config-attention-error"
            role="alert"
            class="text-xs text-destructive"
          >
            {attentionError}
          </p>{/if}
        <p
          id="launch-config-attention-threshold-help"
          class="text-xs text-muted-foreground"
        >
          忽略过短的回合，或在完成状态保持稳定后再提醒，减少子任务与后台命令造成的抢跑误报。
        </p>
      {/if}
    </div>
  {/if}
</fieldset>
