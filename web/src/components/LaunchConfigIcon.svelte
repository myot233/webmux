<script lang="ts">
  import IconTerminal from "~icons/tabler/terminal-2";
  import IconClaudeCode from "~icons/lobe/claudecode";
  import IconCodex from "~icons/lobe/codex";
  import IconGeminiCLI from "~icons/lobe/geminicli";
  import IconCursor from "~icons/lobe/cursor";
  import IconGitHubCopilot from "~icons/lobe/githubcopilot";
  import IconOpenCode from "~icons/lobe/opencode";
  import IconQwen from "~icons/lobe/qwen";
  import IconKimi from "~icons/lobe/kimi";
  import IconCodeBuddy from "~icons/lobe/codebuddy";
  import IconPi from "~icons/lobe/pi";

  let { icon }: { icon: string } = $props();
  const icons = {
    terminal: IconTerminal,
    claudecode: IconClaudeCode,
    codex: IconCodex,
    geminicli: IconGeminiCLI,
    cursor: IconCursor,
    githubcopilot: IconGitHubCopilot,
    opencode: IconOpenCode,
    qwen: IconQwen,
    kimi: IconKimi,
    codebuddy: IconCodeBuddy,
    pi: IconPi,
  };
  const Glyph = $derived(
    Object.hasOwn(icons, icon)
      ? icons[icon as keyof typeof icons]
      : IconTerminal,
  );
</script>

<span class="inline-flex shrink-0" aria-hidden="true" data-launch-icon={icon}>
  <Glyph class="size-5 shrink-0" />
</span>
