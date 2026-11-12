import { z } from "zod";
import {
  AgentGuidePromptSchema,
  ArtifactsFolderSchema,
  BranchesSchema,
  CommitsSchema,
  EmptyRequestSchema,
  ErrorSchema,
  FetchRemotesResponseSchema,
  ForwardRequestSchema,
  ForwardSchema,
  ForwardTicketSchema,
  ForwardUpdateSchema,
  NoContentSchema,
  RecentDirsSchema,
  RemotesSchema,
  SwitchPreviewRequestSchema,
  SwitchPreviewSchema,
  SwitchRequestSchema,
  UploadSchema,
  WorktreeSchema,
  type ForwardRequest,
  type ForwardUpdate,
  type SessionInfo,
  type SwitchRequest,
  type SwitchTarget,
  type Worktree,
} from "../contracts/schemas";
import { store, stored } from "./storage";
import { currentTransport, TransportUploadError } from "./transport";

export { store, stored };

export class APIError extends Error {
  constructor(
    public status: number,
    message: string,
    // Machine-readable cause some endpoints add, such as push 409 reasons.
    public reason?: string,
    // Emergency tunnel shutdown can succeed even when its disk write fails.
    public notPersisted?: boolean,
    // The parsed error body, for endpoints whose 409 carries choices (such
    // as hybrid recovery candidates); validate it before use.
    public body?: unknown,
  ) {
    super(message);
  }
}
const tokenFragment = "#token=";

// Adopts a `#token=` link: fragments never reach the server, and the token is
// removed from the address bar and current history entry before any request.
// Returns true when a token was stored.
export function consumeTokenFragment(): boolean {
  const hash = location.hash;
  if (!hash.startsWith(tokenFragment)) return false;
  history.replaceState(history.state, "", location.pathname + location.search);
  let token: string;
  try {
    token = decodeURIComponent(hash.slice(tokenFragment.length));
  } catch {
    return false;
  }
  if (!token) return false;
  store("token", token);
  return true;
}
function responseError(status: number, body: unknown, fallback: string) {
  const detail = ErrorSchema.safeParse(body);
  if (status === 401) window.dispatchEvent(new Event("webmux:auth"));
  return detail.success
    ? new APIError(
        status,
        detail.data.error,
        detail.data.reason,
        detail.data.notPersisted,
        body,
      )
    : new APIError(status, fallback, undefined, undefined, body);
}

