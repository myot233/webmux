import { tick } from "svelte";
import { afterEach, expect, test, vi } from "vitest";
import { page, userEvent } from "vitest/browser";
import { cleanup, render } from "vitest-browser-svelte/pure";
import { APIError } from "../../src/platform/api";
import type { SwitchSource } from "../../src/components/workspace/SwitchBranch.svelte";
import SwitchBranchDialog from "../../src/components/workspace/SwitchBranchDialog.svelte";
import type {
  Branches,
  SwitchPreview,
  SwitchTarget,
  Worktree,
} from "../../src/contracts/schemas";
import { base, branch, chat, repository, worktree } from "./shell-fixture";

afterEach(async () => {
  await cleanup();
  await userEvent.cleanup();
});

const head = "a".repeat(40);
const tip = "b".repeat(40);
const main = worktree();
const other = worktree({
  id: "other",
  kind: "linked",
  branch: "feat/taken",
  path: "/repos/webmux.worktrees/taken",
});
const repo = repository({ worktrees: [main, other] });
const branches: Branches = {
  branches: [
    branch("main", head, [main.path]),
    branch("feat/a", tip),
    branch("feat/b", tip),
    branch("feat/taken", head, [other.path]),
    branch("fix/b", tip),
  ],
  bases: [
    base("refs/remotes/origin/fix/b", tip),
    base("refs/remotes/origin/fix/c", tip),
  ],
  defaultBase: head,
  defaultBaseSubject: "",
  defaultBranch: "main",
  defaultParent: "/repos/webmux.worktrees",
};

function previewOf(
  target: SwitchTarget,
  patch: Partial<SwitchPreview> = {},
): SwitchPreview {
  const name =
    target.kind === "local"
      ? target.branch
      : target.kind === "remote"
        ? target.ref.slice(target.ref.indexOf("/") + 1)
        : target.kind === "new"
          ? target.name
          : null;
  return {
    confirmationId: "confirm1",
    expiresAt: "2026-10-28T00:05:00Z",
    worktree: main,
    target: {
      kind: target.kind,
      branch: name,
      commit: tip,
      subject: "目标提交",
      createsBranch: target.kind === "remote" || target.kind === "new",
      upstream: target.kind === "remote" ? target.ref : null,
    },
    sessions: [],
    blockers: [],
    warnings: [],
    ...patch,
  };
}

function setup(
  preview: (target: SwitchTarget) => SwitchPreview | Promise<SwitchPreview> = (
    target,
  ) => previewOf(target),
  switched: () => Promise<Worktree> = async () => ({
    ...main,
    branch: "feat/a",
  }),
) {
  const source = {
    branches: vi.fn<SwitchSource["branches"]>(async () => branches),
    commits: vi.fn<SwitchSource["commits"]>(async () => ({
      commits: [
        { hash: head, subject: "当前提交", author: "dev", committedAt: "" },
      ],
    })),
    preview: vi.fn<SwitchSource["preview"]>(async (_tree, target) =>
      preview(target),
    ),
    switch: vi.fn<SwitchSource["switch"]>(switched),
  } satisfies SwitchSource;
  const handlers = {
    onchange: vi.fn(),
    onnewworktree: vi.fn(),
    ongoto: vi.fn(),
    onclose: vi.fn(),
  };
  render(SwitchBranchDialog, {
    repository: repo,
    worktree: main,
    source,
    ...handlers,
  });
  return { source, handlers };
}

const dialog = () => page.getByRole("dialog", { name: "切换分支：main" });
const switchButton = () =>
  dialog().getByRole("button", { name: "切换", exact: true });
const preview = () => dialog().getByRole("region", { name: "切换预览" });
async function choose(name: string) {
  await dialog().getByRole("button", { name: "目标分支", exact: true }).click();
  await page.getByRole("option", { name, exact: true }).click();
}

