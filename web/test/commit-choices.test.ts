import { expect, test } from "vitest";
import {
  commitChoiceLabel,
  commitChoices,
  relativeTime,
} from "../src/domains/workspace/commit-choices";

const now = Date.parse("2026-11-04T12:00:00Z");
const commit = (hash: string, subject: string, committedAt: string) => ({
  hash,
  subject,
  author: "dev",
  committedAt,
});
const a = "a".repeat(40);
const b = "b".repeat(40);

test("relative time picks the largest whole unit and tolerates bad dates", () => {
  expect(relativeTime("2026-11-04T09:00:00Z", now)).toBe("3小时前");
  expect(relativeTime("2026-11-02T12:00:00Z", now)).toBe("前天");
  expect(relativeTime("2026-11-04T11:59:40Z", now)).toBe("刚刚");
  expect(relativeTime("not a date", now)).toBe("");
});

test("rows carry title, short hash, age and the tags of matching tips", () => {
  const rows = commitChoices(
    [
      commit(b, "上游修复", "2026-11-04T10:00:00Z"),
      commit(a, "", "2026-11-02T12:00:00Z"),
    ],
    {
      tips: [
        { hash: b, tag: "远程最新" },
        { hash: a, tag: "本地最新" },
      ],
      base: a,
      baseSubject: "",
      now,
    },
  );
  expect(rows).toEqual([
    {
      value: b,
      subject: "上游修复",
      shortHash: "bbbbbbb",
      tags: ["远程最新"],
      age: "2小时前",
    },
    {
      value: a,
      subject: "无标题提交",
      shortHash: "aaaaaaa",
      tags: ["本地最新"],
      age: "前天",
    },
  ]);
  expect(commitChoiceLabel(rows[0]!)).toBe(
    "上游修复 · bbbbbbb · 远程最新 · 2小时前",
  );
});

test("a base outside the history stays selectable as the first row", () => {
  const c = "c".repeat(40);
  const rows = commitChoices([commit(a, "x", "2026-11-04T09:00:00Z")], {
    tips: [{ hash: c, tag: "远程最新" }],
    base: c,
    baseSubject: "远程提交",
    now,
  });
  expect(rows.map((row) => row.value)).toEqual([c, a]);
  expect(commitChoiceLabel(rows[0]!)).toBe("远程提交 · ccccccc · 远程最新");
  expect(
    commitChoices([], { tips: [], base: "", baseSubject: "", now }),
  ).toEqual([]);
});
