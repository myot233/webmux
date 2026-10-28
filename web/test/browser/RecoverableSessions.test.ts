import { afterEach, expect, test, vi } from "vitest";
import { page, userEvent } from "vitest/browser";
import { cleanup, render } from "vitest-browser-svelte/pure";
import RecoverableSessions from "../../src/components/RecoverableSessions.svelte";
import type { ResumeOutcome } from "../../src/recoverable";
import type { HybridRecord, SessionInfo } from "../../src/schemas";

afterEach(async () => {
  await cleanup();
  await userEvent.cleanup();
});

const record: HybridRecord = {
  id: "hybrid22",
  name: "修复排序",
  configId: "cfgomp01",
  configMissing: false,
  agent: "omp",
  cwd: "/repos/project",
  cmd: ["omp"],
  side: "acp",
  agentSessionId: "01a",
  previousIds: [],
  state: "interrupted",
  queuedCount: 0,
  createdAt: "2026-10-28T00:00:00Z",
  updatedAt: "2026-10-28T00:00:00Z",
  interruptedAt: "2026-10-28T00:00:00Z",
};
const resumed = { id: record.id } as SessionInfo;
const now = Date.parse("2026-10-28T00:05:00Z");

function show(
  records: HybridRecord[],
  resume: (...args: unknown[]) => Promise<ResumeOutcome>,
) {
  const remove = vi.fn(async () => {});
  const onresumed = vi.fn();
  render(RecoverableSessions, { records, now, resume, remove, onresumed });
  return { remove, onresumed };
}
const group = () => page.getByRole("region", { name: "可恢复" });
const expand = () =>
  group()
    .getByRole("button", { name: /可恢复/ })
    .click();

test("is hidden without records and collapsed by default with a count", async () => {
  show([], vi.fn());
  expect(group().elements()).toHaveLength(0);
  await cleanup();
  show(
    [
      { ...record, activity: "turn", queuedCount: 2 },
      { ...record, id: "hybrid33", state: "closed", activity: "turn" },
    ],
    vi.fn(),
  );
  await expect.element(group()).toMatchTextContent(/可恢复\s*2/);
  expect(page.getByRole("button", { name: "恢复到聊天" }).elements()).toEqual(
    [],
  );
  await expand();
  await expect.element(group().getByText("已中断")).toBeVisible();
  await expect.element(group().getByText("已关闭")).toBeVisible();
  await expect
    .element(group().getByText("/repos/project").first())
    .toBeVisible();
  await expect.element(group().getByText("5 分钟前").first()).toBeVisible();
  // Only the interrupted record warns; a closed one finished cleanly.
  expect(
    group()
      .getByText(/上一轮可能未完成/)
      .elements(),
  ).toHaveLength(1);
  await expect.element(group().getByText(/有 2 条排队消息/)).toBeVisible();
});

test("resumes to either side", async () => {
  const resume = vi.fn(async () => ({
    status: "resumed" as const,
    session: resumed,
  }));
  const { onresumed } = show([record], resume);
  await expand();
  await page.getByRole("button", { name: "恢复到终端" }).click();
  expect(resume).toHaveBeenCalledWith("hybrid22", "tui", {});
  await vi.waitFor(() => expect(onresumed).toHaveBeenCalledWith(resumed));
  await page.getByRole("button", { name: "恢复到聊天" }).click();
  expect(resume).toHaveBeenLastCalledWith("hybrid22", "acp", {});
});

