import { expect, test } from "vitest";
import { createSlugger, headingSlug } from "../src/markdown-slug";

test("slugs follow GitHub's rules", () => {
  expect(headingSlug("Hello World")).toBe("hello-world");
  expect(headingSlug("API: v2 (beta)!")).toBe("api-v2-beta");
  expect(headingSlug("snake_case and-kebab")).toBe("snake_case-and-kebab");
  expect(headingSlug("安装与配置")).toBe("安装与配置");
  expect(headingSlug("第 1 步：安装")).toBe("第-1-步安装");
  expect(headingSlug("🚀 Launch")).toBe("-launch");
  expect(headingSlug("Ünïcödé Straße")).toBe("ünïcödé-straße");
  expect(headingSlug("  two  spaces ")).toBe("--two--spaces-");
});

test("repeated headings get numeric suffixes", () => {
  const slug = createSlugger();
  expect(slug("安装")).toBe("安装");
  expect(slug("安装")).toBe("安装-1");
  expect(slug("安装")).toBe("安装-2");
  expect(slug("Foo")).toBe("foo");
  // A heading that already looks like a suffixed one is skipped over.
  expect(slug("foo-1")).toBe("foo-1");
  expect(slug("Foo")).toBe("foo-2");
  expect(slug("🎉")).toBe("");
  expect(slug("✨")).toBe("-1");
});
