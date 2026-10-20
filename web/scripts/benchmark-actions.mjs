import { execFile } from "node:child_process";
import { mkdir, writeFile } from "node:fs/promises";
import { dirname, resolve } from "node:path";
import { fileURLToPath } from "node:url";
import { parseArgs, promisify } from "node:util";

const { values } = parseArgs({
  options: {
    baseline: { type: "string" },
    candidate: { type: "string" },
    repo: { type: "string", default: "mzdluo123/webmux" },
    output: {
      type: "string",
      default: "test-results/ci-benchmark/actions.json",
    },
  },
});
for (const field of ["baseline", "candidate"])
  if (!/^\d+$/.test(values[field] ?? ""))
    throw new Error(`Pass --${field} RUN_ID`);
const exec = promisify(execFile);
async function readRun(id) {
  const { stdout } = await exec(
    "gh",
    [
      "run",
      "view",
      id,
      "--repo",
      values.repo,
      "--json",
      "jobs,status,conclusion,headSha,url,createdAt",
    ],
    { maxBuffer: 10 * 1024 * 1024 },
  );
  const run = JSON.parse(stdout);
  if (run.status !== "completed" || run.conclusion !== "success")
    throw new Error(`${run.url}: require a completed successful workflow`);
  if (!run.jobs.length || run.jobs.some((job) => job.conclusion !== "success"))
    throw new Error(`${run.url}: every job must succeed`);
  const started = Math.min(...run.jobs.map((job) => Date.parse(job.startedAt)));
  const completed = Math.max(
    ...run.jobs.map((job) => Date.parse(job.completedAt)),
  );
  if (!Number.isFinite(started + completed) || completed < started)
    throw new Error(`${run.url}: missing job timestamps`);
  return {
    id,
    url: run.url,
    commit: run.headSha,
    seconds: (completed - started) / 1000,
    queueSeconds: (started - Date.parse(run.createdAt)) / 1000,
    jobs: run.jobs.map((job) => job.name).sort(),
  };
}
const [baseline, candidate] = await Promise.all([
  readRun(values.baseline),
  readRun(values.candidate),
]);
if (JSON.stringify(baseline.jobs) !== JSON.stringify(candidate.jobs))
  throw new Error(
    "Required job names changed; compare equivalent workflow coverage",
  );
const improvementPercent = (1 - candidate.seconds / baseline.seconds) * 100;
const result = {
  baseline,
  candidate,
  improvementPercent,
  targetMet: improvementPercent >= 50,
  timing:
    "first job started to last job completed; queue time reported separately",
};
const webRoot = resolve(dirname(fileURLToPath(import.meta.url)), "..");
const output = resolve(webRoot, values.output);
await mkdir(dirname(output), { recursive: true });
await writeFile(output, JSON.stringify(result, null, 2) + "\n");
console.log(
  `${baseline.seconds}s -> ${candidate.seconds}s; ${improvementPercent.toFixed(2)}% faster; 50% target ${result.targetMet ? "MET" : "NOT MET"}`,
);
console.log(output);
if (!result.targetMet) process.exitCode = 1;
