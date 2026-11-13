import { describe, expect, test } from "vitest";
import { joinArgs, parseArgs, quoteArg } from "../src/platform/api";
import {
  defaultLaunchAttentionSettings,
  launchCommandDraft,
  launchConfigIcon,
  launchConfigSummary,
  validateLaunchDraft,
} from "../src/domains/settings/launch-configs";
import {
  LaunchConfigSchema,
  LaunchConfigsSchema,
} from "../src/contracts/schemas";

describe("launch icons", () => {
  test.each([
    ["claude", "claudecode"],
    ["codex", "codex"],
    ["agy", "antigravity"],
    ["gemini", "geminicli"],
    ["agent", "cursor"],
    ["/usr/local/bin/cursor-agent", "cursor"],
    ["copilot", "githubcopilot"],
    ["opencode", "opencode"],
    ["qwen", "qwen"],
    ["kimi", "kimi"],
    ["codebuddy", "codebuddy"],
    ["devin", "devin"],
    ["omp", "pi"],
    ["/usr/local/bin/pi", "pi"],
    ["C:\\tools\\codex.exe", "codex"],
    ["bash", "terminal"],
  ])("matches legacy %s commands without changing argv", (program, icon) => {
    const cmd = [program, "--resume"];
    expect(launchConfigIcon({ cmd })).toBe(icon);
    expect(
      launchConfigIcon({
        cmd: ["bash", "-ic", 'exec -- "$@"', "webmux", ...cmd],
      }),
    ).toBe(icon);
    expect(
      launchConfigIcon({
        cmd: ["zsh", "-ic", 'exec -- "$@"', "webmux", ...cmd],
      }),
    ).toBe(icon);
    expect(cmd).toEqual([program, "--resume"]);
  });

  test("an explicit icon overrides matching and arbitrary shell scripts stay generic", () => {
    expect(launchConfigIcon({ icon: "terminal", cmd: ["claude"] })).toBe(
      "terminal",
    );
    expect(launchConfigIcon({ cmd: ["bash", "-ic", "exec claude"] })).toBe(
      "terminal",
    );
  });
});

describe("quoteArg / joinArgs", () => {
  test("leaves plain arguments readable and quotes the rest", () => {
    expect(quoteArg("claude")).toBe("claude");
    expect(quoteArg("--model=opus-4.1")).toBe("--model=opus-4.1");
    expect(quoteArg("")).toBe("''");
    expect(quoteArg("a b")).toBe("'a b'");
    expect(quoteArg("$HOME")).toBe("'$HOME'");
    expect(quoteArg("it's")).toBe(`"it's"`);
    expect(quoteArg(`it's "$x"`)).toBe(`"it's \\"\\$x\\""`);
    expect(joinArgs(["bash", "-lc", "npm run dev"])).toBe(
      "bash -lc 'npm run dev'",
    );
  });

  const tricky = [
    "",
    " ",
    "a b",
    "\t\n",
    "'",
    '"',
    "''",
    '""',
    "\\",
    "\\\\",
    "a\\b",
    "C:\\path\\",
    "$",
    "$HOME",
    "${X:-y}",
    "`id`",
    `it's "quoted" \\ $x`,
    `'"\\$`,
    "line1\nline2",
    "中文 参数",
    "emoji 😀",
    "a;b|c&d",
    "*?[]{}()<>!#~",
    "--flag=value with space",
    "=",
    "-",
  ];

  test.each(tricky)("round-trips %j", (arg) => {
    expect(parseArgs(quoteArg(arg))).toEqual([arg]);
  });

  test("round-trips every combination of tricky arguments", () => {
    for (const first of tricky)
      for (const second of tricky) {
        const argv = ["cmd", first, second, ""];
        expect(parseArgs(joinArgs(argv))).toEqual(argv);
      }
  });

  test("round-trips random argv built from tricky characters", () => {
    const alphabet = [..."ab \t\n'\"\\$`;|中😀-="];
    let seed = 1;
    const random = (n: number) => {
      seed = (seed * 1103515245 + 12345) % 2 ** 31;
      return seed % n;
    };
    for (let run = 0; run < 500; run++) {
      const argv = Array.from({ length: 1 + random(5) }, () =>
        Array.from(
          { length: random(8) },
          () => alphabet[random(alphabet.length)],
        ).join(""),
      );
      expect(parseArgs(joinArgs(argv))).toEqual(argv);
    }
  });
});

