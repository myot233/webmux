import { afterEach, expect, test } from "vitest";
import { page, userEvent } from "vitest/browser";
import { cleanup, render } from "vitest-browser-svelte/pure";
import Harness from "./CodeImageHarness.svelte";
import {
  CodeDiffSchema,
  CodeFileSchema,
  type CodeDocument,
  type CodeFile,
} from "../../src/schemas";
import { htmlPreviewPolicy, staticHtml } from "../../src/code-html";

const now = "2026-10-24T00:00:00Z";

afterEach(async () => {
  await cleanup();
  await userEvent.cleanup();
});

function textDocument(
  path: string,
  text: string,
  overrides: Partial<CodeDocument> = {},
): CodeDocument {
  return {
    nodeId: btoa(path).replaceAll("=", ""),
    path,
    absolutePath: `/repo/${path}`,
    source: "worktree",
    oid: "",
    contentVersion: "v1",
    kind: "text",
    text,
    lineCount: text.split("\n").length,
    referenceable: true,
    reason: "",
    mediaType: "",
    size: text.length,
    downloadable: true,
    ...overrides,
  };
}
const fileOf = (document: CodeDocument, snapshotId = "snap-1"): CodeFile =>
  CodeFileSchema.parse({ snapshotId, checkedAt: now, document });
const previewButton = () =>
  page.getByRole("button", { name: "预览", exact: true });
const sourceButton = () => page.getByRole("button", { name: "返回源码" });
const reader = () => page.getByLabelText("只读代码");
const frame = () =>
  document.querySelector<HTMLIFrameElement>('iframe[title="HTML 静态预览"]');

test("format previews are offered only for full text documents with a known extension", async () => {
  const rendered = await render(Harness, {
    file: fileOf(textDocument("report.html", "<p>x</p>\n")),
  });
  await expect.element(previewButton()).toBeVisible();
  for (const path of ["index.HTM", "data.csv", "data.tsv", "result.json"]) {
    await rendered.rerender({ file: fileOf(textDocument(path, "x\n")) });
    await expect.element(previewButton()).toBeVisible();
  }
  const absent = [
    textDocument("notes.txt", "x\n"),
    textDocument("big.json", "", {
      kind: "too-large",
      reason: "超过 2 MiB",
      size: 3 * 1024 * 1024,
    }),
    textDocument("data.csv", "", { kind: "binary", reason: "二进制内容" }),
    textDocument("legacy.json", "", {
      kind: "unsupported",
      reason: "不支持的文本编码",
    }),
  ];
  for (const document of absent) {
    await rendered.rerender({ file: fileOf(document) });
    await expect
      .element(page.getByText(document.absolutePath).first())
      .toBeInTheDocument();
    expect(previewButton().query()).toBeNull();
  }
  // Diffs never preview these formats.
  const old = textDocument("data.json", '{"a": 1}\n', { source: "index" });
  const next = textDocument("data.json", '{"a": 2}\n');
  await rendered.rerender({
    file: undefined,
    diff: CodeDiffSchema.parse({
      snapshotId: "diff-1",
      checkedAt: now,
      kind: "text",
      reason: "",
      change: {
        nodeId: next.nodeId,
        changeId: "change",
        group: "unstaged",
        oldPath: "data.json",
        newPath: "data.json",
        status: "M",
        oldOid: "",
        newOid: "",
        oldMode: "100644",
        newMode: "100644",
      },
      old,
      new: next,
    }),
  });
  await expect
    .element(page.getByRole("button", { name: "完整文件", exact: true }))
    .toBeVisible();
  expect(previewButton().query()).toBeNull();
});

test("the preview resets to source on another file or a refreshed snapshot", async () => {
  const json = textDocument("result.json", '{"a": 1}\n');
  const rendered = await render(Harness, { file: fileOf(json) });
  await previewButton().click();
  await expect.element(page.getByLabelText("JSON 预览")).toBeVisible();
  await rendered.rerender({ file: fileOf(json, "snap-2") });
  await expect.element(reader()).toBeVisible();
  await expect
    .element(previewButton())
    .toHaveAttribute("aria-pressed", "false");
  await previewButton().click();
  await expect.element(page.getByLabelText("JSON 预览")).toBeVisible();
  await rendered.rerender({
    file: fileOf(textDocument("data.csv", "a,b\n1,2\n"), "snap-3"),
  });
  await expect.element(reader()).toBeVisible();
  expect(document.querySelector("table")).toBeNull();
});

