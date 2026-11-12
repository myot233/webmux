import { describe, expect, test } from "vitest";
import { APIError } from "../src/platform/api";
import {
  SymlinkTargetError,
  isExternalLink,
  isWorkspaceReference,
  parseChatCodeLink,
  resolveMarkdownReference,
  targetFailure,
} from "../src/domains/code/markdown-link";

const resolve = (reference: string, document = "docs/guide/intro.md") =>
  resolveMarkdownReference(document, reference);

describe("resolveMarkdownReference", () => {
  test("resolves siblings and parents against the document directory", () => {
    expect(resolve("config.md")).toEqual({
      kind: "path",
      path: "docs/guide/config.md",
      fragment: "",
    });
    expect(resolve("../../README.md")).toEqual({
      kind: "path",
      path: "README.md",
      fragment: "",
    });
    expect(resolve("./a/./b//c.md")).toEqual({
      kind: "path",
      path: "docs/guide/a/b/c.md",
      fragment: "",
    });
  });

  test("rejects paths that climb above the root", () => {
    expect(resolve("../../../../etc/passwd")).toEqual({ kind: "outside" });
    expect(resolve("../x", "README.md")).toEqual({ kind: "outside" });
    expect(resolve("/../x")).toEqual({ kind: "outside" });
  });

  test("decodes percent-encoding before applying dot segments", () => {
    expect(resolve("%2e%2e/%2E%2E/%2e%2e/secret", "a/b.md")).toEqual({
      kind: "outside",
    });
    expect(resolve("%2e%2e/x.md", "a/b.md")).toEqual({
      kind: "path",
      path: "x.md",
      fragment: "",
    });
    expect(resolve("my%20file.md", "README.md")).toEqual({
      kind: "path",
      path: "my file.md",
      fragment: "",
    });
    expect(resolve("%E0%A4%A.md")).toEqual({ kind: "invalid" });
    expect(resolve("a%00b.md")).toEqual({ kind: "invalid" });
  });

  test("a leading slash starts at the workspace root", () => {
    expect(resolve("/docs/deploy.md#systemd")).toEqual({
      kind: "path",
      path: "docs/deploy.md",
      fragment: "systemd",
    });
    expect(resolve("/")).toEqual({ kind: "path", path: "", fragment: "" });
  });

  test("directory trailing slashes collapse", () => {
    expect(resolve("./examples/", "README.md")).toEqual({
      kind: "path",
      path: "examples",
      fragment: "",
    });
    expect(resolve("..", "docs/a.md")).toEqual({
      kind: "path",
      path: "",
      fragment: "",
    });
  });

  test("ignores queries and decodes fragments", () => {
    expect(resolve("deploy.md?plain=1#%E5%AE%89%E8%A3%85-1", "a.md")).toEqual({
      kind: "path",
      path: "deploy.md",
      fragment: "安装-1",
    });
  });

  test("empty paths and bare fragments", () => {
    expect(resolve("")).toEqual({ kind: "invalid" });
    expect(resolve("?x=1")).toEqual({ kind: "invalid" });
    expect(resolve("#")).toEqual({ kind: "invalid" });
    expect(resolve("#安装")).toEqual({ kind: "anchor", fragment: "安装" });
    expect(resolve("#%E5%AE%89%E8%A3%85")).toEqual({
      kind: "anchor",
      fragment: "安装",
    });
  });
});

test("only scheme-less, authority-less addresses are workspace references", () => {
  expect(isWorkspaceReference("docs/a.md")).toBe(true);
  expect(isWorkspaceReference("/a.md")).toBe(true);
  expect(isWorkspaceReference("#top")).toBe(true);
  expect(isWorkspaceReference("")).toBe(false);
  expect(isWorkspaceReference("https://example.com/a.png")).toBe(false);
  expect(isWorkspaceReference("//example.com/a.png")).toBe(false);
  expect(isWorkspaceReference("javascript:alert(1)")).toBe(false);
  expect(isWorkspaceReference("data:image/png;base64,AA")).toBe(false);
  expect(isWorkspaceReference("file:///etc/passwd")).toBe(false);
});

