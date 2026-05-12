import {
  currentUserAgent,
  mobileRootEntryForUserAgent,
} from "../../mobile/mobileEntry";

const DEFAULT_REDIRECT = "/";

function defaultRedirectTarget(userAgent: string): string {
  return mobileRootEntryForUserAgent(DEFAULT_REDIRECT, userAgent) ?? DEFAULT_REDIRECT;
}

function safeRedirectTarget(value: string | null, fallback: string, userAgent: string): string {
  const target = value?.trim();
  if (!target) return fallback;
  if (!target.startsWith("/") || target.startsWith("//")) return fallback;
  if (target.includes("\\")) return fallback;
  if (target === "/login" || target.startsWith("/login?") || target.startsWith("/login#")) {
    return fallback;
  }
  return mobileRootEntryForUserAgent(target, userAgent) ?? target;
}

export function loginHrefForRedirect(
  pathname: string,
  search: string,
  hash: string,
  userAgent = currentUserAgent(),
): string {
  const fallback = defaultRedirectTarget(userAgent);
  const requestedTarget =
    mobileRootEntryForUserAgent(pathname, userAgent) ?? `${pathname}${search}${hash}`;
  const target = safeRedirectTarget(requestedTarget, fallback, userAgent);
  return `/login?redirect=${encodeURIComponent(target)}`;
}

export function redirectTargetFromLoginSearch(
  search: string,
  userAgent = currentUserAgent(),
): string {
  const params = new URLSearchParams(search);
  return safeRedirectTarget(params.get("redirect"), defaultRedirectTarget(userAgent), userAgent);
}
