// Frontend architecture guard: import direction between layers.
// Run with `pnpm arch:deps`. See docs/frontend-architecture.md.
//
// Layers: entry/App -> pages/components -> hooks -> domains -> platform/contracts.
// Domain topics follow business ownership; lib is reusable design-system code.
// Type-only dependencies obey the same boundaries as runtime imports.

const PRIMITIVES = "node_modules/(bits-ui|vaul-svelte|paneforge)/";
const VARIANTS = "node_modules/(tailwind-variants|class-variance-authority)/";

// Explicit topic edges prevent an acyclic but ever-more-coupled domain graph.
const DOMAIN_DEPENDENCIES = {
  settings: [],
  sessions: ["settings"],
  workspace: ["settings", "sessions"],
  terminal: ["settings", "sessions"],
  notifications: ["sessions"],
  code: ["terminal"],
  chat: [],
};

/** @type {import('dependency-cruiser').IConfiguration} */
module.exports = {
  forbidden: [
    ...Object.entries(DOMAIN_DEPENDENCIES).map(([topic, dependencies]) => ({
      name: `${topic}-dependency-boundary`,
      severity: /** @type {const} */ ("error"),
      comment:
        "Keep domain dependencies explicit; coordinate unrelated topics in hooks.",
      from: { path: `^src/domains/${topic}/` },
      to: {
        path: "^src/domains/",
        pathNot: `^src/domains/(${[topic, ...dependencies].join("|")})/`,
      },
    })),
    {
      name: "no-circular",
      severity: "error",
      comment:
        "Import cycles blur ownership; move the shared part down a layer.",
      // shadcn-svelte barrels re-export their own parts; that cycle is generated.
      from: { pathNot: "^src/lib/components/ui/" },
      to: { circular: true },
    },
    {
      name: "no-unresolved",
      severity: "error",
      comment: "Moved modules must resolve; update every consumer, not a shim.",
      from: {},
      // Vite resolves virtual icons and this package worker resource query.
      to: {
        couldNotResolve: true,
        pathNot: ["^~icons/", "^@pierre/diffs/worker/worker\\.js\\?worker$"],
      },
    },
    {
      name: "contracts-are-independent",
      severity: "error",
      comment: "Wire schemas cannot depend on runtime or business state.",
      from: { path: "^src/contracts/" },
      to: { path: "^src/", pathNot: "^src/contracts/" },
    },
    {
      name: "platform-does-not-know-domains",
      severity: "error",
      comment:
        "Browser/network infrastructure must remain reusable across domains.",
      from: { path: "^src/platform/" },
      to: { path: "^src/", pathNot: "^src/(platform|contracts|lib)/" },
    },
    {
      name: "platform-does-not-render",
      severity: "error",
      comment: "Infrastructure owns browser I/O, not UI rendering.",
      from: { path: "^src/platform/" },
      to: { path: "\\.svelte$" },
    },
    {
      name: "ui-is-a-leaf",
      severity: "error",
      comment:
        "Design-system components must not depend on business code; " +
        "otherwise they cannot be reused or regenerated safely.",
      from: { path: "^src/lib/components/ui/" },
      to: { path: "^src/", pathNot: "^src/lib/" },
    },
    {
      name: "lib-is-framework-free",
      severity: "error",
      comment:
        "src/lib helpers must remain independent of application and domain code.",
      from: { path: "^src/lib/", pathNot: "^src/lib/components/ui/" },
      to: { path: "^src/", pathNot: "^src/lib/" },
    },
    {
      name: "only-app-composes-pages",
      severity: "error",
      comment:
        "Pages are composed by App.svelte; share code through components or hooks instead.",
      from: { pathNot: "^src/(App\\.svelte|pages/)" },
      to: { path: "^src/pages/" },
    },
    {
      name: "components-do-not-know-pages",
      severity: "error",
      comment:
        "Shared components receive page state through props, not page imports.",
      from: { path: "^src/components/" },
      to: { path: "^src/pages/" },
    },
    {
      name: "components-do-not-know-app",
      severity: "error",
      comment: "Application orchestration belongs above reusable components.",
      from: { path: "^src/components/" },
      to: { path: "^src/app/" },
    },
    {
      name: "hooks-do-not-render",
      severity: "error",
      comment:
        "Hooks own resources and state; rendering belongs to components and pages.",
      from: { path: "^src/hooks/" },
      to: { path: "\\.svelte$" },
    },
    {
      name: "hooks-do-not-know-composition",
      severity: "error",
      comment:
        "Resource controllers cannot depend on app or visual composition.",
      from: { path: "^src/hooks/" },
      to: { path: "^src/(app|components|pages)/" },
    },
    {
      name: "domain-does-not-render",
      severity: "error",
      comment:
        "Domain algorithms and state must not depend on application composition.",
      from: { path: "^src/domains/" },
      to: { path: "(^src/(app|hooks|components|pages)/|\\.svelte$)" },
    },
    {
      name: "primitives-only-in-ui",
      severity: "error",
      comment:
        "Headless primitives are wrapped only in src/lib/components/ui " +
        "(backs up the ESLint no-restricted-imports rule).",
      from: { pathNot: "^src/lib/components/ui/" },
      to: { path: PRIMITIVES },
    },
    {
      name: "variants-only-in-ui",
      severity: "error",
      comment:
        "Variant helpers define looks only in src/lib/components/ui; add a variant there.",
      from: { pathNot: "^src/lib/components/ui/" },
      to: { path: VARIANTS },
    },
    {
      name: "no-orphans",
      severity: "error",
      comment:
        "An unused module is usually a leftover after a rewrite; delete it.",
      from: {
        orphan: true,
        pathNot: [
          "\\.d\\.ts$",
          "^src/main\\.ts$",
          "^src/lib/components/ui/", // generated on demand by shadcn-svelte
          "\\.css$",
        ],
      },
      to: {},
    },
  ],
  options: {
    doNotFollow: { path: "node_modules" },
    exclude: { path: "^(dist|storybook-static)/" },
    tsConfig: { fileName: "tsconfig.json" },
    tsPreCompilationDeps: true,
    enhancedResolveOptions: {
      exportsFields: ["exports"],
      conditionNames: [
        "import",
        "require",
        "node",
        "default",
        "types",
        "svelte",
      ],
      extensions: [".ts", ".js", ".svelte"],
    },
    reporterOptions: { text: { highlightFocused: true } },
  },
};
