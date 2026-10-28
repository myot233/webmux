<script lang="ts">
  import { untrack } from "svelte";
  import IconForms from "~icons/tabler/forms";
  import { Button } from "$lib/components/ui/button";
  import { Checkbox } from "$lib/components/ui/checkbox";
  import { Input } from "$lib/components/ui/input";
  import { Label } from "$lib/components/ui/label";
  import * as NativeSelect from "$lib/components/ui/native-select";
  import type { RequestAnswer } from "../../chat-api";
  import type { ChatFormField, ChatRequest } from "../../schemas";

  // A form the agent asks the user to fill in (elicitation, form mode):
  // text, choice and yes/no fields. A form with any other field type can
  // only be declined or cancelled here.
  let {
    request,
    disabled,
    onanswer,
  }: {
    request: ChatRequest;
    disabled: boolean;
    onanswer: (answer: RequestAnswer) => void;
  } = $props();
  const form = $derived(
    request.form ?? { message: "", fields: [], supported: false },
  );
  const initial = (field: ChatFormField) =>
    field.type === "boolean"
      ? field.default === true
      : typeof field.default === "string"
        ? field.default
        : field.type === "enum"
          ? (field.enum?.[0] ?? "")
          : "";
  let values = $state<Record<string, string | boolean>>(
    untrack(() =>
      Object.fromEntries(
        form.fields.map((field) => [field.name, initial(field)]),
      ),
    ),
  );
  const missing = $derived(
    form.fields.some(
      (field) =>
        field.required && field.type === "string" && !values[field.name],
    ),
  );
  const fieldId = (field: ChatFormField) =>
    `form-${request.requestId}-${field.name}`;
</script>

<form
  class="space-y-3 rounded-xl border border-warning/40 bg-card p-3 shadow-sm"
  aria-label="Agent 的表单"
  onsubmit={(event) => {
    event.preventDefault();
    if (form.supported && !missing)
      onanswer({ outcome: "accept", content: { ...values } });
  }}
>
  <p class="flex items-center gap-1.5 text-sm font-medium">
    <IconForms class="size-4 shrink-0 text-warning" aria-hidden="true" />
    {request.title || "Agent 需要更多信息"}
  </p>
  {#if form.message}<p class="text-sm break-words whitespace-pre-wrap">
      {form.message}
    </p>{/if}
  {#if form.supported}
    {#each form.fields as field (field.name)}
      <div class="space-y-1">
        {#if field.type === "boolean"}
          <Label>
            <Checkbox
              checked={values[field.name] === true}
              {disabled}
              onCheckedChange={(checked) => (values[field.name] = checked)}
            />{field.title || field.name}
          </Label>
        {:else}
          <Label for={fieldId(field)}
            >{field.title || field.name}{field.required ? " *" : ""}</Label
          >
          {#if field.type === "enum"}
            <NativeSelect.Root
              id={fieldId(field)}
              class="w-full"
              {disabled}
              bind:value={
                () => String(values[field.name] ?? ""),
                (value) => (values[field.name] = value)
              }
            >
              {#each field.enum ?? [] as choice, i (choice)}
                <NativeSelect.Option value={choice}
                  >{field.enumNames?.[i] ?? choice}</NativeSelect.Option
                >
              {/each}
            </NativeSelect.Root>
          {:else}
            <Input
              id={fieldId(field)}
              required={field.required}
              {disabled}
              bind:value={
                () => String(values[field.name] ?? ""),
                (value) => (values[field.name] = value)
              }
            />
          {/if}
        {/if}
        {#if field.description}<p class="text-xs text-muted-foreground">
            {field.description}
          </p>{/if}
      </div>
    {/each}
  {:else}
    <p class="text-sm text-muted-foreground">该表单需要在终端中完成</p>
  {/if}
  <div class="flex flex-wrap justify-end gap-2">
    <Button
      variant="ghost"
      size="sm"
      {disabled}
      onclick={() => onanswer({ outcome: "cancel" })}>取消</Button
    >
    <Button
      variant="outline"
      size="sm"
      {disabled}
      onclick={() => onanswer({ outcome: "decline" })}>拒绝</Button
    >
    {#if form.supported}
      <Button type="submit" size="sm" disabled={disabled || missing}
        >提交</Button
      >
    {/if}
  </div>
</form>