describe("validateLaunchDraft", () => {
  test("trims the name, parses the command and collects env", () => {
    const result = validateLaunchDraft("  Claude  ", "claude --resume ''", [
      { key: 1, name: "API_KEY", value: "secret" },
      { key: 2, name: "", value: "" },
    ]);
    expect(result).toMatchObject({
      ok: true,
      value: {
        name: "Claude",
        cmd: ["claude", "--resume", ""],
        env: { API_KEY: "secret" },
      },
    });
  });

  test("rejects reserved, malformed and duplicate env names", () => {
    const result = validateLaunchDraft("x", "sh", [
      { key: 1, name: "WEBMUX_SESSION_ID", value: "1" },
      { key: 2, name: "1BAD", value: "1" },
      { key: 3, name: "OK", value: "1" },
      { key: 4, name: "OK", value: "2" },
      { key: 5, name: "", value: "orphan" },
    ]);
    expect(result.ok).toBe(false);
    expect(Object.keys(result.errors.rows)).toEqual(["1", "2", "4", "5"]);
  });

  test("enforces name, argv and value limits", () => {
    expect(validateLaunchDraft(" ", "sh", []).errors.name).toBeTruthy();
    expect(
      validateLaunchDraft("名".repeat(64), "sh", []).errors.name,
    ).toBeUndefined();
    expect(validateLaunchDraft("名".repeat(65), "sh", []).errors.name).toBe(
      "名称最多 64 个字符",
    );
    expect(validateLaunchDraft("x", "'' a", []).errors.command).toBeTruthy();
    expect(validateLaunchDraft("x", "'a", []).errors.command).toBeTruthy();
    expect(
      validateLaunchDraft("x", Array(129).fill("a").join(" "), []).errors
        .command,
    ).toBeTruthy();
    expect(
      validateLaunchDraft("x", "a".repeat(8193), []).errors.command,
    ).toBeTruthy();
    expect(
      validateLaunchDraft("x", "sh", [
        { key: 1, name: "A", value: "é".repeat(4097) },
      ]).errors.rows[1]?.value,
    ).toBeTruthy();
    expect(
      validateLaunchDraft(
        "x",
        "sh",
        Array.from({ length: 65 }, (_, key) => ({
          key,
          name: `V${key}`,
          value: "",
        })),
      ).errors.env,
    ).toBeTruthy();
  });

  test("validates optional preview settings and inherits when null", () => {
    const valid = validateLaunchDraft("Agent", "agent", [], null, {
      lines: 20,
      skip: 3,
    });
    expect(valid.ok && valid.value.preview).toEqual({ lines: 20, skip: 3 });
    expect(
      validateLaunchDraft("Agent", "agent", [], null, { lines: 0, skip: 0 })
        .errors.preview,
    ).toBe("显示行数须为 1–50 的整数");
    expect(
      validateLaunchDraft("Agent", "agent", [], null, {
        lines: 10,
        skip: 51,
      }).errors.preview,
    ).toBe("跳过末尾行数须为 0–50 的整数");
    const inherited = validateLaunchDraft("Agent", "agent", [], null, null);
    expect(inherited.ok && inherited.value.preview).toBeNull();
  });

  test("validates optional attention settings, inherits when null, and formats summaries", () => {
    const valid = validateLaunchDraft("Agent", "agent", [], null, null, {
      ...defaultLaunchAttentionSettings,
      mode: "all",
      programStatus: false,
      minTurnSeconds: 15,
      doneDelaySeconds: 3,
    });
    expect(valid.ok && valid.value.attention).toEqual({
      mode: "all",
      programStatus: false,
      agentProtocol: true,
      notify: true,
      bell: true,
      minTurnSeconds: 15,
      doneDelaySeconds: 3,
    });
    expect(
      validateLaunchDraft("Agent", "agent", [], null, null, {
        ...defaultLaunchAttentionSettings,
        minTurnSeconds: 301,
      }).errors.attention,
    ).toBe("最短回合耗时须为 0–300 秒的整数");
    expect(
      validateLaunchDraft("Agent", "agent", [], null, null, {
        ...defaultLaunchAttentionSettings,
        doneDelaySeconds: 61,
      }).errors.attention,
    ).toBe("完成确认延迟须为 0–60 秒的整数");
    const inherited = validateLaunchDraft(
      "Agent",
      "agent",
      [],
      null,
      null,
      null,
    );
    expect(inherited.ok && inherited.value.attention).toBeNull();
    expect(
      launchConfigSummary({
        chat: true,
        envNames: ["A"],
        preview: null,
        attention: { ...defaultLaunchAttentionSettings, mode: "urgent" },
      }),
    ).toBe("混合会话 · 1 个环境变量 · 仅需介入");
    expect(
      launchConfigSummary({
        chat: false,
        envNames: [],
        preview: null,
        attention: { ...defaultLaunchAttentionSettings, mode: "off" },
      }),
    ).toBe("终端 · 提醒已关闭");
    expect(
      launchConfigSummary({
        chat: false,
        envNames: [],
        preview: null,
        attention: { ...defaultLaunchAttentionSettings, mode: "all" },
      }),
    ).toBe("终端 · 自定义提醒");
  });

  test.each(["bash", "zsh"] as const)(
    "%s loading keeps quoted, empty and shell-looking arguments literal",
    (shell) => {
      const argv = [
        "opencode",
        "two words",
        "",
        "$HOME",
        "$(touch sentinel)",
        "; exit 9",
      ];
      const result = validateLaunchDraft(
        "Open Code",
        joinArgs(argv),
        [],
        shell,
      );
      expect(result.ok).toBe(true);
      if (!result.ok) throw new Error(`Expected valid ${shell} launch`);
      expect(result.value.cmd).toEqual([
        shell,
        "-ic",
        'exec -- "$@"',
        "webmux",
        ...argv,
      ]);
      const restored = launchCommandDraft(result.value.cmd);
      expect(restored.startupShell).toBe(shell);
      expect(parseArgs(restored.command)).toEqual(argv);
      expect(
        validateLaunchDraft("Open Code", restored.command, [], shell),
      ).toEqual(result);
      const direct = validateLaunchDraft(
        "Open Code",
        restored.command,
        [],
        null,
      );
      expect(direct.ok && direct.value.cmd).toEqual(argv);
    },
  );

  test("shell loading cannot hide an empty command or bypass the stored argv limit", () => {
    for (const command of ["", "''", "echo 'unfinished"])
      expect(validateLaunchDraft("Agent", command, [], "bash").ok).toBe(false);
    expect(
      validateLaunchDraft("Agent", Array(124).fill("arg").join(" "), [], "bash")
        .ok,
    ).toBe(true);
    expect(
      validateLaunchDraft("Agent", Array(125).fill("arg").join(" "), [], "bash")
        .ok,
    ).toBe(false);
  });

  test.each(["bash", "zsh"] as const)(
    "editing preserves a user-written %s script that is not the checkbox wrapper",
    (shell) => {
      const argv = [shell, "-ic", 'exec opencode "$@"', "opencode"];
      const restored = launchCommandDraft(argv);
      expect(restored.startupShell).toBeNull();
      expect(parseArgs(restored.command)).toEqual(argv);
    },
  );

  test("wrappers of one shell never restore another shell's choice", () => {
    const zsh = ["zsh", "-ic", 'exec -- "$@"', "webmux", "omp"];
    expect(launchCommandDraft(zsh)).toEqual({
      command: "omp",
      startupShell: "zsh",
    });
    const barePrefix = ["bash", "-ic", 'exec -- "$@"', "webmux"];
    expect(launchCommandDraft(barePrefix)).toEqual({
      command: joinArgs(barePrefix),
      startupShell: null,
    });
  });
});

