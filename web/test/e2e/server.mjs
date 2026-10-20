import { spawn } from "node:child_process";
import { once } from "node:events";
import { mkdtemp, mkdir, realpath, rm, writeFile } from "node:fs/promises";
import { tmpdir } from "node:os";
import { createServer as createPortReservation } from "node:net";
import { dirname, join, resolve } from "node:path";
import { fileURLToPath } from "node:url";
import { setTimeout as delay } from "node:timers/promises";
import { createServer } from "vite";

const webRoot = resolve(dirname(fileURLToPath(import.meta.url)), "../..");
const repoRoot = resolve(webRoot, "..");

function signalChild(child, signal, group = false) {
  try {
    if (group) process.kill(-child.pid, signal);
    else child.kill(signal);
  } catch (error) {
    if (error.code !== "ESRCH") throw error;
  }
}

async function stopChild(child, group = false) {
  if (!child?.pid || child.exitCode !== null || child.signalCode !== null)
    return;
  const exited = once(child, "exit");
  signalChild(child, "SIGTERM", group);
  const deadline = setTimeout(
    () => signalChild(child, "SIGKILL", group),
    15_000,
  );
  deadline.unref();
  try {
    await exited;
  } finally {
    clearTimeout(deadline);
  }
}

/**
 * @typedef {object} E2EServer
 * @property {string} baseURL
 * @property {string} apiURL
 * @property {string} workDir
 * @property {() => Promise<void>} restart
 * @property {() => string} logs
 * @property {() => Promise<void>} close
 */

