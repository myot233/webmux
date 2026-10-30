import {
  LanguageDescription,
  type LanguageSupport,
} from "@codemirror/language";
import { languages } from "@codemirror/language-data";

/**
 * Picks and loads the grammar for a file path. Filename patterns such as
 * `Dockerfile` are tried before extensions, and a lowercase retry covers
 * uppercase extensions such as `README.MD`. language-data loads each grammar
 * on demand, so the reader only pays for the file it shows.
 *
 * Svelte is not part of language-data; highlight its script blocks with the
 * JavaScript grammar the reader used before.
 */
export async function loadCodeLanguage(
  path: string,
): Promise<LanguageSupport | null> {
  const name = path.split(/[\\/]/).pop() || path;
  const description =
    LanguageDescription.matchFilename(languages, name) ??
    LanguageDescription.matchFilename(languages, name.toLowerCase());
  if (description) return description.load();
  if (/\.svelte$/i.test(name))
    return (await import("@codemirror/lang-javascript")).javascript();
  return null;
}

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