test("asks which agent session to use when the recorded one is gone", async () => {
  const resume = vi
    .fn<(...args: unknown[]) => Promise<ResumeOutcome>>()
    .mockResolvedValueOnce({
      status: "candidates",
      message: "agent 会话 01a 已不存在",
      candidates: [
        {
          agentSessionId: "01b",
          title: "较早的对话",
          updatedAt: record.updatedAt,
        },
        { agentSessionId: "01c", title: "", updatedAt: record.updatedAt },
      ],
    })
    .mockResolvedValueOnce({ status: "resumed", session: resumed });
  const { onresumed } = show([record], resume);
  await expand();
  await page.getByRole("button", { name: "恢复到聊天" }).click();
  const dialog = page.getByRole("alertdialog");
  await expect.element(dialog).toMatchTextContent(/01a 已不存在/);
  await dialog
    .getByRole("combobox", { name: "选择要恢复的会话" })
    .selectOptions("01c");
  await dialog.getByRole("button", { name: "确认" }).click();
  await vi.waitFor(() => expect(onresumed).toHaveBeenCalledOnce());
  expect(resume).toHaveBeenLastCalledWith("hybrid22", "acp", {
    agentSessionId: "01c",
  });
});

test("confirms resuming without the deleted launch config's environment", async () => {
  const resume = vi
    .fn<(...args: unknown[]) => Promise<ResumeOutcome>>()
    .mockResolvedValueOnce({
      status: "config_missing",
      message: "启动配置已删除",
    })
    .mockResolvedValueOnce({ status: "resumed", session: resumed });
  show([record], resume);
  await expand();
  await page.getByRole("button", { name: "恢复到终端" }).click();
  const dialog = page.getByRole("alertdialog");
  await expect
    .element(dialog)
    .toMatchTextContent(/不再带有该配置保存的环境变量/);
  await dialog.getByRole("button", { name: "确认" }).click();
  await vi.waitFor(() =>
    expect(resume).toHaveBeenLastCalledWith("hybrid22", "tui", {
      confirmWithoutEnv: true,
    }),
  );
});

test("a second question keeps the first answer", async () => {
  const resume = vi
    .fn<(...args: unknown[]) => Promise<ResumeOutcome>>()
    .mockResolvedValueOnce({
      status: "config_missing",
      message: "启动配置已删除",
    })
    .mockResolvedValueOnce({
      status: "candidates",
      message: "agent 会话 01a 已不存在",
      candidates: [
        { agentSessionId: "01b", title: "", updatedAt: record.updatedAt },
      ],
    })
    .mockResolvedValueOnce({ status: "resumed", session: resumed });
  const { onresumed } = show([record], resume);
  await expand();
  await page.getByRole("button", { name: "恢复到聊天" }).click();
  await page
    .getByRole("alertdialog")
    .getByRole("button", { name: "确认" })
    .click();
  await expect
    .element(page.getByRole("alertdialog"))
    .toMatchTextContent(/01a 已不存在/);
  await page
    .getByRole("alertdialog")
    .getByRole("button", { name: "确认" })
    .click();
  await vi.waitFor(() => expect(onresumed).toHaveBeenCalledOnce());
  expect(resume).toHaveBeenLastCalledWith("hybrid22", "acp", {
    confirmWithoutEnv: true,
    agentSessionId: "01b",
  });
});

test("removing needs confirmation and failures stay inline", async () => {
  const { remove } = show([record], vi.fn());
  await expand();
  await page.getByRole("button", { name: "移除" }).click();
  const dialog = page.getByRole("alertdialog");
  await expect.element(dialog).toMatchTextContent(/修复排序/);
  await dialog.getByRole("button", { name: "取消" }).click();
  expect(remove).not.toHaveBeenCalled();
  await page.getByRole("button", { name: "移除" }).click();
  await page
    .getByRole("alertdialog")
    .getByRole("button", { name: "确认" })
    .click();
  await vi.waitFor(() => expect(remove).toHaveBeenCalledWith("hybrid22"));

  await cleanup();
  show(
    [record],
    vi.fn(async () => Promise.reject(new Error("网络中断"))),
  );
  await expand();
  await page.getByRole("button", { name: "恢复到聊天" }).click();
  await expect
    .element(page.getByRole("alert"))
    .toMatchTextContent(/无法恢复“修复排序”：网络中断/);
});
