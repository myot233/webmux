<script lang="ts">
  import IconMessageCircle from "~icons/tabler/message-circle";
  import IconTerminal2 from "~icons/tabler/terminal-2";
  import * as Tabs from "$lib/components/ui/tabs";
  import type { SwitchTarget } from "../hybrid";

  // "聊天 | 终端" in a hybrid session's header: the selected tab is where the
  // session is. Narrow screens show only the icons, named for assistive
  // technology. A side that cannot be reached now stays focusable with its
  // reason; choosing it explains instead of switching.
  let {
    side,
    target,
    switching,
    chatUnavailable,
    request,
  }: {
    side: SwitchTarget | null;
    /** Where a running switch goes. */
    target: SwitchTarget | null;
    switching: boolean;
    chatUnavailable: string;
    request: (to: SwitchTarget) => void;
  } = $props();
  const shown = $derived(target ?? side ?? "");
</script>

<Tabs.Root
  bind:value={
    () => shown,
    (to) => {
      if (to === "acp" || to === "tui") request(to);
    }
  }
>
  <Tabs.List aria-label="会话视图">
    <Tabs.Trigger
      value="acp"
      disabled={switching}
      aria-disabled={chatUnavailable && !switching ? "true" : undefined}
      aria-label={chatUnavailable ? `聊天（${chatUnavailable}）` : "聊天"}
      title={chatUnavailable ? `现在不能切到聊天：${chatUnavailable}` : "聊天"}
      ><IconMessageCircle aria-hidden="true" /><span class="max-sm:hidden"
        >聊天</span
      ></Tabs.Trigger
    >
    <Tabs.Trigger
      value="tui"
      disabled={switching}
      aria-label="终端"
      title="终端"
      ><IconTerminal2 aria-hidden="true" /><span class="max-sm:hidden"
        >终端</span
      ></Tabs.Trigger
    >
  </Tabs.List>
</Tabs.Root>
