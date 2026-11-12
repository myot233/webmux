import { afterEach, expect, test, vi } from "vitest";
import { page, userEvent } from "vitest/browser";
import { cleanup, render } from "vitest-browser-svelte/pure";
import ReleaseWorktreeDialog from "../../src/components/workspace/ReleaseWorktreeDialog.svelte";
import RemoveWorktreeDialog from "../../src/components/workspace/RemoveWorktreeDialog.svelte";
import type { PushCheck } from "../../src/contracts/schemas";
import { chat, record, worktree } from "./shell-fixture";

afterEach(async () => {
  try {
    await cleanup();
    await userEvent.cleanup();
  } finally {
    vi.unstubAllGlobals();
  }
});

const tree = worktree({
  id: "wt1",
  repositoryId: "repo1",
  kind: "linked",
  path: "/repos/webmux.worktrees/wt1",
  branch: "wt/fix-sort",
});
const kept = record({ id: "keptabcd", title: "分支聊天", cwd: tree.path });
const commit = "c".repeat(40);
const unpushed: PushCheck = { state: "unpushed", branches: [], more: 0 };

type Preview = Record<string, unknown>;

function preview(patch: Preview = {}): Preview {
  return {
    confirmationId: "conf1234",
    expiresAt: "2099-01-01T00:00:00Z",
    worktree: tree,
    sessions: [],
    archived: [kept],
    commit,
    push: unpushed,
    ignored: [],
    ignoredMore: 0,
    sizeComplete: true,
    blockers: [],
    warnings: [],
    forceAllowed: true,
    ...patch,
  };
}

function serve(body: Preview) {
  const fetch = vi.fn<typeof globalThis.fetch>(async (input) =>
    String(input).endsWith("-preview")
      ? Response.json(body)
      : new Response(null, { status: 204 }),
  );
  vi.stubGlobal("fetch", fetch);
  return fetch;
}

const removeDialog = () => page.getByRole("dialog", { name: "删除 worktree" });
const releaseDialog = () => page.getByRole("dialog", { name: "释放空间" });

function showRemove(
  patch: Preview = {},
  worktreePatch = {},
  forgetRecords = [],
) {
  const fetch = serve(preview(patch));
  render(RemoveWorktreeDialog, {
    worktree: { ...tree, ...worktreePatch },
    forgetRecords,
    onclose: vi.fn(),
    onchange: vi.fn(),
  });
  return fetch;
}

test("the green hide button unpins the worktree from the home page without removing it", async () => {
  const fetch = serve(preview());
  const onhide = vi.fn();
  const onclose = vi.fn();
  render(RemoveWorktreeDialog, {
    worktree: tree,
    onclose,
    onchange: vi.fn(),
    onhide,
  });
  // Offered beside the deletion actions, also when deletion is blocked.
  await expect
    .element(page.getByRole("button", { name: "确认安全删除" }))
    .toBeVisible();
  await page.getByRole("button", { name: "仅从主页隐藏" }).click();
  expect(onhide).toHaveBeenCalledOnce();
  expect(onclose).toHaveBeenCalledOnce();
  expect(
    fetch.mock.calls.some(([url]) => String(url).endsWith("/remove")),
  ).toBe(false);
});

test("the hide button is absent without a hide handler", async () => {
  showRemove();
  await expect.element(removeDialog()).toBeVisible();
  expect(page.getByRole("button", { name: "仅从主页隐藏" }).elements()).toEqual(
    [],
  );
});

test("removal keeps archived records and warns, without blocking, when the commit is unpushed", async () => {
  const fetch = showRemove();
  const dialog = removeDialog();
  await expect
    .element(dialog)
    .toMatchTextContent(/保留的已归档会话（之后可重建并恢复）.*分支聊天/);
  await expect
    .element(dialog)
    .toMatchTextContent(
      /该提交尚未推送到任何远程；仓库本地丢失或被清理后将无法重建，请先推送.*本地远程跟踪引用.*未联网核对/,
    );
  await expect
    .element(dialog)
    .toMatchTextContent(/未提交、未跟踪及忽略文件不会保留，只保留提交/);
  expect(dialog.getByText("将删除的已归档会话").elements()).toEqual([]);
  const confirm = dialog.getByRole("button", { name: "确认安全删除" });
  await expect.element(confirm).toBeEnabled();
  await confirm.click();
  expect(fetch.mock.calls.at(-1)?.[0]).toBe(
    "/api/repositories/repo1/worktrees/wt1/remove",
  );
});