test("HTML preprocessing removes active content, navigation and form targets", () => {
  const html = staticHtml(`<!doctype html><html><head>
<base href="https://example.com/">
<meta http-equiv="refresh" content="0;url=https://example.com">
<meta charset="utf-8">
<link rel="stylesheet" href="a.css">
<style>td { color: red }</style>
<script>window.htmlScript = true</script>
</head><body onload="window.htmlScript = true">
<a href="https://example.com" onclick="x()">link</a>
<map><area href="https://example.com" alt="a"></map>
<form action="/api/sessions"><button formaction="/api/sessions">go</button></form>
<img src="javascript:alert(1)" onerror="x()"><img src="data:image/png;base64,AAAA">
<img src="https://example.com/x.png" srcset="b.png 2x"><video poster="p.png"></video>
<img src="data:image/png;base64,AAAA" srcset="data:image/png;base64,AAAA 1x, https://example.com/x.png 2x">
<img src="&nbsp;data:image/png;base64,AAAA">
<iframe src="https://example.com"></iframe><object data="x"></object><embed src="x">
<frameset></frameset><portal src="x"></portal>
<svg><a href="https://example.com"><text>s</text></a></svg>
<div><template shadowrootmode="open"><a href="https://example.com">shadow</a></template></div>
<svg><a><set attributeName="href" to="https://example.com"/><animate attributeName="href" values="https://example.com"/><animateMotion dur="1s"/><text>t</text></a></svg>
<p style="color: blue">kept</p>
</body></html>`);
  const doc = new DOMParser().parseFromString(html, "text/html");
  expect(
    doc.querySelectorAll(
      'script, base, link, template, meta[http-equiv="refresh"], iframe, object, embed, frameset, portal',
    ),
  ).toHaveLength(0);
  const policy = doc.head.firstElementChild as HTMLMetaElement;
  expect(policy.httpEquiv).toBe("Content-Security-Policy");
  expect(policy.content).toBe(htmlPreviewPolicy);
  // The CSP would block remote references, but the browser would still try:
  // only inline data: references remain.
  expect(
    doc.querySelectorAll("[href], [action], [formaction], [srcset], [poster]"),
  ).toHaveLength(0);
  // A leading no-break space is not stripped by the URL parser, so that
  // value is a relative URL resolved against the app origin.
  expect(
    [...doc.querySelectorAll("[src]")].map((e) => e.getAttribute("src")),
  ).toEqual(["data:image/png;base64,AAAA", "data:image/png;base64,AAAA"]);
  // A declarative shadow root would go live in the frame, unsanitised, and
  // SVG animation would put an `href` back without any script.
  expect(html).not.toContain("shadowrootmode");
  expect(
    [...doc.querySelectorAll("*")].filter((e) =>
      /^(set|animate|animatemotion|animatetransform)$/i.test(e.localName),
    ),
  ).toHaveLength(0);
  for (const element of doc.querySelectorAll("*"))
    for (const attribute of element.attributes)
      expect(attribute.name.startsWith("on")).toBe(false);
  const images = doc.querySelectorAll("img");
  expect(images[0]?.hasAttribute("src")).toBe(false);
  expect(images[1]?.getAttribute("src")).toBe("data:image/png;base64,AAAA");
  expect(doc.querySelector("style")?.textContent).toContain("color: red");
  expect(doc.querySelector("p")?.getAttribute("style")).toBe("color: blue");
  expect(doc.querySelector("a")?.textContent).toBe("link");
});

test("HTML previews in an empty sandbox without scripts, submission or navigation", async () => {
  const text = `<html><head><style>.cell { border: 1px solid; }</style>
<meta http-equiv="refresh" content="0;url=about:blank#moved"></head><body>
<table><tr><td class="cell" style="color: rgb(0, 128, 0)">报告</td></tr></table>
<img alt="icon" src="data:image/svg+xml,%3Csvg xmlns='http://www.w3.org/2000/svg' width='4' height='4'/%3E">
<script>parent.htmlScript = true; location = "about:blank#script";</script>
<form action="/api/sessions" method="post"><button>提交</button></form>
<a href="https://example.com">外链</a></body></html>`;
  await render(Harness, { file: fileOf(textDocument("report.html", text)) });
  await previewButton().click();
  await expect
    .element(page.getByText("静态预览，脚本与外部资源已禁用"))
    .toBeVisible();
  const iframe = frame()!;
  expect(iframe.getAttribute("sandbox")).toBe("");
  expect(iframe.sandbox.length).toBe(0);
  const srcdoc = iframe.getAttribute("srcdoc") ?? "";
  expect(srcdoc).not.toMatch(/<script|http-equiv="refresh"|action=|href=/i);
  expect(srcdoc).toContain("报告");
  expect(srcdoc).toContain("data:image/svg+xml");
  // An opaque origin: the parent cannot reach the document, nor it the parent.
  expect(iframe.contentDocument).toBeNull();
  // Proving that a refresh or script navigation never happens needs a real
  // wait: there is no event for an absent load, and the frame is opaque. The
  // initial srcdoc load may land before or after the listener attaches.
  let loads = 0;
  iframe.addEventListener("load", () => loads++);
  const { promise, resolve } = Promise.withResolvers<void>();
  setTimeout(resolve, 500);
  await promise;
  expect(loads).toBeLessThanOrEqual(1);
  expect("htmlScript" in window).toBe(false);
  await sourceButton().click();
  await expect.element(reader()).toBeVisible();
  expect(frame()).toBeNull();
});

