<script lang="ts">
  import { changeSettings, saveSettings, settings } from "../settings.svelte";
  import { Button } from "$lib/components/ui/button";
  import { Input } from "$lib/components/ui/input";
  import { Label } from "$lib/components/ui/label";

  // Server-side and synced: hybrid sessions on the chat side stop their agent
  // after this many idle minutes and wake on the next command.
  const defaultMinutes = 30;
  const maxMinutes = 1440;
  let edited = $state<number | undefined>();
  const minutes = $derived(
    edited ?? settings.values.chatIdleSleepMinutes ?? defaultMinutes,
  );
  const valid = $derived(
    Number.isInteger(minutes) && minutes >= 0 && minutes <= maxMinutes,
  );

  async function save() {
    if (!valid) return;
    try {
      await saveSettings({ chatIdleSleepMinutes: minutes });
      edited = undefined;
    } catch {
      // Keep the typed value; the settings card shows the save error.
    }
  }
</script>

<div class="space-y-2">
  <Label for="settings-chat-idle">混合会话聊天端空闲休眠（分钟）</Label>
  <Input
    id="settings-chat-idle"
    type="number"
    min={0}
    max={maxMinutes}
    step={1}
    disabled={!settings.ready}
    value={minutes}
    oninput={(event) => {
      edited = event.currentTarget.valueAsNumber;
    }}
    onchange={() => void save()}
    aria-invalid={!valid}
    aria-describedby="settings-chat-idle-help"
  />
  <p id="settings-chat-idle-help" class="text-xs text-muted-foreground">
    聊天端没有回合、待答复请求与排队消息，且这段时间内没有操作时，结束 agent
    进程以节省资源；再次打开或发送时自动唤醒。0 表示不休眠，默认 {defaultMinutes}
    分钟，在已连接的设备间同步。
  </p>
  {#if !valid}<p role="alert" class="text-sm text-destructive">
      请输入 0–{maxMinutes} 之间的整数。
    </p>{/if}
  <Button
    variant="ghost"
    size="sm"
    disabled={!settings.ready}
    onclick={() => {
      edited = undefined;
      changeSettings({ chatIdleSleepMinutes: null });
    }}>恢复默认</Button
  >
</div>