async function request(
  path: string,
  method = "GET",
  body?: unknown,
  signal?: AbortSignal,
): Promise<Response> {
  const response = await currentTransport().fetch(path, {
    method,
    signal,
    headers: {
      Authorization: "Bearer " + stored("token"),
      "Content-Type": "application/json",
    },
    body: body === undefined ? undefined : JSON.stringify(body),
  });
  if (!response.ok)
    throw responseError(
      response.status,
      await response.json().catch(() => null),
      response.statusText,
    );
  return response;
}
export async function api<T extends z.ZodType>(
  path: string,
  schema: T,
  method = "GET",
  body?: unknown,
  signal?: AbortSignal,
  bodySchema?: z.ZodType,
): Promise<z.output<T>> {
  const response = await request(
    path,
    method,
    bodySchema ? bodySchema.parse(body) : body,
    signal,
  );
  return schema.parse(
    response.status === 204 ? undefined : await response.json(),
  );
}
export async function apiBuffer(id: string): Promise<Uint8Array> {
  return new Uint8Array(
    await (await request(`/sessions/${id}/buffer`)).arrayBuffer(),
  );
}
// Raw bytes share the JSON transport's Bearer header, APIError and 401 event.
// received, when given, sees the running byte count of a streamed body.
export async function apiBlob(
  path: string,
  signal?: AbortSignal,
  received?: (bytes: number) => void,
): Promise<Blob> {
  const response = await request(path, "GET", undefined, signal);
  if (!received || !response.body) return response.blob();
  const reader = response.body.getReader();
  const chunks: Uint8Array<ArrayBuffer>[] = [];
  let total = 0;
  for (;;) {
    const { done, value } = await reader.read();
    if (done) break;
    chunks.push(value);
    total += value.byteLength;
    received(total);
  }
  return new Blob(chunks, {
    type: response.headers.get("Content-Type") ?? "",
  });
}
export function uploadFile(
  id: string,
  file: File,
  progress: (percent: number) => void,
  signal: AbortSignal,
): Promise<z.infer<typeof UploadSchema>> {
  return uploadForm(
    `/sessions/${id}/upload`,
    file,
    UploadSchema,
    progress,
    signal,
  );
}
/** Stores a quick start attachment before its session exists. */
export function uploadQuickStartFile(
  file: File,
  progress: (percent: number) => void,
  signal: AbortSignal,
): Promise<z.infer<typeof UploadSchema>> {
  return uploadForm("/uploads", file, UploadSchema, progress, signal);
}
// Upload progress, authentication and errors stay shared across transports,
// while the default HTTP transport uses XHR and browser multipart boundaries.
export async function uploadForm<T extends z.ZodType>(
  path: string,
  file: File,
  schema: T,
  progress: (percent: number) => void,
  signal: AbortSignal,
): Promise<z.output<T>> {
  if (signal.aborted) throw new DOMException("上传已取消", "AbortError");
  const form = new FormData();
  form.append("file", file);
  let result;
  try {
    result = await currentTransport().upload({
      path,
      form,
      token: stored("token"),
      timeoutMs: 5 * 60 * 1000,
      signal,
      onProgress: progress,
    });
  } catch (err) {
    if (err instanceof TransportUploadError) throw new APIError(0, err.message);
    throw err;
  }
  if (result.status < 200 || result.status >= 300)
    throw responseError(result.status, result.body, result.statusText);
  return schema.parse(result.body);
}
// Creates the session's artifacts folder when missing and returns its path.
export async function ensureArtifactsFolder(
  id: string,
  signal?: AbortSignal,
): Promise<string> {
  return (
    await api(
      `/sessions/${id}/artifacts`,
      ArtifactsFolderSchema,
      "POST",
      undefined,
      signal,
    )
  ).path;
}
// The session's WebMux brief that `{{webmux}}` expands to; it also creates
// the artifacts folder the brief names.
export async function agentGuidePrompt(
  id: string,
  signal?: AbortSignal,
): Promise<string> {
  return (
    await api(
      `/sessions/${id}/agent-guide`,
      AgentGuidePromptSchema,
      "POST",
      undefined,
      signal,
    )
  ).prompt;
}
// A repository's branches with their upstream tracking, and its remotes with
// redacted URLs; both are local reads that never fetch.
export function repositoryBranches(id: string, signal?: AbortSignal) {
  return api(
    `/repositories/${id}/branches`,
    BranchesSchema,
    "GET",
    undefined,
    signal,
  );
}
export function repositoryRemotes(id: string, signal?: AbortSignal) {
  return api(
    `/repositories/${id}/remotes`,
    RemotesSchema,
    "GET",
    undefined,
    signal,
  );
}
// The recent history reachable from the given commits, newest first.
export function repositoryCommits(
  id: string,
  from: readonly string[],
  signal?: AbortSignal,
) {
  const query = new URLSearchParams(from.map((hash) => ["from", hash]));
  return api(
    `/repositories/${id}/commits?${query}`,
    CommitsSchema,
    "GET",
    undefined,
    signal,
  );
}
// Switching a worktree's branch in place: a preview, then its confirmation.
const worktreeRoute = (tree: Pick<Worktree, "repositoryId" | "id">) =>
  `/repositories/${tree.repositoryId}/worktrees/${tree.id}`;
