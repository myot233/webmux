// Suggested names are a form convenience. Creation still sends an explicit branch;
// the server keeps check-ref-format and the 409 conflict as the authority.
const ADJECTIVES = [
  "agile",
  "amber",
  "ample",
  "astral",
  "azure",
  "blithe",
  "bold",
  "breezy",
  "bright",
  "brisk",
  "calm",
  "candid",
  "clear",
  "clever",
  "cobalt",
  "coral",
  "cosmic",
  "cozy",
  "crisp",
  "dapper",
  "dawn",
  "deft",
  "dewy",
  "dusk",
  "eager",
  "earthy",
  "emerald",
  "even",
  "fair",
  "fleet",
  "floral",
  "fond",
  "fresh",
  "frosty",
  "gentle",
  "gilded",
  "glad",
  "glassy",
  "golden",
  "grand",
  "green",
  "halcyon",
  "hardy",
  "hazel",
  "humble",
  "ivory",
  "jade",
  "jolly",
  "keen",
  "kind",
  "lively",
  "lofty",
  "lucid",
  "lunar",
  "lush",
  "mellow",
  "merry",
  "mild",
  "misty",
  "modern",
  "mossy",
  "neat",
  "nimble",
  "noble",
  "ochre",
  "olive",
  "opal",
  "pastel",
  "patient",
  "pearly",
  "placid",
  "plucky",
  "poised",
  "polite",
  "prime",
  "proud",
  "pure",
  "quiet",
  "radiant",
  "rapid",
  "ready",
  "regal",
  "rosy",
  "rustic",
  "sage",
  "sandy",
  "serene",
  "silky",
  "silver",
  "sleek",
  "smooth",
  "snowy",
  "solar",
  "solid",
  "sound",
  "spry",
  "starry",
  "steady",
  "still",
  "stout",
  "subtle",
  "sunny",
  "supple",
  "sure",
  "sweet",
  "swift",
  "tender",
  "tidy",
  "timely",
  "topaz",
  "tranquil",
  "true",
  "trusty",
  "velvet",
  "vernal",
  "vivid",
  "warm",
  "wild",
  "windy",
  "wise",
  "witty",
] as const;

const NOUNS = [
  "acorn",
  "alder",
  "alpine",
  "anchor",
  "anvil",
  "arbor",
  "aspen",
  "aster",
  "atoll",
  "aurora",
  "badger",
  "bamboo",
  "basin",
  "bayou",
  "beacon",
  "beaver",
  "belfry",
  "birch",
  "bison",
  "blossom",
  "bluff",
  "boulder",
  "breeze",
  "bridge",
  "brook",
  "cairn",
  "canyon",
  "canopy",
  "cavern",
  "cedar",
  "chime",
  "citrus",
  "cliff",
  "clover",
  "cloud",
  "coaster",
  "comet",
  "compass",
  "condor",
  "copper",
  "cosmos",
  "cove",
  "crane",
  "crater",
  "creek",
  "crest",
  "cricket",
  "crystal",
  "cypress",
  "dahlia",
  "delta",
  "dolphin",
  "dove",
  "dune",
  "eagle",
  "echo",
  "egret",
  "elm",
  "ember",
  "estuary",
  "falcon",
  "fern",
  "field",
  "finch",
  "firth",
  "fjord",
  "flint",
  "forest",
  "forge",
  "fountain",
  "fox",
  "frost",
  "galaxy",
  "garden",
  "garnet",
  "gazelle",
  "geyser",
  "glacier",
  "glade",
  "glen",
  "granite",
  "grotto",
  "grove",
  "gull",
  "harbor",
  "harvest",
  "haven",
  "hawk",
  "heath",
  "hedge",
  "heron",
  "hill",
  "hollow",
  "horizon",
  "ibis",
  "inlet",
  "island",
  "jasper",
  "juniper",
  "kelp",
  "kestrel",
  "lagoon",
  "lake",
  "lantern",
  "larch",
  "lark",
  "laurel",
  "ledge",
  "lichen",
  "lilac",
  "linden",
  "linen",
  "lotus",
  "lynx",
  "magnet",
  "maple",
  "marble",
  "marina",
  "marsh",
  "marten",
  "meadow",
  "mesa",
  "meteor",
  "mirror",
  "monolith",
  "moon",
  "moor",
  "moss",
  "nebula",
  "nectar",
  "needle",
  "nest",
  "oasis",
  "ocean",
  "onyx",
  "orchard",
  "orchid",
  "oriole",
  "osprey",
  "otter",
  "owl",
  "palisade",
  "panda",
  "panther",
  "pasture",
  "pebble",
  "pelican",
  "petal",
  "phoenix",
  "pillar",
  "pine",
  "planet",
  "plateau",
  "plover",
  "plume",
  "pond",
  "poplar",
  "prairie",
  "prism",
  "puffin",
  "pulsar",
  "quarry",
  "quartz",
  "quasar",
  "quince",
  "quiver",
  "rabbit",
  "rapids",
  "raven",
  "ravine",
  "reef",
  "ribbon",
  "ridge",
  "ripple",
  "river",
  "robin",
  "rowan",
  "ruby",
  "saddle",
  "salmon",
  "sandal",
  "savanna",
  "scepter",
  "sequoia",
  "shadow",
  "shingle",
  "shoal",
  "shore",
  "sierra",
  "signal",
  "skyline",
  "slate",
  "solstice",
  "sparrow",
  "spire",
  "spring",
  "spruce",
  "squirrel",
  "starlight",
  "steppe",
  "stone",
  "stork",
  "stream",
  "summit",
  "sundial",
  "sunset",
  "swallow",
  "swan",
  "sycamore",
  "talon",
  "tamarack",
  "terrace",
  "thicket",
  "thistle",
  "thrush",
  "thunder",
  "timber",
  "torrent",
  "tower",
  "trail",
  "trellis",
  "tundra",
  "turtle",
  "twilight",
  "valley",
  "vapor",
  "verge",
  "vessel",
  "viaduct",
  "violet",
  "vista",
  "vortex",
  "voyage",
  "walnut",
  "warbler",
  "water",
  "whisper",
  "willow",
  "wind",
  "winter",
  "woodland",
  "wren",
  "yarrow",
  "zenith",
  "zephyr",
] as const;

