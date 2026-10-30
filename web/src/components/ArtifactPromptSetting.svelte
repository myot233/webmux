<script lang="ts">
  import { changeSettings, saveSettings, settings } from "../settings.svelte";
  import {
    BUILTIN_ARTIFACT_PROMPT,
    artifactPromptError,
    normalizeArtifactPrompt,
  } from "../artifactPrompt";
  import { Button } from "$lib/components/ui/button";
  import { Label } from "$lib/components/ui/label";
  import { Textarea } from "$lib/components/ui/textarea";

  // Synced template for the quick-command menu's fixed artifacts item; null
  // on the server means the built-in text.
  const current = $derived(
    settings.values.artifactPrompt ?? BUILTIN_ARTIFACT_PROMPT,
  );
  // Typed text survives unavailable settings and failed saves.
  let edited = $state<string | undefined>();
  const text = $derived(edited ?? current);
  const normalized = $derived(normalizeArtifactPrompt(text));
  const problem = $derived(artifactPromptError(normalized));
  const changed = $derived(normalized !== current);

  async function save() {
    if (problem || !changed) return;
    try {
      await saveSettings({ artifactPrompt: normalized });
      edited = undefined;
    } catch {
      // Keep the typed template; the page shows the shared save error.
    }
  }
</script>

<div class="space-y-2">
  <Label for="settings-artifact-prompt">临时文件夹提示词</Label>
  <p id="settings-artifact-prompt-help" class="text-xs text-muted-foreground">
    快捷指令菜单的“插入临时文件夹提示词”使用此模板，<code>{"{dir}"}</code>
    会替换为本会话临时文件夹的路径；在已连接的设备间同步。
  </p>
  <Textarea
    id="settings-artifact-prompt"
    mono
    rows={6}
    class="max-h-80"
    value={text}
    oninput={(event) => {
      edited = event.currentTarget.value;
    }}
    aria-invalid={!!problem}
    aria-describedby="settings-artifact-prompt-help"
  />
  {#if problem}<p role="alert" class="text-sm text-destructive">
      {problem}
    </p>{/if}
  <div class="flex flex-wrap gap-2">
    <Button
      size="sm"
      disabled={!settings.ready || !!problem || !changed}
      onclick={() => void save()}>保存</Button
    >
    <Button
      variant="ghost"
      size="sm"
      disabled={!settings.ready ||
        (settings.values.artifactPrompt === null && edited === undefined)}
      onclick={() => {
        edited = undefined;
        changeSettings({ artifactPrompt: null });
      }}>恢复默认</Button
    >
  </div>
</div>
