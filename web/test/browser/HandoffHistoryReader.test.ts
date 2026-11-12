import { afterEach, expect, test, vi } from "vitest";
import { page, userEvent } from "vitest/browser";
import { cleanup, render } from "vitest-browser-svelte/pure";
import HandoffHistoryReader from "../../src/components/chat/HandoffHistoryReader.svelte";

const sourceId = "a".repeat(32);
const handoffId = "b".repeat(32);
const id = (position: number) => position.toString(16).padStart(32, "0");

function serveHistory(secondRevision = 1) {
  vi.stubGlobal(
    "fetch",
    vi.fn<typeof fetch>(async (input) => {
      const url = new URL(String(input), location.origin);
      if (url.pathname.endsWith("/selection")) {
        const after = url.searchParams.get("afterPosition") ?? "0";
        const first = after === "0";
        return Response.json({
          sourceLocation: {
            cwd: "/source",
            repositoryId: "",
            worktreeId: "",
            newWorktreeBase: "",
            newWorktreeBranch: "",
          },
          sourceConversationId: sourceId,
          sourcePosition: "101",
          revision: first ? 1 : secondRevision,
          historyOrigin: "complete",
          reconcileRequired: false,
          manual: false,
          selectedCount: 99,
          omittedCount: 2,
          items: first
            ? Array.from({ length: 100 }, (_, index) => ({
                activityId: id(index + 1),
                position: String(index + 1),
                selected: index !== 0,
                reason: index === 0 ? "budget" : "",
              }))
            : [
                {
                  activityId: id(101),
                  position: "101",
                  selected: false,
                  reason: "budget",
                },
              ],
          nextPosition: first ? "100" : "101",
          done: !first,
        });
      }
      const activityId = url.pathname.split("/").at(-1)!;
      const position = activityId === id(1) ? "1" : "101";
      const text =
        position === "1" ? "First retained constraint" : "Later omitted result";
      return Response.json({
        exportable: true,
        exclusionReason: "",
        activityId,
        conversationId: sourceId,
        providerThreadId: "c".repeat(32),
        runId: "d".repeat(32),
        position,
        runOrdinal: "1",
        kind: "message",
        role: "agent",
        status: "completed",
        text,
        source: "live",
        providerItemId: "",
        byteLength: String(text.length),
        nextTextOffset: String(text.length),
        truncated: false,
        createdAt: "2026-11-12T00:00:00Z",
        attachments: [],
      });
    }),
  );
  render(HandoffHistoryReader, {
    handoffId,
    revision: 1,
    sourceId,
    point: "101",
    omittedCount: 2,
  });
}

afterEach(async () => {
  await cleanup();
  await userEvent.cleanup();
  vi.unstubAllGlobals();
});

const fragment = () => page.getByRole("textbox", { name: "省略历史片段" });

test("reads omitted items across bounded selection pages without including selected history", async () => {
  serveHistory();
  await page.getByRole("button", { name: "查看省略历史", exact: true }).click();
  await expect.element(fragment()).toHaveValue("First retained constraint");
  await expect
    .element(page.getByRole("button", { name: "上一项", exact: true }))
    .toBeDisabled();
  await page.getByRole("button", { name: "下一项", exact: true }).click();
  await expect.element(fragment()).toHaveValue("Later omitted result");
  await expect
    .element(page.getByRole("button", { name: "下一项", exact: true }))
    .toBeDisabled();
  await page.getByRole("button", { name: "上一项", exact: true }).click();
  await expect.element(fragment()).toHaveValue("First retained constraint");
});

test("retains the displayed history instead of mixing a changed preview revision", async () => {
  serveHistory(2);
  await page.getByRole("button", { name: "查看省略历史", exact: true }).click();
  await expect.element(fragment()).toHaveValue("First retained constraint");
  await page.getByRole("button", { name: "下一项", exact: true }).click();
  await expect
    .element(page.getByText("预览已变化，请刷新预览后重试", { exact: true }))
    .toBeVisible();
  await expect.element(fragment()).toHaveValue("First retained constraint");
});
