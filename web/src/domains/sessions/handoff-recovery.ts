import { APIError } from "../../platform/api";
import { threadAPI } from "../../platform/thread-api";
import { newRequestId } from "../../platform/ids";
import type {
  HandoffDelivery,
  HandoffDraft,
  HandoffPreview,
} from "../../contracts/schemas";
import {
  emptyLocation,
  type HandoffChoice,
  type HandoffRecovery,
} from "./handoff-draft";

export function isHandoffComplete(delivery: HandoffDelivery | null) {
  return delivery?.status === "inline" || delivery?.status === "injected";
}

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

export async function generateHandoffPreview(
  sourceId: string,
  fork: boolean,
  choice: Omit<HandoffChoice, "locationKind">,
  editedContext?: string,
) {
  const preview = await threadAPI.preview(sourceId, {
    clientRequestId: newRequestId(),
    configId: choice.configId,
    agentConfig: choice.model ? { model: choice.model } : {},
    userText: choice.task,
    fork,
    location: fork ? choice.location : emptyLocation(),
    attachments: choice.attachments,
  });
  return editedContext === undefined
    ? preview
    : threadAPI.edit(preview.id, {
        revision: preview.revision,
        text: editedContext,
      });
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
