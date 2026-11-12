import type { Component } from "svelte";
import { api, apiBlob, uploadForm } from "../../platform/api";
import { saveBlob } from "../../platform/download";
import { formatBytes, rawError } from "./code-image";
import {
  CodeCopyRequestSchema,
  CodeMkdirRequestSchema,
  CodeMutationSchema,
  CodeNameSchema,
  CodeRenameRequestSchema,
  CodeUsageSchema,
  NoContentSchema,
  type CodeNavigatorState,
  type CodeNode,
} from "../../contracts/schemas";

/** A tree entry the menu acts on; the root has the empty path. */
export type FileTarget = Pick<CodeNode, "nodeId" | "path" | "name" | "kind">;

export type CodeFileClipboard = NonNullable<CodeNavigatorState["clipboard"]>;

export type CodeFileActionItem = {
  label: string;
  icon?: Component;
  run: () => void;
  disabled?: boolean;
  destructive?: boolean;
  // Actions of one group sit between separators.
  group: number;
};

export function makeRootFileTarget(rootPath: string | undefined): FileTarget {
  return {
    nodeId: "",
    path: "",
    name: rootPath?.split("/").filter(Boolean).at(-1) ?? "根目录",
    kind: "directory",
  };
}

export function buildCodeFileActions(input: {
  node: FileTarget | undefined;
  writable: boolean;
  pasteReady: boolean;
  clipboard: CodeFileClipboard | undefined;
  contextId: string;
  icons: {
    eye: Component;
    download: Component;
    fileZip: Component;
    upload: Component;
    folderPlus: Component;
    pencil: Component;
    copy: Component;
    scissors: Component;
    clipboard: Component;
    trash: Component;
  };
  openFile: (path: string, nodeId: string) => void;
  save: (node: FileTarget, archive: boolean) => void;
  openUploadPicker: (node: FileTarget) => void;
  startNaming: (kind: "rename" | "mkdir", node: FileTarget) => void;
  setClipboard: (clipboard: CodeFileClipboard) => void;
  paste: (node: FileTarget) => void;
  askRemove: (node: FileTarget) => void;
}): CodeFileActionItem[] {
  const { node, writable, pasteReady, clipboard, icons } = input;
  if (!node) return [];
  if (node.kind === "boundary" || node.kind === "special")
    return [
      { label: "此类条目不支持文件操作", disabled: true, group: 0, run() {} },
    ];
  const folder = node.kind === "directory";
  const editable = node.path !== "" && (node.kind === "file" || folder);
  const hold = (mode: CodeFileClipboard["mode"]) => () => {
    input.setClipboard({
      contextId: input.contextId,
      nodes: [node],
      mode,
    });
  };
  const list: CodeFileActionItem[] = [];
  if (node.kind === "file")
    list.push(
      {
        label: "打开",
        icon: icons.eye,
        group: 0,
        run: () => input.openFile(node.path, node.nodeId),
      },
      {
        label: "下载",
        icon: icons.download,
        group: 0,
        run: () => input.save(node, false),
      },
    );
  if (folder)
    list.push(
      {
        label: "下载为 ZIP",
        icon: icons.fileZip,
        group: 0,
        run: () => input.save(node, true),
      },
      {
        label: "上传到此目录",
        icon: icons.upload,
        group: 1,
        disabled: !writable,
        run: () => input.openUploadPicker(node),
      },
      {
        label: "新建文件夹",
        icon: icons.folderPlus,
        group: 1,
        disabled: !writable,
        run: () => input.startNaming("mkdir", node),
      },
    );
  if (editable)
    list.push(
      {
        label: "重命名",
        icon: icons.pencil,
        group: 2,
        disabled: !writable,
        run: () => input.startNaming("rename", node),
      },
      { label: "复制", icon: icons.copy, group: 2, run: hold("copy") },
      {
        label: "剪切",
        icon: icons.scissors,
        group: 2,
        disabled: !writable,
        run: hold("move"),
      },
    );
  if (folder)
    list.push({
      label: `粘贴${pasteReady && clipboard ? `（${clipboard.mode === "move" ? "移动" : "复制"} ${clipboard.nodes[0]?.name ?? ""}）` : ""}`,
      icon: icons.clipboard,
      group: 2,
      disabled: !writable || !pasteReady,
      run: () => input.paste(node),
    });
  if (node.path !== "")
    list.push({
      label: "删除…",
      icon: icons.trash,
      group: 3,
      destructive: true,
      disabled: !writable,
      run: () => input.askRemove(node),
    });
  return list;
}

function buildCodeNodeQuery(
  contextId: string,
  node: FileTarget,
  extra: Record<string, string> = {},
) {
  return new URLSearchParams({
    contextId,
    nodeId: node.nodeId,
    ...extra,
  });
}

export function toMenuAnchor(at: HTMLElement | { x: number; y: number }) {
  return at instanceof HTMLElement
    ? at
    : { getBoundingClientRect: () => new DOMRect(at.x, at.y, 0, 0) };
}

export async function runNamingMutation(
  base: string,
  contextId: string,
  kind: "rename" | "mkdir",
  node: FileTarget,
  name: string,
  changed: (removed: string[]) => void,
  forget: (paths: string[]) => void,
): Promise<{ notice?: string; error?: string }> {
  const checked = CodeNameSchema.safeParse(name);
  if (!checked.success) {
    return { error: checked.error.issues[0]?.message ?? "名称无效" };
  }
  const result =
    kind === "rename"
      ? await api(
          `${base}/rename`,
          CodeMutationSchema,
          "POST",
          { contextId, nodeId: node.nodeId, name },
          undefined,
          CodeRenameRequestSchema,
        )
      : await api(
          `${base}/mkdir`,
          CodeMutationSchema,
          "POST",
          { contextId, parentNodeId: node.nodeId, name },
          undefined,
          CodeMkdirRequestSchema,
        );
  const placed = result.nodes[0]?.name ?? name;
  if (kind === "rename" && placed !== node.name) {
    forget([node.path]);
    changed([node.path]);
  } else changed([]);
  return {
    notice: placed === name ? "" : `名称已被占用，已改为 ${placed}`,
  };
}