test("CSV and TSV render as tables with truncation and parse errors", async () => {
  const rows = Array.from({ length: 5000 }, (_, i) => `${i},"v ""${i}"""`);
  const rendered = await render(Harness, {
    file: fileOf(textDocument("data.csv", ["id,value", ...rows].join("\n"))),
  });
  await previewButton().click();
  await expect
    .element(page.getByText("仅显示前 2000 行，共 5000 行"))
    .toBeVisible();
  const table = page.getByRole("table", { name: "表格预览" });
  await expect.element(table).toBeVisible();
  expect(table.element().querySelectorAll("tbody tr")).toHaveLength(2000);
  expect(
    [...table.element().querySelectorAll("th")].map((th) => th.textContent),
  ).toEqual(["id", "value"]);
  expect(
    table.element().querySelector("tbody tr td:last-child")?.textContent,
  ).toBe('v "0"');
  const head = table.element().querySelector("thead")!;
  expect(getComputedStyle(head).position).toBe("sticky");

  await rendered.rerender({
    file: fileOf(
      textDocument("data.tsv", 'a\tb\n"x\ny"\t<b>z</b>\n'),
      "snap-2",
    ),
  });
  await previewButton().click();
  const cell = page.getByRole("cell", { name: "<b>z</b>" });
  await expect.element(cell).toBeVisible();
  expect(cell.element().querySelector("b")).toBeNull();
  expect(
    page.getByRole("table").element().querySelector("td")?.textContent,
  ).toBe("x\ny");

  await rendered.rerender({
    file: fileOf(textDocument("bad.csv", 'a,b\n1,"open\n'), "snap-3"),
  });
  await previewButton().click();
  await expect.element(page.getByRole("alert")).toBeVisible();
  expect(page.getByRole("alert").element().textContent).toContain(
    "第 2 行引号未闭合",
  );
  expect(document.querySelector("table")).toBeNull();
  // The failure offers its own way back beside the document bar's toggle.
  await sourceButton().last().click();
  await expect.element(reader()).toBeVisible();
});

test("JSON previews as a tree with raw numbers and batched members", async () => {
  const items = Array.from({ length: 1000 }, (_, i) => `{"n": ${i}}`);
  const rendered = await render(Harness, {
    file: fileOf(
      textDocument(
        "result.json",
        `{"id": 12345678901234567890, "ratio": 1.10, "items": [${items.join(", ")}], "deep": {"x": {"y": 1}}}`,
      ),
    ),
  });
  await previewButton().click();
  const tree = page.getByLabelText("JSON 预览");
  await expect.element(tree).toBeVisible();
  expect(tree.element().textContent).toContain("12345678901234567890");
  expect(tree.element().textContent).toContain("1.10");
  expect(tree.element().textContent).toContain("1000 项");
  const collections = () =>
    tree.element().querySelectorAll('[data-slot="collapsible"]').length;
  // Two levels are open: the root and its members (items, deep). Their
  // members (item objects, deep.x) show collapsed; deep.x's own stay unrendered.
  expect(collections()).toBe(1 + 1 + 200 + 1 + 1);
  expect(tree.element().textContent).not.toContain("n:");
  expect(tree.element().textContent).not.toContain("y:");
  await page.getByRole("button", { name: "再显示 200 项" }).click();
  await expect.poll(collections).toBe(1 + 1 + 400 + 1 + 1);

  await rendered.rerender({
    file: fileOf(
      textDocument("bad.json", '{\n  "a": 1\n  "b": 2\n}\n'),
      "snap-2",
    ),
  });
  await previewButton().click();
  await expect
    .element(page.getByRole("alert"))
    .toHaveTextContent("第 3 行第 3 列：JSON 语法错误");
  await sourceButton().last().click();
  await expect.element(reader()).toBeVisible();
});
