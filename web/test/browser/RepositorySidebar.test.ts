import { afterEach, expect, test, vi } from "vitest";
import { page } from "vitest/browser";
import { cleanup, render } from "vitest-browser-svelte/pure";
import RepositorySidebar from "../../src/components/RepositorySidebar.svelte";
import type { Repository } from "../../src/schemas";
import { session } from "../../stories/fixtures";

let fixture: HTMLElement | undefined;
afterEach(async () => {
  await cleanup();
  vi.restoreAllMocks();
  fixture?.remove();
  fixture = undefined;
  window.scrollTo(0, 0);
});

function repositoryFixtures(count: number): Repository[] {
  return Array.from({ length: count }, (_, index) => ({
    id: `repository-${index}`,
    name: `project-${index}-${"long-name-".repeat(12)}`,
    anchorPath: `/projects/${"nested/".repeat(20)}project-${index}`,
    commonDir: `/projects/project-${index}/.git`,
    availability: "available",
    error: "",
    checkedAt: "2026-10-29T00:00:00Z",
    worktrees: [],
  }));
}

test("many repositories scroll within the sidebar while long names stay inside its column", async () => {
  await page.viewport(1280, 900);
  const repositories = repositoryFixtures(40);
  const onselect = vi.fn();
  render(RepositorySidebar, {
    repositories,
    sessions: [],
    directSessions: [],
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

test("aggregate waiting badges include direct and unregistered-repository sessions and update live", async () => {
  await page.viewport(1280, 900);
  const repositories = repositoryFixtures(1);
  const direct = [
    { ...session, id: "direct-waiting", workspace: null },
    {
      ...session,
      id: "unregistered-waiting",
      workspace: { repositoryId: "removed", worktreeId: "main" },
    },
    { ...session, id: "direct-idle", workspace: null, status: "idle" as const },
  ];
  const sessions = [
    ...direct,
    {
      ...session,
      id: "repository-waiting",
      workspace: { repositoryId: repositories[0]!.id, worktreeId: "main" },
    },
    {
      ...session,
      id: "repository-idle",
      status: "idle" as const,
      workspace: { repositoryId: repositories[0]!.id, worktreeId: "main" },
    },
  ];
  const view = await render(RepositorySidebar, {
    repositories,
    sessions,
    directSessions: direct,
    selected: "all",
    label: "全部仓库",
    onselect: vi.fn(),
  });
  const all = page.getByRole("button", { name: "全部仓库", exact: true });
  const directEntry = page.getByRole("button", {
    name: "直接会话",
    exact: true,
  });
  const repository = page.getByRole("button", {
    name: `仓库 ${repositories[0]!.name}`,
    exact: true,
  });
  await expect.element(all.getByText("待 3", { exact: true })).toBeVisible();
  await expect.element(all.getByText("5", { exact: true })).toBeVisible();
  await expect
    .element(directEntry.getByText("待 2", { exact: true }))
    .toBeVisible();
  await expect
    .element(directEntry.getByText("3", { exact: true }))
    .toBeVisible();
  await expect
    .element(repository.getByText("待 1", { exact: true }))
    .toBeVisible();
  await expect
    .element(repository.getByText("2", { exact: true }))
    .toBeVisible();
  await view.rerender({
    sessions: sessions.map((item) => ({ ...item, status: "idle" })),
    directSessions: direct.map((item) => ({ ...item, status: "idle" })),
  });
  expect(page.getByText(/^待 \d+$/).elements()).toHaveLength(0);
  await expect.element(all.getByText("5", { exact: true })).toBeVisible();
});

for (const installed of [false, true])
  test(`sidebar clears the changing header and fits the viewport (${installed ? "standalone" : "browser tab"})`, async () => {
    await page.viewport(1280, 900);
    const nativeMatchMedia = window.matchMedia.bind(window);
    const standalone = Object.assign(new EventTarget(), { matches: installed });
    vi.spyOn(window, "matchMedia").mockImplementation((query) =>
      query === "(display-mode: standalone)"
        ? (standalone as MediaQueryList)
        : nativeMatchMedia(query),
    );
    fixture = document.createElement("div");
    fixture.className = "safe-area-page";
    const header = document.createElement("header");
    header.className = "app-header";
    header.style.cssText = `height:112px;position:${installed ? "sticky" : "static"};top:0;z-index:10`;
    const content = document.createElement("div");
    content.style.cssText =
      "min-height:2000px;display:grid;grid-template-columns:15rem 1fr";
    fixture.append(header, content);
    document.body.append(fixture);
    await render(RepositorySidebar, {
      target: content,
      props: {
        repositories: repositoryFixtures(40),
        sessions: [],
        directSessions: [],
        selected: "all",
        label: "全部仓库",
        onselect: vi.fn(),
      },
    });
    const sidebar = content.querySelector("aside")!;
    const list = sidebar.querySelector<HTMLElement>(
      '[data-slot="repository-list"]',
    )!;
    window.scrollTo(0, 300);
    const checkGeometry = async () => {
      await expect
        .poll(() => Math.round(sidebar.getBoundingClientRect().top))
        .toBe(installed ? header.getBoundingClientRect().height + 24 : 24);
      expect(sidebar.getBoundingClientRect().bottom).toBeLessThanOrEqual(
        innerHeight,
      );
      expect(list.scrollHeight).toBeGreaterThan(list.clientHeight);
    };
    await checkGeometry();
    const previousHeight = list.clientHeight;
    header.style.height = "176px";
    await checkGeometry();
    await expect
      .poll(() => list.clientHeight)
      .toBe(previousHeight - (installed ? 64 : 0));
    if (installed) {
      standalone.matches = false;
      standalone.dispatchEvent(new Event("change"));
      await expect.poll(() => getComputedStyle(sidebar).top).toBe("24px");
    }
  });