// Every Playwright worker owns real servers and a fresh HOME, config and cwd.
// Port zero asks the OS to allocate ports; no local developer server is reused.
/** @param {{noGit?: boolean}} options @returns {Promise<E2EServer>} */
export async function startE2EServer({ noGit = false } = {}) {
  // macOS tmpdir() sits behind /var -> /private/var; the server reports
  // canonical paths, so expectations must start from the canonical root.
  const root = await realpath(await mkdtemp(join(tmpdir(), "webmux-e2e-")));
  const workDir = join(root, "work");
  const home = join(root, "home");
  const binary = join(root, "webmux");
  const config = join(root, "config.toml");
  let backend;
  let builder;
  let vite;
  let closing;
  let backendOutput = "";
  let buildOutput = "";
  let viteOutput = "";
  const logs = () =>
    `Go build:\n${buildOutput}\nGo backend:\n${backendOutput}\nVite:\n${viteOutput}`;
  const close = () =>
    (closing ??= (async () => {
      try {
        await vite?.close();
      } finally {
        try {
          await stopChild(backend);
          await stopChild(builder, true);
        } finally {
          await rm(root, { recursive: true, force: true });
        }
      }
    })());
  const interrupted = () => {
    void close().finally(() => process.exit(130));
  };
  process.once("SIGINT", interrupted);
  process.once("SIGTERM", interrupted);
  try {
    await Promise.all([
      mkdir(workDir),
      mkdir(home),
      writeFile(
        config,
        `listen = "127.0.0.1:0"\ndefault_cwd = ${JSON.stringify(workDir)}\nstate_dir = ${JSON.stringify(join(root, "state"))}\nshell = "/bin/bash"\nidle_after_seconds = 30\n\n[[presets]]\nname = "shell"\nlabel = "Shell"\ncmd = ["/bin/bash", "--noprofile", "--norc", "-i"]\n`,
      ),
    ]);
    // Build into the temporary directory, never bin/ or the embedded frontend.
    builder = spawn("go", ["build", "-o", binary, "./cmd/webmux"], {
      cwd: repoRoot,
      stdio: ["ignore", "pipe", "pipe"],
      detached: true,
    });
    builder.stdout.on("data", (chunk) => (buildOutput += chunk));
    builder.stderr.on("data", (chunk) => (buildOutput += chunk));
    const buildDeadline = setTimeout(
      () => signalChild(builder, "SIGKILL", true),
      180_000,
    );
    try {
      const [code] = await once(builder, "exit");
      if (code !== 0) throw new Error(`Go build exited ${code}`);
    } finally {
      clearTimeout(buildDeadline);
    }
    async function launchBackend(listen) {
      const outputStart = backendOutput.length;
      backend = spawn(
        binary,
        ["--config", config, ...(listen ? ["--listen", listen] : [])],
        {
          cwd: workDir,
          stdio: ["ignore", "pipe", "pipe"],
          env: {
            PATH: noGit
              ? join(root, "no-git")
              : (process.env.PATH ?? "/usr/bin:/bin"),
            HOME: home,
            XDG_CONFIG_HOME: join(home, ".config"),
            TMPDIR: root,
            SHELL: "/bin/bash",
            LANG: "C.UTF-8",
            LC_ALL: "C.UTF-8",
            PS1: "webmux-e2e$ ",
            // Apple's /bin/bash otherwise prints a zsh migration banner.
            BASH_SILENCE_DEPRECATION_WARNING: "1",
            HISTFILE: "/dev/null",
          },
        },
      );
      backend.stdout.on("data", (chunk) => (backendOutput += chunk));
      backend.stderr.on("data", (chunk) => (backendOutput += chunk));
      let spawnError;
      backend.on("error", (error) => (spawnError = error));
      const deadline = Date.now() + 30_000;
      let address;
      while (
        !(address = backendOutput
          .slice(outputStart)
          .match(/address=(127\.0\.0\.1:\d+)/)?.[1])
      ) {
        if (spawnError) throw spawnError;
        if (backend.exitCode !== null || backend.signalCode !== null)
          throw new Error("Go backend exited before listening");
        if (Date.now() > deadline)
          throw new Error("Timed out waiting for Go backend address");
        await delay(50);
      }
      const health = await fetch(`http://${address}/api/health`, {
        signal: AbortSignal.timeout(10_000),
      });
      if (!health.ok)
        throw new Error(`Backend health returned ${health.status}`);
      return address;
    }
    const address = await launchBackend();
    const apiURL = `http://${address}`;
    // Vite treats port 0 as its default 5173. Reserve an OS-assigned port,
    // release it, then require Vite to bind exactly that port (never fall back).
    const reservation = createPortReservation();
    reservation.listen(0, "127.0.0.1");
    await once(reservation, "listening");
    const reservedAddress = reservation.address();
    if (!reservedAddress || typeof reservedAddress === "string")
      throw new Error("Failed to reserve an E2E frontend port");
    const frontendPort = reservedAddress.port;
    const released = once(reservation, "close");
    reservation.close();
    await released;
    vite = await createServer({
      root: webRoot,
      configFile: join(webRoot, "vite.config.ts"),
      // Concurrent workers must not optimize dependencies into one directory.
      // A parallel slot is held by one worker at a time and keeps its cache.
      cacheDir: join(
        webRoot,
        "node_modules/.vite",
        `e2e-${process.env.TEST_PARALLEL_INDEX ?? "0"}`,
      ),
      customLogger: {
        info: (message) => (viteOutput += `${message}\n`),
        warn: (message) => (viteOutput += `${message}\n`),
        warnOnce: (message) => (viteOutput += `${message}\n`),
        error: (message) => (viteOutput += `${message}\n`),
        clearScreen() {},
        hasErrorLogged: () => false,
        hasWarned: false,
      },
      server: {
        host: "127.0.0.1",
        port: frontendPort,
        strictPort: true,
        open: false,
        proxy: {
          "/api": { target: apiURL },
          "/ws": { target: apiURL.replace("http:", "ws:"), ws: true },
        },
      },
    });
    await vite.listen();
    const viteAddress = vite.httpServer.address();
    if (!viteAddress || typeof viteAddress === "string")
      throw new Error("Vite did not bind a TCP port");
    const baseURL = `http://127.0.0.1:${viteAddress.port}`;
    return {
      baseURL,
      apiURL,
      workDir,
      logs,
      async restart() {
        await stopChild(backend);
        await launchBackend(address);
      },
      async close() {
        try {
          await close();
        } finally {
          process.removeListener("SIGINT", interrupted);
          process.removeListener("SIGTERM", interrupted);
        }
      },
    };
  } catch (error) {
    await close();
    process.removeListener("SIGINT", interrupted);
    process.removeListener("SIGTERM", interrupted);
    throw new Error(`${error.message}\n${logs()}`, { cause: error });
  }
}