test("a clean switch to a local branch previews, confirms and closes", async () => {
  const { source, handlers } = setup();
  await expect
    .element(dialog().getByRole("radio", { name: "已有分支" }))
    .toHaveAttribute("aria-checked", "true");
  await choose("feat/a");
  await expect.element(preview()).toMatchTextContent(/main\s*切换到\s*feat\/a/);
  expect(source.preview).toHaveBeenLastCalledWith(
    main,
    { kind: "local", branch: "feat/a" },
    expect.any(AbortSignal),
  );
  await switchButton().click();
  expect(source.switch).toHaveBeenCalledWith(main, {
    confirmationId: "confirm1",
    acknowledgeSessions: false,
  });
  await expect.poll(() => handlers.onclose.mock.calls.length).toBe(1);
  expect(handlers.onchange).toHaveBeenCalledOnce();
});

test("the current and occupied branches are listed but cannot be chosen", async () => {
  setup();
  await dialog().getByRole("button", { name: "目标分支", exact: true }).click();
  await expect
    .element(page.getByRole("option", { name: /^main\s*当前分支/ }))
    .toHaveAttribute("aria-disabled", "true");
  await expect
    .element(
      page.getByRole("option", { name: /^feat\/taken\s*已在 .*taken 检出/ }),
    )
    .toHaveAttribute("aria-disabled", "true");
  // A remote branch whose local name exists points at that branch instead.
  await expect
    .element(page.getByRole("option", { name: /^origin\/fix\/b\s*本地已有/ }))
    .toHaveAttribute("aria-disabled", "true");
  await page.getByRole("option", { name: "origin/fix/c", exact: true }).click();
  await expect
    .element(preview())
    .toMatchTextContent(/将新建分支 fix\/c，上游为 origin\/fix\/c/);
});

test("uncommitted changes disable the switch and offer a new worktree instead", async () => {
  const { handlers } = setup((target) =>
    previewOf(target, {
      confirmationId: "",
      expiresAt: null,
      blockers: [
        { code: "changes", message: "有未提交的修改，请先提交或贮藏" },
      ],
      warnings: [{ code: "untracked", message: "有未跟踪的文件" }],
    }),
  );
  await choose("feat/a");
  await expect
    .element(dialog().getByText("有未提交的修改，请先提交或贮藏"))
    .toBeVisible();
  await expect.element(dialog().getByText("有未跟踪的文件")).toBeVisible();
  await expect.element(switchButton()).toBeDisabled();
  await dialog().getByRole("button", { name: "改为在新工作树打开" }).click();
  expect(handlers.onclose).toHaveBeenCalledOnce();
  expect(handlers.onnewworktree).toHaveBeenCalledWith({
    mode: "existing",
    branch: "feat/a",
  });
});

test("a branch checked out elsewhere offers going to that worktree", async () => {
  const { handlers } = setup((target) =>
    previewOf(target, {
      confirmationId: "",
      expiresAt: null,
      blockers: [
        {
          code: "occupied",
          message: "分支 feat/b 已被另一个工作树检出",
          path: other.path,
        },
      ],
    }),
  );
  await choose("feat/b");
  await expect
    .element(dialog().getByText("分支 feat/b 已被另一个工作树检出"))
    .toBeVisible();
  await expect.element(switchButton()).toBeDisabled();
  expect(
    dialog().getByRole("button", { name: "改为在新工作树打开" }).elements(),
  ).toEqual([]);
  await dialog().getByRole("button", { name: "转到该工作树" }).click();
  expect(handlers.onclose).toHaveBeenCalledOnce();
  expect(handlers.ongoto).toHaveBeenCalledWith(other.path);
});

test("running sessions must be acknowledged before switching", async () => {
  const agent = chat({ id: "agent001", title: "修复排序", status: "running" });
  const { source, handlers } = setup((target) =>
    previewOf(target, {
      sessions: [agent],
      warnings: [
        {
          code: "sessions",
          message: "有 1 个运行中的会话；它们不会被关闭，会看到切换后的文件",
        },
      ],
    }),
  );
  await choose("feat/a");
  await expect.element(dialog().getByText(/修复排序 · 运行中/)).toBeVisible();
  await expect
    .element(dialog().getByText(/有 agent 正在工作，可能正在写文件/))
    .toBeVisible();
  await expect.element(switchButton()).toBeDisabled();
  // Sessions alone also leave a new worktree as the way around them.
  await expect
    .element(dialog().getByRole("button", { name: "改为在新工作树打开" }))
    .toBeVisible();
  await dialog()
    .getByRole("checkbox", { name: /我已了解/ })
    .click();
  await switchButton().click();
  expect(source.switch).toHaveBeenCalledWith(main, {
    confirmationId: "confirm1",
    acknowledgeSessions: true,
  });
  await expect.poll(() => handlers.onclose.mock.calls.length).toBe(1);
});

