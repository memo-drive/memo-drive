import { describe, expect, it } from "vitest";
import {
  loginHrefForRedirect,
  redirectTargetFromLoginSearch,
} from "./authRedirect";

const IPHONE_CHROME_UA =
  "Mozilla/5.0 (iPhone; CPU iPhone OS 17_0 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) CriOS/123.0 Mobile/15E148 Safari/604.1";
const IPAD_UA =
  "Mozilla/5.0 (iPad; CPU OS 17_0 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/17.0 Mobile/15E148 Safari/604.1";

describe("auth redirects", () => {
  it("sends unauthenticated mobile routes to Login with the original target", () => {
    expect(loginHrefForRedirect("/m", "", "")).toBe("/login?redirect=%2Fm");
    expect(loginHrefForRedirect("/m", "?path=%2FDocs", "#top")).toBe(
      "/login?redirect=%2Fm%3Fpath%3D%252FDocs%23top",
    );
  });

  it("returns to the requested same-origin path after Login", () => {
    expect(redirectTargetFromLoginSearch("?redirect=%2Fm")).toBe("/m");
    expect(redirectTargetFromLoginSearch("?redirect=%2Fm%3Fpath%3D%252FDocs")).toBe(
      "/m?path=%2FDocs",
    );
  });

  it("normalizes phone root redirects to the mobile entry", () => {
    expect(loginHrefForRedirect("/", "", "", IPHONE_CHROME_UA)).toBe(
      "/login?redirect=%2Fm",
    );
    expect(redirectTargetFromLoginSearch("?redirect=%2F", IPHONE_CHROME_UA)).toBe(
      "/m",
    );
    expect(redirectTargetFromLoginSearch("", IPHONE_CHROME_UA)).toBe("/m");
  });

  it("keeps tablets and desktops on the desktop root by default", () => {
    expect(loginHrefForRedirect("/", "", "", IPAD_UA)).toBe("/login?redirect=%2F");
    expect(redirectTargetFromLoginSearch("", IPAD_UA)).toBe("/");
  });

  it("falls back to the desktop root for missing or unsafe redirect targets", () => {
    expect(redirectTargetFromLoginSearch("")).toBe("/");
    expect(redirectTargetFromLoginSearch("?redirect=https%3A%2F%2Fevil.test")).toBe("/");
    expect(redirectTargetFromLoginSearch("?redirect=%2F%2Fevil.test")).toBe("/");
    expect(redirectTargetFromLoginSearch("?redirect=%2Flogin")).toBe("/");
  });
});
