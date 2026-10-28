<script lang="ts">
  import { Button } from "$lib/components/ui/button";
  import * as NativeSelect from "$lib/components/ui/native-select";
  import ConfirmDialog from "./ConfirmDialog.svelte";
  import { errorMessage } from "../error";
  import type { ResumeChoice, ResumeOutcome } from "../recoverable";
  import type { HybridCandidate, HybridRecord, SessionInfo } from "../schemas";

  // "恢复到聊天 / 恢复到终端" for one closed or interrupted hybrid session,
  // with the questions a resume may need answered first: another agent
  // session when the recorded one is gone, or resuming without a deleted
  // launch config's environment.
  let {
    record,
    resume,
    onresumed,
    disabled = false,
    onbusy = () => {},
    onerror,
  }: {
    record: HybridRecord;
    resume: (
      id: string,
      side: "tui" | "acp",
      choice?: ResumeChoice,
    ) => Promise<ResumeOutcome>;
    onresumed: (session: SessionInfo) => void;
    disabled?: boolean;
    onbusy?: (busy: boolean) => void;
    /** Reports a failed resume; "" clears it when a new attempt starts. */
    onerror: (message: string) => void;
  } = $props();

  // A resume that needs the user's answer before it can be retried. Earlier
  // answers ride along, so a second question does not undo the first.
  let pending = $state<{
    side: "tui" | "acp";
    choice: ResumeChoice;
    kind: "candidates" | "config_missing";
    message: string;
    candidates: HybridCandidate[];
  } | null>(null);
  let pendingOpen = $state(false);
  let picked = $state("");

  async function run(side: "tui" | "acp", choice: ResumeChoice = {}) {
    onbusy(true);
    onerror("");
    try {
      const outcome = await resume(record.id, side, choice);
      if (outcome.status === "resumed") {
        onresumed(outcome.session);
        return;
      }
      pending = {
        side,
        choice,
        kind: outcome.status,
        message: outcome.message,
        candidates: outcome.status === "candidates" ? outcome.candidates : [],
      };
      picked = pending.candidates[0]?.agentSessionId ?? "";
      pendingOpen = true;
    } catch (err) {
      onerror(`无法恢复“${record.name}”：${errorMessage(err)}`);
    } finally {
      onbusy(false);
    }
  }

  function confirmPending() {
    const answer = pending;
    pendingOpen = false;
    if (!answer) return;
    void run(
      answer.side,
      answer.kind === "candidates"
        ? { ...answer.choice, agentSessionId: picked }
        : { ...answer.choice, confirmWithoutEnv: true },
    );
  }
</script>

<Button size="sm" {disabled} onclick={() => run("acp")}>恢复到聊天</Button>
<Button size="sm" variant="outline" {disabled} onclick={() => run("tui")}
  >恢复到终端</Button
>

<ConfirmDialog
  bind:open={pendingOpen}
  title={pending?.kind === "candidates"
    ? "找不到原来的 agent 会话"
    : "启动配置已删除"}
  onconfirm={confirmPending}
>
  {pending?.message}
  {#if pending?.kind === "candidates"}
    {#if pending.candidates.length}
      <NativeSelect.Root
        aria-label="选择要恢复的会话"
        class="mt-3 w-full"
        bind:value={picked}
      >
        {#each pending.candidates as candidate (candidate.agentSessionId)}
          <NativeSelect.Option value={candidate.agentSessionId}
            >{candidate.title || candidate.agentSessionId}</NativeSelect.Option
          >
        {/each}
      </NativeSelect.Root>
    {:else}
      同一目录下没有其他可恢复的会话。
    {/if}
  {:else}
    恢复后将使用原命令，但不再带有该配置保存的环境变量。
  {/if}
</ConfirmDialog>
