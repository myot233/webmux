import { describe, expect, test } from "vitest";
import {
  chatDocumentScroll,
  fullscreenOffered,
  popoutOffered,
} from "../src/platform/capabilities";

describe("fullscreenOffered", () => {
  test.each([
    // iPad, desktop browsers and their web apps
    { supported: true, standalone: false, offered: true },
    { supported: true, standalone: true, offered: true },
    // iPhone Safari tab: kept, to suggest adding to the Home Screen
    { supported: false, standalone: false, offered: true },
    // iPhone Home Screen app: nothing to offer
    { supported: false, standalone: true, offered: false },
  ])(
    "supported=$supported standalone=$standalone → $offered",
    ({ offered, ...platform }) => {
      expect(fullscreenOffered(platform)).toBe(offered);
    },
  );
});

describe("popoutOffered", () => {
  test.each([
    // Document Picture-in-Picture never covers the opener.
    {
      documentPictureInPicture: true,
      finePointer: false,
      touch: true,
      standalone: true,
      offered: true,
    },
    // Desktop browser tab and desktop Safari web app
    {
      documentPictureInPicture: false,
      finePointer: true,
      touch: false,
      standalone: false,
      offered: true,
    },
    {
      documentPictureInPicture: false,
      finePointer: true,
      touch: false,
      standalone: true,
      offered: true,
    },
    // iPad with a trackpad: Safari tab keeps it, the Home Screen app does not
    {
      documentPictureInPicture: false,
      finePointer: true,
      touch: true,
      standalone: false,
      offered: true,
    },
    {
      documentPictureInPicture: false,
      finePointer: true,
      touch: true,
      standalone: true,
      offered: false,
    },
    // Phones, in a tab or a Home Screen app
    {
      documentPictureInPicture: false,
      finePointer: false,
      touch: true,
      standalone: false,
      offered: false,
    },
    {
      documentPictureInPicture: false,
      finePointer: false,
      touch: true,
      standalone: true,
      offered: false,
    },
  ])(
    "pip=$documentPictureInPicture fine=$finePointer touch=$touch standalone=$standalone → $offered",
    ({ offered, ...platform }) => {
      expect(popoutOffered(platform)).toBe(offered);
    },
  );
});

describe("chatDocumentScroll", () => {
  const restingMobileChat = {
    chatView: true,
    desktop: false,
    codeShown: false,
    singlePane: true,
    standalone: false,
    editing: false,
  };

  test("enables document scroll only for a resting single-pane mobile chat tab", () => {
    expect(chatDocumentScroll(restingMobileChat)).toBe(true);
  });

  test.each([
    ["terminal or switching view", { chatView: false }],
    ["desktop pane (>=1024px)", { desktop: true }],
    ["code panel shown", { codeShown: true }],
    ["multi-pane split", { singlePane: false }],
    ["standalone Home Screen app", { standalone: true }],
    ["focused input or open portal", { editing: true }],
  ] as const)("disables document scroll on %s", (_, override) => {
    expect(chatDocumentScroll({ ...restingMobileChat, ...override })).toBe(
      false,
    );
  });
});
