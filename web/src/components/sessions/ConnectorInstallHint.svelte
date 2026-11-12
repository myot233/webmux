<script lang="ts">
  import type { Snippet } from "svelte";
  import IconCheck from "~icons/tabler/check";
  import IconCopy from "~icons/tabler/copy";
  import IconExternalLink from "~icons/tabler/external-link";
  import IconX from "~icons/tabler/x";
  import { Button } from "$lib/components/ui/button";
  import InlineNotice from "../shared/InlineNotice.svelte";
  import { useCopied } from "../../hooks/shared/copied.svelte";
  import { connectorMissingText } from "../../domains/sessions/hybrid";
  import type { AgentConnector } from "../../contracts/schemas";

  // How to install the third-party connector an agent's chat side needs:
  // the copyable install command and its documentation. The message defaults
  // to the connector being missing; actions add choices such as starting on
  // the terminal side instead.
  let {
    connector,
    message,
    detail,
    tone = "warning",
    actions,
    ondismiss,
    class: className,
  }: {
    connector: AgentConnector;
    message?: string;
    /** A second line under the command, such as when detection reruns. */
    detail?: string;
    tone?: "info" | "warning";
    actions?: Snippet;
    /** Shows a close control when set. */
    ondismiss?: () => void;
    class?: string;
  } = $props();

  const install = useCopied();
  const copy = () => install.copy(connector.installCommand);
  const copyLabel = $derived(
    install.copied
      ? "已复制安装命令"
      : install.failed
        ? "复制失败，请手动复制"
        : "复制安装命令",
  );
</script>

<InlineNotice {tone} class={className}>
  <div class="flex items-start gap-2">
    <p class="min-w-0 flex-1">{message ?? connectorMissingText(connector)}</p>
    {#if ondismiss}<Button
        variant="ghost"
        size="icon-sm"
        class="-my-1 shrink-0"
        aria-label="关闭安装提示"
        title="关闭安装提示"
        onclick={ondismiss}><IconX aria-hidden="true" /></Button
      >{/if}
  </div>
  <div class="mt-2 flex min-w-0 items-center gap-1">
    <code
      class="min-w-0 flex-1 rounded-md bg-muted px-2 py-1 font-mono text-xs break-all text-foreground select-all"
      >{connector.installCommand}</code
    >
    <Button
      variant="ghost"
      size="icon-sm"
      class="shrink-0"
      aria-label={copyLabel}
      title={copyLabel}
      onclick={copy}
      >{#if install.copied}<IconCheck aria-hidden="true" />{:else}<IconCopy
          aria-hidden="true"
        />{/if}</Button
    >
  </div>
  {#if detail}<p class="mt-1 text-xs text-muted-foreground">{detail}</p>{/if}
  <div class="mt-2 flex flex-wrap items-center gap-2">
    <a
      class="inline-flex items-center gap-1 underline underline-offset-2"
      href={connector.docsUrl}
      target="_blank"
      rel="noreferrer noopener"
      >安装说明<IconExternalLink class="size-3.5" aria-hidden="true" /></a
    >
    {@render actions?.()}
  </div>
</InlineNotice>
