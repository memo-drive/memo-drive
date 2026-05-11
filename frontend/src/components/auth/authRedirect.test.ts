import { describe, expect, it } from "vitest";
import {
  loginHrefForRedirect,
  redirectTargetFromLoginSearch,
} from "./authRedirect";

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

  it("falls back to the desktop root for missing or unsafe redirect targets", () => {
    expect(redirectTargetFromLoginSearch("")).toBe("/");
    expect(redirectTargetFromLoginSearch("?redirect=https%3A%2F%2Fevil.test")).toBe("/");
    expect(redirectTargetFromLoginSearch("?redirect=%2F%2Fevil.test")).toBe("/");
    expect(redirectTargetFromLoginSearch("?redirect=%2Flogin")).toBe("/");
  });
});