test("targetFailure names the reason", () => {
  expect(targetFailure("链接目标", new APIError(403, "x"))).toBe(
    "链接目标不在工作区内",
  );
  expect(targetFailure("链接目标", new APIError(404, "x"))).toBe(
    "链接目标不存在",
  );
  expect(targetFailure("图片", new APIError(429, "x"))).toBe(
    "图片传输繁忙，可重试",
  );
  expect(targetFailure("图片", new APIError(410, "x"))).toBe(
    "图片内容已变化，请刷新",
  );
  expect(targetFailure("链接目标", new SymlinkTargetError())).toBe(
    "链接目标是符号链接，不在预览中打开",
  );
  expect(targetFailure("链接目标", new APIError(400, "无法解析路径"))).toBe(
    "链接目标不可读：无法解析路径",
  );
});

describe("parseChatCodeLink", () => {
  test("parses file:// URIs with line ranges, columns, suffixes and headings", () => {
    expect(
      parseChatCodeLink("file:///home/dev/repo/web/src/App.svelte#L42-L50"),
    ).toEqual({
      path: "/home/dev/repo/web/src/App.svelte",
      fromUri: true,
      line: 42,
      endLine: 50,
    });
    expect(
      parseChatCodeLink("file://localhost/home/dev/my%20file.ts#L10C5"),
    ).toEqual({
      path: "/home/dev/my file.ts",
      fromUri: true,
      line: 10,
      column: 5,
    });
    expect(parseChatCodeLink("file:///home/dev/main.go:24:8")).toEqual({
      path: "/home/dev/main.go",
      rawPath: "/home/dev/main.go:24:8",
      fromUri: true,
      line: 24,
      column: 8,
    });
    expect(
      parseChatCodeLink("file:///home/dev/README.md#%E5%AE%89%E8%A3%85"),
    ).toEqual({
      path: "/home/dev/README.md",
      fromUri: true,
      heading: "安装",
    });
  });

  test("parses relative and absolute paths with hash or colon line anchors", () => {
    expect(parseChatCodeLink("web/src/App.svelte#L12")).toEqual({
      path: "web/src/App.svelte",
      line: 12,
    });
    expect(parseChatCodeLink("web/src/App.svelte#L20-10")).toEqual({
      path: "web/src/App.svelte",
      line: 10,
      endLine: 20,
    });
    expect(parseChatCodeLink("/repo/internal/api/server.go:88-96")).toEqual({
      path: "/repo/internal/api/server.go",
      rawPath: "/repo/internal/api/server.go:88-96",
      line: 88,
      endLine: 96,
    });
    expect(parseChatCodeLink("Makefile:15")).toEqual({
      path: "Makefile",
      rawPath: "Makefile:15",
      line: 15,
    });
    expect(parseChatCodeLink("main.go:10:4")).toEqual({
      path: "main.go",
      rawPath: "main.go:10:4",
      line: 10,
      column: 4,
    });
    expect(parseChatCodeLink("docs/deploy.md?plain=1#systemd")).toEqual({
      path: "docs/deploy.md",
      heading: "systemd",
    });
  });

  test("rejects external URLs, remote file hosts, bare fragments and unsafe schemes", () => {
    expect(isExternalLink("https://example.com/a")).toBe(true);
    expect(isExternalLink("mailto:dev@example.com")).toBe(true);
    expect(isExternalLink("file:///tmp/a.ts")).toBe(false);
    for (const value of [
      "",
      "   ",
      "#section",
      "//example.com/path",
      "https://example.com/file.ts#L10",
      "mailto:dev@example.com",
      "javascript:alert(1)",
      "javascript:1",
      "data:text/html,hi",
      "blob:https://example.com/id",
      "vscode://file/repo/a.ts",
      "localhost:8080",
      "file://evil.example/etc/passwd",
      "file:relative.ts",
      "file:///tmp/a%00b.ts",
      "src/a%00b.ts",
    ]) {
      expect(parseChatCodeLink(value), value).toBeNull();
    }
  });
});
