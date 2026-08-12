import { describe, expect, it } from "vitest";
import { authStatusAllowsAccess } from "./authSession";

describe("cookie session auth status", () => {
  it("allows access only when authentication is unnecessary or active", () => {
    expect(authStatusAllowsAccess({ required: false, authenticated: false })).toBe(true);
    expect(authStatusAllowsAccess({ required: true, authenticated: true })).toBe(true);
    expect(authStatusAllowsAccess({ required: true, authenticated: false })).toBe(false);
  });
});
