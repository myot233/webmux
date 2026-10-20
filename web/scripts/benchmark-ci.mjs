import { spawn } from "node:child_process";
import { mkdir, readFile, writeFile } from "node:fs/promises";
import { dirname, resolve } from "node:path";
import { availableParallelism } from "node:os";
import { fileURLToPath } from "node:url";
import { parseArgs } from "node:util";

const { values } = parseArgs({
  options: {
    suite: { type: "string", default: "e2e" },
    runs: { type: "string", default: "3" },
    output: {
      type: "string",
      default: "test-results/ci-benchmark/results.json",
    },
    label: { type: "string", default: "candidate" },
    compare: { type: "string" },
    "allow-worker-change": { type: "boolean", default: false },
    workers: { type: "string", default: "2" },
  },
});
const runs = Number(values.runs);
const workers = Number(values.workers);
if (!Number.isInteger(workers) || workers < 1)
  throw new Error("workers must be positive");
if (!Number.isInteger(runs) || runs < 1)
  throw new Error("runs must be positive");
if (!["e2e", "storybook"].includes(values.suite))
  throw new Error("Unknown suite");
const webRoot = resolve(dirname(fileURLToPath(import.meta.url)), "..");
const output = resolve(webRoot, values.output);
await mkdir(dirname(output), { recursive: true });

async function run(args, log, extraEnv = {}) {
  const chunks = [];
  const start = performance.now();
  const child = spawn("pnpm", args, {
    cwd: webRoot,
    env: { ...process.env, CI: "1", ...extraEnv },
    stdio: ["ignore", "pipe", "pipe"],
  });
  child.stdout.on("data", (chunk) => chunks.push(chunk));
  child.stderr.on("data", (chunk) => chunks.push(chunk));
  const code = await new Promise((resolve, reject) => {
    child.once("error", reject);
    child.once("exit", resolve);
  });
  await writeFile(log, Buffer.concat(chunks));
  if (code !== 0)
    throw new Error(`pnpm ${args.join(" ")} failed (${code}); see ${log}`);
  return (performance.now() - start) / 1000;
}

function cases(suites, parents = []) {
  return suites.flatMap((suite) => [
    ...(suite.specs ?? []).flatMap((spec) =>
      spec.tests.map((test) => ({
        id: (() => {
          const path = [...parents, suite.title];
          const layout =
            test.projectName === "layouts"
              ? path.find((title) =>
                  /^(narrow|mobile|tablet|desktop|short|large-text)-(light|dark)$/.test(
                    title,
                  ),
                )
              : undefined;
          return [
            layout ?? test.projectName,
            spec.file,
            ...path.filter((title) => title !== layout),
            spec.title,
          ].join(" / ");
        })(),
        status: test.status,
        expectedStatus: test.expectedStatus,
        attempts: test.results.length,
      })),
    ),
    ...cases(suite.suites ?? [], [...parents, suite.title]),
  ]);
}

const result = {
  label: values.label,
  suite: values.suite,
  workers: values.workers,
  node: process.version,
  platform: `${process.platform}-${process.arch}`,
  availableCPUs: availableParallelism(),
  samples: [],
};
for (let i = 0; i < runs; i++) {
  const prefix = `${output}.${i + 1}`;
  let seconds = 0;
  if (values.suite === "storybook")
    seconds += await run(["run", "build-storybook"], `${prefix}.build.log`);
  const config =
    values.suite === "e2e"
      ? "playwright.config.ts"
      : "playwright.storybook.config.ts";
  seconds += await run(
    [
      "exec",
      "playwright",
      "test",
      "--config",
      config,
      `--workers=${values.workers}`,
      "--reporter=json",
    ],
    `${prefix}.log`,
    { PLAYWRIGHT_JSON_OUTPUT_NAME: `${prefix}.report.json` },
  );
  const report = JSON.parse(await readFile(`${prefix}.report.json`, "utf8"));
  const tests = cases(report.suites).sort((a, b) => a.id.localeCompare(b.id));
  if (
    tests.some(
      (test) => test.status === "flaky" || test.status === "unexpected",
    )
  )
    throw new Error(`Failed or flaky tests in ${prefix}.report.json`);
  result.samples.push({ seconds, stats: report.stats, tests });
  await writeFile(output, JSON.stringify(result, null, 2) + "\n");
  console.log(
    `${values.label} ${values.suite} ${i + 1}/${runs}: ${seconds.toFixed(3)}s (${tests.length} tests)`,
  );
}
const median = (samples) => {
  const sorted = samples.map((sample) => sample.seconds).sort((a, b) => a - b);
  const middle = Math.floor(sorted.length / 2);
  return sorted.length % 2
    ? sorted[middle]
    : (sorted[middle - 1] + sorted[middle]) / 2;
};
result.medianSeconds = median(result.samples);
if (values.compare) {
  const baseline = JSON.parse(
    await readFile(resolve(webRoot, values.compare), "utf8"),
  );
  if (baseline.availableCPUs && baseline.availableCPUs !== result.availableCPUs)
    throw new Error("Comparison changed available CPU count");
  if (baseline.workers !== result.workers) {
    if (!values["allow-worker-change"])
      throw new Error(
        "Comparison changed workers; pass --allow-worker-change to measure a configuration change on the same machine",
      );
    result.workerChange = `${baseline.workers} -> ${result.workers}`;
    console.log(
      `Worker configuration changed: ${result.workerChange}; same machine, unequal worker counts`,
    );
  }
  for (const field of ["suite", "node", "platform"])
    if (baseline[field] !== result[field])
      throw new Error(`Comparison changed ${field}`);
  const identities = (tests) =>
    JSON.stringify(
      tests.map(({ id, expectedStatus }) => ({ id, expectedStatus })),
    );
  const baselineTests = identities(baseline.samples[0].tests);
  for (const sample of [...baseline.samples, ...result.samples])
    if (identities(sample.tests) !== baselineTests)
      throw new Error("Test identities or skip expectations changed");
  result.improvementPercent =
    (1 - result.medianSeconds / median(baseline.samples)) * 100;
  result.targetMet = result.improvementPercent >= 50;
  console.log(
    `Improvement: ${result.improvementPercent.toFixed(2)}%; 50% target ${result.targetMet ? "MET" : "NOT MET"}`,
  );
}
await writeFile(output, JSON.stringify(result, null, 2) + "\n");
console.log(`Median: ${result.medianSeconds.toFixed(3)}s; ${output}`);
