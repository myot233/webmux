import { z } from "zod";
import { ReportedCwdSchema } from "./session-schemas";

const CodeSourceSchema = z.enum(["worktree", "index", "head", "empty"]);
export const CodeContextSchema = z.object({
  contextId: z.string(),
  rootPath: z.string(),
  rootKind: z.enum(["directory", "worktree", "artifacts"]),
  browseAvailable: z.boolean(),
  diffAvailable: z.boolean(),
  ignoreAvailable: z.boolean(),
  reason: z.string(),
  writableAvailable: z.boolean(),
});
const CodeNodeSchema = z.object({
  nodeId: z.string(),
  path: z.string(),
  name: z.string(),
  kind: z.enum(["file", "directory", "symlink", "boundary", "special"]),
  gitStatus: z.string(),
  ignored: z.boolean(),
  referenceable: z.boolean(),
  mediaType: z.string(),
  linkTarget: z.string().optional(),
});
export const CodeListingSchema = z.object({
  children: z.array(CodeNodeSchema),
  nextCursor: z.string(),
  limited: z.boolean(),
  complete: z.boolean(),
  contentVersion: z.string(),
  checkedAt: z.string(),
});
export type CodeNode = z.infer<typeof CodeNodeSchema>;
// The nodes a file operation created, renamed or moved, under final names.
export const CodeMutationSchema = z.object({
  nodes: z.array(CodeNodeSchema),
});
export const CodeUsageSchema = z.object({
  entries: z.number().int().nonnegative(),
  bytes: z.number().int().nonnegative(),
  limited: z.boolean(),
});
// File operation bodies are validated before any request is sent.
export const CodeNameSchema = z
  .string()
  .min(1)
  .refine((name) => new TextEncoder().encode(name).length <= 255, "名称过长")
  .refine((name) => name !== "." && name !== "..", "名称无效")
  .refine((name) => name.toLowerCase() !== ".git", ".git 为保留名称")
  .refine((name) => !/[/\\]/.test(name), "名称不能包含 / 或 \\")
  // \p{Cc} is the control set the server's unicode.IsControl refuses.
  .refine((name) => !/\p{Cc}/u.test(name), "名称不能包含控制字符");
export const CodeRenameRequestSchema = z.strictObject({
  contextId: z.string().min(1),
  nodeId: z.string().min(1),
  name: CodeNameSchema,
});
export const CodeMkdirRequestSchema = z.strictObject({
  contextId: z.string().min(1),
  parentNodeId: z.string(),
  name: CodeNameSchema,
});
export const CodeCopyRequestSchema = z.strictObject({
  contextId: z.string().min(1),
  nodeIds: z.array(z.string().min(1)).min(1).max(100),
  targetNodeId: z.string(),
  mode: z.enum(["copy", "move"]),
});
export const CodeLocationSchema = z.object({
  nodeId: z.string(),
  path: z.string(),
  kind: z.enum(["file", "directory"]),
});
export const CodeDocumentSchema = z.object({
  nodeId: z.string(),
  path: z.string(),
  absolutePath: z.string(),
  source: CodeSourceSchema,
  oid: z.string(),
  contentVersion: z.string(),
  kind: z.enum([
    "text",
    "binary",
    "unsupported",
    "too-large",
    "symlink",
    "image",
    "pdf",
  ]),
  text: z.string(),
  lineCount: z.number().int().nonnegative().max(20000),
  referenceable: z.boolean(),
  reason: z.string(),
  mediaType: z.string(),
  size: z.number().int().nonnegative(),
  downloadable: z.boolean(),
});
export const CodeFileSchema = z.object({
  snapshotId: z.string(),
  document: CodeDocumentSchema,
  checkedAt: z.string(),
});
export const CodeChangeSchema = z.object({
  nodeId: z.string(),
  changeId: z.string(),
  group: z.enum(["unstaged", "staged", "untracked"]),
  oldPath: z.string(),
  newPath: z.string(),
  status: z.string(),
  oldOid: z.string(),
  newOid: z.string(),
  oldMode: z.string(),
  newMode: z.string(),
  reviewVersion: z.string().optional(),
});
export const CodeChangesSchema = z.object({
  nextCursor: z.string(),
  total: z.number().int().nonnegative(),
  entries: z.array(CodeChangeSchema).max(200),
  contentVersion: z.string(),
  checkedAt: z.string(),
});
export const CodeDiffSchema = z.object({
  snapshotId: z.string(),
  old: CodeDocumentSchema,
  new: CodeDocumentSchema,
  kind: z.enum(["text", "metadata", "image"]),
  reason: z.string(),
  checkedAt: z.string(),
  change: CodeChangeSchema,
});
const CodeStateSchema = z.enum([
  "updated",
  "unchanged",
  "missing",
  "expired",
  "unknown",
]);
export const CodeCheckSchema = z.object({
  snapshots: z.record(z.string(), CodeStateSchema),
  directories: z.record(z.string(), CodeStateSchema),
  changes: CodeStateSchema,
  checkedAt: z.string(),
});
export type CodeDocument = z.infer<typeof CodeDocumentSchema>;
export type CodeFile = z.infer<typeof CodeFileSchema>;
export type CodeDiff = z.infer<typeof CodeDiffSchema>;
export type CodeChange = z.infer<typeof CodeChangeSchema>;
export type CodeListing = z.infer<typeof CodeListingSchema>;