test("removing a record with its worktree lists the record as deleted", async () => {
  showRemove({}, {}, [kept]);
  const dialog = removeDialog();
  await expect.element(dialog).toMatchTextContent(/所选聊天记录会一并删除/);
  await expect
    .element(dialog)
    .toMatchTextContent(/将删除的已归档聊天.*分支聊天/);
  expect(dialog.getByText("保留的已归档会话").elements()).toEqual([]);
  await expect
    .element(dialog.getByRole("button", { name: "确认安全删除" }))
    .toBeEnabled();
});

test("removal lists the pushed branches, and a missing remote or unknown check stays silent", async () => {
  showRemove({
    push: { state: "pushed", branches: ["origin/main", "origin/dev"], more: 1 },
  });
  await expect
    .element(removeDialog())
    .toMatchTextContent(/已包含在远程分支：origin\/main、origin\/dev 等 3 个/);
  expect(removeDialog().getByText("尚未推送").elements()).toEqual([]);
});

test.each(["noRemote", "unknown"] as const)(
  "removal shows no push warning for %s",
  async (state) => {
    showRemove({ push: { state, branches: [], more: 0 } });
    await expect.element(removeDialog()).toMatchTextContent(/保留的已归档会话/);
    expect(removeDialog().getByText("尚未推送").elements()).toEqual([]);
    expect(removeDialog().getByText("已包含在远程分支").elements()).toEqual([]);
    await expect
      .element(removeDialog().getByRole("button", { name: "确认安全删除" }))
      .toBeEnabled();
  },
);

test("a detached worktree keeps its commit warning only without kept records", async () => {
  showRemove({}, { detached: true, branch: null });
  await expect
    .element(removeDialog())
    .toMatchTextContent(/提交由保留的会话记录保护/);
  expect(
    removeDialog().getByText("detached 提交可能没有分支保护").elements(),
  ).toEqual([]);
  await cleanup();
  showRemove({ archived: [] }, { detached: true, branch: null });
  await expect
    .element(removeDialog())
    .toMatchTextContent(/detached 提交可能没有分支保护/);
});

test("without a resolvable commit the records stay unreleased and the dialog says so", async () => {
  showRemove({ commit: "", push: { state: "unknown", branches: [], more: 0 } });
  await expect
    .element(removeDialog())
    .toMatchTextContent(
      /无法保存提交，已归档会话将原样保留（只能查看或手动删除，不能重建）.*分支聊天/,
    );
  expect(removeDialog().getByText("保留的已归档会话").elements()).toEqual([]);
  await expect
    .element(removeDialog().getByRole("button", { name: "确认安全删除" }))
    .toBeEnabled();
});

test("a forced removal also warns for the chat sessions it will close", async () => {
  showRemove({
    archived: [],
    sessions: [chat({ id: "liveabcd", title: "运行中聊天", cwd: tree.path })],
    blockers: ["存在运行中的 WebMux 会话"],
  });
  const dialog = removeDialog();
  await dialog.getByRole("button", { name: "进入强制删除确认" }).click();
  const forced = page.getByRole("dialog", { name: "强制删除 worktree" });
  await expect
    .element(forced)
    .toMatchTextContent(/已归档的混合会话记录保留，只保存提交/);
  await expect.element(forced).toMatchTextContent(/尚未推送到任何远程/);
  await expect
    .element(forced.getByRole("button", { name: "关闭会话并强制删除" }))
    .toBeDisabled();
});

test("release warns about an unpushed commit without disabling the button", async () => {
  serve(preview());
  render(ReleaseWorktreeDialog, {
    worktree: tree,
    onclose: vi.fn(),
    onchange: vi.fn(),
  });
  const dialog = releaseDialog();
  await expect
    .element(dialog)
    .toMatchTextContent(/该提交尚未推送到任何远程.*请先推送.*未联网核对/);
  await expect
    .element(dialog.getByRole("button", { name: "释放空间" }))
    .toBeEnabled();
});

test("release shows pushed branches briefly and nothing for a missing remote", async () => {
  serve(
    preview({
      push: { state: "pushed", branches: ["origin/main"], more: 0 },
    }),
  );
  render(ReleaseWorktreeDialog, {
    worktree: tree,
    onclose: vi.fn(),
    onchange: vi.fn(),
  });
  await expect
    .element(releaseDialog())
    .toMatchTextContent(/已包含在远程分支：origin\/main/);
  await cleanup();
  serve(preview({ push: { state: "noRemote", branches: [], more: 0 } }));
  render(ReleaseWorktreeDialog, {
    worktree: tree,
    onclose: vi.fn(),
    onchange: vi.fn(),
  });
  await expect.element(releaseDialog()).toMatchTextContent(/保留的已归档会话/);
  expect(releaseDialog().getByText("尚未推送").elements()).toEqual([]);
  expect(releaseDialog().getByText("已包含在远程分支").elements()).toEqual([]);
});