export async function requestCodePaste(
  base: string,
  contextId: string,
  current: CodeFileClipboard,
  into: FileTarget,
  changed: (removed: string[]) => void,
  clearClipboard: () => void,
): Promise<string> {
  const result = await api(
    `${base}/copy`,
    CodeMutationSchema,
    "POST",
    {
      contextId,
      nodeIds: current.nodes.map((n) => n.nodeId),
      targetNodeId: into.nodeId,
      mode: current.mode,
    },
    undefined,
    CodeCopyRequestSchema,
  );
  if (current.mode === "move") {
    clearClipboard();
    changed(
      current.nodes
        .filter((n, i) => result.nodes[i]?.path !== n.path)
        .map((n) => n.path),
    );
  } else changed([]);
  const renamed = result.nodes.filter(
    (n, i) => n.name !== current.nodes[i]?.name,
  );
  return renamed.length
    ? `同名条目已存在，已粘贴为 ${renamed.map((n) => n.name).join("、")}`
    : `已${current.mode === "move" ? "移动" : "复制"} ${result.nodes.length} 项`;
}

export async function describeCodeRemoveDetail(
  base: string,
  contextId: string,
  node: FileTarget,
): Promise<string> {
  if (node.kind !== "directory")
    return `将删除${node.kind === "symlink" ? "符号链接（不影响其目标）" : "文件"} ${node.path}。此操作不能撤销。`;
  const usage = await api(
    `${base}/usage?${buildCodeNodeQuery(contextId, node)}`,
    CodeUsageSchema,
  );
  return `将递归删除文件夹 ${node.path} 及其中 ${usage.entries - 1}${usage.limited ? "+" : ""} 个条目（${formatBytes(usage.bytes)}）。此操作不能撤销。`;
}

export async function requestCodeRemove(
  base: string,
  contextId: string,
  node: FileTarget,
  forget: (paths: string[]) => void,
  changed: (removed: string[]) => void,
): Promise<string> {
  await api(
    `${base}/node?${buildCodeNodeQuery(contextId, node, node.kind === "directory" ? { recursive: "1" } : {})}`,
    NoContentSchema,
    "DELETE",
  );
  forget([node.path]);
  changed([node.path]);
  return `已删除 ${node.name}`;
}

export async function downloadCodeNode(input: {
  base: string;
  contextId: string;
  node: FileTarget;
  archive: boolean;
  includeIgnored: boolean;
  signal: AbortSignal;
  ownerDocument: Document;
  filename: string;
  onProgress: (bytes: number) => void;
}) {
  const blob = await apiBlob(
    input.archive
      ? `${input.base}/archive?${buildCodeNodeQuery(input.contextId, input.node, { includeIgnored: String(input.includeIgnored) })}`
      : `${input.base}/node?${buildCodeNodeQuery(input.contextId, input.node)}`,
    input.signal,
    input.onProgress,
  );
  if (!input.signal.aborted)
    saveBlob(input.ownerDocument, blob, input.filename);
}

export function rawDownloadError(e: unknown) {
  return rawError(e, "下载");
}

export async function uploadCodeFilesBatch(input: {
  base: string;
  contextId: string;
  into: FileTarget;
  files: File[];
  signal: AbortSignal;
  onProgress: (progress: {
    name: string;
    percent: number;
    index: number;
    total: number;
    waiting: boolean;
  }) => void;
  onCleanup: (storedCount: number) => void;
}): Promise<string> {
  const stored: string[] = [];
  try {
    for (const [index, file] of input.files.entries()) {
      const current = {
        name: file.name,
        percent: 0,
        index: index + 1,
        total: input.files.length,
        waiting: false,
      };
      input.onProgress(current);
      const result = await uploadForm(
        `${input.base}/upload?${buildCodeNodeQuery(input.contextId, input.into)}`,
        file,
        CodeMutationSchema,
        (percent) => input.onProgress({ ...current, percent }),
        input.signal,
      );
      stored.push(...result.nodes.map((n) => n.name));
    }
    return `已上传 ${stored.join("、")}`;
  } finally {
    input.onCleanup(stored.length);
  }
}

export function selectFileStem(
  input: HTMLInputElement,
  kind: "rename" | "mkdir",
  nodeKind: string,
  name: string,
) {
  const dot =
    kind === "rename" && nodeKind === "file" ? name.lastIndexOf(".") : -1;
  input.focus();
  input.setSelectionRange(0, dot > 0 ? dot : name.length);
}

export function dropClipboardPaths(
  clipboard: CodeFileClipboard | undefined,
  paths: string[],
): CodeFileClipboard | undefined {
  if (!clipboard) return undefined;
  const nodes = clipboard.nodes.filter(
    (n) => !paths.some((p) => n.path === p || n.path.startsWith(p + "/")),
  );
  return nodes.length ? { ...clipboard, nodes } : undefined;
}

export function cloneClipboard(
  clipboard: CodeFileClipboard | undefined,
): CodeFileClipboard | undefined {
  if (!clipboard) return undefined;
  const { contextId, nodes, mode } = clipboard;
  return {
    contextId,
    mode,
    nodes: nodes.map(({ nodeId, path, name, kind }) => ({
      nodeId,
      path,
      name,
      kind,
    })),
  };
}
