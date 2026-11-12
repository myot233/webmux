import { describe, expect, test } from "vitest";
import { forwardAuthNext, matchRoute } from "../src/hooks/shared/route";

describe("route matching", () => {
  test("matches a valid session id", () => {
    expect(matchRoute("/s/abcdefgh")).toEqual({
      name: "terminal",
      id: "abcdefgh",
    });
    expect(matchRoute("/s/234567aa")).toEqual({
      name: "terminal",
      id: "234567aa",
    });
  });

  test("matches a valid code window id", () => {
    expect(matchRoute("/code/abcdefgh")).toEqual({
      name: "code",
      id: "abcdefgh",
    });
  });

  test("matches the settings pages", () => {
    expect(matchRoute("/settings")).toEqual({ name: "settings" });
    expect(matchRoute("/settings/tailscale")).toEqual({
      name: "settings-tailscale",
    });
    expect(matchRoute("/settings/tunnel")).toEqual({ name: "settings-tunnel" });
  });

  test("matches the forward authentication page", () => {
    expect(matchRoute("/forward-auth")).toEqual({ name: "forward-auth" });
    expect(matchRoute("/forward-auth/")).toEqual({ name: "sessions" });
    // Gateway paths never reach the SPA as routes of their own.
    expect(matchRoute("/forward/5173/")).toEqual({ name: "sessions" });
  });

  test("falls back to the session list", () => {
    expect(matchRoute("/")).toEqual({ name: "sessions" });
    expect(matchRoute("/unknown")).toEqual({ name: "sessions" });
  });

  test("keeps the current trailing-slash behavior", () => {
    expect(matchRoute("/settings/")).toEqual({ name: "sessions" });
    expect(matchRoute("/settings/tailscale/")).toEqual({ name: "sessions" });
    expect(matchRoute("/settings/tunnel/")).toEqual({ name: "sessions" });
    expect(matchRoute("/s/abcdefgh/")).toEqual({ name: "sessions" });
    expect(matchRoute("/code/abcdefgh/")).toEqual({ name: "sessions" });
  });

  test("rejects ids outside the base32 alphabet", () => {
    expect(matchRoute("/s/ABCDEFGH")).toEqual({ name: "sessions" });
    expect(matchRoute("/s/12345678")).toEqual({ name: "sessions" });
    expect(matchRoute("/s/abcdefg")).toEqual({ name: "sessions" });
    expect(matchRoute("/code/ABCDEFGH")).toEqual({ name: "sessions" });
  });
});

describe("forward authentication return address", () => {
  test("accepts gateway paths with their query", () => {
    expect(forwardAuthNext("?next=%2Fforward%2F5173%2F")).toBe(
      "/forward/5173/",
    );
    expect(
      forwardAuthNext(
        `?next=${encodeURIComponent("/forward/5173/app?page=2&q=a b")}`,
      ),
    ).toBe("/forward/5173/app?page=2&q=a b");
  });

  test("rejects anything that could leave the gateway", () => {
    for (const next of [
      "",
      "/",
      "/api/sessions",
      "/forwardx/5173/",
      "https://evil.example/forward/1/",
      "//evil.example/",
      "/forward//evil.example/",
      "/forward/5173/\\evil",
      "/forward/5173/\r\nX: 1",
    ])
      expect(forwardAuthNext(`?next=${encodeURIComponent(next)}`)).toBeNull();
    expect(forwardAuthNext("")).toBeNull();
  });
});
