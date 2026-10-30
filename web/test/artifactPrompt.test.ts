import { describe, expect, test } from "vitest";
import {
  BUILTIN_ARTIFACT_PROMPT,
  artifactPromptError,
  normalizeArtifactPrompt,
  renderArtifactPrompt,
} from "../src/artifactPrompt";

describe("renderArtifactPrompt", () => {
  test("replaces every {dir} with the path verbatim", () => {
    expect(
      renderArtifactPrompt("看 {dir}，上传在 {dir}/uploads/", "/tmp/a b/'s1'"),
    ).toBe("看 /tmp/a b/'s1'，上传在 /tmp/a b/'s1'/uploads/");
  });

  test("built-in template names the folder, uploads and cleanup", () => {
    const text = renderArtifactPrompt(BUILTIN_ARTIFACT_PROMPT, "/tmp/x/s1");
    expect(text).not.toContain("{dir}");
    expect(text).toContain("/tmp/x/s1/uploads/");
    expect(text).toContain("临时文件夹");
    expect(text).toContain("相对");
    expect(text).toContain("删除");
  });
});

describe("artifact prompt validation", () => {
  test("normalizes line endings like quick commands", () => {
    expect(normalizeArtifactPrompt("a {dir}\r\nb\rc\n\n")).toBe(
      "a {dir}\nb\nc",
    );
  });

  test("accepts the built-in template", () => {
    expect(artifactPromptError(BUILTIN_ARTIFACT_PROMPT)).toBe("");
  });

  test("requires a non-blank body containing {dir} within 4096 units", () => {
    expect(artifactPromptError("  \n ")).not.toBe("");
    expect(artifactPromptError("no placeholder")).toContain("{dir}");
    expect(artifactPromptError("{dir}" + "x".repeat(4091))).toBe("");
    expect(artifactPromptError("{dir}" + "x".repeat(4092))).not.toBe("");
  });
});
