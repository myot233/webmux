import { APIError } from "./api";
import { threadAPI } from "./thread-api";
import type { HandoffDraft, HandoffPreview } from "./schemas";
import type { HandoffRecovery } from "./handoff-draft";

export async function recoverHandoffPreview(
  draft: HandoffDraft,
  sourceId: string,
  fork: boolean,
  recovery: HandoffRecovery | null,
  signal: AbortSignal,
) {
  if (recovery && draft.preview?.id !== recovery.handoffId) {
    const prior = await threadAPI.get(recovery.handoffId, signal);
    if (prior.sourceConversationId === sourceId && prior.fork === fork) {
      draft.preview = prior;
      try {
        draft.delivery = await threadAPI.delivery(prior.id, signal);
      } catch (cause) {
        if (!(cause instanceof APIError && cause.status === 404)) throw cause;
        draft.delivery = null;
      }
    }
  }
}

export function saveHandoffEdits(
  p: HandoffPreview,
  text: string,
  userText: string,
) {
  return threadAPI.edit(p.id, {
    revision: p.revision,
    ...(text !== p.renderedText ? { text } : {}),
    ...(userText !== p.userText ? { userText } : {}),
  });
}

export function sendReviewedHandoff(
  sourceId: string,
  p: HandoffPreview,
  clientRequestId: string,
  retry: boolean,
) {
  return threadAPI.send(sourceId, {
    clientRequestId,
    handoffId: p.id,
    revision: p.revision,
    text: p.userText,
    configId: p.configId,
    agentConfig: p.agentConfig,
    location: p.location,
    attachments: p.attachments,
    retry,
  });
}
