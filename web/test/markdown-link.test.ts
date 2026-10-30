import { describe, expect, test } from "vitest";
import { APIError } from "../src/api";
import {
  SymlinkTargetError,
  isWorkspaceReference,
  resolveMarkdownReference,
  targetFailure,
} from "../src/markdown-link";

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
