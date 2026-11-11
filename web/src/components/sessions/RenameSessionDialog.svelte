<script lang="ts">
  import { untrack } from "svelte";
  import { api } from "../../platform/api";
  import { errorMessage } from "../../platform/error";
  import { SessionInfoSchema, type SessionInfo } from "../../contracts/schemas";
  import { Button } from "$lib/components/ui/button";
  import { Input } from "$lib/components/ui/input";
  import { Label } from "$lib/components/ui/label";
  import ResponsiveDialog from "../shared/ResponsiveDialog.svelte";

  // Sets the user-chosen title. It outranks titles the terminal or agent
  // report; clearing it hands the title back to them. The new title reaches
  // every device through the session list, so nothing is stored here.
  let {
    session,
    onclose,
  }: {
    session: SessionInfo;
    onclose: () => void;
  } = $props();
  // The field starts from the title shown when the dialog opened.
  let name = $state(untrack(() => session.title));
  let busy = $state(false);
  let error = $state("");

  async function submit() {
    busy = true;
    error = "";
    try {
      await api(`/sessions/${session.id}`, SessionInfoSchema, "PATCH", {
        name: name.trim(),
      });
      onclose();
    } catch (err) {
      error = errorMessage(err);
    } finally {
      busy = false;
    }
  }
</script>

<ResponsiveDialog
  title="修改标题"
  description="手动设置的标题优先于终端或 agent 自动给出的标题；留空则恢复自动标题。"
  {onclose}
  {busy}
>
  <form
    class="space-y-4"
    onsubmit={(event) => {
      event.preventDefault();
      void submit();
    }}
  >
    <div class="space-y-2">
      <Label for="session-title">标题</Label>
      <Input
        id="session-title"
        bind:value={name}
        maxlength={100}
        autocomplete="off"
        onfocus={(event) => event.currentTarget.select()}
      />
    </div>
    {#if error}<p role="alert" class="text-sm break-all text-destructive">
        {error}
      </p>{/if}
    <div class="flex justify-end gap-2">
      <Button type="button" variant="outline" disabled={busy} onclick={onclose}
        >取消</Button
      >
      <Button type="submit" class="min-w-24" disabled={busy}
        >{busy ? "正在保存…" : "保存"}</Button
      >
    </div>
  </form>
</ResponsiveDialog>
