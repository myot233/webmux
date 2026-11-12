import { execFile } from "node:child_process";
import { promisify } from "node:util";
import { mkdir, rm, writeFile } from "node:fs/promises";
import { join } from "node:path";
import { test, expect } from "./fixtures";
import { repositoryMenu } from "./repository-menu";

const exec = promisify(execFile);
async function git(path: string, ...args: string[]) {
  return (
    await exec("git", [
      "-C",
      path,
      "-c",
      "user.name=WebMux QA",
      "-c",
      "user.email=qa@example.invalid",
      ...args,
    ])
  ).stdout.trim();
}

test("fetching remotes shows the new behind count, and stale registrations are pruned with their branches kept", async ({
  page,
  request,
  workDir,
}) => {
  // A local bare remote, the registered clone tracking it, and a second
  // clone that moves the upstream on without the first one knowing.
  const remote = join(workDir, "remote.git");
  const root = join(workDir, "maint-repo");
  const other = join(workDir, "other-clone");
  await mkdir(remote);
  await git(remote, "init", "--bare", "-b", "main");
  await mkdir(root);
  await git(root, "init", "-b", "main");
  await writeFile(join(root, "tracked.txt"), "original\n");
  await git(root, "add", ".");
  await git(root, "commit", "-m", "initial");
  await git(root, "remote", "add", "origin", remote);
  await git(root, "push", "-u", "origin", "main");
  await git(workDir, "clone", remote, other);
  await writeFile(join(other, "tracked.txt"), "upstream\n");
  await git(other, "commit", "-am", "upstream change");
  await git(other, "push", "origin", "main");

  const first = join(workDir, "maint-first");
  const second = join(workDir, "maint-second");
  await git(root, "worktree", "add", "-b", "wt/maint-first", first);
  await git(root, "worktree", "add", "-b", "wt/maint-second", second);
  const response = await request.post("/api/repositories", {
    data: { path: root },
  });
  expect(response.status()).toBe(201);

  await page.goto("/");
  await repositoryMenu(page, "仓库详情…");
  const dialog = page.getByRole("dialog", { name: "maint-repo" });
  await expect(dialog).toBeVisible();
  await dialog.getByRole("tab", { name: "工作树" }).click();
  const list = dialog.getByRole("list", { name: "仓库工作树列表" });
  await expect(list.getByLabel("落后上游 1 个提交")).toHaveCount(0);

  await dialog.getByRole("button", { name: "获取远程" }).click();
  await expect(dialog.getByRole("list", { name: "获取结果" })).toContainText(
    "origin：已获取",
  );
  await expect(list.getByLabel("落后上游 1 个提交")).toBeVisible();
  await page.keyboard.press("Escape");
  await expect(dialog).toHaveCount(0);

  // Two directories deleted outside WebMux leave stale registrations.
  await rm(first, { recursive: true, force: true });
  await rm(second, { recursive: true, force: true });
  await repositoryMenu(page, "管理工作树…");
  await expect(dialog).toBeVisible();
  await dialog.getByRole("button", { name: "清理失效登记（2）" }).click();
  await expect(
    dialog.getByRole("heading", { name: "确认批量清理" }),
  ).toBeVisible();
  await expect(dialog).toContainText("可安全清理 2 个");
  await dialog.getByRole("button", { name: "确认安全删除 (2)" }).click();
  await expect(dialog.getByText("已删除 2 个工作树。")).toBeVisible();
  await expect(
    dialog.getByRole("button", { name: /清理失效登记/ }),
  ).toHaveCount(0);

  const porcelain = await git(root, "worktree", "list", "--porcelain");
  expect(porcelain).not.toContain(first);
  expect(porcelain).not.toContain(second);
  for (const branch of ["wt/maint-first", "wt/maint-second"])
    expect(
      await git(root, "show-ref", "--verify", `refs/heads/${branch}`),
    ).toContain(branch);
});
