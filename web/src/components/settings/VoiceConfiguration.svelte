<script lang="ts">
  import { untrack } from "svelte";
  import { Button } from "$lib/components/ui/button";
  import { Label } from "$lib/components/ui/label";
  import { Textarea } from "$lib/components/ui/textarea";
  import type { EnvRow } from "../../domains/settings/launch-configs";
  import type {
    VoiceSettings,
    VoiceSettingsUpdate,
  } from "../../contracts/schemas";
  import EnvRowsEditor from "./EnvRowsEditor.svelte";

  let {
    current,
    disabled,
    onsave,
    onreset,
  }: {
    current: VoiceSettings;
    disabled: boolean;
    onsave: (draft: VoiceSettingsUpdate) => void;
    onreset: () => void;
  } = $props();

  const envName = /^[A-Za-z_][A-Za-z0-9_]*$/;
  let nextKey = 0;
  const makeRow = (name = "", value = ""): EnvRow => ({
    key: nextKey++,
    name,
    value,
  });
  // The command is the user's own: WebMux ships no recognition service.
  // One argument per line: the command runs as an argv, never through a shell.
  let command = $state(untrack(() => current.cmd.join("\n")));
  let rows = $state<EnvRow[]>(
    untrack(() =>
      Object.entries(current.env).map(([name, value]) => makeRow(name, value)),
    ),
  );
  let attempted = $state(false);

  const argv = $derived(
    command
      .split("\n")
      .map((line) => line.trim())
      .filter(Boolean),
  );
  // A row left empty is ignored, like in launch configs.
  const filled = $derived(rows.filter((row) => row.name.trim() || row.value));
  const problems = $derived.by(() => {
    const found: Record<number, { name: string }> = {};
    for (const [index, row] of filled.entries()) {
      const name = row.name.trim();
      if (!envName.test(name))
        found[row.key] = {
          name: "变量名只能包含字母、数字与下划线，且不以数字开头",
        };
      else if (
        filled.findIndex((other) => other.name.trim() === name) !== index
      )
        found[row.key] = { name: "变量名重复" };
    }
    return found;
  });
  // Problems show once the user tried to save.
  const errors = $derived(attempted ? problems : {});
  const invalid = $derived(Object.keys(problems).length > 0);

  function submit(event: SubmitEvent) {
    event.preventDefault();
    attempted = true;
    if (disabled || invalid) return;
    onsave({
      cmd: argv,
      env: Object.fromEntries(
        filled.map((row) => [row.name.trim(), row.value]),
      ),
    });
  }
</script>

<form onsubmit={submit} class="space-y-4" aria-label="语音识别配置">
  <div class="space-y-1.5">
    <Label for="voice-command">命令与参数</Label>
    <Textarea
      id="voice-command"
      code
      rows={4}
      bind:value={command}
      {disabled}
      placeholder="node&#10;/path/to/your-asr-provider.js"
      aria-describedby="voice-command-help"
    />
    <p id="voice-command-help" class="text-xs text-muted-foreground">
      每行一个参数，第一行是可执行文件，直接执行、不经 shell 展开；识别程序按
      STDIN / STDOUT 协议工作，见下方协议文档。留空会关闭语音识别。
    </p>
  </div>
  <EnvRowsEditor bind:rows {errors} {disabled} {makeRow}>
    {#snippet help()}例如识别服务的 API Key。只保存在服务端，读取需要登录
      Token。{/snippet}
  </EnvRowsEditor>
  <div class="flex flex-wrap gap-2">
    <Button type="submit" {disabled}>保存</Button>
    {#if current.source === "web"}
      <Button type="button" variant="outline" {disabled} onclick={onreset}
        >恢复为配置文件设置</Button
      >
    {/if}
  </div>
</form>
