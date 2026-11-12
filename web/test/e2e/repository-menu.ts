import { expect, type Page } from "@playwright/test";

// Picks an item of a sidebar tree row's "⋯" menu (the phone drawer on narrow
// screens); `label` matches the start of the menu button's name.
async function sidebarMenu(page: Page, label: RegExp, item: string) {
  const tree = page.getByRole("tree", { name: "仓库与会话" });
  const open = page.getByRole("button", { name: "打开侧栏" });
  const trigger = tree.getByRole("button", { name: label }).first();
  // The sidebar renders after the page loads and a phone drawer may still be
  // sliding shut from an earlier step: retry from whichever form is shown.
  await expect(async () => {
    await expect(tree.or(open).first()).toBeVisible({ timeout: 2000 });
    if (!(await tree.isVisible())) await open.click({ timeout: 2000 });
    await trigger.click({ timeout: 2000 });
  }).toPass();
  await page.getByRole("menuitem", { name: item }).click();
}

const escape = (text: string) => text.replace(/[.*+?^${}()|[\]\\]/g, "\\$&");

/**
 * Picks an item of a repository's "⋯" menu in the sidebar tree, the one
 * place that holds repository actions.
 */
export async function repositoryMenu(
  page: Page,
  item: string,
  repositoryName?: string,
) {
  await sidebarMenu(
    page,
    new RegExp(`^仓库操作：${escape(repositoryName ?? "")}`),
    item,
  );
}

/**
 * Picks an item of a worktree's "⋯" menu in the sidebar tree (details and,
 * for a linked worktree, removal). The worktree row must be shown, that is
 * its repository unfolded.
 */
export async function worktreeMenu(
  page: Page,
  item: string,
  worktreeName: string,
) {
  await sidebarMenu(
    page,
    new RegExp(`^工作树操作：${escape(worktreeName)}$`),
    item,
  );
}

/**
 * Unfolds a repository's row in the sidebar tree (the phone drawer on
 * narrow screens) so its worktree rows, and their menus, are shown.
 */
export async function unfoldRepository(page: Page, repositoryName: string) {
  const tree = page.getByRole("tree", { name: "仓库与会话" });
  const open = page.getByRole("button", { name: "打开侧栏" });
  const toggle = tree.getByRole("button", {
    name: new RegExp(`^(展开|收起)${escape(repositoryName)}$`),
  });
  await expect(async () => {
    await expect(tree.or(open).first()).toBeVisible({ timeout: 2000 });
    if (!(await tree.isVisible())) await open.click({ timeout: 2000 });
    await expect(toggle).toBeVisible({ timeout: 2000 });
  }).toPass();
  if ((await toggle.getAttribute("aria-label"))?.startsWith("展开"))
    await toggle.click();
}

/** Asserts the repository panel overview's "主页 n / 共 m" worktree count. */
export async function expectHomeWorktrees(
  page: Page,
  shown: number,
  total: number,
  repositoryName?: string,
) {
  // A closed phone drawer stays in the DOM, so look only at the open dialog.
  const dialog = page.locator('[role="dialog"][data-state="open"]');
  await expect(async () => {
    await repositoryMenu(page, "仓库详情…", repositoryName);
    try {
      await expect(dialog).toContainText(`主页 ${shown} / 共 ${total}`, {
        timeout: 2000,
      });
    } finally {
      await page.keyboard.press("Escape");
      await expect(dialog).toHaveCount(0);
    }
  }).toPass();
}
