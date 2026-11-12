import { execFile } from "node:child_process";
import { mkdir, writeFile } from "node:fs/promises";
import { join } from "node:path";
import { promisify } from "node:util";
import type { APIRequestContext, Page } from "@playwright/test";
import { expect, test } from "./fixtures";
import { repositoryMenu } from "./repository-menu";

// Fast-forward, publish and branch cleanup against a local bare remote, all
// from the repository panel.

const exec = promisify(execFile);
const identity = {
  GIT_AUTHOR_NAME: "WebMux QA",
  GIT_AUTHOR_EMAIL: "qa@example.invalid",
  GIT_COMMITTER_NAME: "WebMux QA",
  GIT_COMMITTER_EMAIL: "qa@example.invalid",
};
async function git(cwd: string, ...args: string[]) {
  const { stdout } = await exec("git", ["-C", cwd, ...args], {
    env: { ...process.env, ...identity },
  });
  return stdout.trim();
}
async function commit(cwd: string, file: string, subject: string) {
  await writeFile(join(cwd, file), `${subject}\n`);
  await git(cwd, "add", file);
  await git(cwd, "commit", "-m", subject);
}

/** A repository whose `origin` is a bare repository next to it. */
async function project(workDir: string, request: APIRequestContext) {
  const root = join(workDir, "project");
  const remote = join(workDir, "remote.git");
  await mkdir(root);
  await git(workDir, "init", "--bare", "-b", "main", remote);
  await git(root, "init", "-b", "main");
  await commit(root, "README.md", "initial");
  await git(root, "remote", "add", "origin", remote);
  await git(root, "push", "-u", "origin", "main");
  const response = await request.post("/api/repositories", {
    data: { path: root },
  });
  expect(response.status(), await response.text()).toBe(201);
  const { id } = (await response.json()) as { id: string };
  return { root, remote, id };
}

async function openPanel(page: Page, tab: "工作树" | "分支") {
  await page.goto("/");
  await repositoryMenu(page, "仓库详情…", "project");
  const panel = page.getByRole("dialog", { name: "project" });
  await panel.getByRole("tab", { name: tab }).click();
  return panel;
}

test("a worktree behind its fetched upstream fast-forwards from the panel", async ({
  page,
  request,
  workDir,
}) => {
  const { root, remote } = await project(workDir, request);
  const tree = join(workDir, "feat-a");
  await git(root, "worktree", "add", "-b", "feat/a", tree);
  await git(tree, "push", "-u", "origin", "feat/a");
  // Someone else pushes two commits to feat/a.
  const other = join(workDir, "other");
  await git(workDir, "clone", "-b", "feat/a", remote, other);
  await commit(other, "a.txt", "upstream one");
  await commit(other, "b.txt", "upstream two");
  await git(other, "push");
  const tip = await git(other, "rev-parse", "HEAD");
  // Fetch from the panel's title bar; the counts come from its refs.
  const panel = await openPanel(page, "工作树");
  await panel.getByRole("button", { name: "获取远程" }).click();
  await expect(panel.getByRole("list", { name: "获取结果" })).toContainText(
    "origin：已获取",
  );
  const row = panel.getByRole("button", { name: "工作树操作：feat/a" });
  await expect(async () => {
    await row.click({ timeout: 2000 });
    await page
      .getByRole("menuitem", { name: "快进到上游" })
      .click({ timeout: 2000 });
  }).toPass();
  const flow = panel.getByRole("region", { name: "快进到上游" });
  await expect(
    flow.getByRole("region", { name: "将前进的提交" }),
  ).toContainText("upstream two");
  await flow.getByRole("button", { name: "快进到上游", exact: true }).click();
  await expect(flow.getByRole("status")).toContainText("已把 feat/a 快进到");
  expect(await git(tree, "rev-parse", "HEAD")).toBe(tip);
  await flow.getByRole("button", { name: "完成" }).click();
  await expect(panel.getByRole("tab", { name: "工作树" })).toBeVisible();
});