describe("launch config schemas", () => {
  const summary = {
    id: "abcdefg2",
    name: "Claude",
    cmd: ["claude"],
    envNames: [],
    chatOptions: [],
    chat: false,
    chatSupport: {
      supported: false,
      supportedAgents: [],
      reason: "unknown_agent" as const,
    },
    createdAt: "2026-10-23T00:00:00Z",
    updatedAt: "2026-10-23T00:00:00Z",
  };

  test("accepts summaries and full configs", () => {
    expect(LaunchConfigsSchema.parse([summary])).toEqual([summary]);
    const { envNames: _names, ...rest } = summary;
    expect(LaunchConfigSchema.parse({ ...rest, env: { A: "1" } }).env).toEqual({
      A: "1",
    });
  });

  test("rejects duplicate ids, more than 100 entries and bad ids", () => {
    expect(LaunchConfigsSchema.safeParse([summary, summary]).success).toBe(
      false,
    );
    expect(
      LaunchConfigsSchema.safeParse(
        Array.from({ length: 101 }, (_, i) => ({
          ...summary,
          id: `abcd${"234567"[i % 6]}${"234567"[Math.floor(i / 6) % 6]}${"abcdef"[Math.floor(i / 36)]}a`,
        })),
      ).success,
    ).toBe(false);
    expect(
      LaunchConfigsSchema.safeParse([{ ...summary, id: "ABC" }]).success,
    ).toBe(false);
  });
});
