import { afterEach, expect, test, vi } from "vitest";
import { page } from "vitest/browser";
import { cleanup, render } from "vitest-browser-svelte/pure";
import RepositorySidebar from "../../src/components/RepositorySidebar.svelte";
import type { Repository } from "../../src/schemas";

afterEach(() => cleanup());

test("many repositories scroll within the sidebar while long names stay inside its column", async () => {
  await page.viewport(1280, 900);
  const repositories: Repository[] = Array.from({ length: 40 }, (_, index) => ({
    id: `repository-${index}`,
    name: `project-${index}-${"long-name-".repeat(12)}`,
    anchorPath: `/projects/${"nested/".repeat(20)}project-${index}`,
    commonDir: `/projects/project-${index}/.git`,
    availability: "available",
    error: "",
    checkedAt: "2026-10-29T00:00:00Z",
    worktrees: [],
  }));
  const onselect = vi.fn();
  render(RepositorySidebar, {
    repositories,
    sessions: [],
    directCount: 0,
    selected: repositories[0]!.id,
    label: repositories[0]!.name,
    onselect,
  });
  // The page grid normally assigns this column width.
  const sidebar = document.querySelector("aside")!;
  sidebar.style.inlineSize = "14rem";
  await expect
    .element(page.getByRole("navigation", { name: "仓库导航" }))
    .toBeVisible();
  const list = sidebar.querySelector<HTMLElement>(
    '[data-slot="repository-list"]',
  )!;
  const selected = sidebar.querySelector(
    '[data-repository-scope="repository-0"]',
  )!;
  const description = document.getElementById(
    selected.getAttribute("aria-describedby")!,
  );
  expect(description?.textContent).toBe(repositories[0]!.anchorPath);
  expect(list.scrollHeight).toBeGreaterThan(list.clientHeight);
  expect(sidebar.getBoundingClientRect().height).toBeLessThan(innerHeight);
  for (const button of sidebar.querySelectorAll("button")) {
    expect(button.getBoundingClientRect().right).toBeLessThanOrEqual(
      sidebar.getBoundingClientRect().right + 1,
    );
  }
  await page.getByRole("button", { name: /直接会话/ }).click();
  expect(list.scrollTop).toBeGreaterThan(0);
  expect(onselect).toHaveBeenCalledWith("direct");
});
