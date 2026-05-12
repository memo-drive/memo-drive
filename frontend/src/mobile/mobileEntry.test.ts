import { describe, expect, it } from "vitest";
import { mobileRootEntryForUserAgent } from "./mobileEntry";

const IPHONE_CHROME_UA =
  "Mozilla/5.0 (iPhone; CPU iPhone OS 17_0 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) CriOS/123.0 Mobile/15E148 Safari/604.1";
const ANDROID_PHONE_UA =
  "Mozilla/5.0 (Linux; Android 14; Pixel 8) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/123.0 Mobile Safari/537.36";
const ANDROID_TABLET_UA =
  "Mozilla/5.0 (Linux; Android 14; Pixel Tablet) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/123.0 Safari/537.36";

describe("mobileEntry", () => {
  it("selects the mobile H5 entry for phone clients opening the root path", () => {
    expect(mobileRootEntryForUserAgent("/", IPHONE_CHROME_UA)).toBe("/m");
    expect(mobileRootEntryForUserAgent("/", ANDROID_PHONE_UA)).toBe("/m");
  });

  it("does not rewrite tablet, desktop, or deep-link paths", () => {
    expect(mobileRootEntryForUserAgent("/", ANDROID_TABLET_UA)).toBeNull();
    expect(mobileRootEntryForUserAgent("/smart-search", IPHONE_CHROME_UA)).toBeNull();
    expect(mobileRootEntryForUserAgent("/m", IPHONE_CHROME_UA)).toBeNull();
  });
});
