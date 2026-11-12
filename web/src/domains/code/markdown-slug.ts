// GitHub-style heading slugs, as github-slugger computes them: lowercase,
// drop everything but letters, marks, numbers, connector punctuation, spaces
// and hyphens, then turn each space into a hyphen.
const removed = /[^\p{L}\p{M}\p{N}\p{Pc} -]/gu;

export const headingSlug = (text: string) =>
  text.toLowerCase().replace(removed, "").replaceAll(" ", "-");

/**
 * Returns a slugger for one document: a repeated slug gets `-1`, `-2`, …
 * appended, skipping suffixes another heading already produced.
 */
export function createSlugger() {
  const occurrences = new Map<string, number>();
  return (text: string) => {
    const original = headingSlug(text);
    let slug = original;
    while (occurrences.has(slug)) {
      const count = occurrences.get(original)! + 1;
      occurrences.set(original, count);
      slug = `${original}-${count}`;
    }
    occurrences.set(slug, 0);
    return slug;
  };
}

/** Element id prefix that keeps heading anchors clear of application ids. */
export const headingIdPrefix = "md-h-";
