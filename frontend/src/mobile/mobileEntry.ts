export const MOBILE_ENTRY_PATH = "/m";

export function isPhoneUserAgent(userAgent: string): boolean {
  const ua = userAgent.toLowerCase();

  if (!ua) return false;
  if (ua.includes("ipad") || ua.includes("tablet")) return false;
  if (ua.includes("android") && !ua.includes("mobile")) return false;

  return (
    ua.includes("iphone") ||
    ua.includes("ipod") ||
    ua.includes("windows phone") ||
    (ua.includes("android") && ua.includes("mobile"))
  );
}

export function mobileRootEntryForUserAgent(path: string, userAgent: string): string | null {
  const pathOnly = path.split(/[?#]/, 1)[0] || "/";
  if (pathOnly !== "/") return null;
  return isPhoneUserAgent(userAgent) ? MOBILE_ENTRY_PATH : null;
}

export function currentUserAgent(): string {
  return globalThis.navigator?.userAgent ?? "";
}
