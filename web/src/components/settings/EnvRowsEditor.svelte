<script lang="ts">
  import type { Snippet } from "svelte";
  import IconEye from "~icons/tabler/eye";
  import IconEyeOff from "~icons/tabler/eye-off";
  import IconPlus from "~icons/tabler/plus";
  import IconTrash from "~icons/tabler/trash";
  import { Button } from "$lib/components/ui/button";
  import { Input } from "$lib/components/ui/input";
  import type { EnvRow } from "../../domains/settings/launch-configs";

  // The environment variable rows of a form: a name and a value (masked until
  // revealed) per row, with per-row and whole-list errors.
  let {
    rows = $bindable(),
    errors = {},
    error = "",
    max = Infinity,
    disabled = false,
    makeRow,
    help,
  }: {
    rows: EnvRow[];
    /** Problems of single rows, keyed by the row's key. */
    errors?: Readonly<
      Record<number, { name?: string; value?: string } | undefined>
    >;
    /** A problem of the list as a whole. */
    error?: string;
    max?: number;
    disabled?: boolean;
    /** Makes an empty row with a key no other row has. */
    makeRow: () => EnvRow;
    help?: Snippet;
  } = $props();
  let revealed = $state<Record<number, boolean>>({});
</script>

<fieldset class="grid min-w-0 gap-2" {disabled}>
  <legend class="mb-2 text-sm font-medium">
    环境变量 <span class="font-normal text-muted-foreground">可选</span>
  </legend>
  {#each rows as envRow, index (envRow.key)}
    {@const rowErrors = errors[envRow.key]}
    <div class="grid min-w-0 gap-1">
      <div class="flex min-w-0 flex-wrap items-center gap-2">
        <Input
          mono
          class="min-w-0 flex-1 basis-32"
          aria-label={`变量名 ${index + 1}`}
          bind:value={envRow.name}
          placeholder="NAME"
          autocomplete="off"
          autocapitalize="off"
          autocorrect="off"
          spellcheck={false}
          aria-invalid={!!rowErrors?.name}
        />
        <div class="flex min-w-0 flex-2 basis-48 items-center gap-1">
          <Input
            mono
            class="min-w-0 flex-1"
            type={revealed[envRow.key] ? "text" : "password"}
            aria-label={`变量值 ${index + 1}`}
            bind:value={envRow.value}
            placeholder="值"
            autocomplete="off"
            autocapitalize="off"
            autocorrect="off"
            spellcheck={false}
            aria-invalid={!!rowErrors?.value}
          />
          <Button
            type="button"
            variant="ghost"
            size="icon"
            aria-label={`显示变量值 ${index + 1}`}
            aria-pressed={!!revealed[envRow.key]}
            title={revealed[envRow.key] ? "隐藏变量值" : "显示变量值"}
            onclick={() => {
              revealed[envRow.key] = !revealed[envRow.key];
            }}
            >{#if revealed[envRow.key]}<IconEyeOff
                aria-hidden="true"
              />{:else}<IconEye aria-hidden="true" />{/if}</Button
          >
          <Button
            type="button"
            variant="ghost"
            size="icon"
            aria-label={`删除变量 ${index + 1}`}
            title="删除变量"
            onclick={() => {
              rows = rows.filter((item) => item.key !== envRow.key);
            }}><IconTrash aria-hidden="true" /></Button
          >
        </div>
      </div>
      {#if rowErrors}<p class="text-xs text-destructive">
          {[rowErrors.name, rowErrors.value].filter(Boolean).join("；")}
        </p>{/if}
    </div>
  {/each}
  {#if error}<p class="text-xs text-destructive">{error}</p>{/if}
  <div>
    <Button
      type="button"
      variant="outline"
      size="sm"
      disabled={rows.length >= max}
      onclick={() => {
        rows = [...rows, makeRow()];
      }}><IconPlus aria-hidden="true" />添加环境变量</Button
    >
  </div>
  {#if help}
    <p class="text-xs text-muted-foreground">{@render help()}</p>
  {/if}
</fieldset>
