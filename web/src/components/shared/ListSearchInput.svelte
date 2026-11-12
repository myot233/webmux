<script lang="ts">
  import IconSearch from "~icons/tabler/search";
  import * as InputGroup from "$lib/components/ui/input-group";

  // The search box above a filtered list in a dialog. Escape clears a typed
  // query first and only closes the dialog once the box is empty.
  let {
    value = $bindable(""),
    label,
    placeholder,
  }: { value?: string; label: string; placeholder: string } = $props();
</script>

<InputGroup.Root class="w-full min-w-0">
  <InputGroup.Addon>
    <IconSearch aria-hidden="true" />
  </InputGroup.Addon>
  <InputGroup.Input
    type="search"
    aria-label={label}
    {placeholder}
    autocomplete="off"
    autocapitalize="off"
    autocorrect="off"
    spellcheck={false}
    bind:value
    onkeydown={(event) => {
      if (event.key === "Escape" && value && !event.isComposing) {
        event.preventDefault();
        event.stopPropagation();
        value = "";
      }
    }}
  />
</InputGroup.Root>
