<script lang="ts" module>
  import IconTerminal from "~icons/tabler/terminal-2";
  import IconAntigravity from "~icons/lobe/antigravity";
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
  import IconQoder from "~icons/lobe/qoder";
  import IconDevin from "~icons/lobe/devin";

  const icons = {
    terminal: IconTerminal,
    antigravity: IconAntigravity,
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
    qoder: IconQoder,
    devin: IconDevin,
  };
  const sizes = { xs: "size-3.5", sm: "size-4", md: "size-5" };

  /** Whether icon is a key of the catalog rather than the terminal fallback. */
  export function hasLaunchIcon(icon: string): boolean {
    return Object.hasOwn(icons, icon);
  }
</script>

<script lang="ts">
  let { icon, size = "md" }: { icon: string; size?: keyof typeof sizes } =
    $props();
  const Glyph = $derived(
    hasLaunchIcon(icon) ? icons[icon as keyof typeof icons] : IconTerminal,
  );
</script>

<span class="inline-flex shrink-0" aria-hidden="true" data-launch-icon={icon}>
  <Glyph class={`${sizes[size]} shrink-0`} />
</span>
