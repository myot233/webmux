import {
  LanguageDescription,
  type LanguageSupport,
} from "@codemirror/language";
import { languages } from "@codemirror/language-data";

/**
 * Picks and loads the grammar for a Markdown fence's info string, matching
 * language names and aliases such as `ts`, `golang` or `sh`. Unknown or empty
 * info strings stay plain text.
 */
export async function loadFenceLanguage(
  info: string,
): Promise<LanguageSupport | null> {
  const name = info.trim().split(/\s/)[0] ?? "";
  if (!name) return null;
  const description = LanguageDescription.matchLanguageName(
    languages,
    name,
    true,
  );
  if (description) return description.load();
  if (name.toLowerCase() === "svelte")
    return (await import("@codemirror/lang-javascript")).javascript();
  return null;
}
