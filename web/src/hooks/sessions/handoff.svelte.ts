import {
  generateHandoffPreview,
  recoverHandoffPreview,
  saveHandoffEdits,
  sendReviewedHandoff,
  isHandoffComplete,
} from "../../domains/sessions/handoff-recovery";
import { APIError } from "../../platform/api";
import { errorMessage } from "../../platform/error";
import { newRequestId } from "../../platform/ids";
import {
  emptyLocation,
  handoffDraftStorage,
  rememberHandoffDraft,
  handoffLocationKind,
  handoffSourceLocation,
  hasConflictingHandoffWork,
  handoffChoicesDiffer,
} from "../../domains/sessions/handoff-draft";
import type {
  ChatAttachment,
  HandoffDelivery,
  HandoffDraft,
  HandoffLocation,
  HandoffPreview,
} from "../../contracts/schemas";
import type { TargetKind } from "../../domains/sessions/new-session-defaults";
import { threadAPI } from "../../platform/thread-api";

// One controller per source/mode; mounted outside ChatView. Persistent preview
// revisions own the history. Local storage holds pre-preview task/choices and
// unsaved user edits only, so a conflict or network failure can be recovered.
export function createHandoffEditor(sourceId: string, fork: boolean) {
  const storage = handoffDraftStorage(sourceId, fork);
  const choice = storage.choice();
  const recovery = storage.recovery();
  let recoveryBase = recovery;
  let orphan = false;
  let unpreviewed = !recovery;
  let source = $state<HandoffDraft["source"] | null>(null);
  let preview = $state<HandoffPreview | null>(null);
  let delivery = $state<HandoffDelivery | null>(null);
  let configId = $state(choice?.configId ?? "");
  let model = $state(choice?.model ?? "");
  let task = $state(choice?.task ?? "");
  let location = $state<HandoffLocation>(choice?.location ?? emptyLocation());
  let locationKind = $state<TargetKind>(choice?.locationKind ?? "directory");
  let attachments = $state<ChatAttachment[]>(choice?.attachments ?? []);
  let context = $state("");
  let loading = $state(true);
  let busy = $state(false);
  let saving = $state(false);
  let conflict = $state(false);
  let error = $state("");
  let saveJob: Promise<boolean> | null = null;
  let disposed = false;
  let sourceMissing = false;
  let lastRequestId = "";
  const lifetime = new AbortController();
  const dirty = $derived(
    preview !== null &&
      (context !== preview.renderedText || task !== preview.userText),
  );
  const locked = $derived(
    delivery !== null && ["pending", "unknown"].includes(delivery.status),
  );
  const sealed = $derived(isHandoffComplete(delivery));
  const choiceChanged = $derived(
    handoffChoicesDiffer(preview, configId, model, attachments, location),
  );

  function adopt(p: HandoffPreview) {
    orphan = false;
    recoveryBase = null;
    unpreviewed = false;
    preview = p;
    context = p.renderedText;
    task = p.userText;
    configId = p.configId;
    model = p.agentConfig.model ?? "";
    location = p.location;
    locationKind = handoffLocationKind(p.location);
    attachments = p.attachments;
    conflict = false;
  }
  function remember() {
    if (loading || sourceMissing) return;
    rememberHandoffDraft(storage, {
      task,
      configId,
      model,
      location,
      locationKind,
      attachments,
      orphan,
      unpreviewed,
      recovery: recoveryBase,
      preview,
      context,
      dirty,
      choiceChanged,
      conflict,
    });
  }

  function nextTask() {
    task = "";
    configId = "";
    model = "";
    attachments = [];
    if (source) {
      location = handoffSourceLocation(source);
      locationKind = handoffLocationKind(location);
    }
  }
  async function load() {
    try {
      const draft = await threadAPI.draft(sourceId, fork, lifetime.signal);
      if (disposed) return;
      source = draft.source;
      await recoverHandoffPreview(
        draft,
        sourceId,
        fork,
        recovery,
        lifetime.signal,
      );
      if (disposed) return;
      delivery = draft.delivery;
      if (draft.preview) {
        preview = draft.preview;
        context = draft.preview.renderedText;
        if (unpreviewed && hasConflictingHandoffWork(choice, draft)) {
          conflict = true;
          error =
            "服务端已有其他版本，已保留本设备尚未预览的任务与选项。请选择读取服务端版本或保留编辑重新预览。";
        } else adopt(draft.preview);
      } else if (!choice) {
        location = handoffSourceLocation(draft.source);
        locationKind = handoffLocationKind(location);
      }
      if (recovery && draft.preview?.id === recovery.handoffId) {
        if (choice) {
          configId = choice.configId;
          model = choice.model;
          location = choice.location;
          locationKind = choice.locationKind;
          attachments = choice.attachments;
        }
        task = recovery.task;
        context = recovery.context ?? preview?.renderedText ?? "";
        conflict =
          recovery.revision !== draft.preview.revision &&
          (task !== draft.preview.userText ||
            context !== draft.preview.renderedText);
        recoveryBase = conflict ? recovery : null;
        if (conflict)
          error =
            "服务端已有其他版本，已保留本地编辑。请选择读取服务端版本或保留编辑重新预览。";
      } else if (sealed && !conflict) nextTask();
    } catch (cause) {
      if (disposed) return;
      if (
        cause instanceof APIError &&
        cause.status === 404 &&
        source === null
      ) {
        sourceMissing = true;
        source = null;
        preview = null;
        delivery = null;
        task = "";
        context = "";
        attachments = [];
        storage.clear();
      } else if (recovery) {
        orphan = true;
        task = recovery.task;
        context = recovery.context ?? "";
      }
      error = orphan
        ? source
          ? "原交接记录暂时无法恢复，本设备的未发送内容已保留，可重新生成预览。"
          : "暂时无法读取来源，本设备的未发送内容已保留。请刷新后重试。"
        : errorMessage(cause);
    } finally {
      if (!disposed) loading = false;
    }
  }
  async function save(): Promise<boolean> {
    if (saveJob) {
      await saveJob;
      return dirty && !conflict ? save() : !conflict;
    }
    if (!source) return false;
    if (!preview || !dirty || locked || sealed) return true;
    if (conflict) return false;
    const p = preview,
      text = context,
      userText = task;
    remember();
    saving = true;
    saveJob = (async () => {
      try {
        const saved = await saveHandoffEdits(p, text, userText);
        if (disposed) return true;
        if (preview?.id === p.id) preview = saved;
        if (context === text && task === userText && !choiceChanged)
          storage.saveRecovery(null);
        error = "";
        return true;
      } catch (cause) {
        if (cause instanceof APIError && cause.status === 409) conflict = true;
        error = conflict
          ? "另一个设备已修改草稿。你的编辑已保留，请读取服务端版本或保留编辑重新预览。"
          : errorMessage(cause);
        return false;
      } finally {
        if (!disposed) saving = false;
        saveJob = null;
      }
    })();
    return saveJob;
  }
  async function refresh() {
    if (!preview || saving || busy) return;
    const p = preview;
    try {
      if (!conflict) {
        const updated = await threadAPI.get(p.id, lifetime.signal);
        if (
          disposed ||
          preview?.id !== p.id ||
          preview.revision !== p.revision ||
          saving ||
          busy
        )
          return;
        if (
          updated.revision !== p.revision &&
          (dirty || choiceChanged) &&
          (updated.renderedText !== context || updated.userText !== task)
        ) {
          conflict = true;
          remember();
          error = "服务端已有其他版本，你的编辑已保留。";
        } else if (!dirty && !choiceChanged) adopt(updated);
        else preview = updated;
      }
      try {
        const next = await threadAPI.delivery(p.id, lifetime.signal);
        if (!disposed && preview?.id === p.id) {
          const completed = sealed;
          delivery = next;
          if (!completed && isHandoffComplete(next)) {
            if (!dirty && !choiceChanged) nextTask();
            else remember();
          }
          if (lastRequestId === next.clientRequestId) lastRequestId = "";
        }
      } catch (cause) {
        if (!(cause instanceof APIError && cause.status === 404)) throw cause;
      }
    } catch (cause) {
      if (!disposed) error = errorMessage(cause);
    }
  }
  async function generate() {
    if (busy || locked || !configId || !source) return;
    busy = true;
    error = "";
    const edited =
      (orphan && recovery?.context !== undefined) ||
      (!unpreviewed && !sealed && preview?.manual) ||
      context !== (preview?.renderedText ?? "");
    try {
      const p = await generateHandoffPreview(
        sourceId,
        fork,
        { configId, model, task, location, attachments },
        edited ? context : undefined,
      );
      if (!disposed) {
        adopt(p);
        delivery = null;
        lastRequestId = "";
        storage.saveRecovery(null);
      }
    } catch (cause) {
      if (!disposed) error = errorMessage(cause);
    } finally {
      if (!disposed) busy = false;
    }
  }
  async function send() {
    if (
      !preview ||
      !source ||
      busy ||
      conflict ||
      choiceChanged ||
      (delivery !== null &&
        ["unknown", "inline", "injected"].includes(delivery.status))
    )
      return;
    busy = true;
    error = "";
    try {
      if (!(await save())) return;
      if (dirty || choiceChanged || conflict) {
        error = "草稿仍有更改，尚未发送。请检查保存后的预览。";
        return;
      }
      const reviewed = preview;
      const p = await threadAPI.get(reviewed.id);
      if (p.revision !== reviewed.revision) {
        conflict = true;
        error =
          "服务端已有其他版本，尚未发送。请读取服务端版本或保留编辑重新预览。";
        remember();
        return;
      }
      preview = p;
      if (
        (p.stale || p.budget.overBudget) &&
        !["pending", "failed"].includes(delivery?.status ?? "")
      )
        return;
      // Pending preparations resume their original receipt after refresh;
      // only a known failed attempt receives a new explicit retry command.
      const retry =
        delivery?.status === "failed" ||
        (delivery?.status === "pending" && delivery.retry);
      const requestId =
        delivery?.status === "pending"
          ? delivery.clientRequestId
          : (lastRequestId ||= newRequestId());
      delivery = await sendReviewedHandoff(sourceId, p, requestId, retry);
      if (sealed && !dirty && !choiceChanged) nextTask();
      lastRequestId = "";
    } catch (cause) {
      if (!disposed) error = errorMessage(cause);
    } finally {
      if (!disposed) busy = false;
    }
  }
  async function closeSave() {
    if (busy) return;
    busy = true;
    try {
      await save();
    } finally {
      busy = false;
    }
  }
  async function serverVersion() {
    if (!preview || busy) return;
    busy = true;
    try {
      adopt(await threadAPI.get(preview.id));
      storage.saveRecovery(null);
      error = "";
    } catch (cause) {
      error = errorMessage(cause);
    } finally {
      busy = false;
    }
  }
  async function replacement() {
    const id = delivery?.replacementHandoffId;
    if (!id || busy) return;
    busy = true;
    try {
      adopt(await threadAPI.get(id));
      delivery = null;
      lastRequestId = "";
      error = "";
    } catch (cause) {
      error = errorMessage(cause);
    } finally {
      busy = false;
    }
  }
  $effect(() => {
    remember();
    if (!dirty || loading || locked || sealed || conflict) return;
    const timer = setTimeout(() => void save(), 350);
    return () => clearTimeout(timer);
  });
  $effect(() => {
    if (!preview) return;
    const timer = setInterval(() => void refresh(), 2000);
    return () => clearInterval(timer);
  });
  $effect(() => {
    void load();
    return () => {
      remember();
      disposed = true;
      lifetime.abort();
    };
  });
  return {
    get source() {
      return source;
    },
    get preview() {
      return preview;
    },
    get delivery() {
      return delivery;
    },
    get configId() {
      return configId;
    },
    set configId(v: string) {
      configId = v;
    },
    get model() {
      return model;
    },
    set model(v: string) {
      model = v;
    },
    get task() {
      return task;
    },
    set task(v: string) {
      task = v;
    },
    get locationKind() {
      return locationKind;
    },
    set locationKind(value: TargetKind) {
      locationKind = value;
    },
    get location() {
      return location;
    },
    set location(v: HandoffLocation) {
      location = v;
    },
    get attachments() {
      return attachments;
    },
    set attachments(value: ChatAttachment[]) {
      attachments = value;
    },
    reportError(message: string) {
      error = message;
    },
    get context() {
      return context;
    },
    set context(v: string) {
      context = v;
    },
    get loading() {
      return loading;
    },
    get busy() {
      return busy;
    },
    get saving() {
      return saving;
    },
    get dirty() {
      return dirty;
    },
    get conflict() {
      return conflict;
    },
    get unpreviewed() {
      return unpreviewed;
    },
    get locked() {
      return locked;
    },
    get sealed() {
      return sealed;
    },
    get choiceChanged() {
      return choiceChanged;
    },
    get error() {
      return error;
    },
    save,
    closeSave,
    generate,
    send,
    refresh,
    serverVersion,
    replacement,
  };
}
export type HandoffEditor = ReturnType<typeof createHandoffEditor>;
