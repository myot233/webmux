<script lang="ts">
  import { untrack } from "svelte";
  import { errorMessage } from "../../platform/error";
  import { api } from "../../platform/api";
  import { quickCommands } from "../../domains/settings/quick-commands.svelte";
  import {
    INIT_INSTRUCTION_MAX,
    RepositorySchema,
    UpdateRepositoryRequestSchema,
    type Repository,
  } from "../../contracts/schemas";
  import { Button } from "$lib/components/ui/button";
  import InitInstructionField from "../sessions/InitInstructionField.svelte";
  import ResponsiveDialog from "../shared/ResponsiveDialog.svelte";

  // Edits a repository's settings from the home toolbar: today the init
  // instruction template; saving it empty removes it.
  let {
    repository,
    onclose,
    onchange,
  }: {
    repository: Repository;
    onclose: () => void;
    onchange: () => void;
  } = $props();
  let instruction = $state(untrack(() => repository.initInstruction));
  let busy = $state(false);
  let error = $state("");
  async function save() {
    busy = true;
    error = "";
    try {
      await api(
        `/repositories/${repository.id}`,
        RepositorySchema,
        "PATCH",
        { initInstruction: instruction.trim() },
        undefined,
        UpdateRepositoryRequestSchema,
      );
      onchange();
      onclose();
    } catch (err) {
      error = errorMessage(err);
    } finally {
      busy = false;
    }
  }
</script>

<ResponsiveDialog
  title="仓库设置"
  description={`${repository.name} 的设置。`}
  {onclose}
  {busy}
>
  <form
    class="space-y-4"
    onsubmit={(event) => {
      event.preventDefault();
      void save();
    }}
  >
    <InitInstructionField
      id="repository-instruction-edit"
      commands={quickCommands.list}
      bind:value={instruction}
    />
    {#if error}<p role="alert" class="break-all text-sm text-destructive">
        {error}
      </p>{/if}
    <div class="flex justify-end gap-2">
      <Button
        type="button"
        variant="ghost"
        class="mr-auto"
        disabled={busy || !instruction}
        onclick={() => (instruction = "")}>清空</Button
      >
      <Button type="button" variant="outline" disabled={busy} onclick={onclose}
        >取消</Button
      >
      <Button
        type="submit"
        disabled={busy || instruction.length > INIT_INSTRUCTION_MAX}
        >{busy ? "正在保存…" : "保存"}</Button
      >
    </div>
  </form>
</ResponsiveDialog>
