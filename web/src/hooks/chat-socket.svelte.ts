import { SvelteURLSearchParams } from "svelte/reactivity";
import {
  applyFrame,
  emptyChat,
  resumePoint,
  type ChatModel,
} from "../chat-model";
import { ChatFrameSchema, type ChatEvent, type ChatState } from "../schemas";
import { connect, type Connection, type SocketConnection } from "../ws";

export interface ChatSocket {
  /** The timeline reduced from every applied event. */
  readonly model: ChatModel;
  /** The latest whole chat state; undefined until the first snapshot. */
  readonly state: ChatState | undefined;
  readonly connection: Connection;
  readonly error: string;
  /** Opens the stream once; later calls do nothing. */
  open(): void;
  /** Reopens a stream that ended (a session that came back). */
  reconnect(): void;
  dispose(): void;
}

// The chat channel of one hybrid session (`/ws/s/{id}/chat`, text JSON frames
// in docs/protocol/http-hybrid.md): a snapshot, the event backlog, then live
// events and state. Reconnects resume after the last applied event, so a
// returning page receives only what it missed. `onLive` sees events that
// arrived after the backlog, for notices that must reach other views.
export function createChatSocket(
  id: string,
  onLive: (events: ChatEvent[]) => void = () => {},
): ChatSocket {
  let model = $state.raw(emptyChat());
  let state = $state.raw<ChatState>();
  let connection = $state<Connection>("connecting");
  let error = $state("");
  let socket: SocketConnection | undefined;
  // The first events frame after a snapshot is the backlog.
  let backlog = true;

  function open() {
    if (socket) return;
    socket = connect(
      () => {
        const query = new SvelteURLSearchParams();
        const point = resumePoint(model);
        if (point) {
          query.set("generation", point.generation);
          query.set("seq", String(point.seq));
        }
        const search = query.toString();
        return `/ws/s/${id}/chat${search ? `?${search}` : ""}`;
      },
      (data) => {
        if (typeof data !== "string")
          throw new Error("聊天通道的帧必须是 JSON 文本");
        const frame = ChatFrameSchema.parse(JSON.parse(data));
        model = applyFrame(model, frame);
        if (frame.type === "events") {
          if (!backlog && frame.items?.length) onLive(frame.items);
          backlog = false;
          return;
        }
        if (frame.type === "snapshot") backlog = true;
        state = frame.state;
      },
      (next) => {
        connection = next;
        if (next === "connected") error = "";
      },
      (message) => {
        error = message;
      },
    );
  }

  return {
    get model() {
      return model;
    },
    get state() {
      return state;
    },
    get connection() {
      return connection;
    },
    get error() {
      return error;
    },
    open,
    reconnect() {
      socket?.reconnect();
    },
    dispose() {
      socket?.close();
      socket = undefined;
    },
  };
}
