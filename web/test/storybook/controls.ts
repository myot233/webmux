import { expect, type Locator } from "@playwright/test";
import type { Story } from "./cases";

const requiredButtons: Record<
  Story,
  { name: string | RegExp; count: number }[]
> = {
  "session-row": [{ name: /^会话菜单 /, count: 1 }],
  "workspace-group": [
    { name: "启动会话", count: 1 },
    { name: /^工作树菜单 /, count: 1 },
    { name: /^会话菜单 /, count: 2 },
  ],
  "quick-command": [
    { name: "保存", count: 1 },
    { name: "取消", count: 1 },
    { name: "Close", count: 1 },
  ],
  "quick-command-error": [
    { name: "保存", count: 1 },
    { name: "取消", count: 1 },
    { name: "Close", count: 1 },
  ],
  "close-session": [
    { name: "取消", count: 1 },
    { name: "确认关闭", count: 1 },
  ],
  "draft-composer": [
    { name: "清空草稿", count: 1 },
    { name: "发送", count: 1 },
  ],
  "code-changes": [{ name: /unbroken_identifier/, count: 3 }],
  "workspace-dialog": [{ name: "关闭预览", count: 1 }],
};

export async function assertRequiredButtons(scope: Locator, story: Story) {
  for (const { name, count } of requiredButtons[story]) {
    const buttons = scope.getByRole("button", { name, exact: true });
    await expect(buttons).toHaveCount(count);
    for (const button of await buttons.all())
      await expect(button).toBeVisible();
  }
}
