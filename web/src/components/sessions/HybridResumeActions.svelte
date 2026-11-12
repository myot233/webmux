<script lang="ts">
  import IconLoader2 from "~icons/tabler/loader-2";
  import { Button } from "$lib/components/ui/button";
  import * as AlertDialog from "$lib/components/ui/alert-dialog";
  import ConfirmDialog from "../shared/ConfirmDialog.svelte";
  import ConnectorInstallHint from "./ConnectorInstallHint.svelte";
  import HybridCandidatePicker from "./HybridCandidatePicker.svelte";
  import RebuildWorktreeDialog from "../workspace/RebuildWorktreeDialog.svelte";
  import { errorMessage } from "../../platform/error";
  import type {
    ResumeChoice,
    ResumeOutcome,
  } from "../../domains/sessions/recoverable";
  import type {
    AgentCandidate,
    AgentConnector,
    ArchivedSession,
    SessionInfo,
    SessionView,
  } from "../../contracts/schemas";

  // Resume on the recorded view, after any required worktree, agent session,
  // or launch environment confirmation.
  let {
    record,
    resume,
    onresumed,
    onrebuilt = () => {},
    disabled = false,
    onbusy = () => {},
    onerror,
  }: {
    record: ArchivedSession;
    resume: (
      id: string,
      view: SessionView,
      choice?: ResumeChoice,
    ) => Promise<ResumeOutcome>;
    onresumed: (session: SessionInfo) => void;
    /**
     * The released worktree was rebuilt and the resume that followed is
     * settled, with every question it asked answered or dismissed: time to
     * refresh. `failure` explains a resume that failed ("" otherwise); it is
     * not reported through `onerror`, whose display a refresh may replace.
     */
    onrebuilt?: (failure: string) => void;
    disabled?: boolean;
    onbusy?: (busy: boolean) => void;
    /** Reports a failed resume; "" clears it when a new attempt starts. */
    onerror: (message: string) => void;
  } = $props();

  // A resume that needs the user's answer before it can be retried. Earlier
  // answers ride along, so a second question does not undo the first.
  let pending = $state<{
    view: SessionView;
    choice: ResumeChoice;
    kind: "candidates" | "config_missing";
    message: string;
    candidates: AgentCandidate[];
  } | null>(null);
  let pendingOpen = $state(false);
  let picked = $state("");

  // Resumes in flight; the refresh after a rebuild waits for them.
  let running = 0;
  // The side whose resume is in flight, for its button's spinner.
  let resumingView = $state<SessionView | "">("");
  async function run(view: SessionView, choice: ResumeChoice = {}) {
    running++;
    resumingView = view;
    onbusy(true);
    onerror("");
    try {
      const outcome = await resume(record.id, view, choice);
      if (outcome.status === "resumed") {
        onresumed(outcome.session);
        return;
      }
      if (outcome.status === "connector_missing") {
        missing = { connector: outcome.connector, choice };
        missingOpen = true;
        return;
      }
      pending = {
        view,
        choice,
        kind: outcome.status,
        message: outcome.message,
        candidates: outcome.status === "candidates" ? outcome.candidates : [],
      };
      picked = pending.candidates[0]?.agentSessionId ?? "";
      pendingOpen = true;
    } catch (err) {
      const message = `无法恢复“${record.title}”：${errorMessage(err)}`;
      if (rebuild) rebuild.failure = message;
      else onerror(message);
    } finally {
      running--;
      resumingView = "";
      onbusy(false);
      settle();
    }
  }

  function confirmPending() {
    const answer = pending;
    pendingOpen = false;
    if (!answer) return;
    const choice =
      answer.kind === "candidates"
        ? { ...answer.choice, agentSessionId: picked }
        : { ...answer.choice, confirmWithoutEnv: true };
    // ConfirmDialog closes itself right after this returns; a retry that asks
    // a second question must reopen it only after that, never before.
    setTimeout(() => void run(answer.view, choice));
  }

  // The chat view's connector is not installed; the terminal view still
  // works, with the answers given so far.
  let missing = $state<{
    connector: AgentConnector;
    choice: ResumeChoice;
  } | null>(null);
  let missingOpen = $state(false);

  function resumeTerminal() {
    const answer = missing;
    missingOpen = false;
    if (answer) void run("terminal", answer.choice);
  }

  // A released record's worktree comes back first, once the user confirms
  // how. The record shown keeps its release until the records refresh, so
  // the release rebuilt here is remembered and not rebuilt twice.
  let rebuildView = $state<SessionView | null>(null);
  let rebuiltRelease = "";
  function start(view: SessionView) {
    if (record.released && record.released.id !== rebuiltRelease)
      rebuildView = view;
    else void run(view);
  }
  // The refresh after a rebuild can regroup the record and so replace this
  // component; it waits until the resume and every question it asked are
  // settled, and hands a failure to the caller, which outlives the regroup.
  let rebuild: { failure: string } | null = null;
  function rebuilt() {
    const view = rebuildView;
    rebuildView = null;
    rebuiltRelease = record.released?.id ?? "";
    rebuild = { failure: "" };
    if (view) void run(view);
    else settle();
  }
  function settle() {
    if (!rebuild || running || pendingOpen || missingOpen) return;
    const { failure } = rebuild;
    rebuild = null;
    onrebuilt(failure);
  }
</script>

<Button
  size="sm"
  {disabled}
  aria-busy={!!resumingView}
  onclick={() => start(record.view)}
  >{#if resumingView}<IconLoader2
      class="animate-spin motion-reduce:animate-none"
      aria-hidden="true"
    />{/if}恢复</Button
>

{#if rebuildView && record.released}<RebuildWorktreeDialog
    id={record.id}
    title={record.title}
    released={record.released}
    onclose={() => (rebuildView = null)}
    onrebuilt={rebuilt}
  />{/if}

<ConfirmDialog
  bind:open={
    () => pendingOpen,
    (open) => {
      pendingOpen = open;
      settle();
    }
  }
  title={pending?.kind === "candidates"
    ? "找不到原来的 agent 会话"
    : "启动配置已删除"}
  onconfirm={confirmPending}
>
  {pending?.message}
  {#if pending?.kind === "candidates"}
    <HybridCandidatePicker
      candidates={pending.candidates}
      bind:picked
      label="选择要恢复的会话"
    />
  {:else}
    恢复后将使用原命令，但不再带有该配置保存的环境变量。
  {/if}
</ConfirmDialog>

<AlertDialog.Root
  bind:open={
    () => missingOpen,
    (open) => {
      missingOpen = open;
      settle();
    }
  }
>
  <AlertDialog.Content>
    <AlertDialog.Header>
      <AlertDialog.Title>聊天端无法启动</AlertDialog.Title>
      <AlertDialog.Description
        >可以先安装后再恢复到聊天，或者恢复到终端。</AlertDialog.Description
      >
    </AlertDialog.Header>
    {#if missing}<ConnectorInstallHint connector={missing.connector} />{/if}
    <AlertDialog.Footer>
      <AlertDialog.Cancel>取消</AlertDialog.Cancel>
      <AlertDialog.Action onclick={resumeTerminal}
        >恢复到终端</AlertDialog.Action
      >
    </AlertDialog.Footer>
  </AlertDialog.Content>
</AlertDialog.Root>
