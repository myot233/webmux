import { ArtifactPromptSchema } from "./schemas";
import { normalizeCommand } from "./quickCommands";

/** Placeholder replaced by the session's artifacts folder path. */
const ARTIFACT_DIR_PLACEHOLDER = "{dir}";

export const BUILTIN_ARTIFACT_PROMPT = [
  "本会话的临时文件夹：{dir}",
  "- 我上传给你的文件在 {dir}/uploads/ 中。",
  "- 你生成给我的截图、报告、图表、导出数据等交付物，请保存到 {dir}（可以建子目录），不要放进项目仓库。",
  "- 我会在 WebMux 代码栏的“临时文件夹”中查看这些文件。",
  "- 完成后告诉我文件相对于该文件夹的路径。",
  "- 该文件夹会在会话删除后清理，不要存放需要长期保留的内容。",
].join("\n");

/** Replaces every `{dir}` with the folder path verbatim; prompts are prose, not shell. */
export function renderArtifactPrompt(template: string, dir: string) {
  return template.replaceAll(ARTIFACT_DIR_PLACEHOLDER, dir);
}

/** Line-ending normalization shared with quick commands and the server. */
export function normalizeArtifactPrompt(text: string) {
  return normalizeCommand(text);
}

/** The first problem with a normalized template, or "" when it can be saved. */
export function artifactPromptError(text: string) {
  const result = ArtifactPromptSchema.safeParse(text);
  return result.success
    ? ""
    : (result.error.issues[0]?.message ?? "提示词无效");
}
