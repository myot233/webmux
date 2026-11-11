import { z } from "zod";
import { api } from "./api";
import {
  ConversationMessageRequestSchema,
  ConversationActivitySchema,
  HandoffDeliverySchema,
  HandoffDraftSchema,
  HandoffEditRequestSchema,
  HandoffPreviewRequestSchema,
  HandoffPreviewSchema,
} from "./schemas";

export const threadAPI = {
  activity: (source: string, id: string, offset = "0", signal?: AbortSignal) =>
    api(
      `/conversations/${encodeURIComponent(source)}/activities/${encodeURIComponent(id)}?textOffset=${encodeURIComponent(offset)}&maxBytes=4000`,
      ConversationActivitySchema,
      "GET",
      undefined,
      signal,
    ),
  draft: (source: string, fork: boolean, signal?: AbortSignal) =>
    api(
      `/conversations/${source}/handoff-previews?fork=${fork ? "1" : "0"}`,
      HandoffDraftSchema,
      "GET",
      undefined,
      signal,
    ),
  preview: (
    source: string,
    body: z.infer<typeof HandoffPreviewRequestSchema>,
    signal?: AbortSignal,
  ) =>
    api(
      `/conversations/${source}/handoff-previews`,
      HandoffPreviewSchema,
      "POST",
      body,
      signal,
      HandoffPreviewRequestSchema,
    ),
  get: (id: string, signal?: AbortSignal) =>
    api(`/handoffs/${id}`, HandoffPreviewSchema, "GET", undefined, signal),
  edit: (
    id: string,
    body: z.infer<typeof HandoffEditRequestSchema>,
    signal?: AbortSignal,
  ) =>
    api(
      `/handoffs/${id}`,
      HandoffPreviewSchema,
      "PATCH",
      body,
      signal,
      HandoffEditRequestSchema,
    ),
  delivery: (id: string, signal?: AbortSignal) =>
    api(
      `/handoffs/${id}/delivery`,
      HandoffDeliverySchema,
      "GET",
      undefined,
      signal,
    ),
  send: (
    source: string,
    body: z.infer<typeof ConversationMessageRequestSchema>,
  ) =>
    api(
      `/conversations/${source}/messages`,
      HandoffDeliverySchema,
      "POST",
      body,
      undefined,
      ConversationMessageRequestSchema,
    ),
};