export const worktreeBranchAdjectives = ADJECTIVES;
export const worktreeBranchNouns = NOUNS;

const PAIR_COUNT = ADJECTIVES.length * NOUNS.length;
const RANDOM_ATTEMPTS = 64;
const MAX_PAIR_SUFFIX = 20;

export interface BranchNameInput {
  branchNames: readonly string[];
  worktreePaths: readonly string[];
  avoid?: string;
}

export interface BranchNameOptions {
  random?: () => number;
  now?: Date;
}

function randomIndex(limit: number, random: () => number): number {
  const raw = random();
  const scaled = Math.floor((Number.isFinite(raw) ? raw : 0) * limit);
  return scaled <= 0 ? 0 : scaled >= limit ? limit - 1 : scaled;
}

function pairSegment(index: number): string {
  const adjective = ADJECTIVES[Math.floor(index / NOUNS.length)]!;
  const noun = NOUNS[index % NOUNS.length]!;
  return `${adjective}-${noun}`;
}

function free(
  name: string,
  segment: string,
  refs: ReadonlySet<string>,
  segments: ReadonlySet<string>,
  avoid?: string,
): boolean {
  return name !== avoid && !refs.has(name) && !segments.has(segment);
}

function timestampBranch(
  now: Date,
  refs: ReadonlySet<string>,
  segments: ReadonlySet<string>,
  avoid?: string,
): string {
  const stamp = `${now.getFullYear()}${String(now.getMonth() + 1).padStart(2, "0")}${String(now.getDate()).padStart(2, "0")}-${String(now.getHours()).padStart(2, "0")}${String(now.getMinutes()).padStart(2, "0")}${String(now.getSeconds()).padStart(2, "0")}`;
  for (let suffix = 1; suffix <= 1000; suffix++) {
    const segment = suffix === 1 ? stamp : `${stamp}-${suffix}`;
    const name = `wt/${segment}`;
    if (free(name, segment, refs, segments, avoid)) return name;
  }
  return `wt/${stamp}-${now.getMilliseconds()}`;
}

export function suggestWorktreeBranch(
  input: BranchNameInput,
  options: BranchNameOptions = {},
): string {
  const random = options.random ?? Math.random;
  const refs = new Set(input.branchNames);
  const segments = new Set(
    input.worktreePaths
      .map(
        (path) =>
          path
            .split("/")
            .filter((part) => part !== "")
            .at(-1) ?? "",
      )
      .filter((segment) => segment !== ""),
  );
  for (let attempt = 0; attempt < RANDOM_ATTEMPTS; attempt++) {
    const segment = pairSegment(randomIndex(PAIR_COUNT, random));
    const name = `wt/${segment}`;
    if (free(name, segment, refs, segments, input.avoid)) return name;
  }
  const start = randomIndex(PAIR_COUNT, random);
  for (let offset = 0; offset < PAIR_COUNT; offset++) {
    const segment = pairSegment((start + offset) % PAIR_COUNT);
    const name = `wt/${segment}`;
    if (free(name, segment, refs, segments, input.avoid)) return name;
  }
  for (let offset = 0; offset < PAIR_COUNT; offset++) {
    const base = pairSegment((start + offset) % PAIR_COUNT);
    for (let suffix = 2; suffix <= MAX_PAIR_SUFFIX; suffix++) {
      const segment = `${base}-${suffix}`;
      const name = `wt/${segment}`;
      if (free(name, segment, refs, segments, input.avoid)) return name;
    }
  }
  return timestampBranch(
    options.now ?? new Date(),
    refs,
    segments,
    input.avoid,
  );
}

const REMOTE_REF_PREFIX = "refs/remotes/";

export interface RemoteBranchBase {
  name: string;
  head: string;
  subject: string;
}

/**
 * Extracts remote-tracking branches (`origin/main`, `origin/feature/x`, etc.)
 * from `/api/repositories/{id}/branches` bases, skipping symbolic `/HEAD` refs
 * and any short name that collides with an existing local branch name.
 */
export function remoteBranchBases(
  bases: readonly RemoteBranchBase[],
  localNames: Iterable<string> = [],
): RemoteBranchBase[] {
  const locals = new Set(localNames);
  const result: RemoteBranchBase[] = [];
  for (const base of bases) {
    if (
      !base.name.startsWith(REMOTE_REF_PREFIX) ||
      base.name.endsWith("/HEAD")
    ) {
      continue;
    }
    const short = base.name.slice(REMOTE_REF_PREFIX.length);
    if (!short || !short.includes("/") || locals.has(short)) continue;
    result.push({ name: short, head: base.head, subject: base.subject });
  }
  return result;
}