test("publishing a branch without an upstream leaves it in sync", async ({
  page,
  request,
  workDir,
}) => {
  const { root, remote } = await project(workDir, request);
  const tree = join(workDir, "feat-c");
  await git(root, "worktree", "add", "-b", "feat/c", tree);
  await commit(tree, "c.txt", "local change");

  const panel = await openPanel(page, "分支");
  const local = panel.getByRole("region", { name: "本地分支" });
  await local.getByRole("button", { name: "分支操作：feat/c" }).click();
  await page.getByRole("menuitem", { name: "发布分支…" }).click();
  const flow = panel.getByRole("region", { name: "发布分支" });
  await expect(flow.getByRole("combobox", { name: "远程" })).toHaveValue(
    "origin",
  );
  await expect(flow.getByRole("textbox", { name: "远端分支名" })).toHaveValue(
    "feat/c",
  );
  await expect(flow.getByText(remote, { exact: true })).toBeVisible();
  await flow.getByRole("button", { name: "发布", exact: true }).click();
  await expect(flow.getByRole("status")).toContainText(
    "已发布到 origin/feat/c",
  );
  expect(await git(root, "config", "branch.feat/c.merge")).toBe(
    "refs/heads/feat/c",
  );
  await flow.getByRole("button", { name: "完成" }).click();
  // Back on the branches, read again: no longer ahead.
  const row = local.getByRole("listitem").filter({ hasText: "origin/feat/c" });
  await expect(row).toContainText("已同步");
});

test("cleanup after removing a worktree deletes merged branches and leaves the squashed one unticked", async ({
  page,
  request,
  workDir,
}) => {
  const { root, id } = await project(workDir, request);
  // feat/old is merged into main; feat/sq was squash-merged, then its
  // remote branch deleted.
  const old = join(workDir, "feat-old");
  await git(root, "worktree", "add", "-b", "feat/old", old);
  await commit(old, "old.txt", "old work");
  await git(root, "merge", "--ff-only", "feat/old");
  await git(root, "branch", "feat/sq");
  await git(root, "switch", "feat/sq");
  await commit(root, "sq.txt", "squash me");
  await git(root, "push", "-u", "origin", "feat/sq");
  await git(root, "switch", "main");
  await git(root, "merge", "--squash", "feat/sq");
  await git(root, "commit", "-m", "squashed feat/sq");
  await git(root, "push", "origin", "--delete", "feat/sq");

  // Removing the worktree keeps its branch.
  const trees = (await (
    await request.get(`/api/repositories/${id}/worktrees`)
  ).json()) as { worktrees: { id: string; path: string }[] };
  const tree = trees.worktrees.find((item) => item.path === old)!;
  const route = `/api/repositories/${id}/worktrees/${tree.id}`;
  const preview = await (
    await request.post(`${route}/removal-preview`, { data: {} })
  ).json();
  expect(
    (
      await request.post(`${route}/remove`, {
        data: { confirmationId: preview.confirmationId, mode: "safe" },
      })
    ).status(),
  ).toBe(204);

  const panel = await openPanel(page, "分支");
  const local = panel.getByRole("region", { name: "本地分支" });
  await local.getByRole("button", { name: "清理分支（2）" }).click();
  const flow = panel.getByRole("region", { name: "清理分支" });
  await expect(page.getByRole("dialog")).toHaveCount(1);
  await expect(
    flow.getByRole("checkbox", { name: "删除 feat/old" }),
  ).toBeChecked();
  await expect(
    flow.getByRole("checkbox", { name: /^删除 feat\/sq/ }),
  ).not.toBeChecked();
  await expect(flow.getByRole("region", { name: "有独有提交" })).toContainText(
    "squash me",
  );
  await flow.getByRole("button", { name: "删除 1 个分支" }).click();
  const results = flow.getByRole("list", { name: "删除结果" });
  await expect(results).toContainText("feat/old");
  await expect(results).toContainText("已删除");
  expect(await git(root, "branch", "--list", "feat/old")).toBe("");
  expect(await git(root, "branch", "--list", "feat/sq")).not.toBe("");
  await flow.getByRole("button", { name: "完成" }).click();
  await expect(
    local.getByRole("button", { name: "清理分支（1）" }),
  ).toBeVisible();
});