test("a refused confirmation previews again and keeps the dialog open", async () => {
  let reads = 0;
  const late = chat({ id: "agent002", title: "新会话" });
  const { source, handlers } = setup(
    (target) =>
      previewOf(target, {
        confirmationId: `confirm${++reads}`,
        sessions: reads > 1 ? [late] : [],
      }),
    async () => {
      throw new APIError(409, "会话集合已变化，请重新预览");
    },
  );
  await choose("feat/a");
  await expect.element(switchButton()).toBeEnabled();
  await switchButton().click();
  await expect
    .element(dialog().getByText(/会话集合已变化，请重新预览；已按当前状态/))
    .toBeVisible();
  await expect.element(dialog().getByText(/新会话/)).toBeVisible();
  expect(source.preview).toHaveBeenCalledTimes(2);
  expect(handlers.onchange).toHaveBeenCalledOnce();
  expect(handlers.onclose).not.toHaveBeenCalled();
  // The new preview needs its own acknowledgement.
  await expect.element(switchButton()).toBeDisabled();
});

test("a late preview of an earlier choice is dropped", async () => {
  const first = Promise.withResolvers<SwitchPreview>();
  const { source } = setup((target) =>
    target.kind === "local" && target.branch === "feat/a"
      ? first.promise
      : previewOf(target),
  );
  await choose("feat/a");
  await expect.poll(() => source.preview.mock.calls.length).toBe(1);
  await choose("feat/b");
  await expect.element(preview()).toMatchTextContent(/main\s*切换到\s*feat\/b/);
  expect(source.preview.mock.calls[0]![2].aborted).toBe(true);
  first.resolve(previewOf({ kind: "local", branch: "feat/a" }));
  // The component awaits this reply first; once it has settled, its
  // continuation has run and been dropped.
  await source.preview.mock.results[0]!.value;
  await tick();
  await expect.element(preview()).toMatchTextContent(/main\s*切换到\s*feat\/b/);
});

test("a new branch previews the typed name from the current branch", async () => {
  const { source, handlers } = setup((target) =>
    previewOf(target, {
      confirmationId: "",
      expiresAt: null,
      blockers: [{ code: "changes", message: "有未提交的修改" }],
    }),
  );
  await dialog().getByRole("radio", { name: "新建分支" }).click();
  const name = dialog().getByRole("textbox", { name: "新分支名称" });
  await expect
    .poll(() => (name.element() as HTMLInputElement).value)
    .toMatch(/^wt\//);
  await name.fill("feat/a");
  await expect.element(dialog().getByText(/已有同名本地分支/)).toBeVisible();
  await name.fill("feat/c");
  await expect
    .element(dialog().getByRole("button", { name: "起点", exact: true }))
    .toHaveTextContent("main");
  await expect
    .element(preview())
    .toMatchTextContent(/将新建分支 feat\/c，不设置上游/);
  expect(source.preview).toHaveBeenLastCalledWith(
    main,
    { kind: "new", name: "feat/c", startPoint: "main" },
    expect.any(AbortSignal),
  );
  await dialog().getByRole("button", { name: "改为在新工作树打开" }).click();
  expect(handlers.onnewworktree).toHaveBeenCalledWith({
    mode: "new",
    name: "feat/c",
    startBranch: "main",
  });
});

test("detaching lists the current history", async () => {
  const { source } = setup();
  await dialog().getByRole("radio", { name: "分离到提交" }).click();
  await dialog().getByRole("combobox", { name: "提交" }).click();
  await page
    .getByRole("option", { name: /^当前提交 · aaaaaaa · 当前/ })
    .click();
  await expect.element(preview()).toMatchTextContent(/分离 · bbbbbbbb/);
  expect(source.commits).toHaveBeenCalledWith(
    "repo1",
    [head],
    expect.any(AbortSignal),
  );
  expect(source.preview).toHaveBeenLastCalledWith(
    main,
    { kind: "detach", commit: head },
    expect.any(AbortSignal),
  );
});
