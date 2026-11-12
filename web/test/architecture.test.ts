// Frontend architecture guard: the gaps that ESLint, dependency-cruiser,
// jscpd and knip do not cover. See docs/frontend-architecture.md.
import { globSync, readFileSync } from "node:fs";
import { basename, dirname, extname } from "node:path";
import { fileURLToPath } from "node:url";
import { describe, expect, test } from "vitest";

const root = fileURLToPath(new URL("..", import.meta.url));
const UI_DIR = "src/lib/components/ui/";

const files = globSync("src/**/*.svelte", { cwd: root })
  .map((file) => file.replaceAll("\\", "/"))
  .sort();

/** SessionRow / session-row / session_row / session-row/index → "sessionrow" */
function componentKey(file: string): string {
  let name = basename(file, extname(file));
  if (name === "index") name = basename(dirname(file));
  return name.toLowerCase().replace(/[-_.]/g, "");
}

function groupBy<T>(items: T[], key: (item: T) => string): Map<string, T[]> {
  const groups = new Map<string, T[]>();
  for (const item of items) {
    const k = key(item);
    groups.set(k, [...(groups.get(k) ?? []), item]);
  }
  return groups;
}

// Static class strings: class="…", class='…' and cn("…").
const CLASS_PATTERNS = [
  /\bclass\s*=\s*"([^"{}$]+)"/g,
  /\bclass\s*=\s*'([^'{}$]+)'/g,
  /\bcn\(\s*["'`]([^"'`$]+)["'`]/g,
];

function classStrings(source: string): string[] {
  return CLASS_PATTERNS.flatMap((pattern) =>
    [...source.matchAll(pattern)]
      .map((match) => match[1])
      .filter((value): value is string => value !== undefined),
  );
}

// Placement classes are a call site's business; only appearance is compared.
const LAYOUT_CLASS =
  /^(?:[a-z0-9-]+:)*(?:m[trblxyse]?-|w-|h-|min-|max-|col-|row-|order-|self-|justify-self-|place-self-|grow|shrink|basis-|flex-1|hidden|block)/;

function appearanceKey(classString: string): string {
  return classString
    .split(/\s+/)
    .filter((name) => name && !LAYOUT_CLASS.test(name))
    .sort()
    .join(" ");
}

const MIN_CLASSES = 6;
const MAX_FILES = 2;

/** Accepted repeats; each needs a reason and review. */
const CLASS_STRING_ALLOWLIST = new Set<string>([]);

// Radius roles (docs/design-system.md#tokens): tags sm, controls md, content
// containers lg, overlays xl; none and full for flush or round shapes, and
// [inherit] for a layer that follows its parent's corner.
const RADIUS_CLASS =
  /(?<![\w-])rounded(?:-(?:[trblse]|tl|tr|br|bl|ss|se|es|ee))?(?:-([\w.]+|\[[^\]\s]*\]))?(?![\w-])/g;
const RADIUS_SCALE = new Set([
  "none",
  "sm",
  "md",
  "lg",
  "xl",
  "full",
  "[inherit]",
]);
// The 12px overlay radius belongs to the modal surfaces alone.
const OVERLAY_FILES = new Set([
  `${UI_DIR}alert-dialog/alert-dialog-content.svelte`,
  `${UI_DIR}dialog/dialog-content.svelte`,
  `${UI_DIR}drawer/drawer-content.svelte`,
]);
const CSS_RADIUS = /border(?:-[a-z]+)*-radius\s*:\s*([^;}]+)/g;
const CSS_RADIUS_VALUE =
  /^(?:0|50%|999px|inherit|var\(--radius(?:-(?:sm|md|lg|xl))?\))$/;

const styleFiles = globSync("src/**/*.{svelte,ts,css}", { cwd: root })
  .map((file) => file.replaceAll("\\", "/"))
  .sort();

/** Source without comments, so prose such as "rounded corner" is ignored. */
function withoutComments(source: string): string {
  return source
    .replace(/<!--[\s\S]*?-->/g, "")
    .replace(/\/\*[\s\S]*?\*\//g, "")
    .replace(/(^|\s)\/\/.*$/gm, "$1");
}

function radiusViolations(file: string): string[] {
  const source = withoutComments(readFileSync(`${root}/${file}`, "utf8"));
  const found: string[] = [];
  for (const match of source.matchAll(RADIUS_CLASS)) {
    const size = match[1] ?? "";
    if (!RADIUS_SCALE.has(size))
      found.push(`${match[0]}: use rounded-sm/md/lg/xl/full`);
    else if (size === "xl" && !OVERLAY_FILES.has(file))
      found.push(`${match[0]}: xl is reserved for dialogs and drawers`);
  }
  for (const match of source.matchAll(CSS_RADIUS)) {
    const value = (match[1] ?? "").trim();
    if (!CSS_RADIUS_VALUE.test(value))
      found.push(`border-radius: ${value}: use var(--radius-sm|md|lg|xl)`);
  }
  return found.map((message) => `${file}: ${message}`);
}

describe("frontend architecture", () => {
  test("source root contains only bootstrapping, declarations and global styles", () => {
    const entryFiles: Record<string, true> = {
      "src/App.svelte": true,
      "src/main.ts": true,
      "src/app.d.ts": true,
      "src/styles.css": true,
      "src/design-tokens.css": true,
      "src/fonts.css": true,
      "src/fonts-terminal.css": true,
    };
    const layers: Record<string, true> = {
      app: true,
      contracts: true,
      platform: true,
      domains: true,
      hooks: true,
      components: true,
      pages: true,
      lib: true,
      icons: true,
    };
    const misplaced = globSync("src/*", { cwd: root, withFileTypes: true })
      .filter((entry) =>
        entry.isDirectory()
          ? !Object.hasOwn(layers, entry.name)
          : !Object.hasOwn(entryFiles, `src/${entry.name}`),
      )
      .map((entry) => `src/${entry.name}`);
    expect(
      misplaced,
      "Place schemas in contracts/, browser I/O in platform/, business logic " +
        "in domains/<topic>/, and application orchestration in app/.",
    ).toEqual([]);
  });

  test("domain logic, controllers and business components have topic ownership", () => {
    const topics =
      "sessions|terminal|chat|code|workspace|settings|notifications";
    const owned = new RegExp(
      `^src/(domains/(?:${topics})|hooks/(?:${topics}|shared)|` +
        `components/(?:${topics}|shared|shell))/[^/]+`,
    );
    const unowned = globSync(
      "src/{domains,hooks,components}/**/*.{ts,svelte}",
      { cwd: root },
    )
      .map((file) => file.replaceAll("\\", "/"))
      .filter((file) => !owned.test(file));
    expect(
      unowned,
      "Group files by business topic; reusable cross-domain UI belongs in " +
        "components/shared and resource controllers in hooks/shared.",
    ).toEqual([]);
  });

  test("each business component name has one implementation", () => {
    const duplicates = [
      ...groupBy(
        files.filter((file) => !file.startsWith(UI_DIR)),
        componentKey,
      ),
    ]
      .filter(([, list]) => list.length > 1)
      .map(([key, list]) => `${key}: ${list.join(", ")}`);

    expect(
      duplicates,
      "Keep one implementation in src/components and import it elsewhere.",
    ).toEqual([]);
  });

  test("business code does not re-create design-system components", () => {
    const uiKeys = new Map(
      files
        .filter((file) => file.startsWith(UI_DIR))
        .map((file) => [componentKey(file), file] as const),
    );
    const shadowing = files
      .filter(
        (file) => !file.startsWith(UI_DIR) && uiKeys.has(componentKey(file)),
      )
      .map((file) => `${file} → ${uiKeys.get(componentKey(file))}`);

    expect(
      shadowing,
      "Use the design-system component, adding a variant if needed.",
    ).toEqual([]);
  });

  test(`appearance of ${MIN_CLASSES}+ classes is not copied into more than ${MAX_FILES} files`, () => {
    const seen = new Map<string, Set<string>>();
    for (const file of files) {
      if (file.startsWith(UI_DIR)) continue;
      const source = readFileSync(`${root}/${file}`, "utf8");
      for (const raw of classStrings(source)) {
        const key = appearanceKey(raw);
        if (key.split(" ").length < MIN_CLASSES) continue;
        if (CLASS_STRING_ALLOWLIST.has(key)) continue;
        seen.set(key, (seen.get(key) ?? new Set()).add(file));
      }
    }
    const copied = [...seen]
      .filter(([, where]) => where.size > MAX_FILES)
      .map(([key, where]) => `"${key}": ${[...where].join(", ")}`);

    expect(
      copied,
      "Extract a component into src/components or add a design-system variant.",
    ).toEqual([]);
  });

  test("corner radii follow the role scale", () => {
    expect(styleFiles).toContain("src/styles.css");
    expect(
      styleFiles.flatMap(radiusViolations),
      "Tags sm, controls md, content containers lg, dialogs and drawers xl " +
        "(docs/design-system.md#tokens).",
    ).toEqual([]);
  });
});

// File naming (docs/agent-rules/naming.md): non-component TypeScript is
// kebab-case. Svelte components are PascalCase; browser tests and stories
// are PascalCase when named after a component, kebab-case for helpers.
const KEBAB_FILE = /^[a-z0-9]+(-[a-z0-9]+)*(\.[a-z0-9]+)*\.ts$/;
const PASCAL_FILE = /^[A-Z][A-Za-z0-9]*(\.[a-z0-9]+)*\.(svelte|ts)$/;

describe("file naming", () => {
  const sources = globSync("src/**/*.ts", { cwd: root })
    .map((file) => file.replaceAll("\\", "/"))
    .filter((file) => !file.startsWith(UI_DIR));
  const unitTests = globSync("test/*.ts", { cwd: root });
  const components = globSync("src/**/*.svelte", { cwd: root }).filter(
    (file) => !file.replaceAll("\\", "/").startsWith(UI_DIR),
  );
  const componentTests = [
    ...globSync("test/browser/*.ts", { cwd: root }),
    ...globSync("stories/*.ts", { cwd: root }),
  ];

  test("non-component TypeScript files are kebab-case", () => {
    expect(
      [...sources, ...unitTests].filter(
        (file) => !KEBAB_FILE.test(basename(file)),
      ),
      "Rename to kebab-case, e.g. quickCommands.ts → quick-commands.ts.",
    ).toEqual([]);
  });

  test("Svelte components are PascalCase", () => {
    expect(
      components.filter((file) => !PASCAL_FILE.test(basename(file))),
      "Name components in PascalCase.",
    ).toEqual([]);
  });

  test("browser tests and stories are PascalCase or kebab-case", () => {
    expect(
      componentTests.filter((file) => {
        const name = basename(file);
        return !PASCAL_FILE.test(name) && !KEBAB_FILE.test(name);
      }),
      "Use <Component>.test.ts for components, kebab-case for helpers.",
    ).toEqual([]);
  });

  test("production source files stay under 500 lines", () => {
    const maxLines = 499;
    const prodFiles = [...sources, ...components].sort();
    const oversized = prodFiles.flatMap((file) => {
      const normalized = file.replaceAll("\\", "/");
      const text = readFileSync(`${root}/${normalized}`, "utf8");
      const rawLines = text.split("\n");
      const lineCount =
        rawLines.at(-1) === "" ? rawLines.length - 1 : rawLines.length;
      return lineCount > maxLines
        ? [`${normalized} (${lineCount} lines, max ${maxLines})`]
        : [];
    });

    expect(
      oversized,
      [
        "Production frontend source files must stay strictly under 500 lines (<= 499 lines).",
        "How to split an oversized file:",
        "1. Move pure domain logic, parsers, formatters, and DOM/API algorithms into src/domains/<topic>/ (never import hooks or rendering components).",
        "2. Move reusable reactive controllers and $state/$effect lifecycle coordination into src/hooks/<topic>/.",
        "3. Split large visual regions, dialogs, toolbars, or headers into focused PascalCase subcomponents in src/components/<topic>/.",
        "4. Name new files after their concrete domain concept — never use catch-all names like helpers.ts, utils.ts, common.ts, misc.ts, or part2.ts.",
        "5. Do NOT strip comments or cram multiple statements onto one line just to pass the line count.",
      ].join("\n"),
    ).toEqual([]);
  });
});