export function previewSwitch(
  tree: Worktree,
  target: SwitchTarget,
  signal?: AbortSignal,
) {
  return api(
    worktreeRoute(tree) + "/switch-preview",
    SwitchPreviewSchema,
    "POST",
    { target },
    signal,
    SwitchPreviewRequestSchema,
  );
}
export function switchWorktree(tree: Worktree, request: SwitchRequest) {
  return api(
    worktreeRoute(tree) + "/switch",
    WorktreeSchema,
    "POST",
    request,
    undefined,
    SwitchRequestSchema,
  );
}
// Fetches every remote of a repository; the server finishes the fetch even
// when this request is abandoned, and answers with the repository read again.
export function fetchRepositoryRemotes(id: string) {
  return api(
    `/repositories/${id}/fetch`,
    FetchRemotesResponseSchema,
    "POST",
    {},
    undefined,
    EmptyRequestSchema,
  );
}
export function navigate(path: string) {
  history.pushState(null, "", path);
  window.dispatchEvent(new PopStateEvent("popstate"));
}
const statusText = {
  running: "运行中",
  waiting: "可能在等你",
  idle: "空闲",
  exited: "已退出",
};
const attentionText = {
  question: "需要回答",
  permission: "等待授权",
  done: "本轮完成",
  error: "出错停止",
  notify: "有通知",
  bell: "响铃提醒",
};
// An agent-reported signal is more specific than the heuristic status label.
export function sessionStatusText(
  session: Pick<SessionInfo, "status" | "attention">,
) {
  return session.attention
    ? attentionText[session.attention.kind]
    : statusText[session.status];
}
export function recentDirs(): string[] {
  try {
    return RecentDirsSchema.parse(JSON.parse(stored("recentDirs", "[]")));
  } catch {
    return [];
  }
}
// Split extra argv without executing a shell. Quoted paths and empty arguments are preserved.
export function parseArgs(input: string): string[] {
  const result: string[] = [];
  let token = "",
    quote = "",
    escaped = false,
    started = false;
  for (const c of input) {
    if (escaped) {
      token += c;
      escaped = false;
      started = true;
    } else if (c === "\\" && quote !== "'") {
      escaped = true;
      started = true;
    } else if (quote) {
      if (c === quote) quote = "";
      else token += c;
    } else if (c === '"' || c === "'") {
      quote = c;
      started = true;
    } else if (/\s/.test(c)) {
      if (started) {
        result.push(token);
        token = "";
        started = false;
      }
    } else {
      token += c;
      started = true;
    }
  }
  if (escaped || quote) throw new Error("参数引号或反斜杠未闭合");
  if (started) result.push(token);
  return result;
}
// Characters that need quoting for parseArgs (whitespace, quotes, backslash)
// plus shell metacharacters, so the text also reads unambiguously as argv.
const plainArg = /^[^\s'"\\$`;&|<>()*?![\]{}#~]+$/u;
// Renders one argument so parseArgs reads it back unchanged.
export function quoteArg(arg: string): string {
  if (plainArg.test(arg)) return arg;
  if (!arg.includes("'")) return `'${arg}'`;
  return `"${arg.replace(/["\\$`]/g, "\\$&")}"`;
}
// Inverse of parseArgs: parseArgs(joinArgs(argv)) equals argv.
export function joinArgs(argv: readonly string[]): string {
  return argv.map(quoteArg).join(" ");
}
// Port forwarding (`/forward/{port}/` gateway). Failures reject with APIError
// carrying the server's message: 400 for an invalid or WebMux's own port, 409
// for a port already forwarded, 404 for a forward that no longer exists.
export function createForward(request: ForwardRequest) {
  return api(
    "/forwards",
    ForwardSchema,
    "POST",
    request,
    undefined,
    ForwardRequestSchema,
  );
}
export function updateForward(port: number, update: ForwardUpdate) {
  return api(
    `/forwards/${port}`,
    ForwardSchema,
    "PATCH",
    update,
    undefined,
    ForwardUpdateSchema,
  );
}
export function deleteForward(port: number) {
  return api(`/forwards/${port}`, NoContentSchema, "DELETE");
}
// A one-time code (60 s) the `/forward-auth` page exchanges for the gateway
// cookie through a top-level form POST, so it never appears in a URL.
export function forwardTicket() {
  return api("/forwards/tickets", ForwardTicketSchema, "POST");
}