// Reading state of one code workspace, handed between the opener and the
// pop-out window. Positions anchor on content (lines, pages, headings), so a
// window of another size shows the same place.
const CodeSearchSpecSchema = z.object({
  search: z.string(),
  caseSensitive: z.boolean(),
  regexp: z.boolean(),
  wholeWord: z.boolean(),
});
const CodeLineSideSchema = z.enum(["deletions", "additions"]);
// Search panel and match cursor shared by the file and diff readers.
const CodeFindStateSchema = z.object({
  search: CodeSearchSpecSchema,
  panel: z.enum(["search", "goto", "zoom"]).optional(),
  cursor: z
    .object({
      matches: z.array(z.number().int()),
      index: z.number().int(),
    })
    .optional(),
});
const CodeReaderStateSchema = CodeFindStateSchema.extend({
  // First visible line and how far (0-1 of its height) it is scrolled past.
  line: z.number().int().min(1),
  offset: z.number().min(0).max(1),
  // The selected line range, or null when nothing is selected.
  selection: z
    .object({
      start: z.number().int().min(1),
      end: z.number().int().min(1),
    })
    .nullable(),
});
const CodeDiffReaderStateSchema = CodeFindStateSchema.extend({
  selected: z
    .object({
      start: z.number().int(),
      end: z.number().int(),
      side: CodeLineSideSchema.optional(),
      endSide: CodeLineSideSchema.optional(),
    })
    .nullable(),
  // Unmodified lines revealed per hunk separator, as the library counts them.
  expanded: z.array(
    z.object({
      index: z.number().int().nonnegative(),
      fromStart: z.number().int().nonnegative(),
      fromEnd: z.number().int().nonnegative(),
    }),
  ),
  top: z
    .object({ line: z.number().int().min(1), side: CodeLineSideSchema })
    .optional(),
});
const CodeMarkdownStateSchema = z.object({
  // Slug of the last heading at or above the top edge, "" when none.
  heading: z.string(),
  // Pixels past that heading, or the scroll ratio (0-1) without a heading.
  offset: z.number(),
});
const CodeImageStateSchema = z.object({
  fitted: z.boolean(),
  scale: z.number().positive(),
  // Image point (0-1 of its natural size) at the viewport centre.
  centerX: z.number(),
  centerY: z.number(),
});
const CodePdfStateSchema = z.object({
  fitted: z.boolean(),
  zoom: z.number().positive(),
  page: z.number().int().min(1),
  // How far (0-1 of the page height) the top edge is into `page`.
  offset: z.number().min(0).max(1),
});
const CodeJsonStateSchema = z.object({
  opened: z.array(z.tuple([z.string(), z.boolean()])),
  shown: z.array(z.tuple([z.string(), z.number().int().nonnegative()])),
});
const CodeDocumentViewStateSchema = z.object({
  mode: z.enum(["file", "diff"]),
  side: z.enum(["old", "new"]),
  start: z.number().int().optional(),
  end: z.number().int().optional(),
  svgView: z.enum(["preview", "source"]),
  formatView: z.enum(["preview", "source"]),
  reader: CodeReaderStateSchema.optional(),
  diff: CodeDiffReaderStateSchema.optional(),
  markdown: CodeMarkdownStateSchema.optional(),
  image: CodeImageStateSchema.optional(),
  pdf: CodePdfStateSchema.optional(),
  json: CodeJsonStateSchema.optional(),
});
const CodeFileTargetSchema = CodeNodeSchema.pick({
  nodeId: true,
  path: true,
  name: true,
  kind: true,
});
const CodeNavigatorStateSchema = z.object({
  rootOpen: z.boolean(),
  // Expanded folders and the listings they had loaded, by path.
  expanded: z.array(z.string()),
  branches: z.array(z.tuple([z.string(), CodeListingSchema])),
  // Index of the first visible row of the changes list.
  changesTop: z.number().int().nonnegative(),
  clipboard: z
    .object({
      contextId: z.string(),
      nodes: z.array(CodeFileTargetSchema),
      mode: z.enum(["copy", "move"]),
    })
    .optional(),
});
const CodeWorkspaceSnapshotSchema = z.object({
  version: z.literal(1),
  root: z.enum(["workspace", "artifacts"]),
  context: CodeContextSchema.optional(),
  listing: CodeListingSchema.optional(),
  rootListing: CodeListingSchema.optional(),
  rootIncludeIgnored: z.boolean(),
  directoryVersions: z.record(z.string(), z.string()),
  navIdentity: z.string(),
  navStatus: z.string(),
  changes: CodeChangesSchema.optional(),
  changeQuery: z.string(),
  changeSearch: z.string(),
  changeGroup: z.string(),
  includeIgnored: z.boolean(),
  directory: z.string(),
  query: z.string(),
  nav: z.string(),
  navOpen: z.boolean(),
  navWidth: z.number(),
  active: z.string(),
  tabs: z
    .array(
      z.object({
        key: z.string(),
        contextId: z.string(),
        rootKind: CodeContextSchema.shape.rootKind,
        file: CodeFileSchema.optional(),
        diff: CodeDiffSchema.optional(),
        comparison: CodeDiffSchema.optional(),
        // Between windows, a body the receiver already holds is sent as its
        // snapshot id instead.
        fileRef: z.string().optional(),
        diffRef: z.string().optional(),
        comparisonRef: z.string().optional(),
        status: z.string(),
        view: CodeDocumentViewStateSchema.optional(),
      }),
    )
    .max(20),
  navigator: CodeNavigatorStateSchema.optional(),
});
export type CodeReaderState = z.infer<typeof CodeReaderStateSchema>;
export type CodeDiffReaderState = z.infer<typeof CodeDiffReaderStateSchema>;
export type CodeMarkdownState = z.infer<typeof CodeMarkdownStateSchema>;
export type CodeImageState = z.infer<typeof CodeImageStateSchema>;
export type CodePdfState = z.infer<typeof CodePdfStateSchema>;
export type CodeJsonState = z.infer<typeof CodeJsonStateSchema>;
export type CodeDocumentViewState = z.infer<typeof CodeDocumentViewStateSchema>;
export type CodeNavigatorState = z.infer<typeof CodeNavigatorStateSchema>;
export type CodeWorkspaceSnapshot = z.infer<typeof CodeWorkspaceSnapshotSchema>;

// What the code workspace needs from the terminal page that hosts it.
const CodeHostStateSchema = z.object({
  draftEnabled: z.boolean(),
  connection: z.string(),
  sessionCwd: z.string(),
  reportedCwd: ReportedCwdSchema.nullable(),
});
export type CodeHostState = z.infer<typeof CodeHostStateSchema>;
const CodeInsertionSchema = z.enum(["draft", "terminal", "failed"]);
// The first message a pop-out frame accepts; it carries the channel port.
export const CodePopoutHandshakeSchema = z.object({
  type: z.literal("webmux-code-popout"),
  version: z.literal(1),
});
export const codePopoutHandshake: z.infer<typeof CodePopoutHandshakeSchema> = {
  type: "webmux-code-popout",
  version: 1,
};
// A code location another view asks the code workspace to open.
const CodeRevealSchema = z.object({
  path: z.string(),
  key: z.number(),
  mode: z.enum(["diff", "file"]).optional(),
  rawPath: z.string().optional(),
  fromUri: z.boolean().optional(),
  line: z.number().int().positive().optional(),
  endLine: z.number().int().positive().optional(),
  column: z.number().int().positive().optional(),
  heading: z.string().optional(),
});
export type CodeReveal = z.infer<typeof CodeRevealSchema>;
// Opener to pop-out frame, over the handshake's port.
export const CodePopoutInboundSchema = z.discriminatedUnion("type", [
  z.object({
    type: z.literal("init"),
    snapshot: CodeWorkspaceSnapshotSchema.nullable(),
    host: CodeHostStateSchema,
  }),
  z.object({ type: z.literal("host"), host: CodeHostStateSchema }),
  z.object({
    type: z.literal("reveal"),
    reveal: CodeRevealSchema,
  }),
  z.object({
    type: z.literal("pasteResult"),
    requestId: z.number().int(),
    result: CodeInsertionSchema,
  }),
  z.object({ type: z.literal("collect"), requestId: z.number().int() }),
]);
// Pop-out frame to opener.
export const CodePopoutOutboundSchema = z.discriminatedUnion("type", [
  z.object({ type: z.literal("ready") }),
  z.object({
    type: z.literal("paste"),
    requestId: z.number().int(),
    text: z.string(),
  }),
  z.object({
    type: z.literal("snapshot"),
    requestId: z.number().int().optional(),
    snapshot: CodeWorkspaceSnapshotSchema,
  }),
  z.object({ type: z.literal("close") }),
]);
export type CodePopoutInbound = z.infer<typeof CodePopoutInboundSchema>;
export type CodePopoutOutbound = z.infer<typeof CodePopoutOutboundSchema>;
